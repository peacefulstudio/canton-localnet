// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;
using System.Text;
using System.Text.Json;
using Microsoft.Extensions.Logging;
using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

public class JsonLedgerAdminClientTests
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

    [Fact]
    public async Task GetParticipantIdAsync_calls_v2_parties_participant_id_with_bearer_token()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent("""{"participantId":"participant::1220abc"}""", Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var tokenProvider = StaticTokenProvider("tok-abc");
        var client = new JsonLedgerAdminClient(http, tokenProvider);

        var participantId = await client.GetParticipantIdAsync();

        Assert.Equal("participant::1220abc", participantId);
        var recorded = Assert.Single(handler.Requests);
        Assert.Equal(HttpMethod.Get, recorded.Method);
        Assert.Equal(new Uri(JsonApiBase, "v2/parties/participant-id"), recorded.Uri);
        Assert.True(recorded.Headers.TryGetValue("Authorization", out var auth));
        Assert.Equal("Bearer tok-abc", auth);
        Assert.True(recorded.Headers.TryGetValue("Accept", out var accept));
        Assert.Contains("application/json", accept);
    }

    [Fact]
    public async Task GetParticipantIdAsync_throws_when_constructor_HttpClient_has_no_BaseAddress()
    {
        using var http = new HttpClient();
        var tokenProvider = StaticTokenProvider("tok");
        Assert.Throws<ArgumentException>(() => new JsonLedgerAdminClient(http, tokenProvider));
    }

    [Fact]
    public async Task GetParticipantIdAsync_throws_JsonLedgerApiException_on_non_success_status()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.Unauthorized)
        {
            Content = new StringContent("""{"cause":"missing token"}""", Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"));

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(() => client.GetParticipantIdAsync());
        Assert.Equal(HttpStatusCode.Unauthorized, exception.StatusCode);
        Assert.Contains("missing token", exception.ResponseBody);
    }

    [Fact]
    public async Task GetParticipantIdAsync_throws_when_body_has_no_participantId()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent("""{"somethingElse":"x"}""", Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"));

        await Assert.ThrowsAsync<JsonLedgerApiException>(() => client.GetParticipantIdAsync());
    }

    public static TheoryData<string, string> MalformedSuccessBodies() => new()
    {
        { "participant-id", "" },
        { "participant-id", "<html>proxy</html>" },
        { "participant-id", "{\"participantId\":" },
        { "connected-synchronizers", "" },
        { "connected-synchronizers", "<html>proxy</html>" },
        { "connected-synchronizers", "{\"connectedSynchronizers\":" },
        { "package-vetting", "" },
        { "package-vetting", "<html>proxy</html>" },
        { "package-vetting", "{\"vettedPackages\":" },
    };

    [Theory]
    [MemberData(nameof(MalformedSuccessBodies))]
    public async Task JsonLedgerAdminClient_wraps_an_empty_or_malformed_2xx_body_in_JsonLedgerApiException(string call, string body)
    {
        var handler = new RecordingHandler((request, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent(
                request.RequestUri!.AbsolutePath.EndsWith("participant-id", StringComparison.Ordinal) && call == "package-vetting"
                    ? """{"participantId":"participant::1220aa"}"""
                    : body,
                Encoding.UTF8,
                "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"));
        Func<Task> act = call switch
        {
            "participant-id" => () => client.GetParticipantIdAsync(),
            "connected-synchronizers" => () => client.GetConnectedSynchronizersAsync(),
            _ => () => client.IsPackageVettedAsync("b1008daac9810baa8ca7ff313c13bd48d65f18040b2d33faabe5b5d39acaac9e", "global::122a"),
        };

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(act);

        Assert.Equal(HttpStatusCode.OK, exception.StatusCode);
        Assert.Equal(body, exception.ResponseBody);
        Assert.Contains("not valid JSON", exception.Message);
        Assert.IsType<JsonException>(exception.InnerException, exactMatch: false);
    }

    [Fact]
    public async Task GetConnectedSynchronizersAsync_calls_endpoint_with_party_and_parses()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent(
                """{"connectedSynchronizers":[{"synchronizerAlias":"global","synchronizerId":"global::122a","permission":"PARTICIPANT_PERMISSION_SUBMISSION"},{"synchronizerAlias":"app-synchronizer","synchronizerId":"app-synchronizer::122b"}]}""",
                Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"));

        var result = await client.GetConnectedSynchronizersAsync("alice::122a");

        Assert.Equal(2, result.Count);
        Assert.Equal("app-synchronizer", result[1].Alias);
        Assert.Equal("app-synchronizer::122b", result[1].Id);
        var recorded = Assert.Single(handler.Requests);
        Assert.Equal(HttpMethod.Get, recorded.Method);
        Assert.Equal(new Uri(JsonApiBase, "v2/state/connected-synchronizers?party=alice%3A%3A122a"), recorded.Uri);
        Assert.Equal("Bearer tok", recorded.Headers["Authorization"]);
    }

    [Fact]
    public async Task GetAppSynchronizerIdAsync_returns_id_for_app_synchronizer_alias()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent(
                """{"connectedSynchronizers":[{"synchronizerAlias":"global","synchronizerId":"global::122a"},{"synchronizerAlias":"app-synchronizer","synchronizerId":"app-synchronizer::122b"}]}""",
                Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"));

        var id = await client.GetAppSynchronizerIdAsync("alice::122a");

        Assert.Equal("app-synchronizer::122b", id);
    }

    [Fact]
    public async Task GetAppSynchronizerIdAsync_throws_when_only_global_connected()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent(
                """{"connectedSynchronizers":[{"synchronizerAlias":"global","synchronizerId":"global::122a"}]}""",
                Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"));

        var ex = await Assert.ThrowsAsync<JsonLedgerApiException>(() => client.GetAppSynchronizerIdAsync("alice::122a"));
        Assert.Equal(HttpStatusCode.NotFound, ex.StatusCode);
        Assert.Contains("app-synchronizer", ex.Message);
    }

    [Fact]
    public async Task GetConnectedSynchronizersAsync_throws_when_entry_has_empty_synchronizerId()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent(
                """{"connectedSynchronizers":[{"synchronizerAlias":"global","synchronizerId":""}]}""",
                Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"));

        var ex = await Assert.ThrowsAsync<JsonLedgerApiException>(() => client.GetConnectedSynchronizersAsync("alice::122a"));
        Assert.Contains("synchronizerId", ex.Message);
    }

    [Fact]
    public async Task GetConnectedSynchronizersAsync_throws_when_connectedSynchronizers_field_absent()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent("""{"connectedSynchronizers":null}""", Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"));

        var ex = await Assert.ThrowsAsync<JsonLedgerApiException>(() => client.GetConnectedSynchronizersAsync("alice::122a"));
        Assert.Contains("connectedSynchronizers", ex.Message);
    }

    [Fact]
    public async Task GetConnectedSynchronizersAsync_omits_party_query_segment_when_none_supplied()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent(
                """{"connectedSynchronizers":[{"synchronizerAlias":"global","synchronizerId":"global::122a"}]}""",
                Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"));

        var result = await client.GetConnectedSynchronizersAsync();

        Assert.Single(result);
        var recorded = Assert.Single(handler.Requests);
        Assert.Equal(new Uri(JsonApiBase, "v2/state/connected-synchronizers"), recorded.Uri);
    }

    [Fact]
    public async Task GetGlobalSynchronizerIdAsync_returns_id_for_global_alias()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent(
                """{"connectedSynchronizers":[{"synchronizerAlias":"global","synchronizerId":"global::122a"},{"synchronizerAlias":"app-synchronizer","synchronizerId":"app-synchronizer::122b"}]}""",
                Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"));

        var id = await client.GetGlobalSynchronizerIdAsync();

        Assert.Equal("global::122a", id);
        var recorded = Assert.Single(handler.Requests);
        Assert.Equal(new Uri(JsonApiBase, "v2/state/connected-synchronizers"), recorded.Uri);
    }

    [Fact]
    public async Task GetGlobalSynchronizerIdAsync_throws_when_no_synchronizer_connected()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent("""{"connectedSynchronizers":[]}""", Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"));

        var ex = await Assert.ThrowsAsync<JsonLedgerApiException>(() => client.GetGlobalSynchronizerIdAsync());
        Assert.Equal(HttpStatusCode.NotFound, ex.StatusCode);
        Assert.Contains("global", ex.Message);
    }

    private const string ParticipantId = "participant::1220aa";
    private const string PackageId = "b1008daac9810baa8ca7ff313c13bd48d65f18040b2d33faabe5b5d39acaac9e";
    private const string SynchronizerId = "global-domain::1220cd";

    private static HttpResponseMessage Json(string body) => new(HttpStatusCode.OK)
    {
        Content = new StringContent(body, Encoding.UTF8, "application/json"),
    };

    private static string VettedEntry(string participantId, string synchronizerId, string packageId) =>
        $$"""{"packages":[{"packageId":"{{packageId}}","validFromInclusive":null,"validUntilExclusive":null,"packageName":"canton-localnet-noop","packageVersion":"0.0.1"}],"participantId":"{{participantId}}","synchronizerId":"{{synchronizerId}}","topologySerial":3}""";

    private static RecordingHandler VettingHandler(Func<HttpRequestMessage, string, HttpResponseMessage> onVettingList) =>
        new((request, _) =>
        {
            if (request.Method == HttpMethod.Get && request.RequestUri!.AbsolutePath.EndsWith("v2/parties/participant-id", StringComparison.Ordinal))
            {
                return Task.FromResult(Json($$"""{"participantId":"{{ParticipantId}}"}"""));
            }
            var body = request.Content!.ReadAsStringAsync().GetAwaiter().GetResult();
            return Task.FromResult(onVettingList(request, body));
        });

    [Fact]
    public async Task IsPackageVettedAsync_posts_package_and_topology_filters_for_this_participant_and_synchronizer()
    {
        var handler = VettingHandler((_, _) => Json($$"""{"vettedPackages":[{{VettedEntry(ParticipantId, SynchronizerId, PackageId)}}],"nextPageToken":""}"""));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok-v"));

        var vetted = await client.IsPackageVettedAsync(PackageId, SynchronizerId);

        Assert.True(vetted);
        var recorded = Assert.Single(handler.Requests, r => r.Method == HttpMethod.Post);
        Assert.Equal(new Uri(JsonApiBase, "v2/package-vetting/list"), recorded.Uri);
        Assert.Equal("Bearer tok-v", recorded.Headers["Authorization"]);
        using var doc = JsonDocument.Parse(recorded.Body);
        var root = doc.RootElement;
        Assert.Equal(
            new[] { PackageId },
            root.GetProperty("packageMetadataFilter").GetProperty("packageIds").EnumerateArray().Select(e => e.GetString()).ToArray());
        Assert.Equal(
            new[] { ParticipantId },
            root.GetProperty("topologyStateFilter").GetProperty("participantIds").EnumerateArray().Select(e => e.GetString()).ToArray());
        Assert.Equal(
            new[] { SynchronizerId },
            root.GetProperty("topologyStateFilter").GetProperty("synchronizerIds").EnumerateArray().Select(e => e.GetString()).ToArray());
    }

    [Fact]
    public void Constructor_keeps_the_published_three_parameter_signature()
    {
        var published = typeof(JsonLedgerAdminClient).GetConstructor(
            new[] { typeof(HttpClient), typeof(OAuth2TokenProvider), typeof(ILogger<JsonLedgerAdminClient>) });

        Assert.NotNull(published);
    }

    [Theory]
    [InlineData(null, null, true)]
    [InlineData("2026-11-24T11:59:59Z", null, true)]
    [InlineData("2026-11-24T12:00:00Z", null, true)]
    [InlineData("2026-11-24T12:00:01Z", null, false)]
    [InlineData(null, "2026-11-24T12:00:01Z", true)]
    [InlineData(null, "2026-11-24T12:00:00Z", false)]
    [InlineData(null, "2026-11-24T11:59:59Z", false)]
    [InlineData("2026-11-24T11:00:00Z", "2026-11-24T13:00:00Z", true)]
    [InlineData("2026-11-24T13:00:00Z", "2026-11-24T14:00:00Z", false)]
    [InlineData("2026-11-24T10:00:00.123456Z", "2026-11-24T11:00:00.654321Z", false)]
    [InlineData("", "", true)]
    [InlineData("", "2026-11-24T12:00:00Z", false)]
    public async Task IsPackageVettedAsync_counts_an_entry_only_inside_its_validity_window(string? validFromInclusive, string? validUntilExclusive, bool expected)
    {
        var validFrom = validFromInclusive is null ? "null" : $"\"{validFromInclusive}\"";
        var validUntil = validUntilExclusive is null ? "null" : $"\"{validUntilExclusive}\"";
        var entry = $$"""{"packages":[{"packageId":"{{PackageId}}","validFromInclusive":{{validFrom}},"validUntilExclusive":{{validUntil}},"packageName":"canton-localnet-noop","packageVersion":"0.0.1"}],"participantId":"{{ParticipantId}}","synchronizerId":"{{SynchronizerId}}","topologySerial":3}""";
        var handler = VettingHandler((_, _) => Json($$"""{"vettedPackages":[{{entry}}],"nextPageToken":""}"""));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var clock = new FakeTimeProvider(new DateTimeOffset(2026, 11, 24, 12, 0, 0, TimeSpan.Zero));
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"), null, clock);

        Assert.Equal(expected, await client.IsPackageVettedAsync(PackageId, SynchronizerId));
    }

    [Theory]
    [InlineData("validFromInclusive", "yesterday")]
    [InlineData("validUntilExclusive", "2026-13-45T99:00:00Z")]
    public async Task IsPackageVettedAsync_fails_loudly_on_an_unparseable_bound_of_a_matching_entry(string field, string value)
    {
        var from = field == "validFromInclusive" ? $"\"{value}\"" : "null";
        var until = field == "validUntilExclusive" ? $"\"{value}\"" : "null";
        var entry = $$"""{"packages":[{"packageId":"{{PackageId}}","validFromInclusive":{{from}},"validUntilExclusive":{{until}}}],"participantId":"{{ParticipantId}}","synchronizerId":"{{SynchronizerId}}"}""";
        var handler = VettingHandler((_, _) => Json($$"""{"vettedPackages":[{{entry}}],"nextPageToken":""}"""));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"), null, new FakeTimeProvider(new DateTimeOffset(2026, 11, 24, 12, 0, 0, TimeSpan.Zero)));

        var ex = await Assert.ThrowsAsync<JsonLedgerApiException>(() => client.IsPackageVettedAsync(PackageId, SynchronizerId));

        Assert.Contains(field, ex.Message);
    }

    [Fact]
    public async Task IsPackageVettedAsync_ignores_an_unparseable_bound_on_an_entry_that_does_not_match()
    {
        var entry = $$"""{"packages":[{"packageId":"d0048119d779f3dc1c02dd2549c6281cdd72998bc3c9bc514fe07ff5d5365bbd","validFromInclusive":"yesterday","validUntilExclusive":null}],"participantId":"{{ParticipantId}}","synchronizerId":"{{SynchronizerId}}"}""";
        var handler = VettingHandler((_, _) => Json($$"""{"vettedPackages":[{{entry}}],"nextPageToken":""}"""));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"), null, new FakeTimeProvider(new DateTimeOffset(2026, 11, 24, 12, 0, 0, TimeSpan.Zero)));

        Assert.False(await client.IsPackageVettedAsync(PackageId, SynchronizerId));
    }

    [Fact]
    public async Task IsPackageVettedAsync_keeps_paging_after_an_expired_entry_and_finds_a_live_one()
    {
        var expired = """{"packages":[{"packageId":"PKG","validFromInclusive":null,"validUntilExclusive":"2026-11-24T11:00:00Z"}],"participantId":"PART","synchronizerId":"SYNC"}"""
            .Replace("PKG", PackageId).Replace("PART", ParticipantId).Replace("SYNC", SynchronizerId);
        var live = """{"packages":[{"packageId":"PKG","validFromInclusive":"2026-11-24T11:30:00Z","validUntilExclusive":null}],"participantId":"PART","synchronizerId":"SYNC"}"""
            .Replace("PKG", PackageId).Replace("PART", ParticipantId).Replace("SYNC", SynchronizerId);
        var calls = 0;
        var handler = VettingHandler((_, _) => ++calls == 1
            ? Json($$"""{"vettedPackages":[{{expired}}],"nextPageToken":"page-2"}""")
            : Json($$"""{"vettedPackages":[{{live}}],"nextPageToken":""}"""));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var clock = new FakeTimeProvider(new DateTimeOffset(2026, 11, 24, 12, 0, 0, TimeSpan.Zero));
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"), null, clock);

        Assert.True(await client.IsPackageVettedAsync(PackageId, SynchronizerId));
        Assert.Equal(2, calls);
    }

    [Fact]
    public async Task IsPackageVettedAsync_returns_false_when_no_vetting_entry_comes_back()
    {
        var handler = VettingHandler((_, _) => Json("""{"vettedPackages":[],"nextPageToken":""}"""));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"));

        Assert.False(await client.IsPackageVettedAsync(PackageId, SynchronizerId));
    }

    [Fact]
    public async Task IsPackageVettedAsync_ignores_an_entry_that_belongs_to_another_participant()
    {
        var handler = VettingHandler((_, _) => Json($$"""{"vettedPackages":[{{VettedEntry("participant::1220bb", SynchronizerId, PackageId)}}],"nextPageToken":""}"""));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"));

        Assert.False(await client.IsPackageVettedAsync(PackageId, SynchronizerId));
    }

    [Fact]
    public async Task IsPackageVettedAsync_ignores_an_entry_for_a_different_package_id()
    {
        var handler = VettingHandler((_, _) => Json($$"""{"vettedPackages":[{{VettedEntry(ParticipantId, SynchronizerId, "d0048119d779f3dc1c02dd2549c6281cdd72998bc3c9bc514fe07ff5d5365bbd")}}],"nextPageToken":""}"""));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"));

        Assert.False(await client.IsPackageVettedAsync(PackageId, SynchronizerId));
    }

    [Fact]
    public async Task IsPackageVettedAsync_omits_the_synchronizer_filter_when_none_is_given()
    {
        var handler = VettingHandler((_, _) => Json($$"""{"vettedPackages":[{{VettedEntry(ParticipantId, SynchronizerId, PackageId)}}],"nextPageToken":""}"""));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"));

        var vetted = await client.IsPackageVettedAsync(PackageId, synchronizerId: null);

        Assert.True(vetted);
        var recorded = Assert.Single(handler.Requests, r => r.Method == HttpMethod.Post);
        using var doc = JsonDocument.Parse(recorded.Body);
        Assert.False(doc.RootElement.GetProperty("topologyStateFilter").TryGetProperty("synchronizerIds", out _));
    }

    [Fact]
    public async Task IsPackageVettedAsync_follows_nextPageToken_until_a_match_or_the_last_page()
    {
        var pageTokensSent = new List<string?>();
        var handler = VettingHandler((_, body) =>
        {
            using var doc = JsonDocument.Parse(body);
            var token = doc.RootElement.TryGetProperty("pageToken", out var t) ? t.GetString() : null;
            pageTokensSent.Add(token);
            return string.IsNullOrEmpty(token)
                ? Json("""{"vettedPackages":[],"nextPageToken":"page-2"}""")
                : Json($$"""{"vettedPackages":[{{VettedEntry(ParticipantId, SynchronizerId, PackageId)}}],"nextPageToken":""}""");
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"));

        var vetted = await client.IsPackageVettedAsync(PackageId, SynchronizerId);

        Assert.True(vetted);
        Assert.Equal(new string?[] { null, "page-2" }, pageTokensSent);
    }

    [Fact]
    public async Task IsPackageVettedAsync_throws_on_unexpected_status()
    {
        var handler = VettingHandler((_, _) => new HttpResponseMessage(HttpStatusCode.InternalServerError)
        {
            Content = new StringContent("boom", Encoding.UTF8, "text/plain"),
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"));

        var ex = await Assert.ThrowsAsync<JsonLedgerApiException>(() => client.IsPackageVettedAsync(PackageId, SynchronizerId));
        Assert.Equal(HttpStatusCode.InternalServerError, ex.StatusCode);
        Assert.Contains("boom", ex.ResponseBody);
    }

    [Fact]
    public async Task IsPackageVettedAsync_rejects_empty_packageId()
    {
        using var http = new HttpClient(new RecordingHandler((_, _) =>
            Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)))) { BaseAddress = JsonApiBase };
        var client = new JsonLedgerAdminClient(http, StaticTokenProvider("tok"));

        await Assert.ThrowsAsync<ArgumentException>(() => client.IsPackageVettedAsync("", SynchronizerId));
    }
}
