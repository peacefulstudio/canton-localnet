// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	conflictingPackageID  = "07cbf58f65bbb4ec48c4ae4ca6b6c1e0d66e2c9edb8e6a2bb08e0a7f8b03a111"
	knownPackageVersion   = `{"code":"KNOWN_PACKAGE_VERSION","cause":"Tried to vet two packages with the same name and version: ` + conflictingPackageID + ` (canton-localnet-noop v0.0.1) and ` + noopMainPackageID + ` (canton-localnet-noop v0.0.1)."}`
	appSynchronizersRaw   = `{"connectedSynchronizers":[{"synchronizerAlias":"global","synchronizerId":"global::122a"},{"synchronizerAlias":"app-synchronizer","synchronizerId":"app::122b"}]}`
	noSynchronizersRaw    = `{"connectedSynchronizers":[]}`
	darStubParticipantRaw = `{"participantId":"p1::122a"}`
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

type recordedUpload struct {
	method         string
	path           string
	synchronizerID string
	auth           string
	contentType    string
	accept         string
	body           []byte
}

type darStub struct {
	server *httptest.Server

	mu       sync.Mutex
	uploads  []recordedUpload
	vettings []map[string]any
}

func (s *darStub) uploadCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.uploads)
}

func (s *darStub) uploadedSynchronizers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.uploads))
	for _, u := range s.uploads {
		ids = append(ids, u.synchronizerID)
	}
	return ids
}

type darStubBehaviour struct {
	synchronizers string
	upload        func(attempt int, synchronizerID string, w http.ResponseWriter)
	vettedOn      map[string]bool
}

