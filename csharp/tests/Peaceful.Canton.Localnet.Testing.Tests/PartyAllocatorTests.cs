// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;
using System.Text;
using System.Text.Json;
using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

public class PartyAllocatorTests
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

    private static HttpResponseMessage AllocateResponse(string party, bool isLocal = true)
    {
        var payload = new
        {
            partyDetails = new
            {
                party,
                isLocal,
            },
        };
        return new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent(JsonSerializer.Serialize(payload), Encoding.UTF8, "application/json"),
        };
    }

    private static DarUploaderRetryOptions NoSleepRetryOptions(int maxAttempts, List<TimeSpan>? capturedDelays = null) =>
        new(
            MaxAttempts: maxAttempts,
            BaseDelay: TimeSpan.FromSeconds(1),
            MaxDelay: TimeSpan.FromSeconds(16),
            Delay: (delay, _) =>
            {
                capturedDelays?.Add(delay);
                return Task.CompletedTask;
            });

    private static HttpResponseMessage Failure(HttpStatusCode status, string body = "") =>
        new(status) { Content = new StringContent(body, Encoding.UTF8, "application/json") };

    private static HttpResponseMessage KnownParties(params string[] parties) =>
        new(HttpStatusCode.OK)
        {
            Content = new StringContent(
                JsonSerializer.Serialize(new
                {
                    partyDetails = parties.Select(party => new { party, isLocal = true }),
                    nextPageToken = string.Empty,
                }),
                Encoding.UTF8,
                "application/json"),
        };

    private static Func<HttpRequestMessage, CancellationToken, Task<HttpResponseMessage>> ScriptedAllocations(
        Func<int, HttpResponseMessage> allocationForAttempt,
        Func<HttpRequestMessage, HttpResponseMessage>? partyLookup = null)
    {
        var allocationAttempts = 0;
        return SynchronizerDiscoveryResponder.WithConnectedSynchronizers((request, _) =>
        {
            if (request.Method == HttpMethod.Get)
            {
                return Task.FromResult(partyLookup!(request));
            }

            return Task.FromResult(allocationForAttempt(Interlocked.Increment(ref allocationAttempts)));
        });
    }

    private const string PartyAlreadyExistsBody =
        """{"code":"INVALID_ARGUMENT","cause":"The submitted request has invalid arguments: Party already exists: party globex-abcdef::122034ab... is already allocated on this node"}""";

    [Fact]
    public async Task AllocateAsync_retries_transient_503_then_returns_the_party()
    {
        var handler = new RecordingHandler(ScriptedAllocations(attempt => attempt switch
        {
            1 => Failure(HttpStatusCode.ServiceUnavailable, """{"code":"REQUEST_TIME_OUT"}"""),
            _ => AllocateResponse("globex-abcdef::namespace"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef", retryOptions: NoSleepRetryOptions(6));

        var party = await allocator.AllocateAsync("globex");

        Assert.Equal("globex-abcdef::namespace", party.PartyId);
        Assert.Equal(2, handler.Requests.Count(r => r.Method == HttpMethod.Post));
    }

    private const string RequestAlreadyInFlightBody =
        """{"code":"REQUEST_ALREADY_IN_FLIGHT","cause":"Request already in flight"}""";

    [Fact]
    public async Task AllocateAsync_retries_transient_504_then_returns_the_party()
    {
        var handler = new RecordingHandler(ScriptedAllocations(attempt => attempt switch
        {
            1 => Failure(HttpStatusCode.GatewayTimeout),
            _ => AllocateResponse("globex-abcdef::namespace"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef", retryOptions: NoSleepRetryOptions(6));

        var party = await allocator.AllocateAsync("globex");

        Assert.Equal("globex-abcdef::namespace", party.PartyId);
        Assert.Equal(2, handler.Requests.Count(r => r.Method == HttpMethod.Post));
    }

    [Fact]
    public async Task AllocateAsync_waits_out_a_409_request_already_in_flight_then_accepts_already_exists_as_success()
    {
        var handler = new RecordingHandler(ScriptedAllocations(
            attempt => attempt switch
            {
                1 => Failure(HttpStatusCode.ServiceUnavailable),
                2 => Failure(HttpStatusCode.Conflict, RequestAlreadyInFlightBody),
                _ => Failure(HttpStatusCode.BadRequest, PartyAlreadyExistsBody),
            },
            _ => KnownParties("globex-abcdef::122034ab5c6d")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef", retryOptions: NoSleepRetryOptions(6));

        var party = await allocator.AllocateAsync("globex");

        Assert.Equal("globex-abcdef::122034ab5c6d", party.PartyId);
        Assert.Equal(3, handler.Requests.Count(r => r.Method == HttpMethod.Post));
    }

    [Fact]
    public async Task AllocateAsync_retries_a_409_request_already_in_flight_on_the_first_attempt()
    {
        var handler = new RecordingHandler(ScriptedAllocations(attempt => attempt switch
        {
            1 => Failure(HttpStatusCode.Conflict, RequestAlreadyInFlightBody),
            _ => AllocateResponse("globex-abcdef::namespace"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef", retryOptions: NoSleepRetryOptions(6));

        var party = await allocator.AllocateAsync("globex");

        Assert.Equal("globex-abcdef::namespace", party.PartyId);
        Assert.Equal(2, handler.Requests.Count(r => r.Method == HttpMethod.Post));
    }

    [Fact]
    public async Task AllocateAsync_gives_up_after_the_attempt_bound_on_a_persistent_409_request_already_in_flight()
    {
        var handler = new RecordingHandler(ScriptedAllocations(_ => Failure(HttpStatusCode.Conflict, RequestAlreadyInFlightBody)));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef", retryOptions: NoSleepRetryOptions(3));

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(() => allocator.AllocateAsync("globex"));

        Assert.Equal(HttpStatusCode.Conflict, exception.StatusCode);
        Assert.Contains("REQUEST_ALREADY_IN_FLIGHT", exception.ResponseBody);
        Assert.Equal(3, handler.Requests.Count(r => r.Method == HttpMethod.Post));
    }

    [Fact]
    public async Task AllocateAsync_backs_off_exponentially_between_503_retries()
    {
        var delays = new List<TimeSpan>();
        var handler = new RecordingHandler(ScriptedAllocations(attempt => attempt switch
        {
            <= 3 => Failure(HttpStatusCode.ServiceUnavailable),
            _ => AllocateResponse("globex-abcdef::namespace"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef", retryOptions: NoSleepRetryOptions(6, delays));

        await allocator.AllocateAsync("globex");

        Assert.Equal(new[] { TimeSpan.FromSeconds(1), TimeSpan.FromSeconds(2), TimeSpan.FromSeconds(4) }, delays);
    }

    [Fact]
    public async Task AllocateAsync_retries_a_request_timeout_then_returns_the_party()
    {
        var allocationAttempts = 0;
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers((_, _) =>
            Interlocked.Increment(ref allocationAttempts) == 1
                ? throw new TaskCanceledException("The request was canceled due to the configured HttpClient.Timeout of 100 seconds elapsing.", new TimeoutException())
                : Task.FromResult(AllocateResponse("globex-abcdef::namespace"))));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef", retryOptions: NoSleepRetryOptions(6));

        var party = await allocator.AllocateAsync("globex");

        Assert.Equal("globex-abcdef::namespace", party.PartyId);
        Assert.Equal(2, allocationAttempts);
    }

    [Fact]
    public async Task AllocateAsync_returns_the_existing_party_when_a_retry_finds_it_already_allocated()
    {
        var handler = new RecordingHandler(ScriptedAllocations(
            attempt => attempt switch
            {
                1 => Failure(HttpStatusCode.ServiceUnavailable),
                _ => Failure(HttpStatusCode.BadRequest, PartyAlreadyExistsBody),
            },
            _ => KnownParties("globex-abcdef::122034ab5c6d")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef", retryOptions: NoSleepRetryOptions(6));

        var party = await allocator.AllocateAsync("globex");

        Assert.Equal("globex-abcdef::122034ab5c6d", party.PartyId);
        Assert.Equal("globex-abcdef", party.PartyIdHint);
        Assert.True(party.IsLocal);
        Assert.Equal(2, handler.Requests.Count(r => r.Method == HttpMethod.Post));
    }

    [Fact]
    public async Task AllocateAsync_looks_the_party_up_by_hint_prefix_when_a_retry_finds_it_already_allocated()
    {
        var handler = new RecordingHandler(ScriptedAllocations(
            attempt => attempt switch
            {
                1 => Failure(HttpStatusCode.ServiceUnavailable),
                _ => Failure(HttpStatusCode.BadRequest, PartyAlreadyExistsBody),
            },
            _ => KnownParties("globex-abcdef2::122034ffff", "globex-abcdef::122034ab5c6d")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef", retryOptions: NoSleepRetryOptions(6));

        var party = await allocator.AllocateAsync("globex");

        Assert.Equal("globex-abcdef::122034ab5c6d", party.PartyId);
        var lookup = Assert.Single(handler.Requests, r => r.Method == HttpMethod.Get && r.Uri.AbsolutePath == "/v2/parties");
        Assert.Equal("?filter-party=globex-abcdef%3A%3A", lookup.Uri.Query);
    }

    [Fact]
    public async Task AllocateAsync_gives_up_after_the_attempt_bound_on_persistent_503()
    {
        var handler = new RecordingHandler(ScriptedAllocations(
            _ => Failure(HttpStatusCode.ServiceUnavailable, """{"code":"REQUEST_TIME_OUT"}""")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef", retryOptions: NoSleepRetryOptions(3));

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(() => allocator.AllocateAsync("globex"));

        Assert.Equal(HttpStatusCode.ServiceUnavailable, exception.StatusCode);
        Assert.Contains("REQUEST_TIME_OUT", exception.ResponseBody);
        Assert.Contains("503", exception.Message);
        Assert.Equal(3, handler.Requests.Count(r => r.Method == HttpMethod.Post));
    }

    [Fact]
    public async Task AllocateAsync_gives_up_after_the_attempt_bound_on_persistent_request_timeout()
    {
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers((_, _) =>
            throw new TaskCanceledException("timed out", new TimeoutException())));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef", retryOptions: NoSleepRetryOptions(3));

        var exception = await Assert.ThrowsAsync<TimeoutException>(() => allocator.AllocateAsync("globex"));

        Assert.Contains("3 attempts", exception.Message);
        Assert.Contains("globex-abcdef", exception.Message);
        Assert.Equal(3, handler.Requests.Count(r => r.Method == HttpMethod.Post));
    }

    [Theory]
    [InlineData(HttpStatusCode.BadRequest)]
    [InlineData(HttpStatusCode.Unauthorized)]
    [InlineData(HttpStatusCode.Forbidden)]
    [InlineData(HttpStatusCode.Conflict)]
    [InlineData(HttpStatusCode.InternalServerError)]
    public async Task AllocateAsync_does_not_retry_a_non_transient_error(HttpStatusCode status)
    {
        var handler = new RecordingHandler(ScriptedAllocations(_ => Failure(status, """{"code":"INVALID_ARGUMENT"}""")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef", retryOptions: NoSleepRetryOptions(6));

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(() => allocator.AllocateAsync("globex"));

        Assert.Equal(status, exception.StatusCode);
        Assert.Single(handler.Requests, r => r.Method == HttpMethod.Post);
    }

    [Fact]
    public async Task AllocateAsync_does_not_treat_already_exists_on_the_first_attempt_as_success()
    {
        var handler = new RecordingHandler(ScriptedAllocations(
            _ => Failure(HttpStatusCode.BadRequest, PartyAlreadyExistsBody),
            _ => KnownParties("globex-abcdef::122034ab5c6d")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef", retryOptions: NoSleepRetryOptions(6));

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(() => allocator.AllocateAsync("globex"));

        Assert.Equal(HttpStatusCode.BadRequest, exception.StatusCode);
        Assert.Contains("Party already exists", exception.ResponseBody);
        Assert.DoesNotContain(handler.Requests, r => r.Method == HttpMethod.Get && r.Uri.AbsolutePath == "/v2/parties");
    }

    [Fact]
    public async Task AllocateAsync_surfaces_the_already_exists_error_when_the_lookup_finds_no_party()
    {
        var handler = new RecordingHandler(ScriptedAllocations(
            attempt => attempt switch
            {
                1 => Failure(HttpStatusCode.ServiceUnavailable),
                _ => Failure(HttpStatusCode.BadRequest, PartyAlreadyExistsBody),
            },
            _ => KnownParties()));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef", retryOptions: NoSleepRetryOptions(6));

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(() => allocator.AllocateAsync("globex"));

        Assert.Equal(HttpStatusCode.BadRequest, exception.StatusCode);
        Assert.Contains("Party already exists", exception.ResponseBody);
    }

    [Fact]
    public async Task AllocateAsync_posts_party_hint_in_consumer_prefix_dash_instance_suffix_format()
    {
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers(
            (_, _) => Task.FromResult(AllocateResponse("globex-abcdef::namespace"))));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef");

        var party = await allocator.AllocateAsync("globex");

        Assert.Equal("globex-abcdef::namespace", party.PartyId);
        Assert.Equal("globex-abcdef", party.PartyIdHint);
        var recorded = Assert.Single(handler.Requests, r => r.Method == HttpMethod.Post);
        Assert.Equal(new Uri(JsonApiBase, "v2/parties"), recorded.Uri);
        using var bodyDoc = JsonDocument.Parse(recorded.Body);
        Assert.Equal("globex-abcdef", bodyDoc.RootElement.GetProperty("partyIdHint").GetString());
        Assert.Equal("globex-abcdef", bodyDoc.RootElement.GetProperty("displayName").GetString());
    }

    [Fact]
    public async Task AllocateAsync_uses_explicit_display_name_when_provided()
    {
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers(
            (_, _) => Task.FromResult(AllocateResponse("initech-deadbe::n"))));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "deadbe");

        await allocator.AllocateAsync("initech", displayName: "Ledger API Party");

        var recorded = Assert.Single(handler.Requests, r => r.Method == HttpMethod.Post);
        using var bodyDoc = JsonDocument.Parse(recorded.Body);
        Assert.Equal("initech-deadbe", bodyDoc.RootElement.GetProperty("partyIdHint").GetString());
        Assert.Equal("Ledger API Party", bodyDoc.RootElement.GetProperty("displayName").GetString());
    }

    [Fact]
    public async Task AllocateAsync_defaults_synchronizerId_to_global_alias_resolved_via_discovery()
    {
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers(
            (_, _) => Task.FromResult(AllocateResponse("globex-abcdef::namespace")),
            ("global", "global::122a"),
            ("app-synchronizer", "app-synchronizer::122b")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef");

        await allocator.AllocateAsync("globex");

        var recorded = Assert.Single(handler.Requests, r => r.Method == HttpMethod.Post);
        using var bodyDoc = JsonDocument.Parse(recorded.Body);
        Assert.Equal("global::122a", bodyDoc.RootElement.GetProperty("synchronizerId").GetString());
        Assert.Single(handler.Requests, r => r.Method == HttpMethod.Get);
    }

    [Fact]
    public async Task AllocateOnSynchronizerAsync_with_explicit_synchronizerId_skips_discovery_and_sends_it_verbatim()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(AllocateResponse("globex-abcdef::namespace")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef");

        await allocator.AllocateOnSynchronizerAsync("globex", displayName: null, synchronizerId: "app-synchronizer::122b");

        var recorded = Assert.Single(handler.Requests);
        Assert.Equal(HttpMethod.Post, recorded.Method);
        using var bodyDoc = JsonDocument.Parse(recorded.Body);
        Assert.Equal("app-synchronizer::122b", bodyDoc.RootElement.GetProperty("synchronizerId").GetString());
    }

    [Fact]
    public void ComposeHint_format_is_prefix_dash_suffix()
    {
        using var http = new HttpClient(new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK))))
        {
            BaseAddress = JsonApiBase,
        };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "fixed");
        Assert.Equal("globex-fixed", allocator.ComposeHint("globex"));
        Assert.Equal("initech-fixed", allocator.ComposeHint("initech"));
    }

    [Fact]
    public void Default_instance_suffix_is_random_per_instance()
    {
        using var http1 = new HttpClient(new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK))))
        {
            BaseAddress = JsonApiBase,
        };
        using var http2 = new HttpClient(new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK))))
        {
            BaseAddress = JsonApiBase,
        };
        var first = new PartyAllocator(http1, StaticTokenProvider("tok"));
        var second = new PartyAllocator(http2, StaticTokenProvider("tok"));

        Assert.NotEqual(first.InstanceSuffix, second.InstanceSuffix);
        Assert.False(string.IsNullOrWhiteSpace(first.InstanceSuffix));
        Assert.False(string.IsNullOrWhiteSpace(second.InstanceSuffix));
    }

    [Theory]
    [InlineData(null)]
    [InlineData("")]
    [InlineData("   ")]
    [InlineData("\t")]
    public void Whitespace_only_instance_suffix_falls_back_to_random(string? supplied)
    {
        using var http = new HttpClient(new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK))))
        {
            BaseAddress = JsonApiBase,
        };

        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: supplied);

        Assert.False(string.IsNullOrWhiteSpace(allocator.InstanceSuffix));
        Assert.NotEqual(supplied, allocator.InstanceSuffix);
        Assert.DoesNotContain(' ', allocator.InstanceSuffix);
        Assert.DoesNotContain('\t', allocator.InstanceSuffix);
    }

    [Fact]
    public async Task AllocateAsync_throws_JsonLedgerApiException_on_non_success_status()
    {
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.Conflict)
        {
            Content = new StringContent("""{"cause":"PARTY_ALREADY_EXISTS"}""", Encoding.UTF8, "application/json"),
        })));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abc");

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(() => allocator.AllocateAsync("globex"));
        Assert.Equal(HttpStatusCode.Conflict, exception.StatusCode);
        Assert.Contains("PARTY_ALREADY_EXISTS", exception.ResponseBody);
    }

    [Fact]
    public async Task AllocateAsync_throws_when_response_has_no_party_field()
    {
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent("""{"somethingElse":"x"}""", Encoding.UTF8, "application/json"),
        })));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abc");

        await Assert.ThrowsAsync<JsonLedgerApiException>(() => allocator.AllocateAsync("globex"));
    }

    [Fact]
    public async Task AllocateAsync_sends_bearer_token()
    {
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers(
            (_, _) => Task.FromResult(AllocateResponse("globex-x::n"))));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok-bearer-x"), instanceSuffix: "x");

        await allocator.AllocateAsync("globex");

        var recorded = Assert.Single(handler.Requests, r => r.Method == HttpMethod.Post);
        Assert.Equal("Bearer tok-bearer-x", recorded.Headers["Authorization"]);
    }

    [Fact]
    public void Constructor_throws_when_HttpClient_has_no_BaseAddress()
    {
        using var http = new HttpClient();
        Assert.Throws<ArgumentException>(() => new PartyAllocator(http, StaticTokenProvider("tok")));
    }

    [Fact]
    public async Task AllocateAsync_rejects_empty_prefix()
    {
        using var http = new HttpClient(new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK))))
        {
            BaseAddress = JsonApiBase,
        };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abc");
        await Assert.ThrowsAsync<ArgumentException>(() => allocator.AllocateAsync(""));
    }
}
