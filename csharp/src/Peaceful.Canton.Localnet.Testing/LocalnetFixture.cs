// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging;

namespace Peaceful.Canton.Localnet.Testing;

/// <summary>
/// xUnit fixture that composes <see cref="EndpointDiscovery"/>,
/// <see cref="OAuth2TokenProvider"/>, and <see cref="JsonLedgerAdminClient"/>
/// into a single user-facing surface. Resolve the fixture from your test class
/// constructor (xUnit v3 <c>IAsyncLifetime</c>) and call methods on
/// <see cref="AdminClient"/>.
/// </summary>
public sealed class LocalnetFixture : IAsyncDisposable
{
    private readonly ServiceProvider _services;

    private LocalnetFixture(ServiceProvider services, LocalnetEndpoints endpoints, LocalnetProfile profile)
    {
        _services = services;
        Endpoints = endpoints;
        Profile = profile;
        AdminClient = services.GetRequiredService<JsonLedgerAdminClient>();
        TokenProvider = services.GetRequiredService<OAuth2TokenProvider>();
        DarUploader = services.GetRequiredService<DarUploader>();
        PartyAllocator = services.GetRequiredService<PartyAllocator>();
        UserBuilder = services.GetRequiredService<UserBuilder>();
    }

    public LocalnetEndpoints Endpoints { get; }
    public LocalnetProfile Profile { get; }
    public JsonLedgerAdminClient AdminClient { get; }
    public OAuth2TokenProvider TokenProvider { get; }
    public DarUploader DarUploader { get; }
    public PartyAllocator PartyAllocator { get; }
    public UserBuilder UserBuilder { get; }

    /// <summary>
    /// Builds a fixture from the ambient environment. The profile is selected
    /// by <c>CANTON_LOCALNET_PROFILE</c> (default: app-provider).
    /// </summary>
    public static LocalnetFixture FromEnvironment(ILoggerFactory? loggerFactory = null)
    {
        var profile = EndpointDiscovery.ResolveProfile();
        var endpoints = EndpointDiscovery.Resolve(profile);
        return Build(endpoints, profile, loggerFactory);
    }

    /// <summary>
    /// Builds a fixture with explicit endpoints — useful in tests that
    /// pre-fabricate URLs (e.g. against ephemeral containers).
    /// </summary>
    public static LocalnetFixture FromEndpoints(
        LocalnetEndpoints endpoints,
        LocalnetProfile profile = LocalnetProfile.AppProvider,
        ILoggerFactory? loggerFactory = null) => Build(endpoints, profile, loggerFactory);

