// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

// Package repo locates the canton-localnet repository root that hosts
// the compose/ stack the CLI wraps.
package repo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrNotFound is returned when no ancestor of the starting directory
// contains a compose/ subtree.
var ErrNotFound = errors.New("repo: could not locate canton-localnet root (no compose/ directory found)")

// FindRoot walks up from start until it finds a directory containing a
// compose/modules subtree. start may be relative; it is resolved
// against the process working directory.
func FindRoot(start string) (string, error) {
	if start == "" {
		var err error
		start, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("repo: resolving working directory: %w", err)
		}
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("repo: %w", err)
	}
	dir := abs
	for {
		candidate := filepath.Join(dir, "compose", "modules")
		info, err := os.Stat(candidate)
		switch {
		case err == nil && info.IsDir():
			return dir, nil
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			return "", fmt.Errorf("repo: stat %s: %w", candidate, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNotFound
		}
		dir = parent
	}
}
