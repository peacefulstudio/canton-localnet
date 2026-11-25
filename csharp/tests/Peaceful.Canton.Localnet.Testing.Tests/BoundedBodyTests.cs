// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;
using System.Text;
using System.Text.Json;
using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

public class BoundedBodyTests
{
    private const int Bound = 16_777_216;
    private const long StreamedBytes = 64L << 20;
    private const long MaxBytesPulled = Bound + (1 << 20);
    private static readonly Uri JsonApiBase = new("http://localhost:11975/");

    [Fact]
    public async Task ReadAsync_returns_a_small_body_whole()
    {
        using var response = new HttpResponseMessage(HttpStatusCode.BadRequest)
        {
            Content = new StringContent("""{"cause":"nope"}""", Encoding.UTF8, "application/json"),
        };

        var body = await BoundedBody.ReadAsync(response, "GET v2/x", Timeout.InfiniteTimeSpan, CancellationToken.None);

        Assert.Equal("""{"cause":"nope"}""", body);
    }

    [Fact]
    public async Task ReadAsync_returns_a_body_of_exactly_sixteen_mebibytes_whole()
    {
        using var response = new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StreamContent(new CountingXStream(Bound)),
        };

        var body = await BoundedBody.ReadAsync(response, "GET v2/x", Timeout.InfiniteTimeSpan, CancellationToken.None);