func newDarStub(t *testing.T, behaviour darStubBehaviour) *darStub {
	t.Helper()
	if behaviour.synchronizers == "" {
		behaviour.synchronizers = defaultConnectedSyncsRaw
	}
	if behaviour.upload == nil {
		behaviour.upload = func(int, string, http.ResponseWriter) {}
	}
	stub := &darStub{}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/state/connected-synchronizers":
			_, _ = io.WriteString(w, behaviour.synchronizers)
		case r.Method == http.MethodGet && r.URL.Path == "/v2/parties/participant-id":
			_, _ = io.WriteString(w, darStubParticipantRaw)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/package-vetting/list":
			raw, _ := io.ReadAll(r.Body)
			var decoded struct {
				TopologyStateFilter struct {
					SynchronizerIDs []string `json:"synchronizerIds"`
				} `json:"topologyStateFilter"`
				PackageMetadataFilter struct {
					PackageIDs []string `json:"packageIds"`
				} `json:"packageMetadataFilter"`
			}
			_ = json.Unmarshal(raw, &decoded)
			var generic map[string]any
			_ = json.Unmarshal(raw, &generic)
			stub.mu.Lock()
			stub.vettings = append(stub.vettings, generic)
			stub.mu.Unlock()
			synchronizerID := ""
			if len(decoded.TopologyStateFilter.SynchronizerIDs) > 0 {
				synchronizerID = decoded.TopologyStateFilter.SynchronizerIDs[0]
			}
			if behaviour.vettedOn[synchronizerID] {
				_, _ = io.WriteString(w, `{"vettedPackages":[{"packages":[{"packageId":"`+decoded.PackageMetadataFilter.PackageIDs[0]+`"}],"participantId":"p1::122a","synchronizerId":"`+synchronizerID+`"}]}`)
				return
			}
			_, _ = io.WriteString(w, `{"vettedPackages":[]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/dars":
			body, _ := io.ReadAll(r.Body)
			stub.mu.Lock()
			stub.uploads = append(stub.uploads, recordedUpload{
				method:         r.Method,
				path:           r.URL.Path,
				synchronizerID: r.URL.Query().Get("synchronizerId"),
				auth:           r.Header.Get("Authorization"),
				contentType:    r.Header.Get("Content-Type"),
				accept:         r.Header.Get("Accept"),
				body:           body,
			})
			attempt := len(stub.uploads)
			stub.mu.Unlock()
			behaviour.upload(attempt, r.URL.Query().Get("synchronizerId"), w)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func newDarUploaderFor(t *testing.T, stub *darStub, opts ...DarUploaderOption) *DarUploader {
	t.Helper()
	u, err := NewDarUploader(stub.server.URL, staticTokenProvider{token: "tok-dar"}, opts...)
	if err != nil {
		t.Fatalf("NewDarUploader: %v", err)
	}
	return u
}

func answerKnownPackageVersion(_ int, _ string, w http.ResponseWriter) {
	w.WriteHeader(http.StatusBadRequest)
	_, _ = io.WriteString(w, knownPackageVersion)
}

func TestDarUploader_RequestShape(t *testing.T) {
	payload := noopDar(t)
	stub := newDarStub(t, darStubBehaviour{})
	u := newDarUploaderFor(t, stub)

	if err := u.Upload(context.Background(), writeTempDar(t, "tiny.dar", payload)); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if len(stub.uploads) != 1 {
		t.Fatalf("got %d uploads, want 1", len(stub.uploads))
	}
	got := stub.uploads[0]
	if got.method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.method)
	}
	if got.path != "/v2/dars" {
		t.Errorf("path = %q, want /v2/dars", got.path)
	}
	if got.synchronizerID != "global::122a" {
		t.Errorf("synchronizerId = %q, want global::122a", got.synchronizerID)
	}
	if got.auth != "Bearer tok-dar" {
		t.Errorf("auth = %q", got.auth)
	}
	if got.contentType != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want application/octet-stream", got.contentType)
	}
	if got.accept != "application/json" {
		t.Errorf("Accept = %q", got.accept)
	}
	if !bytes.Equal(got.body, payload) {
		t.Errorf("body differs from the DAR payload (%d bytes vs %d)", len(got.body), len(payload))
	}
}

func TestDarUploader_UploadsOncePerConnectedSynchronizerWithItsOwnSynchronizerID(t *testing.T) {
	stub := newDarStub(t, darStubBehaviour{synchronizers: appSynchronizersRaw})
	u := newDarUploaderFor(t, stub)

	if err := u.Upload(context.Background(), writeTempDar(t, "a.dar", noopDar(t))); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	got := stub.uploadedSynchronizers()
	if len(got) != 2 || got[0] != "global::122a" || got[1] != "app::122b" {
		t.Errorf("uploaded synchronizers = %v, want [global::122a app::122b]", got)
	}
}

func TestDarUploader_UploadsOnceWithoutASynchronizerIDWhenNoneIsConnected(t *testing.T) {
	stub := newDarStub(t, darStubBehaviour{synchronizers: noSynchronizersRaw})
	u := newDarUploaderFor(t, stub)

	if err := u.Upload(context.Background(), writeTempDar(t, "a.dar", noopDar(t))); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	got := stub.uploadedSynchronizers()
	if len(got) != 1 || got[0] != "" {
		t.Errorf("uploaded synchronizers = %q, want a single upload without synchronizerId", got)
	}
}

func TestDarUploader_KnownPackageVersionWithTheMainPackageVettedIsSuccess(t *testing.T) {
	stub := newDarStub(t, darStubBehaviour{
		upload:   answerKnownPackageVersion,
		vettedOn: map[string]bool{"global::122a": true},
	})
	u := newDarUploaderFor(t, stub)

	if err := u.Upload(context.Background(), writeTempDar(t, "dup.dar", noopDar(t))); err != nil {
		t.Fatalf("expected success when the main package is vetted, got: %v", err)
	}

	if len(stub.vettings) != 1 {
		t.Fatalf("got %d vetting checks, want 1", len(stub.vettings))
	}
	want := `{"packageMetadataFilter":{"packageIds":["` + noopMainPackageID + `"]},"topologyStateFilter":{"participantIds":["p1::122a"],"synchronizerIds":["global::122a"]}}`
	got, _ := json.Marshal(stub.vettings[0])
	if string(got) != want {
		t.Errorf("vetting request = %s, want %s", got, want)
	}
}

func TestDarUploader_KnownPackageVersionWithTheMainPackageNotVettedFailsNamingTheIDs(t *testing.T) {
	stub := newDarStub(t, darStubBehaviour{upload: answerKnownPackageVersion})
	u := newDarUploaderFor(t, stub)

	err := u.Upload(context.Background(), writeTempDar(t, "other.dar", noopDar(t)))

	var notVetted *DarNotVettedError
	if !errors.As(err, &notVetted) {
		t.Fatalf("error = %v, want *DarNotVettedError", err)
	}
	if notVetted.MainPackageID != noopMainPackageID {
		t.Errorf("MainPackageID = %q, want %q", notVetted.MainPackageID, noopMainPackageID)
	}
	if notVetted.SynchronizerID != "global::122a" {
		t.Errorf("SynchronizerID = %q, want global::122a", notVetted.SynchronizerID)
	}
	if len(notVetted.ConflictingPackageIDs) != 1 || notVetted.ConflictingPackageIDs[0] != conflictingPackageID {
		t.Errorf("ConflictingPackageIDs = %v, want [%s]", notVetted.ConflictingPackageIDs, conflictingPackageID)
	}
	for _, id := range []string{noopMainPackageID, conflictingPackageID} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("error %q does not name package id %s", err.Error(), id)
		}
	}
}

func TestDarUploader_FailsWhenTheMainPackageIsNotVettedOnALaterSynchronizer(t *testing.T) {
	stub := newDarStub(t, darStubBehaviour{
		synchronizers: appSynchronizersRaw,
		upload:        answerKnownPackageVersion,
		vettedOn:      map[string]bool{"global::122a": true},
	})
	u := newDarUploaderFor(t, stub)

	err := u.Upload(context.Background(), writeTempDar(t, "other.dar", noopDar(t)))

	var notVetted *DarNotVettedError
	if !errors.As(err, &notVetted) {
		t.Fatalf("error = %v, want *DarNotVettedError", err)
	}
	if notVetted.SynchronizerID != "app::122b" {
		t.Errorf("SynchronizerID = %q, want app::122b", notVetted.SynchronizerID)
	}
}

func TestDarUploader_KnownPackageVersionOnAPayloadWithoutAManifestFails(t *testing.T) {
	stub := newDarStub(t, darStubBehaviour{upload: answerKnownPackageVersion})
	u := newDarUploaderFor(t, stub)

	err := u.Upload(context.Background(), writeTempDar(t, "junk.dar", []byte("not a dar")))
	if err == nil || !strings.Contains(err.Error(), "manifest") {
		t.Fatalf("error = %v, want one naming the unreadable manifest", err)
	}
}

func TestDarUploader_IdenticalRepeatedUploadIsSuccess(t *testing.T) {
	stub := newDarStub(t, darStubBehaviour{})
	u := newDarUploaderFor(t, stub)
	path := writeTempDar(t, "same.dar", noopDar(t))
	ctx := context.Background()

	if err := u.Upload(ctx, path); err != nil {
		t.Fatalf("first Upload: %v", err)
	}
	if err := u.Upload(ctx, path); err != nil {
		t.Fatalf("second Upload: %v", err)
	}
	if got := stub.uploadCount(); got != 2 {
		t.Errorf("uploads = %d, want 2", got)
	}
}

func TestDarUploader_RetriesATransient503ThenSucceeds(t *testing.T) {
	stub := newDarStub(t, darStubBehaviour{
		upload: func(attempt int, _ string, w http.ResponseWriter) {
			if attempt < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, `warming up`)
			}
		},
	})
	u := newDarUploaderFor(t, stub, withDarRetry(noSleepRetry(6, nil)))

	if err := u.Upload(context.Background(), writeTempDar(t, "a.dar", noopDar(t))); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if got := stub.uploadCount(); got != 3 {
		t.Errorf("uploads = %d, want 3 (two 503s then success)", got)
	}
}

func TestDarUploader_Propagates503WhenEveryAttemptFails(t *testing.T) {
	stub := newDarStub(t, darStubBehaviour{
		upload: func(_ int, _ string, w http.ResponseWriter) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `down`)
		},
	})
	u := newDarUploaderFor(t, stub, withDarRetry(noSleepRetry(3, nil)))

	err := u.Upload(context.Background(), writeTempDar(t, "x.dar", noopDar(t)))
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("error = %v, want one naming 503", err)
	}
	if got := stub.uploadCount(); got != 3 {
		t.Errorf("uploads = %d, want 3 attempts", got)
	}
}

func TestDarUploader_UploadAllSequential(t *testing.T) {
	stub := newDarStub(t, darStubBehaviour{})
	u := newDarUploaderFor(t, stub)
	paths := []string{
		writeTempDar(t, "a.dar", []byte("AAA")),
		writeTempDar(t, "b.dar", []byte("BBB")),
		writeTempDar(t, "c.dar", []byte("CCC")),
	}

	if err := u.UploadAll(context.Background(), paths...); err != nil {
		t.Fatalf("UploadAll: %v", err)
	}

	if len(stub.uploads) != 3 {
		t.Fatalf("got %d uploads, want 3", len(stub.uploads))
	}
	for i, want := range []string{"AAA", "BBB", "CCC"} {
		if string(stub.uploads[i].body) != want {
			t.Errorf("upload %d body = %q, want %q (sequential order)", i, stub.uploads[i].body, want)
		}
	}
}

func TestDarUploader_UploadAllStopsAtFirstFailure(t *testing.T) {
	stub := newDarStub(t, darStubBehaviour{
		upload: func(attempt int, _ string, w http.ResponseWriter) {
			if attempt == 2 {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, `boom`)
			}
		},
	})
	u := newDarUploaderFor(t, stub)
	paths := []string{
		writeTempDar(t, "a.dar", []byte("A")),
		writeTempDar(t, "b.dar", []byte("B")),
		writeTempDar(t, "c.dar", []byte("C")),
	}

	err := u.UploadAll(context.Background(), paths...)
	if err == nil {
		t.Fatal("expected error from second upload")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not include status 500", err.Error())
	}
	if got := stub.uploadCount(); got != 2 {
		t.Errorf("uploads = %d, want 2 (third upload must not run)", got)
	}
}

func TestDarUploader_PropagatesNon2xxNotKnownPackage(t *testing.T) {
	stub := newDarStub(t, darStubBehaviour{
		upload: func(_ int, _ string, w http.ResponseWriter) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"cause":"INVALID_DAR"}`)
		},
	})
	u := newDarUploaderFor(t, stub)

	err := u.Upload(context.Background(), writeTempDar(t, "bad.dar", []byte("x")))
	if err == nil {
		t.Fatal("expected error on plain 400")
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "INVALID_DAR") {
		t.Errorf("error %q missing status or body", err.Error())
	}
}

