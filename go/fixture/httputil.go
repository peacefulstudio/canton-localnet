// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// normalizeBaseURL trims trailing slashes so callers can safely concatenate
// path segments that always begin with "/". Multiple trailing slashes are
// stripped; an empty input is returned unchanged.
func normalizeBaseURL(s string) string {
	return strings.TrimRight(s, "/")
}

// httpRequest describes one authenticated request issued by an internal
// HTTP-using component (DarUploader, PartyAllocator, UserBuilder,
// JsonLedgerAdminClient). It is an internal type — fields are populated by
// the call site and consumed by doRequest.
type httpRequest struct {
	// method is the HTTP method, e.g. http.MethodPost.
	method string
	// path is appended to baseURL verbatim, e.g. "/v2/packages".
	path string
	// body is the (already-marshalled) request body, or nil for no body.
	body []byte
	// contentType, when non-empty and body is non-nil, is written to the
	// Content-Type header. Accept is always application/json.
	contentType string
	// errPrefix names the calling component for error messages, e.g.
	// "dar", "party", "user", "admin".
	errPrefix string
	// pathLabel, when non-empty, replaces path in error messages. The DAR
	// uploader uses this to include the source file path in errors —
	// e.g. "/v2/packages (foo.dar)" — without polluting the actual URL.
	pathLabel string
	// treat400AsSuccess, when non-nil, is consulted on a 400 response.
	// If it returns true, doRequest returns (body, nil) so the call site
	// can treat the response as success. Used by DarUploader for the
	// KNOWN_PACKAGE_VERSION idempotency shortcut.
	treat400AsSuccess func(body []byte) bool
}

// doRequest issues req against client with bearer auth from tokens. It
// returns the response body on 2xx (or on 400 if treat400AsSuccess opts
// in). All errors are wrapped with req.errPrefix so callers don't need
// their own wrapping layer.
func doRequest(ctx context.Context, client *http.Client, tokens TokenProvider, baseURL string, req httpRequest) ([]byte, error) {
	var bodyReader io.Reader
	if req.body != nil {
		bodyReader = bytes.NewReader(req.body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.method, baseURL+req.path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("%s: build %s %s: %w", req.errPrefix, req.method, req.errorPath(), err)
	}
	token, err := tokens.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: acquire token: %w", req.errPrefix, err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Accept", "application/json")
	if req.body != nil {
		if req.contentType != "" {
			httpReq.Header.Set("Content-Type", req.contentType)
		}
		httpReq.ContentLength = int64(len(req.body))
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s: %s %s: %w", req.errPrefix, req.method, req.errorPath(), err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: read %s %s response: %w", req.errPrefix, req.method, req.errorPath(), err)
	}
	if resp.StatusCode == http.StatusBadRequest && req.treat400AsSuccess != nil && req.treat400AsSuccess(respBody) {
		return respBody, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: %s %s returned HTTP %d: %s", req.errPrefix, req.method, req.errorPath(), resp.StatusCode, truncateBodyForError(respBody))
	}
	return respBody, nil
}

func (r httpRequest) errorPath() string {
	if r.pathLabel != "" {
		return r.pathLabel
	}
	return r.path
}
