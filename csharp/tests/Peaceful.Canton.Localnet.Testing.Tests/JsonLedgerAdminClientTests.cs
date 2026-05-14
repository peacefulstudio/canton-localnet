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
}
