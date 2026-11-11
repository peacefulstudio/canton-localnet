// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"archive/zip"
	"bytes"
	"os"
	"strings"
	"testing"
)

const (
	noopManifestPath  = "../../testdata/dar-manifest/canton-localnet-noop-0.0.1.MF"
	noopMainPackageID = "b1008daac9810baa8ca7ff313c13bd48d65f18040b2d33faabe5b5d39acaac9e"
)

func noopManifestText(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(noopManifestPath)
	if err != nil {
		t.Fatalf("read shared manifest fixture: %v", err)
	}
	return string(raw)
}

func darWithManifest(t *testing.T, manifest string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("META-INF/MANIFEST.MF")
	if err != nil {
		t.Fatalf("create manifest entry: %v", err)
	}
	if _, err := w.Write([]byte(manifest)); err != nil {
		t.Fatalf("write manifest entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func noopDar(t *testing.T) []byte {
	t.Helper()
	return darWithManifest(t, noopManifestText(t))
}

func TestMainPackageID_UnfoldsTheWrappedMainDalfLineOfARealDamlcManifest(t *testing.T) {
	for name, lineEnding := range map[string]string{"LF": "\n", "CRLF": "\r\n"} {
		t.Run(name, func(t *testing.T) {
			manifest := strings.ReplaceAll(noopManifestText(t), "\n", lineEnding)

			id, err := mainPackageID(darWithManifest(t, manifest))
			if err != nil {
				t.Fatalf("mainPackageID: %v", err)
			}
			if id != "b1008daac9810baa8ca7ff313c13bd48d65f18040b2d33faabe5b5d39acaac9e" {
				t.Errorf("id = %q, want b1008daa...acaac9e", id)
			}
		})
	}
}

func TestMainPackageID_ReadsAnUnwrappedMainDalfLine(t *testing.T) {
	id64 := strings.Repeat("a", 64)
	manifest := "Manifest-Version: 1.0\nMain-Dalf: pkg-1.0.0-" + id64 + "/pkg-1.0.0-" + id64 + ".dalf\nDalfs: x.dalf\n"

	id, err := mainPackageID(darWithManifest(t, manifest))
	if err != nil {
		t.Fatalf("mainPackageID: %v", err)
	}
	if id != id64 {
		t.Errorf("id = %q, want %q", id, id64)
	}
}

func TestMainPackageID_FailsWhenThePayloadIsNotAZipArchive(t *testing.T) {
	if _, err := mainPackageID([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected error for non-zip payload")
	}
}

func TestMainPackageID_FailsWhenTheArchiveHasNoManifest(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if _, err := zw.Create("other.txt"); err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	_, err := mainPackageID(buf.Bytes())
	if err == nil || !strings.Contains(err.Error(), "META-INF/MANIFEST.MF") {
		t.Fatalf("error = %v, want one naming META-INF/MANIFEST.MF", err)
	}
}

func TestMainPackageID_FailsWhenTheManifestHasNoMainDalf(t *testing.T) {
	_, err := mainPackageID(darWithManifest(t, "Manifest-Version: 1.0\nCreated-By: damlc\n"))
	if err == nil || !strings.Contains(err.Error(), "Main-Dalf") {
		t.Fatalf("error = %v, want one naming Main-Dalf", err)
	}
}

func TestMainPackageID_FailsWhenTheMainDalfNameCarriesNoPackageID(t *testing.T) {
	for _, line := range []string{
		"Main-Dalf: pkg-1.0.0/pkg-1.0.0.dalf",
		"Main-Dalf: pkg-1.0.0-XYZ/pkg-1.0.0-XYZ.dalf",
	} {
		t.Run(line, func(t *testing.T) {
			_, err := mainPackageID(darWithManifest(t, "Manifest-Version: 1.0\n"+line+"\n"))
			if err == nil || !strings.Contains(err.Error(), "package id") {
				t.Fatalf("error = %v, want one naming package id", err)
			}
		})
	}
}
