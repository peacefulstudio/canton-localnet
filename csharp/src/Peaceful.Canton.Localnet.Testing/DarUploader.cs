// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;
using System.Net.Http.Headers;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;

namespace Peaceful.Canton.Localnet.Testing;

/// <summary>
/// Uploads DAR archives to the Canton JSON Ledger API
/// (<c>POST /v2/packages</c>) using the bearer token supplied by
/// <see cref="OAuth2TokenProvider"/>. The upload is idempotent: a
/// <c>400 KNOWN_PACKAGE_VERSION</c> response from a repeated upload is
/// treated as success, matching the splice-onboarding behaviour. Every
/// connected synchronizer is vetted in turn via <c>?synchronizerId=</c>,
/// so this works unchanged on both single-sync and multi-sync stacks.
/// </summary>
public sealed class DarUploader
{
    private const string KnownPackageVersionMarker = "KNOWN_PACKAGE_VERSION";
    private const string UploadPath = "v2/packages";

    private readonly HttpClient _httpClient;
    private readonly OAuth2TokenProvider _tokenProvider;
    private readonly ILogger<DarUploader> _logger;
    private readonly DarUploaderRetryOptions _retryOptions;
    private readonly JsonLedgerAdminClient _adminClient;

    /// <summary>
    /// Creates an uploader bound to a JSON Ledger API <see cref="HttpClient"/>.
    /// </summary>
    /// <param name="httpClient">
    /// Client whose <see cref="HttpClient.BaseAddress"/> is the JSON Ledger API
    /// root (e.g. <c>http://localhost:11975/</c>). Required; an unset base
    /// address throws.
    /// </param>
    /// <param name="tokenProvider">Supplies the bearer token for each upload.</param>
    /// <param name="logger">Optional logger; defaults to a no-op logger.</param>
    /// <param name="options">
    /// Transient-503 retry policy; defaults to <see cref="DarUploaderRetryOptions.Default"/>.
    /// </param>
    /// <exception cref="ArgumentException">
    /// <paramref name="httpClient"/> has no <see cref="HttpClient.BaseAddress"/>.
    /// </exception>
    public DarUploader(
        HttpClient httpClient,
        OAuth2TokenProvider tokenProvider,
        ILogger<DarUploader>? logger = null,
        DarUploaderRetryOptions? options = null)
    {
        _httpClient = httpClient ?? throw new ArgumentNullException(nameof(httpClient));
        _tokenProvider = tokenProvider ?? throw new ArgumentNullException(nameof(tokenProvider));
        _logger = logger ?? NullLogger<DarUploader>.Instance;
        _retryOptions = options ?? DarUploaderRetryOptions.Default;

        if (_httpClient.BaseAddress is null)
        {
            throw new ArgumentException(
                "HttpClient must have a BaseAddress set to the JSON Ledger API root (e.g. http://localhost:11975/).",
                nameof(httpClient));
        }

        _adminClient = new JsonLedgerAdminClient(_httpClient, _tokenProvider);
    }

    /// <summary>
    /// Uploads a single DAR file at <paramref name="darPath"/>. Returns the
    /// outcome — <see cref="DarUploadOutcome.Uploaded"/> for a fresh package,
    /// <see cref="DarUploadOutcome.AlreadyKnown"/> when the participant returns
    /// <c>KNOWN_PACKAGE_VERSION</c>.
    /// </summary>
    public async Task<DarUploadOutcome> UploadAsync(string darPath, CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(darPath))
        {
            throw new ArgumentException("DAR path must be non-empty.", nameof(darPath));
        }

