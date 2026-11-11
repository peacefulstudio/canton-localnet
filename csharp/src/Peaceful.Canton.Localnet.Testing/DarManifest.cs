// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.IO.Compression;
using System.Text;
using System.Text.RegularExpressions;

namespace Peaceful.Canton.Localnet.Testing;

/// <summary>
/// Reads the main package id of a DAR from its <c>META-INF/MANIFEST.MF</c>.
/// The JSON Ledger API answers a DAR upload with an empty body, so the
/// manifest is the only place the id of the uploaded package is named.
/// </summary>
internal static partial class DarManifest
{
    private const string ManifestEntryName = "META-INF/MANIFEST.MF";
    private const string MainDalfKey = "Main-Dalf:";

    [GeneratedRegex("^[0-9a-f]{64}$")]
    private static partial Regex PackageIdPattern();

    /// <summary>
    /// Returns the package id embedded in the <c>Main-Dalf</c> entry of the
    /// DAR's manifest, e.g. <c>canton-localnet-noop-0.0.1-&lt;id&gt;/canton-localnet-noop-0.0.1-&lt;id&gt;.dalf</c>.
    /// </summary>
    /// <param name="darBytes">The raw DAR (zip archive) bytes.</param>
    /// <exception cref="InvalidDataException">
    /// The payload is not a zip archive, has no manifest or no <c>Main-Dalf</c>
    /// entry, or the main dalf's name carries no package id.
    /// </exception>
    internal static string ReadMainPackageId(byte[] darBytes)
    {
        ArgumentNullException.ThrowIfNull(darBytes);

        using var archive = new ZipArchive(new MemoryStream(darBytes), ZipArchiveMode.Read);
        var entry = archive.GetEntry(ManifestEntryName)
            ?? throw new InvalidDataException($"DAR has no {ManifestEntryName}.");

        using var reader = new StreamReader(entry.Open(), Encoding.UTF8);
        var mainDalf = UnfoldedLines(reader.ReadToEnd())
            .Where(line => line.StartsWith(MainDalfKey, StringComparison.OrdinalIgnoreCase))
            .Select(line => line[MainDalfKey.Length..].Trim())
            .FirstOrDefault()
            ?? throw new InvalidDataException($"{ManifestEntryName} has no Main-Dalf entry.");

        var dalfFileName = Path.GetFileNameWithoutExtension(mainDalf.Split('/')[^1]);
        var packageId = dalfFileName[(dalfFileName.LastIndexOf('-') + 1)..];
        if (!PackageIdPattern().IsMatch(packageId))
        {
            throw new InvalidDataException($"Main-Dalf '{mainDalf}' carries no package id.");
        }

        return packageId;
    }

    private static IEnumerable<string> UnfoldedLines(string manifest)
    {
        var current = new StringBuilder();
        foreach (var line in manifest.Split('\n').Select(l => l.TrimEnd('\r')))
        {
            if (line.StartsWith(' '))
            {
                current.Append(line, 1, line.Length - 1);
                continue;
            }
            if (current.Length > 0)
            {
                yield return current.ToString();
                current.Clear();
            }
            current.Append(line);
        }
        if (current.Length > 0)
        {
            yield return current.ToString();
        }
    }
}
