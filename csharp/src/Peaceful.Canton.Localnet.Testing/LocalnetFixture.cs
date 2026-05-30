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
    private readonly ILoggerFactory? _loggerFactory;
    private readonly Dictionary<LocalnetProfile, ValidatorFixture> _validators = new();
    private readonly object _validatorsLock = new();

    private LocalnetFixture(ServiceProvider services, LocalnetEndpoints endpoints, LocalnetProfile profile, ILoggerFactory? loggerFactory)
    {
        _services = services;
        _loggerFactory = loggerFactory;
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

    /// <summary>
    /// The ledger user id the slot's <c>client_credentials</c> token
    /// authenticates as. Grant this user <c>CanActAs</c> (via
    /// <see cref="GrantUserRightsAsync"/>) before submitting commands as an
    /// allocated party. See <see cref="LocalnetEndpoints.ValidatorUserId"/>.
    /// </summary>
    public string ValidatorUserId => Endpoints.ValidatorUserId;
    public JsonLedgerAdminClient AdminClient { get; }
    public OAuth2TokenProvider TokenProvider { get; }
    public DarUploader DarUploader { get; }
    public PartyAllocator PartyAllocator { get; }
    public UserBuilder UserBuilder { get; }

    /// <summary>
    /// Builds a fixture from the ambient environment. The profile is selected
    /// by <c>CANTON_LOCALNET_PROFILE</c> (default: a-validator-1).
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
        LocalnetProfile profile = LocalnetProfile.AValidator1,
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
        return new LocalnetFixture(provider, endpoints, profile, loggerFactory);
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

    /// <summary>
    /// Convenience pass-through to <see cref="UserBuilder.GrantRightsAsync"/>.
    /// Grants <c>CanActAs</c> (and optionally <c>CanReadAs</c>) for the given
    /// parties to an already-existing ledger user — e.g. the validator's
    /// service-account token user (<see cref="ValidatorUserId"/>) — so that
    /// <c>client_credentials</c>-authenticated command submission is authorized.
    /// </summary>
    public Task GrantUserRightsAsync(
        string userId,
        IEnumerable<string>? actAs = null,
        IEnumerable<string>? readAs = null,
        CancellationToken cancellationToken = default)
        => UserBuilder.GrantRightsAsync(userId, actAs, readAs, cancellationToken);

    /// <summary>
    /// Returns a per-slot view of the fixture. The first call for each
    /// slot resolves that slot's endpoints (via
    /// <see cref="EndpointDiscovery.Resolve(LocalnetProfile, IReadOnlyDictionary{string, string?}?)"/>)
    /// and constructs scoped admin, DAR, party, and user clients. Repeat
    /// calls return the cached instance. Passing the same slot the
    /// fixture was constructed with returns a view that shares the
    /// fixture's clients, so existing code that mixes both surfaces stays
    /// consistent.
    /// </summary>
    /// <param name="slot">Canonical slot name, e.g. <c>"a-validator-1"</c>.</param>
    public ValidatorFixture Validator(string slot)
    {
        if (string.IsNullOrWhiteSpace(slot))
        {
            throw new ArgumentException("Slot name is required.", nameof(slot));
        }
        return Validator(ParseSlot(slot));
    }

    /// <summary>
    /// Returns a per-slot view of the fixture for the given profile. See
    /// <see cref="Validator(string)"/> for caching semantics.
    /// </summary>
    public ValidatorFixture Validator(LocalnetProfile profile)
    {
        lock (_validatorsLock)
        {
            if (_validators.TryGetValue(profile, out var existing))
            {
                return existing;
            }
            var view = BuildValidator(profile);
            _validators[profile] = view;
            return view;
        }
    }

    private ValidatorFixture BuildValidator(LocalnetProfile profile)
    {
        if (profile == Profile)
        {
            return new ValidatorFixture(
                slot: SlotName(profile),
                profile: profile,
                endpoints: Endpoints,
                adminClient: AdminClient,
                tokenProvider: TokenProvider,
                darUploader: DarUploader,
                partyAllocator: PartyAllocator,
                userBuilder: UserBuilder,
                ownedServices: null);
        }
        var endpoints = EndpointDiscovery.ResolveForSlot(profile);
        var services = BuildServices(endpoints, _loggerFactory);
        return new ValidatorFixture(
            slot: SlotName(profile),
            profile: profile,
            endpoints: endpoints,
            adminClient: services.GetRequiredService<JsonLedgerAdminClient>(),
            tokenProvider: services.GetRequiredService<OAuth2TokenProvider>(),
            darUploader: services.GetRequiredService<DarUploader>(),
            partyAllocator: services.GetRequiredService<PartyAllocator>(),
            userBuilder: services.GetRequiredService<UserBuilder>(),
            ownedServices: services);
    }

    private static ServiceProvider BuildServices(LocalnetEndpoints endpoints, ILoggerFactory? loggerFactory)
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

        return services.BuildServiceProvider();
    }

    private static LocalnetProfile ParseSlot(string slot) => slot.Trim().ToLowerInvariant() switch
    {
        "sv-validator-1" or "super-validator" or "supervalidator" => LocalnetProfile.SvValidator1,
        "a-validator-1" => LocalnetProfile.AValidator1,
        "b-validator-1" => LocalnetProfile.BValidator1,
        "c-validator-1" => LocalnetProfile.CValidator1,
        "d-validator-1" => LocalnetProfile.DValidator1,
        _ => throw new ArgumentException(
            $"Unknown slot '{slot}'; expected one of: sv-validator-1, a-validator-1, b-validator-1, c-validator-1, d-validator-1.",
            nameof(slot)),
    };

    private static string SlotName(LocalnetProfile profile) => profile switch
    {
        LocalnetProfile.SvValidator1 => "sv-validator-1",
        LocalnetProfile.AValidator1 => "a-validator-1",
        LocalnetProfile.BValidator1 => "b-validator-1",
        LocalnetProfile.CValidator1 => "c-validator-1",
        LocalnetProfile.DValidator1 => "d-validator-1",
        _ => throw new ArgumentOutOfRangeException(nameof(profile), profile, null),
    };

    /// <summary>
    /// Returns the canonical slot names supported by the fixture, in a
    /// stable order: sv, a, b, c, d.
    /// </summary>
    public static IReadOnlyList<string> KnownSlots() => new[]
    {
        "sv-validator-1",
        "a-validator-1",
        "b-validator-1",
        "c-validator-1",
        "d-validator-1",
    };

    public async ValueTask DisposeAsync()
    {
        ValidatorFixture[] extras;
        lock (_validatorsLock)
        {
            extras = _validators.Values.ToArray();
            _validators.Clear();
        }
        foreach (var extra in extras)
        {
            await extra.DisposeAsync().ConfigureAwait(false);
        }
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
