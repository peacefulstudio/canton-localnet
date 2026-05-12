// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;
using System.Text;
using System.Text.Json;
using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

public class PartyAllocatorTests
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

    [Fact]
    public async Task AllocateAsync_posts_party_hint_in_consumer_prefix_dash_instance_suffix_format()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(AllocateResponse("cdg-abcdef::namespace")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef");

        var party = await allocator.AllocateAsync("cdg");

        Assert.Equal("cdg-abcdef::namespace", party.PartyId);
        Assert.Equal("cdg-abcdef", party.PartyIdHint);
        var recorded = Assert.Single(handler.Requests);
        Assert.Equal(HttpMethod.Post, recorded.Method);
        Assert.Equal(new Uri(JsonApiBase, "v2/parties"), recorded.Uri);
        using var bodyDoc = JsonDocument.Parse(recorded.Body);
        Assert.Equal("cdg-abcdef", bodyDoc.RootElement.GetProperty("partyIdHint").GetString());
        Assert.Equal("cdg-abcdef", bodyDoc.RootElement.GetProperty("displayName").GetString());
    }

    [Fact]
    public async Task AllocateAsync_uses_explicit_display_name_when_provided()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(AllocateResponse("lapi-deadbe::n")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "deadbe");

        await allocator.AllocateAsync("lapi", displayName: "Ledger API Party");

        var recorded = Assert.Single(handler.Requests);
        using var bodyDoc = JsonDocument.Parse(recorded.Body);
        Assert.Equal("lapi-deadbe", bodyDoc.RootElement.GetProperty("partyIdHint").GetString());
        Assert.Equal("Ledger API Party", bodyDoc.RootElement.GetProperty("displayName").GetString());
    }

    [Fact]
    public void ComposeHint_format_is_prefix_dash_suffix()
    {
        using var http = new HttpClient(new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK))))
        {
            BaseAddress = JsonApiBase,
        };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "fixed");
        Assert.Equal("cdg-fixed", allocator.ComposeHint("cdg"));
        Assert.Equal("lapi-fixed", allocator.ComposeHint("lapi"));
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
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.Conflict)
        {
            Content = new StringContent("""{"cause":"PARTY_ALREADY_EXISTS"}""", Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abc");

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(() => allocator.AllocateAsync("cdg"));
        Assert.Equal(HttpStatusCode.Conflict, exception.StatusCode);
        Assert.Contains("PARTY_ALREADY_EXISTS", exception.ResponseBody);
    }

    [Fact]
    public async Task AllocateAsync_throws_when_response_has_no_party_field()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent("""{"somethingElse":"x"}""", Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abc");

        await Assert.ThrowsAsync<JsonLedgerApiException>(() => allocator.AllocateAsync("cdg"));
    }

    [Fact]
    public async Task AllocateAsync_sends_bearer_token()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(AllocateResponse("cdg-x::n")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok-bearer-x"), instanceSuffix: "x");

        await allocator.AllocateAsync("cdg");

        var recorded = Assert.Single(handler.Requests);
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