        var bytes = await File.ReadAllBytesAsync(darPath, cancellationToken).ConfigureAwait(false);
        return await UploadAsync(bytes, darPath, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>
    /// Uploads a single DAR file at <paramref name="darPath"/>, then, if every
    /// connected synchronizer reported <c>KNOWN_PACKAGE_VERSION</c>, reads back
    /// <paramref name="expectedMainPackageId"/> and throws unless it is really
    /// present on the participant.
    /// </summary>
    public async Task<DarUploadOutcome> UploadAndVerifyAsync(
        string darPath,
        string? expectedMainPackageId,
        CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(darPath))
        {
            throw new ArgumentException("DAR path must be non-empty.", nameof(darPath));
        }

        var bytes = await File.ReadAllBytesAsync(darPath, cancellationToken).ConfigureAwait(false);
        return await UploadAndVerifyAsync(bytes, darPath, expectedMainPackageId, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>
    /// Uploads the raw DAR bytes. <paramref name="sourceLabel"/> is used only
    /// in log lines and the resulting exception message; pass the file path or
    /// any human-readable identifier.
    /// </summary>
    public Task<DarUploadOutcome> UploadAsync(
        byte[] darBytes,
        string sourceLabel,
        CancellationToken cancellationToken = default)
        => UploadAndVerifyAsync(darBytes, sourceLabel, expectedMainPackageId: null, cancellationToken);

    /// <summary>
    /// Uploads the raw DAR bytes, then, if every connected synchronizer
    /// reported <c>KNOWN_PACKAGE_VERSION</c>, reads back
    /// <paramref name="expectedMainPackageId"/> (<c>GET /v2/packages/{package-id}</c>)
    /// and throws <see cref="JsonLedgerApiException"/> naming it unless it is
    /// really present on the participant. Pass <c>null</c> to skip the check —
    /// an identical re-upload with no expected id still returns
    /// <see cref="DarUploadOutcome.AlreadyKnown"/> unconditionally.
    /// </summary>
    public async Task<DarUploadOutcome> UploadAndVerifyAsync(
        byte[] darBytes,
        string sourceLabel,
        string? expectedMainPackageId,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(darBytes);
        if (darBytes.Length == 0)
        {
            throw new ArgumentException("DAR payload must be non-empty.", nameof(darBytes));
        }

        var targets = await ResolveVettingTargetsAsync(cancellationToken).ConfigureAwait(false);

        var uploadedAnywhere = false;
        foreach (var synchronizerId in targets)
        {
            var token = await _tokenProvider.GetAccessTokenAsync(cancellationToken).ConfigureAwait(false);
            var requestUri = BuildUploadUri(synchronizerId);

            _logger.LogDebug("POST {Uri} (DAR bytes: {Length}, source: {Source})",
                new Uri(_httpClient.BaseAddress!, requestUri), darBytes.Length, sourceLabel);

            using var response = await SendWithRetryAsync(darBytes, token, sourceLabel, requestUri, cancellationToken).ConfigureAwait(false);
            if (response.IsSuccessStatusCode)
            {
                _logger.LogInformation(
                    "Uploaded DAR {Source} ({Length} bytes) on synchronizer {SynchronizerId}",
                    sourceLabel, darBytes.Length, synchronizerId ?? "(default)");
                uploadedAnywhere = true;
                continue;
            }

            var body = await response.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false);
            if (response.StatusCode == HttpStatusCode.BadRequest && body.Contains(KnownPackageVersionMarker, StringComparison.Ordinal))
            {
                _logger.LogInformation(
                    "DAR {Source} already on ledger (KNOWN_PACKAGE_VERSION) on synchronizer {SynchronizerId} — treating as success.",
                    sourceLabel, synchronizerId ?? "(default)");
                continue;
            }

            throw new JsonLedgerApiException(
                $"POST {requestUri} for {sourceLabel} returned {(int)response.StatusCode} {response.ReasonPhrase}: {body}",
                response.StatusCode,
                body);
        }

        if (uploadedAnywhere)
        {
            return DarUploadOutcome.Uploaded;
        }

        if (!string.IsNullOrWhiteSpace(expectedMainPackageId)
            && !await _adminClient.PackageExistsAsync(expectedMainPackageId, cancellationToken).ConfigureAwait(false))
        {
            throw new JsonLedgerApiException(
                $"DAR {sourceLabel} reported KNOWN_PACKAGE_VERSION on every connected synchronizer, but package '{expectedMainPackageId}' is absent from the participant.",
                HttpStatusCode.NotFound,
                string.Empty);
        }

        return DarUploadOutcome.AlreadyKnown;
    }

    private async Task<IReadOnlyList<string?>> ResolveVettingTargetsAsync(CancellationToken cancellationToken)
    {
        var synchronizers = await _adminClient.GetConnectedSynchronizersAsync(party: null, cancellationToken: cancellationToken).ConfigureAwait(false);
        if (synchronizers.Count == 0)
        {
            return new string?[] { null };
        }
        return synchronizers.Select(s => (string?)s.Id).ToList();
    }

    private static string BuildUploadUri(string? synchronizerId) =>
        synchronizerId is null
            ? UploadPath
            : $"{UploadPath}?synchronizerId={Uri.EscapeDataString(synchronizerId)}";

    private async Task<HttpResponseMessage> SendWithRetryAsync(
        byte[] darBytes,
        string token,
        string sourceLabel,
        string requestUri,
        CancellationToken cancellationToken)
    {
        for (var attempt = 1; ; attempt++)
        {
            using var request = new HttpRequestMessage(HttpMethod.Post, requestUri);
            request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
            request.Content = new ByteArrayContent(darBytes);
            request.Content.Headers.ContentType = new MediaTypeHeaderValue("application/octet-stream");

            var response = await _httpClient.SendAsync(request, cancellationToken).ConfigureAwait(false);

            if (response.StatusCode != HttpStatusCode.ServiceUnavailable || attempt >= _retryOptions.MaxAttempts)
            {
                return response;
            }

            response.Dispose();

            var delay = _retryOptions.DelayForAttempt(attempt);
            _logger.LogWarning(
                "POST {Path} for {Source} returned 503 Service Unavailable (attempt {Attempt}/{MaxAttempts}); retrying in {Delay}.",
                requestUri, sourceLabel, attempt, _retryOptions.MaxAttempts, delay);

            await _retryOptions.Delay(delay, cancellationToken).ConfigureAwait(false);
        }
    }

    /// <summary>
    /// Sequentially uploads every DAR path in <paramref name="darPaths"/>,
    /// stopping at the first genuine failure. Idempotent retries are not a
    /// failure; see <see cref="UploadAsync(string, CancellationToken)"/>.
    /// </summary>
    public async Task<IReadOnlyList<DarUploadResult>> UploadManyAsync(
        IEnumerable<string> darPaths,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(darPaths);

        var results = new List<DarUploadResult>();
        foreach (var path in darPaths)
        {
            var outcome = await UploadAsync(path, cancellationToken).ConfigureAwait(false);
            results.Add(new DarUploadResult(path, outcome));
        }
        return results;
    }
}

/// <summary>
/// Result of a single DAR upload.
/// </summary>
public enum DarUploadOutcome
{
    /// <summary>The participant accepted the DAR as a fresh package.</summary>
    Uploaded,