        Assert.Equal(Bound, body.Length);
    }

    public static TheoryData<string, int> LedgerOperations() => new()
    {
        { "GetParticipantIdAsync", 200 },
        { "GetParticipantIdAsync", 500 },
        { "GetConnectedSynchronizersAsync", 200 },
        { "IsPackageVettedAsync", 200 },
        { "AllocateOnSynchronizerAsync", 200 },
        { "AllocateOnSynchronizerAsync", 503 },
        { "CreateAsync", 200 },
        { "GrantRightsAsync", 200 },
        { "RevokeRightsAsync", 200 },
        { "DeleteAsync", 200 },
        { "RightsStillHeldAsync", 200 },
        { "UploadAsync", 200 },
        { "UploadAsync", 503 },
    };

    [Theory]
    [MemberData(nameof(LedgerOperations))]
    public async Task LedgerCall_throws_naming_the_bound_when_a_response_body_over_sixteen_mebibytes_streams_through_HttpClient(
        string operation,
        int status)
    {
        var oversized = new CountingXStream(StreamedBytes);
        var handler = new RecordingHandler((request, _) => Task.FromResult(
            IsTargetRequest(operation, request)
                ? new HttpResponseMessage((HttpStatusCode)status) { Content = new StreamContent(oversized) }
                : Ordinary(request)));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var tokens = StaticTokenProvider();

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(() => Invoke(operation, http, tokens));

        Assert.Equal((HttpStatusCode)status, exception.StatusCode);
        Assert.Contains("16777216-byte bound", exception.Message);
        Assert.InRange(oversized.BytesRead, Bound + 1, MaxBytesPulled);
    }

    [Fact]
    public async Task GetAccessTokenAsync_throws_naming_the_bound_when_the_token_body_over_sixteen_mebibytes_streams_through_HttpClient()
    {
        var oversized = new CountingXStream(StreamedBytes);
        using var http = new HttpClient(new RecordingHandler((_, _) => Task.FromResult(
            new HttpResponseMessage(HttpStatusCode.OK) { Content = new StreamContent(oversized) })));
        var provider = new OAuth2TokenProvider(http, TokenOptions());

        var exception = await Assert.ThrowsAsync<OAuth2TokenException>(() => provider.GetAccessTokenAsync().AsTask());

        Assert.Equal(HttpStatusCode.OK, exception.StatusCode);
        Assert.Contains("16777216-byte bound", exception.Message);
        Assert.InRange(oversized.BytesRead, Bound + 1, MaxBytesPulled);
    }

    [Theory]
    [InlineData(200)]
    [InlineData(500)]
    public async Task LedgerCall_times_out_on_a_stalled_body_within_the_HttpClient_timeout(int status)
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(
            new HttpResponseMessage((HttpStatusCode)status) { Content = new StreamContent(new StalledStream()) }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase, Timeout = TimeSpan.FromMilliseconds(200) };
        var admin = new JsonLedgerAdminClient(http, StaticTokenProvider());

        var call = admin.GetParticipantIdAsync();
        var finished = await Task.WhenAny(call, Task.Delay(TimeSpan.FromSeconds(10)));

        Assert.Same(call, finished);
        await Assert.ThrowsAnyAsync<OperationCanceledException>(() => call);
    }

    [Fact]
    public void ForMessage_keeps_a_512_character_body_whole()
    {
        var body = new string('y', 512);

        Assert.Equal(body, BoundedBody.ForMessage(body));
    }

    [Fact]
    public void ForMessage_cuts_a_longer_body_at_512_characters_and_marks_the_cut()
    {
        var body = new string('y', 513);

        Assert.Equal(new string('y', 512) + "...[truncated]", BoundedBody.ForMessage(body));
    }

    [Fact]
    public void ForMessage_trims_surrounding_whitespace()
    {
        Assert.Equal("""{"cause":"nope"}""", BoundedBody.ForMessage("  {\"cause\":\"nope\"}\n"));
    }

    private static bool IsTargetRequest(string operation, HttpRequestMessage request)
    {
        var path = request.RequestUri!.AbsolutePath;
        return operation switch
        {
            "IsPackageVettedAsync" => path.EndsWith("v2/package-vetting/list", StringComparison.Ordinal),
            "UploadAsync" => request.Method == HttpMethod.Post,
            _ => true,
        };
    }

    private static HttpResponseMessage Ordinary(HttpRequestMessage request)
    {
        var path = request.RequestUri!.AbsolutePath;
        var json = path.EndsWith("participant-id", StringComparison.Ordinal)
            ? """{"participantId":"participant::1220aa"}"""
            : """{"connectedSynchronizers":[{"synchronizerAlias":"global","synchronizerId":"global::122a"}]}""";
        return new HttpResponseMessage(HttpStatusCode.OK) { Content = new StringContent(json, Encoding.UTF8, "application/json") };
    }

    private static async Task Invoke(string operation, HttpClient http, OAuth2TokenProvider tokens)
    {
        var admin = new JsonLedgerAdminClient(http, tokens);
        var users = new UserBuilder(http, tokens);
        var noSleep = new DarUploaderRetryOptions(1, TimeSpan.Zero, TimeSpan.Zero, (_, _) => Task.CompletedTask);
        var right = JsonSerializer.SerializeToElement(new { kind = new { CanActAs = new { value = new { party = "alice::p" } } } });
        switch (operation)
        {
            case "GetParticipantIdAsync":
                await admin.GetParticipantIdAsync();
                break;
            case "GetConnectedSynchronizersAsync":
                await admin.GetConnectedSynchronizersAsync();
                break;
            case "IsPackageVettedAsync":
                await admin.IsPackageVettedAsync("b1008daac9810baa8ca7ff313c13bd48d65f18040b2d33faabe5b5d39acaac9e", "global::122a");
                break;
            case "AllocateOnSynchronizerAsync":
                await new PartyAllocator(http, tokens, instanceSuffix: "abc", retryOptions: noSleep).AllocateOnSynchronizerAsync("globex", "global::122a");
                break;
            case "CreateAsync":
                await users.CreateAsync("frank");
                break;
            case "GrantRightsAsync":
                await users.GrantRightsAsync("frank", actAs: new[] { "alice::p" });
                break;
            case "RevokeRightsAsync":
                await users.RevokeRightsAsync("frank", actAs: new[] { "alice::p" });
                break;
            case "DeleteAsync":
                await users.DeleteAsync("frank", attempt: 1, CancellationToken.None);
                break;
            case "RightsStillHeldAsync":
                await users.RightsStillHeldAsync("tok", "frank", new[] { right }, CancellationToken.None);
                break;
            case "UploadAsync":
                await new DarUploader(http, tokens, options: noSleep).UploadAsync(new byte[] { 1, 2, 3 }, "huge.dar");
                break;
            default:
                throw new ArgumentOutOfRangeException(nameof(operation), operation, null);
        }
    }

    private static OAuth2TokenProviderOptions TokenOptions() => new(
        TokenEndpoint: new Uri("https://keycloak.test/token"),
        ClientId: "id",
        ClientSecret: "sec",
        Audience: "aud",
        Scope: "openid");

    private static OAuth2TokenProvider StaticTokenProvider()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent(
                """{"access_token":"tok","token_type":"Bearer","expires_in":3600}""",
                Encoding.UTF8,
                "application/json"),
        }));
        return new OAuth2TokenProvider(new HttpClient(handler), TokenOptions());
    }

    private sealed class CountingXStream : Stream
    {
        private readonly long _length;
        private long _read;

        public CountingXStream(long length) => _length = length;

        public long BytesRead => Interlocked.Read(ref _read);

        public override bool CanRead => true;
        public override bool CanSeek => false;
        public override bool CanWrite => false;
        public override long Length => throw new NotSupportedException();
        public override long Position { get => throw new NotSupportedException(); set => throw new NotSupportedException(); }

        public override int Read(byte[] buffer, int offset, int count)
        {
            var available = (int)Math.Min(count, _length - _read);
            if (available <= 0)
            {
                return 0;
            }
            Array.Fill(buffer, (byte)'x', offset, available);
            Interlocked.Add(ref _read, available);
            return available;
        }

        public override void Flush() => throw new NotSupportedException();
        public override long Seek(long offset, SeekOrigin origin) => throw new NotSupportedException();
        public override void SetLength(long value) => throw new NotSupportedException();
        public override void Write(byte[] buffer, int offset, int count) => throw new NotSupportedException();
    }
}
