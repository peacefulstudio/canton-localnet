// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;
using System.Text;
using System.Text.Json;
using Microsoft.Extensions.Logging;
using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

public class DarUploaderTests
{
    private static readonly Uri JsonApiBase = new("http://localhost:11975/");

    private static OAuth2TokenProvider StaticTokenProvider(string token)
    {
        var options = new OAuth2TokenProviderOptions(
            TokenEndpoint: new Uri("https://keycloak.test/token"),
            ClientId: "id",
            ClientSecret: "sec",
            Audience: "aud",
            Scope: "openid");
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent(
                JsonSerializer.Serialize(new { access_token = token, token_type = "Bearer", expires_in = 3600 }),
                Encoding.UTF8,
                "application/json"),
        }));
        var http = new HttpClient(handler);
        return new OAuth2TokenProvider(http, options);
    }

    private static string? SynchronizerIdFromQuery(Uri uri)
    {
        var query = uri.Query.TrimStart('?');
        foreach (var pair in query.Split('&', StringSplitOptions.RemoveEmptyEntries))
        {
            var parts = pair.Split('=', 2);
            if (parts[0] == "synchronizerId")
            {
                return Uri.UnescapeDataString(parts[1]);
            }
        }
        return null;
    }

    private static DarUploader NoSleepUploader(HttpClient http, OAuth2TokenProvider tokenProvider) =>
        new(http, tokenProvider, options: new DarUploaderRetryOptions(
            MaxAttempts: 6,
            BaseDelay: TimeSpan.FromSeconds(1),
            MaxDelay: TimeSpan.FromSeconds(16),
            Delay: (_, _) => Task.CompletedTask));

    private const string VettedElsewhereId = "d0048119d779f3dc1c02dd2549c6281cdd72998bc3c9bc514fe07ff5d5365bbd";
    private const string ParticipantId = "participant::1220aa";

    private static string KnownPackageVersionBody(string alreadyVettedId, string rejectedId) =>
        $$"""{"code":"KNOWN_PACKAGE_VERSION","cause":"Tried to vet two packages with the same name and version: {{alreadyVettedId}} (canton-localnet-noop v0.0.1) and {{rejectedId}} (canton-localnet-noop v0.0.1).","correlationId":null,"traceId":"63e93725641d9cce173de2599a13a6b7","context":{"participant":"a-validator-1","secondPackage":"{{rejectedId}} (canton-localnet-noop v0.0.1)","firstPackage":"{{alreadyVettedId}} (canton-localnet-noop v0.0.1)","category":"8","action":"package-vetting"},"errorCategory":8,"grpcCodeValue":3}""";

    private static HttpResponseMessage KnownPackageVersion(string alreadyVettedId = VettedElsewhereId, string rejectedId = DarFixtures.NoopMainPackageId) =>
        new(HttpStatusCode.BadRequest)
        {
            Content = new StringContent(KnownPackageVersionBody(alreadyVettedId, rejectedId), Encoding.UTF8, "application/json"),
        };

    private static Func<HttpRequestMessage, CancellationToken, Task<HttpResponseMessage>> WithVetting(
        Func<HttpRequestMessage, CancellationToken, Task<HttpResponseMessage>> uploadResponder,
        Func<string, bool> isVettedOn,
        params (string Alias, string Id)[] synchronizers) =>
        SynchronizerDiscoveryResponder.WithConnectedSynchronizers((request, ct) =>
        {
            var path = request.RequestUri!.AbsolutePath;
            if (request.Method == HttpMethod.Get && path.EndsWith("v2/parties/participant-id", StringComparison.Ordinal))
            {
                return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
                {
                    Content = new StringContent($$"""{"participantId":"{{ParticipantId}}"}""", Encoding.UTF8, "application/json"),
                });
            }
            if (request.Method == HttpMethod.Post && path.EndsWith("v2/package-vetting/list", StringComparison.Ordinal))
            {
                using var doc = JsonDocument.Parse(request.Content!.ReadAsStringAsync(ct).GetAwaiter().GetResult());
                var requested = doc.RootElement.GetProperty("packageMetadataFilter").GetProperty("packageIds")[0].GetString();
                var synchronizerId = doc.RootElement.GetProperty("topologyStateFilter").GetProperty("synchronizerIds")[0].GetString()!;
                var entries = isVettedOn(synchronizerId)
                    ? $$"""[{"packages":[{"packageId":"{{requested}}","packageName":"canton-localnet-noop","packageVersion":"0.0.1"}],"participantId":"{{ParticipantId}}","synchronizerId":"{{synchronizerId}}","topologySerial":3}]"""
                    : "[]";
                return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
                {
                    Content = new StringContent($$"""{"vettedPackages":{{entries}},"nextPageToken":""}""", Encoding.UTF8, "application/json"),
                });
            }
            return uploadResponder(request, ct);
        }, synchronizers);

    [Fact]
    public async Task UploadAsync_retries_transient_503_then_succeeds()
    {
        var calls = 0;
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers((_, _) =>
        {
            calls++;
            var status = calls < 3 ? HttpStatusCode.ServiceUnavailable : HttpStatusCode.OK;
            return Task.FromResult(new HttpResponseMessage(status));
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = NoSleepUploader(http, StaticTokenProvider("tok"));

        var outcome = await uploader.UploadAsync(new byte[] { 1, 2, 3 }, "warming.dar");

        Assert.Equal(DarUploadOutcome.Uploaded, outcome);
        Assert.Equal(3, calls);
    }

    [Fact]
    public async Task UploadAsync_throws_503_with_body_preserved_after_budget_exhausted()
    {
        var calls = 0;
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers((_, _) =>
        {
            calls++;
            return Task.FromResult(new HttpResponseMessage(HttpStatusCode.ServiceUnavailable)
            {
                Content = new StringContent("""{"cause":"still warming up"}""", Encoding.UTF8, "application/json"),
            });
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = NoSleepUploader(http, StaticTokenProvider("tok"));

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(
            () => uploader.UploadAsync(new byte[] { 1, 2, 3 }, "cold.dar"));

        Assert.Equal(HttpStatusCode.ServiceUnavailable, exception.StatusCode);
        Assert.Contains("still warming up", exception.ResponseBody);
        Assert.Equal(6, calls);
    }

    [Theory]
    [InlineData(HttpStatusCode.GatewayTimeout, "")]
    [InlineData(HttpStatusCode.Conflict, """{"code":"REQUEST_ALREADY_IN_FLIGHT"}""")]
    public async Task UploadAsync_retries_every_transient_status_then_succeeds(HttpStatusCode status, string body)
    {
        var calls = 0;
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers((_, _) =>
        {
            calls++;
            return Task.FromResult(calls < 3
                ? new HttpResponseMessage(status) { Content = new StringContent(body, Encoding.UTF8, "application/json") }
                : new HttpResponseMessage(HttpStatusCode.OK));
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = NoSleepUploader(http, StaticTokenProvider("tok"));

        var outcome = await uploader.UploadAsync(new byte[] { 1, 2, 3 }, "warming.dar");

        Assert.Equal(DarUploadOutcome.Uploaded, outcome);
        Assert.Equal(3, calls);
    }

    [Fact]
    public async Task UploadAsync_sends_a_fresh_token_on_each_attempt()
    {
        var tokens = new ExpiringTokens();
        var bearers = new List<string?>();
        var calls = 0;
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers((request, _) =>
        {
            bearers.Add(request.Headers.Authorization?.Parameter);
            calls++;
            return Task.FromResult(new HttpResponseMessage(calls < 2 ? HttpStatusCode.ServiceUnavailable : HttpStatusCode.OK));
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = new DarUploader(http, tokens.Provider, options: new DarUploaderRetryOptions(
            6, TimeSpan.FromSeconds(1), TimeSpan.FromSeconds(16), tokens.ExpireCurrentTokenAsync));

        await uploader.UploadAsync(new byte[] { 1, 2, 3 }, "warming.dar");

        Assert.Equal(new[] { "tok-1", "tok-2" }, bearers);
    }

    [Fact]
    public async Task UploadAsync_bounds_the_error_body_in_the_exception_message()
    {
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers((_, _) =>
            Task.FromResult(new HttpResponseMessage(HttpStatusCode.Forbidden)
            {
                Content = new StringContent(new string('z', 10_000), Encoding.UTF8, "text/html"),
            })));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = NoSleepUploader(http, StaticTokenProvider("tok"));

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(
            () => uploader.UploadAsync(new byte[] { 1, 2, 3 }, "big.dar"));

        Assert.EndsWith(new string('z', 512) + "...[truncated]", exception.Message);
        Assert.Equal(10_000, exception.ResponseBody.Length);
    }

    [Theory]
    [InlineData(HttpStatusCode.Unauthorized)]
    [InlineData(HttpStatusCode.Forbidden)]
    [InlineData(HttpStatusCode.Conflict)]
    public async Task UploadAsync_does_not_retry_non_503_failure(HttpStatusCode status)
    {
        var calls = 0;
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers((_, _) =>
        {
            calls++;
            return Task.FromResult(new HttpResponseMessage(status)
            {
                Content = new StringContent("""{"cause":"no token"}""", Encoding.UTF8, "application/json"),
            });
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = NoSleepUploader(http, StaticTokenProvider("tok"));

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(
            () => uploader.UploadAsync(new byte[] { 1, 2, 3 }, "unauth.dar"));

        Assert.Equal(status, exception.StatusCode);
        Assert.Equal(1, calls);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(-1)]
    public void DarUploaderRetryOptions_rejects_MaxAttempts_below_one(int maxAttempts)
    {
        Assert.Throws<ArgumentOutOfRangeException>(() => new DarUploaderRetryOptions(
            MaxAttempts: maxAttempts,
            BaseDelay: TimeSpan.FromSeconds(1),
            MaxDelay: TimeSpan.FromSeconds(16),
            Delay: (_, _) => Task.CompletedTask));
    }

    [Fact]
    public void DarUploaderRetryOptions_rejects_null_Delay()
    {
        Assert.Throws<ArgumentNullException>(() => new DarUploaderRetryOptions(
            MaxAttempts: 6,
            BaseDelay: TimeSpan.FromSeconds(1),
            MaxDelay: TimeSpan.FromSeconds(16),
            Delay: null!));
    }

    [Fact]
    public void DarUploaderRetryOptions_rejects_negative_BaseDelay()
    {
        Assert.Throws<ArgumentOutOfRangeException>(() => new DarUploaderRetryOptions(
            MaxAttempts: 6,
            BaseDelay: TimeSpan.FromSeconds(-1),
            MaxDelay: TimeSpan.FromSeconds(16),
            Delay: (_, _) => Task.CompletedTask));
    }

    [Fact]
    public void DarUploaderRetryOptions_rejects_MaxDelay_below_BaseDelay()
    {
        Assert.Throws<ArgumentOutOfRangeException>(() => new DarUploaderRetryOptions(
            MaxAttempts: 6,
            BaseDelay: TimeSpan.FromSeconds(16),
            MaxDelay: TimeSpan.FromSeconds(1),
            Delay: (_, _) => Task.CompletedTask));
    }

    [Theory]
    [InlineData(1, 1000)]
    [InlineData(2, 2000)]
    [InlineData(3, 4000)]
    [InlineData(4, 8000)]
    [InlineData(5, 16000)]
    [InlineData(6, 16000)]
    public void DelayForAttempt_doubles_and_caps_at_MaxDelay(int attempt, int expectedMilliseconds)
    {
        var options = new DarUploaderRetryOptions(
            MaxAttempts: 6,
            BaseDelay: TimeSpan.FromSeconds(1),
            MaxDelay: TimeSpan.FromSeconds(16),
            Delay: (_, _) => Task.CompletedTask);

        Assert.Equal(TimeSpan.FromMilliseconds(expectedMilliseconds), options.DelayForAttempt(attempt));
    }

    [Fact]
    public async Task UploadAsync_passes_capped_exponential_delays_to_Delay_delegate()
    {
        var capturedDelays = new List<TimeSpan>();
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers((_, _) =>
            Task.FromResult(new HttpResponseMessage(HttpStatusCode.ServiceUnavailable)
            {
                Content = new StringContent("""{"cause":"still warming up"}""", Encoding.UTF8, "application/json"),
            })));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = new DarUploader(http, StaticTokenProvider("tok"), options: new DarUploaderRetryOptions(
            MaxAttempts: 6,
            BaseDelay: TimeSpan.FromSeconds(1),
            MaxDelay: TimeSpan.FromSeconds(16),
            Delay: (delay, _) =>
            {
                capturedDelays.Add(delay);
                return Task.CompletedTask;
            }));

        await Assert.ThrowsAsync<JsonLedgerApiException>(
            () => uploader.UploadAsync(new byte[] { 1, 2, 3 }, "cold.dar"));

        Assert.Equal(
            new[]
            {
                TimeSpan.FromSeconds(1),
                TimeSpan.FromSeconds(2),
                TimeSpan.FromSeconds(4),
                TimeSpan.FromSeconds(8),
                TimeSpan.FromSeconds(16),
            },
            capturedDelays);
    }

    [Fact]
    public async Task UploadAsync_logs_a_warning_for_each_503_retry()
    {
        var calls = 0;
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers((_, _) =>
        {
            calls++;
            var status = calls < 3 ? HttpStatusCode.ServiceUnavailable : HttpStatusCode.OK;
            return Task.FromResult(new HttpResponseMessage(status));
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var logger = new RecordingLogger<DarUploader>();
        var uploader = new DarUploader(http, StaticTokenProvider("tok"), logger,
            new DarUploaderRetryOptions(6, TimeSpan.FromSeconds(1), TimeSpan.FromSeconds(16), (_, _) => Task.CompletedTask));

        await uploader.UploadAsync(new byte[] { 1, 2, 3 }, "warming.dar");

        var warnings = logger.Entries.Count(e => e.Level == LogLevel.Warning);
        Assert.Equal(2, warnings);
    }

    [Fact]
    public async Task UploadAsync_posts_dar_bytes_with_octet_stream_and_bearer_token()
    {
        byte[]? capturedBytes = null;
        string? capturedContentType = null;
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers(async (req, ct) =>
        {
            capturedBytes = req.Content is null ? null : await req.Content.ReadAsByteArrayAsync(ct);
            capturedContentType = req.Content?.Headers.ContentType?.MediaType;
            return new HttpResponseMessage(HttpStatusCode.OK);
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = new DarUploader(http, StaticTokenProvider("tok-1"));
        var bytes = new byte[] { 0x50, 0x4B, 0x03, 0x04, 0xDE, 0xAD };

        var outcome = await uploader.UploadAsync(bytes, "fixture.dar");

        Assert.Equal(DarUploadOutcome.Uploaded, outcome);
        var recorded = Assert.Single(handler.Requests, r => r.Method == HttpMethod.Post);
        Assert.Equal("/v2/dars", recorded.Uri.AbsolutePath);
        Assert.Equal(
            $"synchronizerId={Uri.EscapeDataString(SynchronizerDiscoveryResponder.DefaultGlobalSynchronizerId)}",
            recorded.Uri.Query.TrimStart('?'));
        Assert.Equal("Bearer tok-1", recorded.Headers["Authorization"]);
        Assert.Equal("application/octet-stream", capturedContentType);
        Assert.Equal(bytes, capturedBytes);
    }

    [Fact]
    public async Task UploadAsync_returns_already_known_when_known_package_version_and_main_package_is_vetted()
    {
        var handler = new RecordingHandler(WithVetting((_, _) => Task.FromResult(KnownPackageVersion()), _ => true));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = new DarUploader(http, StaticTokenProvider("tok"));

        var outcome = await uploader.UploadAsync(DarFixtures.NoopDar(), "again.dar");

        Assert.Equal(DarUploadOutcome.AlreadyKnown, outcome);
        var vettingRequest = Assert.Single(handler.Requests, r => r.Uri.AbsolutePath.EndsWith("v2/package-vetting/list", StringComparison.Ordinal));
        using var doc = JsonDocument.Parse(vettingRequest.Body);
        Assert.Equal(
            "b1008daac9810baa8ca7ff313c13bd48d65f18040b2d33faabe5b5d39acaac9e",
            doc.RootElement.GetProperty("packageMetadataFilter").GetProperty("packageIds")[0].GetString());
        Assert.Equal(
            SynchronizerDiscoveryResponder.DefaultGlobalSynchronizerId,
            doc.RootElement.GetProperty("topologyStateFilter").GetProperty("synchronizerIds")[0].GetString());
    }

    [Fact]
    public async Task UploadAsync_throws_naming_both_package_ids_when_known_package_version_and_main_package_is_not_vetted()
    {
        var handler = new RecordingHandler(WithVetting((_, _) => Task.FromResult(KnownPackageVersion()), _ => false));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = new DarUploader(http, StaticTokenProvider("tok"));

        var exception = await Assert.ThrowsAsync<DarNotVettedException>(
            () => uploader.UploadAsync(DarFixtures.NoopDar(), "rebuilt.dar"));

        Assert.Equal("b1008daac9810baa8ca7ff313c13bd48d65f18040b2d33faabe5b5d39acaac9e", exception.MainPackageId);
        Assert.Equal(new[] { "d0048119d779f3dc1c02dd2549c6281cdd72998bc3c9bc514fe07ff5d5365bbd" }, exception.ConflictingPackageIds);
        Assert.Equal(SynchronizerDiscoveryResponder.DefaultGlobalSynchronizerId, exception.SynchronizerId);
        Assert.Equal(HttpStatusCode.BadRequest, exception.StatusCode);
        Assert.Contains("KNOWN_PACKAGE_VERSION", exception.ResponseBody);
        Assert.Contains("b1008daac9810baa8ca7ff313c13bd48d65f18040b2d33faabe5b5d39acaac9e", exception.Message);
        Assert.Contains("d0048119d779f3dc1c02dd2549c6281cdd72998bc3c9bc514fe07ff5d5365bbd", exception.Message);
        Assert.Contains("rebuilt.dar", exception.Message);
    }

    [Fact]
    public async Task UploadAsync_throws_without_conflicting_ids_when_the_known_package_version_body_names_none()
    {
        var handler = new RecordingHandler(WithVetting(
            (_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.BadRequest)
            {
                Content = new StringContent("KNOWN_PACKAGE_VERSION", Encoding.UTF8, "text/plain"),
            }),
            _ => false));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = new DarUploader(http, StaticTokenProvider("tok"));

        var exception = await Assert.ThrowsAsync<DarNotVettedException>(
            () => uploader.UploadAsync(DarFixtures.NoopDar(), "rebuilt.dar"));

        Assert.Equal("b1008daac9810baa8ca7ff313c13bd48d65f18040b2d33faabe5b5d39acaac9e", exception.MainPackageId);
        Assert.Empty(exception.ConflictingPackageIds);
    }

    [Fact]
    public async Task UploadAsync_throws_when_the_main_package_is_not_vetted_on_the_second_synchronizer_only()
    {
        var handler = new RecordingHandler(WithVetting(
            (_, _) => Task.FromResult(KnownPackageVersion()),
            synchronizerId => synchronizerId == "global::122a",
            ("global", "global::122a"),
            ("app-synchronizer", "app-synchronizer::122b")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = new DarUploader(http, StaticTokenProvider("tok"));

        var exception = await Assert.ThrowsAsync<DarNotVettedException>(
            () => uploader.UploadAsync(DarFixtures.NoopDar(), "partial.dar"));

        Assert.Equal("app-synchronizer::122b", exception.SynchronizerId);
    }

    [Fact]
    public async Task UploadAsync_returns_uploaded_when_one_synchronizer_accepts_and_the_other_is_known_and_vetted()
    {
        var handler = new RecordingHandler(WithVetting(
            (req, _) => Task.FromResult(SynchronizerIdFromQuery(req.RequestUri!) == "global::122a"
                ? new HttpResponseMessage(HttpStatusCode.OK)
                : KnownPackageVersion()),
            _ => true,
            ("global", "global::122a"),
            ("app-synchronizer", "app-synchronizer::122b")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = new DarUploader(http, StaticTokenProvider("tok"));

        var outcome = await uploader.UploadAsync(DarFixtures.NoopDar(), "mixed.dar");

        Assert.Equal(DarUploadOutcome.Uploaded, outcome);
    }

    [Fact]
    public async Task UploadAsync_reports_the_unreadable_manifest_when_known_package_version_comes_back_for_a_non_dar_payload()
    {
        var handler = new RecordingHandler(WithVetting((_, _) => Task.FromResult(KnownPackageVersion()), _ => true));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = new DarUploader(http, StaticTokenProvider("tok"));

        await Assert.ThrowsAsync<InvalidDataException>(
            () => uploader.UploadAsync(new byte[] { 1, 2, 3 }, "garbage.dar"));
    }

    [Fact]
    public async Task UploadAsync_throws_for_genuine_server_failure()
    {
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.InternalServerError)
        {
            Content = new StringContent("""{"cause":"boom"}""", Encoding.UTF8, "application/json"),
        })));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = new DarUploader(http, StaticTokenProvider("tok"));

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(
            () => uploader.UploadAsync(new byte[] { 1, 2, 3 }, "broken.dar"));
        Assert.Equal(HttpStatusCode.InternalServerError, exception.StatusCode);
        Assert.Contains("boom", exception.ResponseBody);
    }

    [Fact]
    public async Task UploadAsync_throws_for_non_known_package_400()
    {
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.BadRequest)
        {
            Content = new StringContent("""{"cause":"INVALID_ARGUMENT"}""", Encoding.UTF8, "application/json"),
        })));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = new DarUploader(http, StaticTokenProvider("tok"));

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(
            () => uploader.UploadAsync(new byte[] { 1, 2, 3 }, "bad.dar"));
        Assert.Equal(HttpStatusCode.BadRequest, exception.StatusCode);
        Assert.Contains("INVALID_ARGUMENT", exception.ResponseBody);
    }

    [Fact]
    public async Task UploadManyAsync_uploads_multiple_dars_sequentially_in_order()
    {
        var directory = Directory.CreateTempSubdirectory("dar-uploader-tests-");
        try
        {
            var first = Path.Combine(directory.FullName, "a.dar");
            var second = Path.Combine(directory.FullName, "b.dar");
            await File.WriteAllBytesAsync(first, new byte[] { 1 });
            await File.WriteAllBytesAsync(second, DarFixtures.NoopDar());

            var calls = 0;
            var requestUris = new List<Uri>();
            var handler = new RecordingHandler(WithVetting(
                (req, _) =>
                {
                    calls++;
                    requestUris.Add(req.RequestUri!);
                    return Task.FromResult(calls == 1
                        ? new HttpResponseMessage(HttpStatusCode.OK)
                        : KnownPackageVersion());
                },
                _ => true));
            using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
            var uploader = new DarUploader(http, StaticTokenProvider("tok"));

            var results = await uploader.UploadManyAsync(new[] { first, second });

            Assert.Equal(2, results.Count);
            Assert.Equal(first, results[0].Source);
            Assert.Equal(DarUploadOutcome.Uploaded, results[0].Outcome);
            Assert.Equal(second, results[1].Source);
            Assert.Equal(DarUploadOutcome.AlreadyKnown, results[1].Outcome);
            Assert.Equal(2, calls);
            Assert.All(requestUris, uri => Assert.EndsWith("v2/dars", uri.AbsolutePath));
        }
        finally
        {
            directory.Delete(recursive: true);
        }
    }

    [Fact]
    public async Task UploadManyAsync_stops_on_genuine_failure()
    {
        var directory = Directory.CreateTempSubdirectory("dar-uploader-tests-");
        try
        {
            var first = Path.Combine(directory.FullName, "a.dar");
            var second = Path.Combine(directory.FullName, "b.dar");
            await File.WriteAllBytesAsync(first, new byte[] { 1 });
            await File.WriteAllBytesAsync(second, new byte[] { 2 });

            var calls = 0;
            var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers((_, _) =>
            {
                calls++;
                return Task.FromResult(new HttpResponseMessage(HttpStatusCode.InternalServerError)
                {
                    Content = new StringContent("oops", Encoding.UTF8, "text/plain"),
                });
            }));
            using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
            var uploader = new DarUploader(http, StaticTokenProvider("tok"));

            await Assert.ThrowsAsync<JsonLedgerApiException>(
                () => uploader.UploadManyAsync(new[] { first, second }));
            Assert.Equal(1, calls);
        }
        finally
        {
            directory.Delete(recursive: true);
        }
    }

    [Fact]
    public async Task UploadAsync_vets_on_every_connected_synchronizer()
    {
        var postedSynchronizerIds = new List<string?>();
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers(
            (req, _) =>
            {
                postedSynchronizerIds.Add(SynchronizerIdFromQuery(req.RequestUri!));
                return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK));
            },
            ("global", "global::122a"),
            ("app-synchronizer", "app-synchronizer::122b")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = new DarUploader(http, StaticTokenProvider("tok"));

        var outcome = await uploader.UploadAsync(new byte[] { 1, 2, 3 }, "multi-sync.dar");

        Assert.Equal(DarUploadOutcome.Uploaded, outcome);
        Assert.Equal(new[] { "global::122a", "app-synchronizer::122b" }, postedSynchronizerIds);
    }

    [Fact]
    public async Task UploadAsync_falls_back_to_no_synchronizerId_when_none_connected()
    {
        var handler = new RecordingHandler((req, _) =>
        {
            if (req.Method == HttpMethod.Get)
            {
                return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
                {
                    Content = new StringContent("""{"connectedSynchronizers":[]}""", Encoding.UTF8, "application/json"),
                });
            }
            return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK));
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = new DarUploader(http, StaticTokenProvider("tok"));

        var outcome = await uploader.UploadAsync(new byte[] { 1, 2, 3 }, "no-sync.dar");

        Assert.Equal(DarUploadOutcome.Uploaded, outcome);
        var recorded = Assert.Single(handler.Requests, r => r.Method == HttpMethod.Post);
        Assert.Equal(new Uri(JsonApiBase, "v2/dars"), recorded.Uri);
    }

    [Fact]
    public async Task UploadAsync_rejects_empty_payload()
    {
        using var http = new HttpClient(new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK))))
        {
            BaseAddress = JsonApiBase,
        };
        var uploader = new DarUploader(http, StaticTokenProvider("tok"));

        await Assert.ThrowsAsync<ArgumentException>(
            () => uploader.UploadAsync(Array.Empty<byte>(), "empty.dar"));
    }

    [Fact]
    public void Constructor_throws_when_HttpClient_has_no_BaseAddress()
    {
        using var http = new HttpClient();
        Assert.Throws<ArgumentException>(() => new DarUploader(http, StaticTokenProvider("tok")));
    }
}