    private static LocalnetFixture Build(LocalnetEndpoints endpoints, LocalnetProfile profile, ILoggerFactory? loggerFactory)
    {
        var services = new ServiceCollection();
        if (loggerFactory is not null)
        {
            services.AddSingleton(loggerFactory);
            services.AddLogging();
        }
        else
        {
            services.AddLogging();
        }

        services.AddSingleton(endpoints);
        services.AddSingleton(new OAuth2TokenProviderOptions(
            endpoints.TokenEndpoint,
            endpoints.ClientId,
            endpoints.ClientSecret,
            endpoints.Audience,
            endpoints.Scope));

        services.AddHttpClient("oauth2");
        services.AddHttpClient("json-ledger", client =>
        {
            client.BaseAddress = endpoints.JsonLedgerApi.EnsureTrailingSlash();
        });

        services.AddSingleton(sp =>
        {
            var httpClientFactory = sp.GetRequiredService<IHttpClientFactory>();
            var opts = sp.GetRequiredService<OAuth2TokenProviderOptions>();
            var logger = sp.GetService<ILogger<OAuth2TokenProvider>>();
            return new OAuth2TokenProvider(httpClientFactory.CreateClient("oauth2"), opts, logger);
        });

        services.AddSingleton(sp =>
        {
            var httpClientFactory = sp.GetRequiredService<IHttpClientFactory>();
            var tokenProvider = sp.GetRequiredService<OAuth2TokenProvider>();
            var logger = sp.GetService<ILogger<JsonLedgerAdminClient>>();
            return new JsonLedgerAdminClient(httpClientFactory.CreateClient("json-ledger"), tokenProvider, logger);
        });

        services.AddSingleton(sp =>
        {
            var httpClientFactory = sp.GetRequiredService<IHttpClientFactory>();
            var tokenProvider = sp.GetRequiredService<OAuth2TokenProvider>();
            var logger = sp.GetService<ILogger<DarUploader>>();
            return new DarUploader(httpClientFactory.CreateClient("json-ledger"), tokenProvider, logger);
        });

        services.AddSingleton(sp =>
        {
            var httpClientFactory = sp.GetRequiredService<IHttpClientFactory>();
            var tokenProvider = sp.GetRequiredService<OAuth2TokenProvider>();
            var logger = sp.GetService<ILogger<PartyAllocator>>();
            return new PartyAllocator(httpClientFactory.CreateClient("json-ledger"), tokenProvider, logger);
        });

        services.AddSingleton(sp =>
        {
            var httpClientFactory = sp.GetRequiredService<IHttpClientFactory>();
            var tokenProvider = sp.GetRequiredService<OAuth2TokenProvider>();
            var logger = sp.GetService<ILogger<UserBuilder>>();
            return new UserBuilder(httpClientFactory.CreateClient("json-ledger"), tokenProvider, logger);
        });

        var provider = services.BuildServiceProvider();
        return new LocalnetFixture(provider, endpoints, profile);
    }

    /// <summary>
    /// Convenience pass-through to <see cref="JsonLedgerAdminClient.GetParticipantIdAsync"/>.
    /// </summary>
    public Task<string> GetParticipantIdAsync(CancellationToken cancellationToken = default)
        => AdminClient.GetParticipantIdAsync(cancellationToken);

    /// <summary>
    /// Convenience pass-through to <see cref="DarUploader.UploadAsync(string, CancellationToken)"/>.
    /// </summary>
    public Task<DarUploadOutcome> UploadDarAsync(string darPath, CancellationToken cancellationToken = default)
        => DarUploader.UploadAsync(darPath, cancellationToken);

    /// <summary>
    /// Convenience pass-through to <see cref="DarUploader.UploadManyAsync"/>.
    /// </summary>
    public Task<IReadOnlyList<DarUploadResult>> UploadDarsAsync(
        IEnumerable<string> darPaths,
        CancellationToken cancellationToken = default)
        => DarUploader.UploadManyAsync(darPaths, cancellationToken);

    /// <summary>
    /// Convenience pass-through to <see cref="PartyAllocator.AllocateAsync"/>.
    /// </summary>
    public Task<AllocatedParty> AllocatePartyAsync(
        string consumerPrefix,
        string? displayName = null,
        CancellationToken cancellationToken = default)
        => PartyAllocator.AllocateAsync(consumerPrefix, displayName, cancellationToken);

    /// <summary>
    /// Convenience pass-through to <see cref="UserBuilder.CreateAsync"/>.
    /// </summary>
    public Task<string> CreateUserAsync(
        string userId,
        string? primaryParty = null,
        IEnumerable<string>? actAs = null,
        IEnumerable<string>? readAs = null,
        CancellationToken cancellationToken = default)
        => UserBuilder.CreateAsync(userId, primaryParty, actAs, readAs, cancellationToken);

    public async ValueTask DisposeAsync()
    {
        TokenProvider.Dispose();
        await _services.DisposeAsync().ConfigureAwait(false);
    }
}

internal static class UriExtensions
{
    public static Uri EnsureTrailingSlash(this Uri uri)
    {
        var s = uri.ToString();
        return s.EndsWith('/') ? uri : new Uri(s + "/", UriKind.Absolute);
    }
}
