// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeBaseURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty input is unchanged", in: "", want: ""},
		{name: "no trailing slash is unchanged", in: "http://localhost:3975", want: "http://localhost:3975"},
		{name: "single trailing slash is stripped", in: "http://localhost:3975/", want: "http://localhost:3975"},
		{name: "multiple trailing slashes are stripped", in: "http://localhost:3975///", want: "http://localhost:3975"},
		{name: "trailing slash preserves path", in: "http://localhost:3975/api/", want: "http://localhost:3975/api"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeBaseURL(tc.in); got != tc.want {
				t.Errorf("normalizeBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestDoRequest_SuccessReturnsBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/v2/things" {
			t.Errorf("path = %q, want /v2/things", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("auth = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("accept = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content-type = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"k":"v"}` {
			t.Errorf("body = %q", body)
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	got, err := doRequest(context.Background(), http.DefaultClient, staticTokenProvider{token: "tok"}, server.URL, httpRequest{
		method:      http.MethodPost,
		path:        "/v2/things",
		body:        []byte(`{"k":"v"}`),
		contentType: "application/json",
		errPrefix:   "thing",
	})
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if string(got) != `{"ok":true}` {
		t.Errorf("response = %q", got)
	}
}

func TestDoRequest_OmitsContentTypeWhenBodyIsNil(t *testing.T) {
	var contentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	if _, err := doRequest(context.Background(), http.DefaultClient, staticTokenProvider{token: "tok"}, server.URL, httpRequest{
		method:    http.MethodGet,
		path:      "/v2/anything",
		errPrefix: "thing",
	}); err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if contentType != "" {
		t.Errorf("Content-Type sent for body-less request = %q, want empty", contentType)
	}
}

func TestDoRequest_WrapsBuildError(t *testing.T) {
	_, err := doRequest(context.Background(), http.DefaultClient, staticTokenProvider{token: "tok"}, "http://[::1]:0", httpRequest{
		method:    "BAD METHOD",
		path:      "/x",
		errPrefix: "thing",
	})
	if err == nil {
		t.Fatal("expected error for invalid method, got nil")
	}
	if !strings.Contains(err.Error(), "thing: build BAD METHOD /x") {
		t.Errorf("error = %v, want prefix \"thing: build BAD METHOD /x\"", err)
	}
}

func TestDoRequest_WrapsTokenError(t *testing.T) {
	_, err := doRequest(context.Background(), http.DefaultClient, staticTokenProvider{err: errors.New("token boom")}, "http://localhost", httpRequest{
		method:    http.MethodGet,
		path:      "/v2/things",
		errPrefix: "thing",
	})
	if err == nil {
		t.Fatal("expected error for token failure, got nil")
	}
	if !strings.Contains(err.Error(), "thing: acquire token") {
		t.Errorf("error = %v, want prefix \"thing: acquire token\"", err)
	}
	if !strings.Contains(err.Error(), "token boom") {
		t.Errorf("error = %v, want wrapped \"token boom\"", err)
	}
}

func TestDoRequest_WrapsNetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	_, err := doRequest(context.Background(), http.DefaultClient, staticTokenProvider{token: "tok"}, server.URL, httpRequest{
		method:    http.MethodGet,
		path:      "/v2/things",
		errPrefix: "thing",
	})
	if err == nil {
		t.Fatal("expected error for closed server, got nil")
	}
	if !strings.Contains(err.Error(), "thing: GET /v2/things") {
		t.Errorf("error = %v, want prefix \"thing: GET /v2/things\"", err)
	}
}

func TestDoRequest_WrapsNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "kaboom")
	}))
	t.Cleanup(server.Close)

	_, err := doRequest(context.Background(), http.DefaultClient, staticTokenProvider{token: "tok"}, server.URL, httpRequest{
		method:    http.MethodGet,
		path:      "/v2/things",
		errPrefix: "thing",
	})
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
	if !strings.Contains(err.Error(), "thing: GET /v2/things returned HTTP 500") {
		t.Errorf("error = %v, want \"thing: GET /v2/things returned HTTP 500\"", err)
	}
	if !strings.Contains(err.Error(), "kaboom") {
		t.Errorf("error = %v, want body \"kaboom\"", err)
	}
}

func TestDoRequest_Treat400AsSuccessReturnsBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "already-known")
	}))
	t.Cleanup(server.Close)

	got, err := doRequest(context.Background(), http.DefaultClient, staticTokenProvider{token: "tok"}, server.URL, httpRequest{
		method:            http.MethodGet,
		path:              "/v2/things",
		errPrefix:         "thing",
		treat400AsSuccess: func(b []byte) bool { return strings.Contains(string(b), "already-known") },
	})
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if string(got) != "already-known" {
		t.Errorf("response = %q", got)
	}
}

func TestDoRequest_Treat400AsSuccessOptOutPropagatesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "INVALID_DAR: nope")
	}))
	t.Cleanup(server.Close)

	_, err := doRequest(context.Background(), http.DefaultClient, staticTokenProvider{token: "tok"}, server.URL, httpRequest{
		method:            http.MethodGet,
		path:              "/v2/things",
		errPrefix:         "thing",
		treat400AsSuccess: func(b []byte) bool { return false },
	})
	if err == nil {
		t.Fatal("expected error when treat400AsSuccess returns false, got nil")
	}
	if !strings.Contains(err.Error(), "returned HTTP 400") {
		t.Errorf("error = %v, want \"returned HTTP 400\"", err)
	}
}

func TestDoRequest_PathLabelOverridesPathInErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	_, err := doRequest(context.Background(), http.DefaultClient, staticTokenProvider{token: "tok"}, server.URL, httpRequest{
		method:    http.MethodPost,
		path:      "/v2/packages",
		pathLabel: "/v2/packages (foo.dar)",
		errPrefix: "dar",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "/v2/packages (foo.dar)") {
		t.Errorf("error = %v, want pathLabel surface", err)
	}
	if strings.Contains(err.Error(), "/v2/packages returned") {
		t.Errorf("error = %v, must not surface raw path when pathLabel is set", err)
	}
}

func TestDoRequest_HonorsContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := doRequest(ctx, http.DefaultClient, staticTokenProvider{token: "tok"}, server.URL, httpRequest{
		method:    http.MethodGet,
		path:      "/v2/things",
		errPrefix: "thing",
	})
	if err == nil {
		t.Fatal("expected context-cancel error, got nil")
	}
}
