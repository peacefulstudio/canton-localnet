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

    private const string ParticipantAdminRight = """{"kind":{"ParticipantAdmin":{"value":{}}}}""";

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

    private static HttpResponseMessage JsonOk(string json) =>
        new(HttpStatusCode.OK)
        {
            Content = new StringContent(json, Encoding.UTF8, "application/json"),
        };

    private static string RawRightsJson(string field, params string[] rights) =>
        $"{{\"{field}\":[{string.Join(",", rights)}]}}";

    private static string RightsJson(string field, params (string Kind, string Party)[] rights) =>
        RawRightsJson(
            field,
            rights
                .Select(right => JsonSerializer.Serialize(new Dictionary<string, object>
                {
                    ["kind"] = new Dictionary<string, object>
                    {
                        [right.Kind] = new { value = new { party = right.Party } },
                    },
                }))
                .ToArray());

    private static RecordingHandler LeaseHandler(
        List<(HttpMethod Method, string Body)> requests,
        string grantResponse,
        string revokeResponse)
        => new(async (req, ct) =>
        {
            requests.Add((req.Method, req.Content is null ? string.Empty : await req.Content.ReadAsStringAsync(ct)));
            return JsonOk(req.Method == HttpMethod.Patch ? revokeResponse : grantResponse);
        });

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
            return JsonOk("{}");
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
            return JsonOk("{}");
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
            return JsonOk("{}");
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
            return Task.FromResult(JsonOk("{}"));
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        await builder.GrantRightsAsync("some-user", actAs: Array.Empty<string>());

        Assert.Equal(0, calls);
    }

    [Fact]
    public async Task GrantRightsAsync_collapses_a_party_repeated_in_actAs()
    {
        var requests = new List<(Uri Uri, string Body)>();
        var handler = new RecordingHandler(async (req, ct) =>
        {
            requests.Add((req.RequestUri!, await req.Content!.ReadAsStringAsync(ct)));
            return JsonOk("{}");
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        await builder.GrantRightsAsync("some-user", actAs: new[] { "alice::p", "alice::p" });

        using var body = JsonDocument.Parse(Assert.Single(requests).Body);
        Assert.Equal(1, body.RootElement.GetProperty("rights").GetArrayLength());
    }

    [Fact]
    public async Task GrantRightsAsync_keeps_act_as_and_read_as_for_the_same_party_as_two_rights()
    {
        var requests = new List<(Uri Uri, string Body)>();
        var handler = new RecordingHandler(async (req, ct) =>
        {
            requests.Add((req.RequestUri!, await req.Content!.ReadAsStringAsync(ct)));
            return JsonOk("{}");
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        await builder.GrantRightsAsync("some-user", actAs: new[] { "alice::p" }, readAs: new[] { "alice::p" });

        using var body = JsonDocument.Parse(Assert.Single(requests).Body);
        Assert.Equal(2, body.RootElement.GetProperty("rights").GetArrayLength());
    }

    [Fact]
    public async Task GrantRightsAsync_succeeds_when_the_participant_returns_a_non_json_body()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent("ok", Encoding.UTF8, "text/plain"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        await builder.GrantRightsAsync("some-user", actAs: new[] { "alice::p" });
    }

    [Fact]
    public async Task CreateAsync_succeeds_when_the_rights_response_is_not_json()
    {
        var handler = new RecordingHandler((req, _) =>
            Task.FromResult(req.RequestUri!.AbsolutePath.EndsWith("/v2/users", StringComparison.Ordinal)
                ? CreateUserResponse("grace")
                : new HttpResponseMessage(HttpStatusCode.OK)
                {
                    Content = new StringContent("ok", Encoding.UTF8, "text/plain"),
                }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var id = await builder.CreateAsync("grace", actAs: new[] { "grace::p" });

        Assert.Equal("grace", id);
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
        var handler = new RecordingHandler((_, _) => Task.FromResult(JsonOk("""{"user":{}}""")));
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

    [Fact]
    public async Task RevokeRightsAsync_patches_the_requested_rights()
    {
        var requests = new List<(HttpMethod Method, Uri Uri, string Body)>();
        var handler = new RecordingHandler(async (req, ct) =>
        {
            requests.Add((req.Method, req.RequestUri!, await req.Content!.ReadAsStringAsync(ct)));
            return JsonOk(RightsJson(
                "newlyRevokedRights",
                ("CanActAs", "alice::p"),
                ("CanReadAs", "observer::p")));
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok-9"));

        await builder.RevokeRightsAsync(
            "some-user",
            actAs: new[] { "alice::p" },
            readAs: new[] { "observer::p" });

        var recorded = Assert.Single(requests);
        Assert.Equal(HttpMethod.Patch, recorded.Method);
        Assert.Equal(new Uri(JsonApiBase, "v2/users/some-user/rights"), recorded.Uri);
        Assert.Equal("Bearer tok-9", Assert.Single(handler.Requests).Headers["Authorization"]);

        using var body = JsonDocument.Parse(recorded.Body);
        Assert.Equal("some-user", body.RootElement.GetProperty("userId").GetString());
        var rights = body.RootElement.GetProperty("rights");
        Assert.Equal(2, rights.GetArrayLength());
        Assert.Equal(
            "alice::p",
            rights[0].GetProperty("kind").GetProperty("CanActAs").GetProperty("value").GetProperty("party").GetString());
        Assert.Equal(
            "observer::p",
            rights[1].GetProperty("kind").GetProperty("CanReadAs").GetProperty("value").GetProperty("party").GetString());
    }

    [Fact]
    public async Task RevokeRightsAsync_throws_when_fewer_rights_come_back_than_were_requested()
    {
        var handler = new RecordingHandler((_, _) =>
            Task.FromResult(JsonOk(RightsJson("newlyRevokedRights", ("CanActAs", "alice::p")))));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(
            () => builder.RevokeRightsAsync("some-user", actAs: new[] { "alice::p", "bob::p" }));

        Assert.Contains("reported 1 of 2", exception.Message, StringComparison.Ordinal);
        Assert.Contains("were not reported as revoked", exception.Message, StringComparison.Ordinal);
    }

    [Fact]
    public async Task RevokeRightsAsync_throws_when_the_response_reports_no_revoked_rights()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(JsonOk("{}")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(
            () => builder.RevokeRightsAsync("some-user", actAs: new[] { "alice::p" }));

        Assert.Contains("reported 0 of 1", exception.Message, StringComparison.Ordinal);
    }

    [Fact]
    public async Task RevokeRightsAsync_accepts_a_party_repeated_in_actAs_as_one_right()
    {
        var requests = new List<(HttpMethod Method, string Body)>();
        var handler = new RecordingHandler(async (req, ct) =>
        {
            requests.Add((req.Method, await req.Content!.ReadAsStringAsync(ct)));
            return JsonOk(RightsJson("newlyRevokedRights", ("CanActAs", "alice::p")));
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        await builder.RevokeRightsAsync("some-user", actAs: new[] { "alice::p", "alice::p" });

        using var body = JsonDocument.Parse(Assert.Single(requests).Body);
        Assert.Equal(1, body.RootElement.GetProperty("rights").GetArrayLength());
    }

    [Fact]
    public async Task RevokeRightsAsync_throws_when_the_patch_request_fails()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.NotFound)
        {
            Content = new StringContent("""{"cause":"USER_NOT_FOUND"}""", Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(
            () => builder.RevokeRightsAsync("some-user", actAs: new[] { "alice::p" }));

        Assert.Equal(HttpStatusCode.NotFound, exception.StatusCode);
        Assert.Contains("USER_NOT_FOUND", exception.ResponseBody, StringComparison.Ordinal);
    }

    [Fact]
    public async Task RevokeRightsAsync_issues_no_request_when_actAs_and_readAs_empty()
    {
        var calls = 0;
        var handler = new RecordingHandler((_, _) =>
        {
            calls++;
            return Task.FromResult(JsonOk("{}"));
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        await builder.RevokeRightsAsync("some-user");

        Assert.Equal(0, calls);
    }

    [Fact]
    public async Task RevokeRightsAsync_rejects_empty_user_id()
    {
        using var http = new HttpClient(new RecordingHandler((_, _) => Task.FromResult(JsonOk("{}"))))
        {
            BaseAddress = JsonApiBase,
        };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        await Assert.ThrowsAsync<ArgumentException>(() => builder.RevokeRightsAsync("", new[] { "alice::p" }));
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_revokes_only_the_newly_granted_rights_on_dispose()
    {
        var requests = new List<(HttpMethod Method, string Body)>();
        var handler = LeaseHandler(
            requests,
            grantResponse: RightsJson("newlyGrantedRights", ("CanActAs", "alice::p")),
            revokeResponse: RightsJson("newlyRevokedRights", ("CanActAs", "alice::p")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var lease = await builder.GrantRightsLeaseAsync(
            "some-user",
            actAs: new[] { "alice::p", "bob::p" });
        await lease.DisposeAsync();

        Assert.Equal(2, requests.Count);
        Assert.Equal(HttpMethod.Post, requests[0].Method);
        using var granted = JsonDocument.Parse(requests[0].Body);
        Assert.Equal(2, granted.RootElement.GetProperty("rights").GetArrayLength());

        Assert.Equal(HttpMethod.Patch, requests[1].Method);
        using var revoked = JsonDocument.Parse(requests[1].Body);
        var revokedRights = revoked.RootElement.GetProperty("rights");
        Assert.Equal(1, revokedRights.GetArrayLength());
        Assert.Equal(
            "alice::p",
            revokedRights[0].GetProperty("kind").GetProperty("CanActAs").GetProperty("value").GetProperty("party").GetString());
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_revokes_a_right_shape_it_does_not_model_verbatim()
    {
        var requests = new List<(HttpMethod Method, string Body)>();
        var handler = LeaseHandler(
            requests,
            grantResponse: RawRightsJson("newlyGrantedRights", ParticipantAdminRight),
            revokeResponse: RawRightsJson("newlyRevokedRights", ParticipantAdminRight));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var lease = await builder.GrantRightsLeaseAsync("some-user", actAs: new[] { "alice::p" });
        await lease.DisposeAsync();

        using var revoked = JsonDocument.Parse(requests[1].Body);
        var right = Assert.Single(revoked.RootElement.GetProperty("rights").EnumerateArray().ToArray());
        Assert.True(right.GetProperty("kind").TryGetProperty("ParticipantAdmin", out _));
        Assert.False(right.GetProperty("kind").TryGetProperty("CanActAs", out _));
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_exposes_an_unmodelled_right_as_raw_json_only()
    {
        var requests = new List<(HttpMethod Method, string Body)>();
        var handler = LeaseHandler(
            requests,
            grantResponse: RawRightsJson("newlyGrantedRights", ParticipantAdminRight),
            revokeResponse: RawRightsJson("newlyRevokedRights", ParticipantAdminRight));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        await using var lease = await builder.GrantRightsLeaseAsync("some-user", actAs: new[] { "alice::p" });

        Assert.Contains("ParticipantAdmin", Assert.Single(lease.Rights), StringComparison.Ordinal);
        Assert.Empty(lease.ActAs);
        Assert.Empty(lease.ReadAs);
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_exposes_the_owned_rights_by_party()
    {
        var requests = new List<(HttpMethod Method, string Body)>();
        var handler = LeaseHandler(
            requests,
            grantResponse: RightsJson(
                "newlyGrantedRights",
                ("CanActAs", "alice::p"),
                ("CanReadAs", "observer::p")),
            revokeResponse: RightsJson(
                "newlyRevokedRights",
                ("CanActAs", "alice::p"),
                ("CanReadAs", "observer::p")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        await using var lease = await builder.GrantRightsLeaseAsync(
            "some-user",
            actAs: new[] { "alice::p" },
            readAs: new[] { "observer::p" });

        Assert.Equal("some-user", lease.UserId);
        Assert.Equal(2, lease.Rights.Count);
        Assert.Equal(new[] { "alice::p" }, lease.ActAs);
        Assert.Equal(new[] { "observer::p" }, lease.ReadAs);
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_owns_nothing_when_the_grant_newly_granted_nothing()
    {
        var requests = new List<(HttpMethod Method, string Body)>();
        var handler = LeaseHandler(
            requests,
            grantResponse: RightsJson("newlyGrantedRights"),
            revokeResponse: RightsJson("newlyRevokedRights"));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var lease = await builder.GrantRightsLeaseAsync("some-user", actAs: new[] { "alice::p" });
        await lease.DisposeAsync();

        Assert.Empty(lease.Rights);
        Assert.Equal(HttpMethod.Post, Assert.Single(requests).Method);
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_dispose_revokes_once_when_disposed_twice()
    {
        var requests = new List<(HttpMethod Method, string Body)>();
        var handler = LeaseHandler(
            requests,
            grantResponse: RightsJson("newlyGrantedRights", ("CanActAs", "alice::p")),
            revokeResponse: RightsJson("newlyRevokedRights", ("CanActAs", "alice::p")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var lease = await builder.GrantRightsLeaseAsync("some-user", actAs: new[] { "alice::p" });
        await lease.DisposeAsync();
        await lease.DisposeAsync();

        Assert.Equal(1, requests.Count(r => r.Method == HttpMethod.Patch));
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_revokes_even_when_the_granting_token_was_cancelled()
    {
        var requests = new List<(HttpMethod Method, string Body)>();
        var handler = LeaseHandler(
            requests,
            grantResponse: RightsJson("newlyGrantedRights", ("CanActAs", "alice::p")),
            revokeResponse: RightsJson("newlyRevokedRights", ("CanActAs", "alice::p")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));
        using var cts = new CancellationTokenSource();

        var lease = await builder.GrantRightsLeaseAsync(
            "some-user",
            actAs: new[] { "alice::p" },
            cancellationToken: cts.Token);
        await cts.CancelAsync();
        await lease.DisposeAsync();

        Assert.Equal(1, requests.Count(r => r.Method == HttpMethod.Patch));
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_issues_the_grant_even_when_the_caller_token_is_already_cancelled()
    {
        var requests = new List<HttpMethod>();
        var handler = new RecordingHandler((req, ct) =>
        {
            ct.ThrowIfCancellationRequested();
            requests.Add(req.Method);
            return Task.FromResult(JsonOk(RightsJson("newlyGrantedRights", ("CanActAs", "alice::p"))));
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var tokenProvider = StaticTokenProvider("tok");
        await tokenProvider.GetAccessTokenAsync();
        var builder = new UserBuilder(http, tokenProvider);
        using var cts = new CancellationTokenSource();
        await cts.CancelAsync();

        var lease = await builder.GrantRightsLeaseAsync(
            "some-user",
            actAs: new[] { "alice::p" },
            cancellationToken: cts.Token);

        Assert.Equal(HttpMethod.Post, Assert.Single(requests));
        Assert.Single(lease.Rights);
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_throws_when_the_grant_response_omits_newly_granted_rights()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.Accepted)
        {
            Content = new StringContent("""{"unexpected":true}""", Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var exception = await Assert.ThrowsAsync<UserRightsGrantedWithoutLeaseException>(
            () => builder.GrantRightsLeaseAsync(
                "some-user",
                actAs: new[] { "alice::p" },
                readAs: new[] { "observer::p" }));

        Assert.Equal("some-user", exception.UserId);
        Assert.Equal(new[] { "alice::p" }, exception.ActAs);
        Assert.Equal(new[] { "observer::p" }, exception.ReadAs);
        Assert.Equal(HttpStatusCode.Accepted, exception.StatusCode);
        Assert.Equal("""{"unexpected":true}""", exception.ResponseBody);
        Assert.Contains("RevokeRightsAsync", exception.Message, StringComparison.Ordinal);
        Assert.IsAssignableFrom<JsonLedgerApiException>(exception);
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_throws_when_the_grant_response_is_not_json()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent("ok", Encoding.UTF8, "text/plain"),
        }));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var exception = await Assert.ThrowsAsync<UserRightsGrantedWithoutLeaseException>(
            () => builder.GrantRightsLeaseAsync("some-user", actAs: new[] { "alice::p" }));

        Assert.Equal("ok", exception.ResponseBody);
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_dispose_throws_when_the_revoke_comes_back_partial()
    {
        var requests = new List<(HttpMethod Method, string Body)>();
        var handler = LeaseHandler(
            requests,
            grantResponse: RightsJson("newlyGrantedRights", ("CanActAs", "alice::p"), ("CanActAs", "bob::p")),
            revokeResponse: RightsJson("newlyRevokedRights", ("CanActAs", "alice::p")));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var lease = await builder.GrantRightsLeaseAsync(
            "some-user",
            actAs: new[] { "alice::p", "bob::p" });

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(
            async () => await lease.DisposeAsync());
        Assert.Contains("reported 1 of 2", exception.Message, StringComparison.Ordinal);
        Assert.Contains("some-user", exception.Message, StringComparison.Ordinal);
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_dispose_narrows_to_the_outstanding_rights_after_a_partial()
    {
        var requests = new List<(HttpMethod Method, string Body)>();
        var patches = 0;
        var handler = new RecordingHandler(async (req, ct) =>
        {
            requests.Add((req.Method, req.Content is null ? string.Empty : await req.Content.ReadAsStringAsync(ct)));
            if (req.Method != HttpMethod.Patch)
            {
                return JsonOk(RightsJson(
                    "newlyGrantedRights",
                    ("CanActAs", "alice::p"),
                    ("CanActAs", "bob::p")));
            }
            patches++;
            return JsonOk(patches == 1
                ? RightsJson("newlyRevokedRights", ("CanActAs", "alice::p"))
                : RightsJson("newlyRevokedRights", ("CanActAs", "bob::p")));
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var lease = await builder.GrantRightsLeaseAsync(
            "some-user",
            actAs: new[] { "alice::p", "bob::p" });

        await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await lease.DisposeAsync());
        Assert.Equal(new[] { "bob::p" }, lease.ActAs);

        await lease.DisposeAsync();

        var retried = requests.Where(r => r.Method == HttpMethod.Patch).ToArray();
        Assert.Equal(2, retried.Length);
        using var body = JsonDocument.Parse(retried[1].Body);
        var rights = body.RootElement.GetProperty("rights");
        Assert.Equal(1, rights.GetArrayLength());
        Assert.Equal(
            "bob::p",
            rights[0].GetProperty("kind").GetProperty("CanActAs").GetProperty("value").GetProperty("party").GetString());
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_dispose_can_be_retried_after_a_failed_revoke()
    {
        var patches = 0;
        var handler = new RecordingHandler((req, _) =>
        {
            if (req.Method != HttpMethod.Patch)
            {
                return Task.FromResult(JsonOk(RightsJson("newlyGrantedRights", ("CanActAs", "alice::p"))));
            }
            patches++;
            return Task.FromResult(patches == 1
                ? new HttpResponseMessage(HttpStatusCode.ServiceUnavailable)
                {
                    Content = new StringContent("""{"cause":"UNAVAILABLE"}""", Encoding.UTF8, "application/json"),
                }
                : JsonOk(RightsJson("newlyRevokedRights", ("CanActAs", "alice::p"))));
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var lease = await builder.GrantRightsLeaseAsync("some-user", actAs: new[] { "alice::p" });

        await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await lease.DisposeAsync());
        await lease.DisposeAsync();

        Assert.Equal(2, patches);
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_dispose_retry_accepts_a_right_the_user_no_longer_holds()
    {
        var methods = new List<HttpMethod>();
        var patches = 0;
        var handler = new RecordingHandler((req, _) =>
        {
            methods.Add(req.Method);
            if (req.Method == HttpMethod.Get)
            {
                return Task.FromResult(JsonOk(RightsJson("rights")));
            }
            if (req.Method != HttpMethod.Patch)
            {
                return Task.FromResult(JsonOk(RightsJson("newlyGrantedRights", ("CanActAs", "alice::p"))));
            }
            patches++;
            return Task.FromResult(patches == 1
                ? new HttpResponseMessage(HttpStatusCode.ServiceUnavailable)
                {
                    Content = new StringContent("""{"cause":"UNAVAILABLE"}""", Encoding.UTF8, "application/json"),
                }
                : JsonOk(RightsJson("newlyRevokedRights")));
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var lease = await builder.GrantRightsLeaseAsync("some-user", actAs: new[] { "alice::p" });

        await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await lease.DisposeAsync());
        await lease.DisposeAsync();

        Assert.Contains(HttpMethod.Get, methods);
        Assert.Equal(2, patches);
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_dispose_retry_still_throws_when_the_user_still_holds_the_right()
    {
        var handler = new RecordingHandler((req, _) =>
        {
            if (req.Method == HttpMethod.Get)
            {
                return Task.FromResult(JsonOk(RightsJson("rights", ("CanActAs", "alice::p"))));
            }
            if (req.Method != HttpMethod.Patch)
            {
                return Task.FromResult(JsonOk(RightsJson("newlyGrantedRights", ("CanActAs", "alice::p"))));
            }
            return Task.FromResult(JsonOk(RightsJson("newlyRevokedRights")));
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var lease = await builder.GrantRightsLeaseAsync("some-user", actAs: new[] { "alice::p" });

        await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await lease.DisposeAsync());
        await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await lease.DisposeAsync());
        Assert.Single(lease.Rights);
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_dispose_retry_throws_when_the_participant_lists_no_rights_field()
    {
        var handler = new RecordingHandler((req, _) =>
        {
            if (req.Method == HttpMethod.Get)
            {
                return Task.FromResult(JsonOk("{}"));
            }
            if (req.Method != HttpMethod.Patch)
            {
                return Task.FromResult(JsonOk(RightsJson("newlyGrantedRights", ("CanActAs", "alice::p"))));
            }
            return Task.FromResult(JsonOk(RightsJson("newlyRevokedRights")));
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var lease = await builder.GrantRightsLeaseAsync("some-user", actAs: new[] { "alice::p" });

        await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await lease.DisposeAsync());
        await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await lease.DisposeAsync());
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_dispose_does_not_consult_the_participant_on_the_first_attempt()
    {
        var methods = new List<HttpMethod>();
        var handler = new RecordingHandler((req, _) =>
        {
            methods.Add(req.Method);
            if (req.Method != HttpMethod.Patch)
            {
                return Task.FromResult(JsonOk(RightsJson("newlyGrantedRights", ("CanActAs", "alice::p"))));
            }
            return Task.FromResult(JsonOk(RightsJson("newlyRevokedRights")));
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var lease = await builder.GrantRightsLeaseAsync("some-user", actAs: new[] { "alice::p" });

        await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await lease.DisposeAsync());

        Assert.DoesNotContain(HttpMethod.Get, methods);
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_issues_no_request_when_actAs_and_readAs_empty()
    {
        var calls = 0;
        var handler = new RecordingHandler((_, _) =>
        {
            calls++;
            return Task.FromResult(JsonOk("{}"));
        });
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        var lease = await builder.GrantRightsLeaseAsync("some-user");
        await lease.DisposeAsync();

        Assert.Equal(0, calls);
        Assert.Empty(lease.Rights);
    }

    [Fact]
    public async Task GrantRightsLeaseAsync_rejects_empty_user_id()
    {
        using var http = new HttpClient(new RecordingHandler((_, _) => Task.FromResult(JsonOk("{}"))))
        {
            BaseAddress = JsonApiBase,
        };
        var builder = new UserBuilder(http, StaticTokenProvider("tok"));

        await Assert.ThrowsAsync<ArgumentException>(
            () => builder.GrantRightsLeaseAsync("", new[] { "alice::p" }));
    }
}