func TestDarUploader_PropagatesTokenError(t *testing.T) {
	u, err := NewDarUploader("http://localhost", staticTokenProvider{err: errors.New("nope")})
	if err != nil {
		t.Fatalf("NewDarUploader: %v", err)
	}
	err = u.Upload(context.Background(), writeTempDar(t, "x.dar", []byte("x")))
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

func TestDarUploader_RejectsAnInvalidRetryPolicy(t *testing.T) {
	_, err := NewDarUploader("http://x", staticTokenProvider{token: "tok"}, withDarRetry(retryOptions{}))
	if err == nil {
		t.Fatal("expected error for a zero retry policy")
	}
}

func TestDarUploader_HonorsContextCancel(t *testing.T) {
	u, err := NewDarUploader("http://localhost", staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewDarUploader: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = u.Upload(ctx, writeTempDar(t, "x.dar", []byte("x")))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Upload(cancelled) = %v, want context.Canceled", err)
	}
}

func TestDarUploader_RetriesEveryTransientStatusThenSucceeds(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"504", http.StatusGatewayTimeout, ""},
		{"409 REQUEST_ALREADY_IN_FLIGHT", http.StatusConflict, requestAlreadyInFlight},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newDarStub(t, darStubBehaviour{
				upload: func(attempt int, _ string, w http.ResponseWriter) {
					if attempt < 3 {
						w.WriteHeader(tc.status)
						_, _ = io.WriteString(w, tc.body)
					}
				},
			})
			u := newDarUploaderFor(t, stub, withDarRetry(noSleepRetry(6, nil)))

			if err := u.Upload(context.Background(), writeTempDar(t, "a.dar", noopDar(t))); err != nil {
				t.Fatalf("Upload: %v", err)
			}
			if got := stub.uploadCount(); got != 3 {
				t.Errorf("uploads = %d, want 3 (two %s answers then success)", got, tc.name)
			}
		})
	}
}

