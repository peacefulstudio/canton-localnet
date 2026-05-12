// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;
using System.Text;
using System.Text.Json;
using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

public class DarUploaderTests
{
    private static readonly Uri JsonApiBase = new("http://localhost:3975/");

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
