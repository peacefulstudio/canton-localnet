// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;
using System.Text;
using System.Text.Json;
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
}
