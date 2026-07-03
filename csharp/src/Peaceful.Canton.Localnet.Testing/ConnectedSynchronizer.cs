// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

namespace Peaceful.Canton.Localnet.Testing;

/// <summary>
/// One synchronizer a participant is connected to, as returned by the JSON
/// Ledger API <c>GET /v2/state/connected-synchronizers</c> endpoint.
/// </summary>
/// <param name="Alias">Stable synchronizer alias, e.g. <c>global</c> or <c>app-synchronizer</c>.</param>
/// <param name="Id">Full synchronizer id (embeds a key fingerprint; not stable across resets).</param>
/// <param name="Permission">The party's permission on the synchronizer, when a party scoped the query.</param>
public sealed record ConnectedSynchronizer(string Alias, string Id, string? Permission);