func TestDarUploader_DoesNotRetryAPlain409(t *testing.T) {
	stub := newDarStub(t, darStubBehaviour{
		upload: func(_ int, _ string, w http.ResponseWriter) {
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"code":"PARTY_ALREADY_EXISTS"}`)
		},
	})
	u := newDarUploaderFor(t, stub, withDarRetry(noSleepRetry(6, nil)))

	if err := u.Upload(context.Background(), writeTempDar(t, "a.dar", noopDar(t))); err == nil {
		t.Fatal("Upload succeeded, want the 409 surfaced")
	}
	if got := stub.uploadCount(); got != 1 {
		t.Errorf("uploads = %d, want 1 (a 409 without REQUEST_ALREADY_IN_FLIGHT is not retried)", got)
	}
}

func TestDarUploader_AcquiresATokenForEachAttempt(t *testing.T) {
	stub := newDarStub(t, darStubBehaviour{
		upload: func(attempt int, _ string, w http.ResponseWriter) {
			if attempt < 2 {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
		},
	})
	u, err := NewDarUploader(stub.server.URL, &countingTokenProvider{}, withDarRetry(noSleepRetry(6, nil)))
	if err != nil {
		t.Fatalf("NewDarUploader: %v", err)
	}

	if err := u.Upload(context.Background(), writeTempDar(t, "a.dar", noopDar(t))); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if len(stub.uploads) != 2 {
		t.Fatalf("uploads = %d, want 2", len(stub.uploads))
	}
	if stub.uploads[0].auth == stub.uploads[1].auth {
		t.Errorf("both attempts sent %q; each attempt must acquire its own token", stub.uploads[0].auth)
	}
}
