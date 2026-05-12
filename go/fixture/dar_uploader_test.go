// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func writeTempDar(t *testing.T, name string, payload []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write temp dar: %v", err)
	}
	return path
}

type recordedRequest struct {
	method      string
	path        string
	auth        string
	contentType string
	accept      string
	body        []byte
}

func TestDarUploader_RequestShape(t *testing.T) {
	payload := []byte("totally a dar file")
	var recorded recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded.method = r.Method
		recorded.path = r.URL.Path
		recorded.auth = r.Header.Get("Authorization")
		recorded.contentType = r.Header.Get("Content-Type")
		recorded.accept = r.Header.Get("Accept")
		body, _ := io.ReadAll(r.Body)
		recorded.body = body
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	u, err := NewDarUploader(server.URL, staticTokenProvider{token: "tok-dar"})
	if err != nil {
		t.Fatalf("NewDarUploader: %v", err)
	}

	path := writeTempDar(t, "tiny.dar", payload)
	if err := u.Upload(context.Background(), path); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if recorded.method != http.MethodPost {
		t.Errorf("method = %q, want POST", recorded.method)
	}
	if recorded.path != "/v2/packages" {
		t.Errorf("path = %q, want /v2/packages", recorded.path)
	}
	if recorded.auth != "Bearer tok-dar" {
		t.Errorf("auth = %q", recorded.auth)
	}
	if recorded.contentType != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want application/octet-stream", recorded.contentType)
	}
	if recorded.accept != "application/json" {
		t.Errorf("Accept = %q", recorded.accept)
	}
	if !bytes.Equal(recorded.body, payload) {
		t.Errorf("body = %q, want %q", recorded.body, payload)
	}
}

func TestDarUploader_TreatsKnownPackageVersionAsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"cause":"KNOWN_PACKAGE_VERSION(Package already known)"}`)
	}))
	t.Cleanup(server.Close)

	u, err := NewDarUploader(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewDarUploader: %v", err)
	}
	path := writeTempDar(t, "dup.dar", []byte("bytes"))
	if err := u.Upload(context.Background(), path); err != nil {
		t.Fatalf("expected success on KNOWN_PACKAGE_VERSION, got: %v", err)
	}
}

func TestDarUploader_RepeatedUploadIsIdempotent(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 1 {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"cause":"KNOWN_PACKAGE_VERSION"}`)
	}))
	t.Cleanup(server.Close)

	u, err := NewDarUploader(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewDarUploader: %v", err)
	}
	path := writeTempDar(t, "same.dar", []byte("a"))
	ctx := context.Background()
	if err := u.Upload(ctx, path); err != nil {
		t.Fatalf("first Upload: %v", err)
	}
	if err := u.Upload(ctx, path); err != nil {
		t.Fatalf("second Upload (should be idempotent): %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("hits = %d, want 2 (both reach server, second one short-circuits)", got)
	}
}

func TestDarUploader_UploadAllSequential(t *testing.T) {
	var (
		mu     sync.Mutex
		bodies [][]byte
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	u, err := NewDarUploader(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewDarUploader: %v", err)
	}
	paths := []string{
		writeTempDar(t, "a.dar", []byte("AAA")),
		writeTempDar(t, "b.dar", []byte("BBB")),
		writeTempDar(t, "c.dar", []byte("CCC")),
	}
	if err := u.UploadAll(context.Background(), paths...); err != nil {
		t.Fatalf("UploadAll: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 3 {
		t.Fatalf("got %d uploads, want 3", len(bodies))
	}
	wants := []string{"AAA", "BBB", "CCC"}
	for i, want := range wants {
		if string(bodies[i]) != want {
			t.Errorf("upload %d body = %q, want %q (sequential order)", i, bodies[i], want)
		}
	}
}

func TestDarUploader_UploadAllStopsAtFirstFailure(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `boom`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	u, err := NewDarUploader(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewDarUploader: %v", err)
	}
	paths := []string{
		writeTempDar(t, "a.dar", []byte("A")),
		writeTempDar(t, "b.dar", []byte("B")),
		writeTempDar(t, "c.dar", []byte("C")),
	}
	err = u.UploadAll(context.Background(), paths...)
	if err == nil {
		t.Fatal("expected error from second upload")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not include status 500", err.Error())
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("hits = %d, want 2 (third upload must not run)", got)
	}
}

func TestDarUploader_PropagatesNon2xxNotKnownPackage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"cause":"INVALID_DAR"}`)
	}))
	t.Cleanup(server.Close)

	u, err := NewDarUploader(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewDarUploader: %v", err)
	}
	path := writeTempDar(t, "bad.dar", []byte("x"))
	err = u.Upload(context.Background(), path)
	if err == nil {
		t.Fatal("expected error on plain 400")
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "INVALID_DAR") {
		t.Errorf("error %q missing status or body", err.Error())
	}
}

func TestDarUploader_Propagates5xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `down`)
	}))
	t.Cleanup(server.Close)

	u, err := NewDarUploader(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewDarUploader: %v", err)
	}
	path := writeTempDar(t, "x.dar", []byte("x"))
	err = u.Upload(context.Background(), path)
	if err == nil {
		t.Fatal("expected 503 error")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("error %q missing status", err.Error())
	}
}

func TestDarUploader_PropagatesTokenError(t *testing.T) {
	u, err := NewDarUploader("http://localhost", staticTokenProvider{err: errors.New("nope")})
	if err != nil {
		t.Fatalf("NewDarUploader: %v", err)
	}
	path := writeTempDar(t, "x.dar", []byte("x"))
	err = u.Upload(context.Background(), path)
	if err == nil {
		t.Fatal("expected token error")
	}
	if !strings.Contains(err.Error(), "acquire token") {
		t.Errorf("error %q does not wrap token error", err.Error())
	}
}

func TestDarUploader_MissingFile(t *testing.T) {
	u, err := NewDarUploader("http://localhost", staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewDarUploader: %v", err)
	}
	err = u.Upload(context.Background(), filepath.Join(t.TempDir(), "does-not-exist.dar"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read") {
		t.Errorf("error %q does not mention read", err.Error())
	}
}

func TestDarUploader_RequiresArguments(t *testing.T) {
	if _, err := NewDarUploader("", staticTokenProvider{token: "tok"}); err == nil {
		t.Error("expected error for empty baseURL")
	}
	if _, err := NewDarUploader("http://x", nil); err == nil {
		t.Error("expected error for nil tokens")
	}
	u, _ := NewDarUploader("http://x", staticTokenProvider{token: "tok"})
	if err := u.Upload(context.Background(), ""); err == nil {
		t.Error("expected error for empty path")
	}
}

func TestDarUploader_HonorsContextCancel(t *testing.T) {
	u, err := NewDarUploader("http://localhost", staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewDarUploader: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	path := writeTempDar(t, "x.dar", []byte("x"))
	err = u.Upload(ctx, path)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Upload(cancelled) = %v, want context.Canceled", err)
	}
}
