// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;

namespace Peaceful.Canton.Localnet.Testing;

/// <summary>
/// Raised by <see cref="DarUploader"/> when a synchronizer answered
/// <c>KNOWN_PACKAGE_VERSION</c> but the DAR's main package is not vetted there:
/// Canton rejected a different package id that shares the name and version of
/// an already vetted package, so the upload changed nothing and the first
/// command against the new package would fail with
/// <c>NO_SYNCHRONIZER_FOR_SUBMISSION</c>.
/// </summary>
public sealed class DarNotVettedException : JsonLedgerApiException
{
    /// <summary>Creates the exception for <paramref name="mainPackageId"/> on <paramref name="synchronizerId"/>.</summary>
    /// <param name="sourceLabel">The file path or label the DAR was uploaded under.</param>
    /// <param name="mainPackageId">Main package id read from the DAR's manifest.</param>
    /// <param name="synchronizerId">Synchronizer that answered <c>KNOWN_PACKAGE_VERSION</c>, or <c>null</c> when the upload named none.</param>
    /// <param name="conflictingPackageIds">Other package ids Canton's error names, usually the vetted package that blocks this one.</param>
    /// <param name="responseBody">Raw <c>KNOWN_PACKAGE_VERSION</c> response body.</param>
    internal DarNotVettedException(
        string sourceLabel,
        string mainPackageId,
        string? synchronizerId,
        IReadOnlyList<string> conflictingPackageIds,
        string responseBody)
        : base(BuildMessage(sourceLabel, mainPackageId, synchronizerId, conflictingPackageIds, responseBody), HttpStatusCode.BadRequest, responseBody)
    {
        MainPackageId = mainPackageId;
        SynchronizerId = synchronizerId;
        ConflictingPackageIds = conflictingPackageIds;
    }

    /// <summary>Main package id of the DAR that was not vetted.</summary>
    public string MainPackageId { get; }

    /// <summary>Synchronizer on which the main package is not vetted, or <c>null</c> when the upload named none.</summary>
    public string? SynchronizerId { get; }

    /// <summary>Other package ids named by Canton's error; empty when the body names none.</summary>
    public IReadOnlyList<string> ConflictingPackageIds { get; }

    private static string BuildMessage(
        string sourceLabel,
        string mainPackageId,
        string? synchronizerId,
        IReadOnlyList<string> conflictingPackageIds,
        string responseBody)
    {
        var conflicting = conflictingPackageIds.Count == 0
            ? "none named"
            : string.Join(", ", conflictingPackageIds);
        return $"DAR {sourceLabel} reported KNOWN_PACKAGE_VERSION on synchronizer {synchronizerId ?? "(default)"}, "
            + $"but its main package '{mainPackageId}' is not vetted there. "
            + $"Canton usually answers this when another package with the same name and version is already vetted (other ids named: {conflicting}); bump the package version. "
            + $"Canton said: {responseBody}";
    }
}
