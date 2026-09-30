// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;
using System.Text;
using System.Text.Json;
using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

/// <summary>
/// Pins the 0.8.3-era call sites that <c>UploadAndVerifyAsync</c>,
/// <c>AllocateOnSynchronizerAsync</c> and friends must not break by adding a
/// same-named overload back. A same-named overload that only differs in a
/// trailing parameter's type makes a bare positional <c>default</c> argument
/// ambiguous (CS0121), so these calls existing at all — regardless of
/// whether they run to completion — is the regression check.
/// </summary>
public class PreRenameOverloadCallSiteTests
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
    public async Task UploadAsync_with_positional_default_cancellation_token_still_resolves_to_the_pre_0_8_4_1_overload()
    {
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers(
            (_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK))));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var uploader = new DarUploader(http, StaticTokenProvider("tok"));
        var directory = Directory.CreateTempSubdirectory("call-site-compat-");
        try
        {
            var darPath = Path.Combine(directory.FullName, "fixture.dar");
            await File.WriteAllBytesAsync(darPath, new byte[] { 1, 2, 3 });

            var outcome = await uploader.UploadAsync(darPath, default);

            Assert.Equal(DarUploadOutcome.Uploaded, outcome);
        }
        finally
        {
            directory.Delete(recursive: true);
        }
    }

    [Fact]
    public async Task AllocateAsync_with_positional_null_display_name_and_default_cancellation_token_still_resolves_to_the_pre_0_8_4_1_overload()
    {
        var payload = new { partyDetails = new { party = "app-abcdef::namespace", isLocal = true } };
        var handler = new RecordingHandler(SynchronizerDiscoveryResponder.WithConnectedSynchronizers(
            (_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
            {
                Content = new StringContent(JsonSerializer.Serialize(payload), Encoding.UTF8, "application/json"),
            })));
        using var http = new HttpClient(handler) { BaseAddress = JsonApiBase };
        var allocator = new PartyAllocator(http, StaticTokenProvider("tok"), instanceSuffix: "abcdef");

        var party = await allocator.AllocateAsync("app", null, default);

        Assert.Equal("app-abcdef::namespace", party.PartyId);
    }

    [Fact]
    public void Fixture_pass_throughs_with_positional_default_arguments_still_resolve_unambiguously()
    {
        Func<LocalnetFixture, Task<DarUploadOutcome>> uploadDar = fixture => fixture.UploadDarAsync("dummy.dar", default);
        Func<LocalnetFixture, Task<AllocatedParty>> allocateParty = fixture => fixture.AllocatePartyAsync("app", null, default);
        Func<ValidatorFixture, Task<DarUploadOutcome>> uploadDarForValidator = view => view.DarUploader.UploadAsync("dummy.dar", default);
        Func<ValidatorFixture, Task<AllocatedParty>> allocatePartyForValidator = view => view.AllocatePartyAsync("app", null, default);

        Assert.NotNull(uploadDar);
        Assert.NotNull(allocateParty);
        Assert.NotNull(uploadDarForValidator);
        Assert.NotNull(allocatePartyForValidator);
    }
}
