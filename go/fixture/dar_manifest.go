// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
)

const (
	manifestEntryName = "META-INF/MANIFEST.MF"
	mainDalfKey       = "Main-Dalf:"
)

var packageIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func mainPackageID(dar []byte) (string, error) {
	archive, err := zip.NewReader(bytes.NewReader(dar), int64(len(dar)))
	if err != nil {
		return "", fmt.Errorf("dar: read archive: %w", err)
	}
	manifest, err := readManifest(archive)
	if err != nil {
		return "", err
	}
	mainDalf, ok := findMainDalf(manifest)
	if !ok {
		return "", fmt.Errorf("dar: %s has no Main-Dalf entry", manifestEntryName)
	}
	dalfName := strings.TrimSuffix(path.Base(mainDalf), path.Ext(mainDalf))
	id := dalfName[strings.LastIndex(dalfName, "-")+1:]
	if !packageIDPattern.MatchString(id) {
		return "", fmt.Errorf("dar: Main-Dalf %q carries no package id", mainDalf)
	}
	return id, nil
}

func readManifest(archive *zip.Reader) (string, error) {
	for _, file := range archive.File {
		if file.Name != manifestEntryName {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return "", fmt.Errorf("dar: open %s: %w", manifestEntryName, err)
		}
		defer rc.Close()
		raw, err := io.ReadAll(rc)
		if err != nil {
			return "", fmt.Errorf("dar: read %s: %w", manifestEntryName, err)
		}
		return string(raw), nil
	}
	return "", errors.New("dar: archive has no " + manifestEntryName)
}

func findMainDalf(manifest string) (string, bool) {
	for _, line := range unfoldManifestLines(manifest) {
		if len(line) >= len(mainDalfKey) && strings.EqualFold(line[:len(mainDalfKey)], mainDalfKey) {
			return strings.TrimSpace(line[len(mainDalfKey):]), true
		}
	}
	return "", false
}

func unfoldManifestLines(manifest string) []string {
	var lines []string
	for _, raw := range strings.Split(manifest, "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if strings.HasPrefix(line, " ") && len(lines) > 0 {
			lines[len(lines)-1] += line[1:]
			continue
		}
		lines = append(lines, line)
	}
	return lines
}
