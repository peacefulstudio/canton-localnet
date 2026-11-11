// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Text.Json;
using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

public class EndpointDiscoverySharedCasesTests
{
    private static readonly JsonSerializerOptions JsonOptions = new() { PropertyNameCaseInsensitive = true };

    public static TheoryData<string> CaseNames()
    {
        var data = new TheoryData<string>();
        foreach (var name in LoadCases().Keys)
        {
            data.Add(name);
        }
        return data;
    }

    [Theory]
    [MemberData(nameof(CaseNames))]
    public void Resolve_matches_the_shared_endpoint_discovery_case(string caseName)
    {
        var sharedCase = LoadCases()[caseName];
        var profile = ProfileFor(sharedCase.Slot);

        if (sharedCase.Expect is null)
        {
            var failure = Assert.Throws<InvalidOperationException>(
                () => EndpointDiscovery.Resolve(profile, sharedCase.Env));
            foreach (var mention in sharedCase.Error!)
            {
                Assert.Contains(mention, failure.Message);
            }
            return;
        }

        var endpoints = EndpointDiscovery.Resolve(profile, sharedCase.Env);

        Assert.Equal(new Uri(sharedCase.Expect.JsonApiUrl), endpoints.JsonLedgerApi);
        Assert.Equal(new Uri(sharedCase.Expect.GrpcLedgerApiUrl), endpoints.GrpcLedgerApi);
        Assert.Equal(new Uri(sharedCase.Expect.TokenUrl), endpoints.TokenEndpoint);
        Assert.Equal(sharedCase.Expect.Audience, endpoints.Audience);
        Assert.Equal(sharedCase.Expect.Scope, endpoints.Scope);
        Assert.Equal(sharedCase.Expect.ClientId, endpoints.ClientId);
        if (sharedCase.Expect.ClientSecret is null)
        {
            Assert.False(string.IsNullOrEmpty(endpoints.ClientSecret));
        }
        else
        {
            Assert.Equal(sharedCase.Expect.ClientSecret, endpoints.ClientSecret);
        }
    }

    private static Dictionary<string, SharedCase> LoadCases()
    {
        var path = Path.Combine(AppContext.BaseDirectory, "endpoint-discovery", "cases.json");
        var file = JsonSerializer.Deserialize<SharedCaseFile>(File.ReadAllText(path), JsonOptions)
            ?? throw new InvalidOperationException($"{path} is empty.");
        return file.Cases.ToDictionary(c => c.Name, StringComparer.Ordinal);
    }

    private static LocalnetProfile ProfileFor(string slot) => slot switch
    {
        "sv-validator-1" => LocalnetProfile.SvValidator1,
        "a-validator-1" => LocalnetProfile.AValidator1,
        "b-validator-1" => LocalnetProfile.BValidator1,
        "c-validator-1" => LocalnetProfile.CValidator1,
        "d-validator-1" => LocalnetProfile.DValidator1,
        _ => throw new ArgumentOutOfRangeException(nameof(slot), slot, null),
    };

    private sealed record SharedCaseFile(List<SharedCase> Cases);

    private sealed record SharedCase(
        string Name,
        string Slot,
        Dictionary<string, string?> Env,
        SharedExpectation? Expect,
        List<string>? Error);

    private sealed record SharedExpectation(
        string JsonApiUrl,
        string GrpcLedgerApiUrl,
        string TokenUrl,
        string Audience,
        string Scope,
        string ClientId,
        string? ClientSecret);
}
