// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.IO.Compression;
using System.Text;

namespace Peaceful.Canton.Localnet.Testing.Tests;

internal static class DarFixtures
{
    public const string NoopMainPackageId = "b1008daac9810baa8ca7ff313c13bd48d65f18040b2d33faabe5b5d39acaac9e";

    public const string NoopManifestResource = "dar-manifest/canton-localnet-noop-0.0.1.MF";

    public static string NoopManifestText() =>
        File.ReadAllText(Path.Combine(AppContext.BaseDirectory, NoopManifestResource), Encoding.UTF8);

    public static byte[] DarWithManifest(string manifestText)
    {
        using var stream = new MemoryStream();
        using (var archive = new ZipArchive(stream, ZipArchiveMode.Create, leaveOpen: true))
        {
            var entry = archive.CreateEntry("META-INF/MANIFEST.MF");
            using var writer = new StreamWriter(entry.Open(), new UTF8Encoding(false));
            writer.Write(manifestText);
        }
        return stream.ToArray();
    }

    public static byte[] NoopDar() => DarWithManifest(NoopManifestText());
}
