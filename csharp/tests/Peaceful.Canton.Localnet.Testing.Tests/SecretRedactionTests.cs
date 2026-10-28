// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

public class SecretRedactionTests
{
    private static LocalnetEndpoints Endpoints() => new(
        JsonLedgerApi: new Uri("http://localhost:11975/"),
        TokenEndpoint: new Uri("https://keycloak.test/token"),
        Audience: "aud-literal",
        ClientId: "client-id-literal",
        ClientSecret: "hunter2-secret-literal",
        Scope: "openid",
        ValidatorUserId: "validator-user-literal");

    private static OAuth2TokenProviderOptions Options() => new(
        TokenEndpoint: new Uri("https://keycloak.test/token"),
        ClientId: "client-id-literal",
        ClientSecret: "hunter2-secret-literal",
        Audience: "aud-literal");

    [Fact]
    public void LocalnetEndpoints_ToString_does_not_contain_the_client_secret()
    {
        Assert.DoesNotContain("hunter2-secret-literal", Endpoints().ToString());
    }

    [Fact]
    public void LocalnetEndpoints_ToString_marks_the_client_secret_as_redacted()
    {
        Assert.Contains("ClientSecret = <redacted>", Endpoints().ToString());
    }

    [Fact]
    public void LocalnetEndpoints_ToString_still_prints_the_other_members()
    {
        var text = Endpoints().ToString();

        Assert.StartsWith("LocalnetEndpoints { JsonLedgerApi = http://localhost:11975/", text);
        Assert.Contains("ClientId = client-id-literal", text);
        Assert.Contains("ValidatorUserId = validator-user-literal", text);
    }

    [Fact]
    public void OAuth2TokenProviderOptions_ToString_does_not_contain_the_client_secret()
    {
        Assert.DoesNotContain("hunter2-secret-literal", Options().ToString());
    }

    [Fact]
    public void OAuth2TokenProviderOptions_ToString_marks_the_client_secret_as_redacted()
    {
        Assert.Contains("ClientSecret = <redacted>", Options().ToString());
    }

    [Fact]
    public void OAuth2TokenProviderOptions_ToString_still_prints_the_other_members()
    {
        var text = Options().ToString();

        Assert.StartsWith("OAuth2TokenProviderOptions { TokenEndpoint = https://keycloak.test/token", text);
        Assert.Contains("ClientId = client-id-literal", text);
        Assert.Contains("Scope = openid", text);
        Assert.Contains("ExpiryLeeway = 00:00:30", text);
    }

    [Fact]
    public void Records_keep_value_equality_on_the_client_secret()
    {
        Assert.NotEqual(Endpoints(), Endpoints() with { ClientSecret = "other" });
        Assert.NotEqual(Options(), Options() with { ClientSecret = "other" });
    }
}
