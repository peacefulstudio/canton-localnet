// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package repo

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFindRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "compose", "modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := FindRoot(nested)
	if err != nil {
		t.Fatalf("FindRoot: %v", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	resolvedGot, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedGot != resolvedRoot {
		t.Errorf("expected %q, got %q", resolvedRoot, resolvedGot)
	}
}

func TestFindRootNotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := FindRoot(dir)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
