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

    private static DarUploader NoSleepUploader(HttpClient http, OAuth2TokenProvider tokenProvider) =>
        new(http, tokenProvider, options: new DarUploaderRetryOptions(
            MaxAttempts: 6,
            BaseDelay: TimeSpan.FromSeconds(1),
            MaxDelay: TimeSpan.FromSeconds(16),
            Delay: (_, _) => Task.CompletedTask));

    [Fact]
    public async Task UploadAsync_retries_transient_503_then_succeeds()
    {
        var calls = 0;
        var handler = new RecordingHandler((_, _) =>
        {
            calls++;
            var status = calls < 3 ? HttpStatusCode.ServiceUnavailable : HttpStatusCode.OK;
            return Task.FromResult(new HttpResponseMessage(status));
        });
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
        var handler = new RecordingHandler((_, _) =>
        {
            calls++;
            return Task.FromResult(new HttpResponseMessage(HttpStatusCode.ServiceUnavailable)
            {
                Content = new StringContent("""{"cause":"still warming up"}""", Encoding.UTF8, "application/json"),
            });
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = NoSleepUploader(http, StaticTokenProvider("tok"));

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(
            () => uploader.UploadAsync(new byte[] { 1, 2, 3 }, "cold.dar"));

        Assert.Equal(HttpStatusCode.ServiceUnavailable, exception.StatusCode);
        Assert.Contains("still warming up", exception.ResponseBody);
        Assert.Equal(6, calls);
    }

    [Theory]
    [InlineData(HttpStatusCode.Unauthorized)]
    [InlineData(HttpStatusCode.Forbidden)]
    public async Task UploadAsync_does_not_retry_non_503_failure(HttpStatusCode status)
    {
        var calls = 0;
        var handler = new RecordingHandler((_, _) =>
        {
            calls++;
            return Task.FromResult(new HttpResponseMessage(status)
            {
                Content = new StringContent("""{"cause":"no token"}""", Encoding.UTF8, "application/json"),
            });
        });
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
        var handler = new RecordingHandler((_, _) =>
            Task.FromResult(new HttpResponseMessage(HttpStatusCode.ServiceUnavailable)
            {
                Content = new StringContent("""{"cause":"still warming up"}""", Encoding.UTF8, "application/json"),
            }));
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
        var handler = new RecordingHandler((_, _) =>
        {
            calls++;
            var status = calls < 3 ? HttpStatusCode.ServiceUnavailable : HttpStatusCode.OK;
            return Task.FromResult(new HttpResponseMessage(status));
        });
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
        var handler = new RecordingHandler(async (req, ct) =>
        {
            capturedBytes = req.Content is null ? null : await req.Content.ReadAsByteArrayAsync(ct);
            capturedContentType = req.Content?.Headers.ContentType?.MediaType;
            return new HttpResponseMessage(HttpStatusCode.OK);
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = new DarUploader(http, StaticTokenProvider("tok-1"));
        var bytes = new byte[] { 0x50, 0x4B, 0x03, 0x04, 0xDE, 0xAD };

        var outcome = await uploader.UploadAsync(bytes, "fixture.dar");

        Assert.Equal(DarUploadOutcome.Uploaded, outcome);
        var recorded = Assert.Single(handler.Requests);
        Assert.Equal(HttpMethod.Post, recorded.Method);
        Assert.Equal(new Uri(JsonApiBase, "v2/packages"), recorded.Uri);
        Assert.Equal("Bearer tok-1", recorded.Headers["Authorization"]);
        Assert.Equal("application/octet-stream", capturedContentType);
        Assert.Equal(bytes, capturedBytes);
    }

    [Fact]
    public async Task UploadAsync_treats_known_package_version_400_as_success()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.BadRequest)
        {
            Content = new StringContent(
                """{"cause":"KNOWN_PACKAGE_VERSION(8,abcdef): package already uploaded"}""",
                Encoding.UTF8,
                "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = new DarUploader(http, StaticTokenProvider("tok"));

        var outcome = await uploader.UploadAsync(new byte[] { 1, 2, 3 }, "again.dar");

        Assert.Equal(DarUploadOutcome.AlreadyKnown, outcome);
    }

    [Fact]
    public async Task UploadAsync_throws_for_genuine_server_failure()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.InternalServerError)
        {
            Content = new StringContent("""{"cause":"boom"}""", Encoding.UTF8, "application/json"),
        }));
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
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.BadRequest)
        {
            Content = new StringContent("""{"cause":"INVALID_ARGUMENT"}""", Encoding.UTF8, "application/json"),
        }));
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
            await File.WriteAllBytesAsync(second, new byte[] { 2 });

            var calls = 0;
            var requestUris = new List<Uri>();
            var handler = new RecordingHandler((req, _) =>
            {
                calls++;
                requestUris.Add(req.RequestUri!);
                var status = calls switch
                {
                    1 => HttpStatusCode.OK,
                    _ => HttpStatusCode.BadRequest,
                };
                var body = calls switch
                {
                    1 => string.Empty,
                    _ => """{"cause":"KNOWN_PACKAGE_VERSION(...)"}""",
                };
                return Task.FromResult(new HttpResponseMessage(status)
                {
                    Content = new StringContent(body, Encoding.UTF8, "application/json"),
                });
            });
            using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
            var uploader = new DarUploader(http, StaticTokenProvider("tok"));

            var results = await uploader.UploadManyAsync(new[] { first, second });

            Assert.Equal(2, results.Count);
            Assert.Equal(first, results[0].Source);
            Assert.Equal(DarUploadOutcome.Uploaded, results[0].Outcome);
            Assert.Equal(second, results[1].Source);
            Assert.Equal(DarUploadOutcome.AlreadyKnown, results[1].Outcome);
            Assert.Equal(2, calls);
            Assert.All(requestUris, uri => Assert.EndsWith("v2/packages", uri.AbsolutePath));
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
            var handler = new RecordingHandler((_, _) =>
            {
                calls++;
                return Task.FromResult(new HttpResponseMessage(HttpStatusCode.InternalServerError)
                {
                    Content = new StringContent("oops", Encoding.UTF8, "text/plain"),
                });
            });
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