    /// <summary>The DAR was already known to the participant (<c>KNOWN_PACKAGE_VERSION</c>).</summary>
    AlreadyKnown,
}

/// <summary>
/// Pairs a DAR source identifier with its upload outcome.
/// </summary>
public sealed record DarUploadResult(string Source, DarUploadOutcome Outcome);

/// <summary>
/// Bounded exponential-backoff configuration for retrying a transient
/// <c>503 Service Unavailable</c> from <c>POST /v2/packages</c> while the
/// package service is still warming up. Defaults give ~6 attempts with a 1s
/// base delay capped at 16s (total budget on the order of ~30s).
/// </summary>
/// <param name="MaxAttempts">Total number of POST attempts, including the first.</param>
/// <param name="BaseDelay">Delay before the first retry; doubles each subsequent retry.</param>
/// <param name="MaxDelay">Upper bound applied to every computed backoff delay.</param>
/// <param name="Delay">
/// Wait primitive invoked between attempts. Production uses
/// <see cref="Task.Delay(TimeSpan, CancellationToken)"/>; tests inject a
/// near-instant delegate so they never actually sleep.
/// </param>
public sealed record DarUploaderRetryOptions(
    int MaxAttempts,
    TimeSpan BaseDelay,
    TimeSpan MaxDelay,
    Func<TimeSpan, CancellationToken, Task> Delay)
{
    /// <summary>Total number of POST attempts, including the first.</summary>
    public int MaxAttempts { get; } = Validated(MaxAttempts, BaseDelay, MaxDelay, Delay);

    /// <summary>Delay before the first retry; doubles each subsequent retry.</summary>
    public TimeSpan BaseDelay { get; } = BaseDelay;

    /// <summary>Upper bound applied to every computed backoff delay.</summary>
    public TimeSpan MaxDelay { get; } = MaxDelay;

    /// <summary>Wait primitive invoked between attempts.</summary>
    public Func<TimeSpan, CancellationToken, Task> Delay { get; } = Delay;

    private static int Validated(
        int maxAttempts,
        TimeSpan baseDelay,
        TimeSpan maxDelay,
        Func<TimeSpan, CancellationToken, Task> delay)
    {
        ArgumentNullException.ThrowIfNull(delay);
        ArgumentOutOfRangeException.ThrowIfLessThan(maxAttempts, 1);
        ArgumentOutOfRangeException.ThrowIfLessThan(baseDelay, TimeSpan.Zero);
        ArgumentOutOfRangeException.ThrowIfLessThan(maxDelay, baseDelay);
        return maxAttempts;
    }

    /// <summary>Production defaults: 6 attempts, 1s base, 16s cap, real <see cref="Task.Delay(TimeSpan, CancellationToken)"/>.</summary>
    public static DarUploaderRetryOptions Default { get; } = new(
        MaxAttempts: 6,
        BaseDelay: TimeSpan.FromSeconds(1),
        MaxDelay: TimeSpan.FromSeconds(16),
        Delay: Task.Delay);

    /// <summary>Computes the capped exponential backoff delay before the retry following <paramref name="attempt"/>.</summary>
    public TimeSpan DelayForAttempt(int attempt)
    {
        var multiplier = Math.Pow(2, attempt - 1);
        var milliseconds = BaseDelay.TotalMilliseconds * multiplier;
        return milliseconds >= MaxDelay.TotalMilliseconds
            ? MaxDelay
            : TimeSpan.FromMilliseconds(milliseconds);
    }
}
