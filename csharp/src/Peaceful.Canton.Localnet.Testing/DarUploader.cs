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
/// treated as success, matching the splice-onboarding behaviour.
/// </summary>
public sealed class DarUploader
{
    private const string KnownPackageVersionMarker = "KNOWN_PACKAGE_VERSION";
    private const string UploadPath = "v2/packages";

    private readonly HttpClient _httpClient;
    private readonly OAuth2TokenProvider _tokenProvider;
    private readonly ILogger<DarUploader> _logger;
    private readonly DarUploaderRetryOptions _retryOptions;

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
    /// Uploads the raw DAR bytes. <paramref name="sourceLabel"/> is used only
    /// in log lines and the resulting exception message; pass the file path or
    /// any human-readable identifier.
    /// </summary>
    public async Task<DarUploadOutcome> UploadAsync(
        byte[] darBytes,
        string sourceLabel,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(darBytes);
        if (darBytes.Length == 0)
        {
            throw new ArgumentException("DAR payload must be non-empty.", nameof(darBytes));
        }

        var token = await _tokenProvider.GetAccessTokenAsync(cancellationToken).ConfigureAwait(false);

        _logger.LogDebug("POST {Uri} (DAR bytes: {Length}, source: {Source})",
            new Uri(_httpClient.BaseAddress!, UploadPath), darBytes.Length, sourceLabel);

        using var response = await SendWithRetryAsync(darBytes, token, sourceLabel, cancellationToken).ConfigureAwait(false);
        if (response.IsSuccessStatusCode)
        {
            _logger.LogInformation("Uploaded DAR {Source} ({Length} bytes)", sourceLabel, darBytes.Length);
            return DarUploadOutcome.Uploaded;
        }

        var body = await response.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false);
        if (response.StatusCode == HttpStatusCode.BadRequest && body.Contains(KnownPackageVersionMarker, StringComparison.Ordinal))
        {
            _logger.LogInformation("DAR {Source} already on ledger (KNOWN_PACKAGE_VERSION) — treating as success.", sourceLabel);
            return DarUploadOutcome.AlreadyKnown;
        }

        throw new JsonLedgerApiException(
            $"POST {UploadPath} for {sourceLabel} returned {(int)response.StatusCode} {response.ReasonPhrase}: {body}",
            response.StatusCode,
            body);
    }

    private async Task<HttpResponseMessage> SendWithRetryAsync(
        byte[] darBytes,
        string token,
        string sourceLabel,
        CancellationToken cancellationToken)
    {
        for (var attempt = 1; ; attempt++)
        {
            using var request = new HttpRequestMessage(HttpMethod.Post, UploadPath);
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
                UploadPath, sourceLabel, attempt, _retryOptions.MaxAttempts, delay);

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
    public int MaxAttempts { get; } = Validated(MaxAttempts, BaseDelay, MaxDelay, Delay);
    public TimeSpan BaseDelay { get; } = BaseDelay;
    public TimeSpan MaxDelay { get; } = MaxDelay;
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
