// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;
using System.Text;
using System.Text.Json;
using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

public class UserBuilderTests
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

    private static HttpResponseMessage CreateUserResponse(string userId) =>
        new(HttpStatusCode.OK)
        {
            Content = new StringContent(
                JsonSerializer.Serialize(new { user = new { id = userId } }),
                Encoding.UTF8,
                "application/json"),
        };

    [Fact]
    public async Task CreateAsync_posts_v2_users_with_id_and_primary_party()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(CreateUserResponse("alice")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok-1"));

        var id = await builder.CreateAsync("alice", primaryParty: "alice::1220abc");

        Assert.Equal("alice", id);
        var recorded = Assert.Single(handler.Requests);
        Assert.Equal(HttpMethod.Post, recorded.Method);
        Assert.Equal(new Uri(JsonApiBase, "v2/users"), recorded.Uri);
        Assert.Equal("Bearer tok-1", recorded.Headers["Authorization"]);
        using var bodyDoc = JsonDocument.Parse(recorded.Body);
        var user = bodyDoc.RootElement.GetProperty("user");
        Assert.Equal("alice", user.GetProperty("id").GetString());
        Assert.Equal("alice::1220abc", user.GetProperty("primaryParty").GetString());
        Assert.False(user.GetProperty("isDeactivated").GetBoolean());
        Assert.Equal(0, bodyDoc.RootElement.GetProperty("rights").GetArrayLength());
    }

    [Fact]
    public async Task CreateAsync_skips_rights_request_when_actAs_and_readAs_are_empty()
    {
        var calls = 0;
        var handler = new RecordingHandler((_, _) =>
        {
            calls++;
            return Task.FromResult(CreateUserResponse("bob"));
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        await builder.CreateAsync("bob");

        Assert.Equal(1, calls);
    }

    [Fact]
    public async Task CreateAsync_grants_actAs_and_readAs_rights_via_followup_request()
    {
        var requests = new List<(Uri Uri, string Body)>();
        var handler = new RecordingHandler(async (req, ct) =>
        {
            var body = req.Content is null ? string.Empty : await req.Content.ReadAsStringAsync(ct);
            requests.Add((req.RequestUri!, body));
            if (req.RequestUri!.AbsolutePath.EndsWith("/v2/users", StringComparison.Ordinal))
            {
                return CreateUserResponse("carol");
            }
            return new HttpResponseMessage(HttpStatusCode.OK)
            {
                Content = new StringContent("{}", Encoding.UTF8, "application/json"),
            };
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        await builder.CreateAsync(
            "carol",
            primaryParty: "carol::p",
            actAs: new[] { "carol::p" },
            readAs: new[] { "observer::p" });

        Assert.Equal(2, requests.Count);
        Assert.Equal(new Uri(JsonApiBase, "v2/users"), requests[0].Uri);
        Assert.Equal(new Uri(JsonApiBase, "v2/users/carol/rights"), requests[1].Uri);

        using var rightsBody = JsonDocument.Parse(requests[1].Body);
        Assert.Equal("carol", rightsBody.RootElement.GetProperty("userId").GetString());
        var rights = rightsBody.RootElement.GetProperty("rights");
        Assert.Equal(2, rights.GetArrayLength());

        var canActAs = rights[0].GetProperty("kind").GetProperty("CanActAs");
        Assert.Equal("carol::p", canActAs.GetProperty("value").GetProperty("party").GetString());

        var canReadAs = rights[1].GetProperty("kind").GetProperty("CanReadAs");
        Assert.Equal("observer::p", canReadAs.GetProperty("value").GetProperty("party").GetString());
    }

    [Fact]
    public async Task GrantRightsAsync_posts_can_act_as_rights_without_creating_user()
    {
        var requests = new List<(Uri Uri, string Body)>();
        var handler = new RecordingHandler(async (req, ct) =>
        {
            var body = req.Content is null ? string.Empty : await req.Content.ReadAsStringAsync(ct);
            requests.Add((req.RequestUri!, body));
            return new HttpResponseMessage(HttpStatusCode.OK)
            {
                Content = new StringContent("{}", Encoding.UTF8, "application/json"),
            };
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        await builder.GrantRightsAsync(
            "c87743ab-80e0-4b83-935a-4c0582226691",
            actAs: new[] { "alice::p" });

        var recorded = Assert.Single(requests);
        Assert.Equal(
            new Uri(JsonApiBase, "v2/users/c87743ab-80e0-4b83-935a-4c0582226691/rights"),
            recorded.Uri);

        using var rightsBody = JsonDocument.Parse(recorded.Body);
        Assert.Equal(
            "c87743ab-80e0-4b83-935a-4c0582226691",
            rightsBody.RootElement.GetProperty("userId").GetString());
        var rights = rightsBody.RootElement.GetProperty("rights");
        Assert.Equal(1, rights.GetArrayLength());
        var canActAs = rights[0].GetProperty("kind").GetProperty("CanActAs");
        Assert.Equal("alice::p", canActAs.GetProperty("value").GetProperty("party").GetString());
    }

    [Fact]
    public async Task GrantRightsAsync_grants_can_read_as_rights_when_readAs_supplied()
    {
        var requests = new List<(Uri Uri, string Body)>();
        var handler = new RecordingHandler(async (req, ct) =>
        {
            var body = req.Content is null ? string.Empty : await req.Content.ReadAsStringAsync(ct);
            requests.Add((req.RequestUri!, body));
            return new HttpResponseMessage(HttpStatusCode.OK)
            {
                Content = new StringContent("{}", Encoding.UTF8, "application/json"),
            };
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        await builder.GrantRightsAsync(
            "some-user",
            actAs: new[] { "alice::p" },
            readAs: new[] { "observer::p" });

        var recorded = Assert.Single(requests);
        using var rightsBody = JsonDocument.Parse(recorded.Body);
        var rights = rightsBody.RootElement.GetProperty("rights");
        Assert.Equal(2, rights.GetArrayLength());
        Assert.Equal(
            "alice::p",
            rights[0].GetProperty("kind").GetProperty("CanActAs").GetProperty("value").GetProperty("party").GetString());
        Assert.Equal(
            "observer::p",
            rights[1].GetProperty("kind").GetProperty("CanReadAs").GetProperty("value").GetProperty("party").GetString());
    }

    [Fact]
    public async Task GrantRightsAsync_issues_no_request_when_actAs_and_readAs_empty()
    {
        var calls = 0;
        var handler = new RecordingHandler((_, _) =>
        {
            calls++;
            return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
            {
                Content = new StringContent("{}", Encoding.UTF8, "application/json"),
            });
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        await builder.GrantRightsAsync("some-user", actAs: Array.Empty<string>());

        Assert.Equal(0, calls);
    }

    [Fact]
    public async Task GrantRightsAsync_throws_when_rights_request_fails()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.BadRequest)
        {
            Content = new StringContent("""{"cause":"INVALID_RIGHT"}""", Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(
            () => builder.GrantRightsAsync("some-user", new[] { "alice::p" }));
        Assert.Equal(HttpStatusCode.BadRequest, exception.StatusCode);
    }

    [Fact]
    public async Task GrantRightsAsync_rejects_empty_user_id()
    {
        using var http = new HttpClient(new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK))))
        {
            BaseAddress = JsonApiBase,
        };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));
        await Assert.ThrowsAsync<ArgumentException>(() => builder.GrantRightsAsync("", new[] { "alice::p" }));
    }

    [Fact]
    public async Task CreateAsync_throws_when_create_request_fails()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.Conflict)
        {
            Content = new StringContent("""{"cause":"USER_ALREADY_EXISTS"}""", Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(() => builder.CreateAsync("dave"));
        Assert.Equal(HttpStatusCode.Conflict, exception.StatusCode);
        Assert.Contains("USER_ALREADY_EXISTS", exception.ResponseBody);
    }

    [Fact]
    public async Task CreateAsync_throws_when_rights_request_fails()
    {
        var handler = new RecordingHandler((req, _) =>
        {
            if (req.RequestUri!.AbsolutePath.EndsWith("/v2/users", StringComparison.Ordinal))
            {
                return Task.FromResult(CreateUserResponse("eve"));
            }
            return Task.FromResult(new HttpResponseMessage(HttpStatusCode.BadRequest)
            {
                Content = new StringContent("""{"cause":"INVALID_RIGHT"}""", Encoding.UTF8, "application/json"),
            });
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(
            () => builder.CreateAsync("eve", actAs: new[] { "eve::p" }));
        Assert.Equal(HttpStatusCode.BadRequest, exception.StatusCode);
    }

    [Fact]
    public async Task CreateAsync_throws_when_response_has_no_user_id()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent("""{"user":{}}""", Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        await Assert.ThrowsAsync<JsonLedgerApiException>(() => builder.CreateAsync("frank"));
    }

    [Fact]
    public async Task CreateAsync_rejects_empty_user_id()
    {
        using var http = new HttpClient(new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK))))
        {
            BaseAddress = JsonApiBase,
        };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));
        await Assert.ThrowsAsync<ArgumentException>(() => builder.CreateAsync(""));
    }

    [Fact]
    public void Constructor_throws_when_HttpClient_has_no_BaseAddress()
    {
        using var http = new HttpClient();
        Assert.Throws<ArgumentException>(() => new UserBuilder(http, StaticTokenProvider("tok")));
    }
}
