// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.IO.Compression;
using System.Text;
using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

public class DarManifestTests
{
    [Theory]
    [InlineData("\n")]
    [InlineData("\r\n")]
    public void ReadMainPackageId_unfolds_the_wrapped_Main_Dalf_line_of_a_real_damlc_manifest(string lineEnding)
    {
        var manifest = DarFixtures.NoopManifestText().Replace("\n", lineEnding);

        var id = DarManifest.ReadMainPackageId(DarFixtures.DarWithManifest(manifest));

        Assert.Equal("b1008daac9810baa8ca7ff313c13bd48d65f18040b2d33faabe5b5d39acaac9e", id);
    }

    [Fact]
    public void ReadMainPackageId_reads_an_unwrapped_Main_Dalf_line()
    {
        var manifest = "Manifest-Version: 1.0\nMain-Dalf: pkg-1.0.0-" + new string('a', 64) + "/pkg-1.0.0-" + new string('a', 64) + ".dalf\nDalfs: x.dalf\n";

        var id = DarManifest.ReadMainPackageId(DarFixtures.DarWithManifest(manifest));

        Assert.Equal(new string('a', 64), id);
    }

    [Fact]
    public void ReadMainPackageId_throws_when_the_payload_is_not_a_zip_archive()
    {
        Assert.Throws<InvalidDataException>(() => DarManifest.ReadMainPackageId(new byte[] { 1, 2, 3 }));
    }

    [Fact]
    public void ReadMainPackageId_throws_when_the_archive_has_no_manifest()
    {
        using var stream = new MemoryStream();
        using (var archive = new ZipArchive(stream, ZipArchiveMode.Create, leaveOpen: true))
        {
            archive.CreateEntry("other.txt");
        }

        var ex = Assert.Throws<InvalidDataException>(() => DarManifest.ReadMainPackageId(stream.ToArray()));
        Assert.Contains("META-INF/MANIFEST.MF", ex.Message);
    }

    [Fact]
    public void ReadMainPackageId_throws_when_the_manifest_has_no_Main_Dalf()
    {
        var dar = DarFixtures.DarWithManifest("Manifest-Version: 1.0\nCreated-By: damlc\n");

        var ex = Assert.Throws<InvalidDataException>(() => DarManifest.ReadMainPackageId(dar));
        Assert.Contains("Main-Dalf", ex.Message);
    }

    [Theory]
    [InlineData("Main-Dalf: pkg-1.0.0/pkg-1.0.0.dalf")]
    [InlineData("Main-Dalf: pkg-1.0.0-XYZ/pkg-1.0.0-XYZ.dalf")]
    public void ReadMainPackageId_throws_when_the_Main_Dalf_name_carries_no_package_id(string mainDalfLine)
    {
        var dar = DarFixtures.DarWithManifest($"Manifest-Version: 1.0\n{mainDalfLine}\n");

        var ex = Assert.Throws<InvalidDataException>(() => DarManifest.ReadMainPackageId(dar));
        Assert.Contains("package id", ex.Message);
    }
}
