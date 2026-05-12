// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// KnownPackageVersionMarker is the error code the Canton JSON Ledger API
// returns (with HTTP 400) when an upload targets a package that is already
// present on the ledger — possibly with a different hash from a non-
// deterministic rebuild. DarUploader treats this as success so that
// repeated uploads of the same DAR are idempotent.
const KnownPackageVersionMarker = "KNOWN_PACKAGE_VERSION"

// DarUploader uploads DAR files to a participant via POST /v2/packages.
//
// Behaviour:
//
//   - Single DAR: Upload posts the raw bytes with content-type
//     application/octet-stream and a bearer token.
//   - Idempotency: an HTTP 400 response whose body contains
//     "KNOWN_PACKAGE_VERSION" is treated as success — the package is already
//     on the ledger (possibly with a different hash) and a retry is safe.
//   - Multi-DAR: UploadAll uploads paths sequentially and stops at the first
//     genuine failure.
type DarUploader struct {
	baseURL    string
	tokens     TokenProvider
	httpClient *http.Client
}

// DarUploaderOption mutates a DarUploader at construction.
type DarUploaderOption func(*DarUploader)

// WithDarHTTPClient overrides the default HTTP client used by DarUploader.
func WithDarHTTPClient(c *http.Client) DarUploaderOption {
	return func(d *DarUploader) {
		if c != nil {
			d.httpClient = c
		}
	}
}

// NewDarUploader constructs a DarUploader. baseURL is the JSON Ledger API
// origin (e.g. "http://localhost:3975"); trailing slashes are tolerated.
func NewDarUploader(baseURL string, tokens TokenProvider, opts ...DarUploaderOption) (*DarUploader, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("dar: baseURL is required")
	}
	if tokens == nil {
		return nil, errors.New("dar: TokenProvider is required")
	}
	d := &DarUploader{
		baseURL:    strings.TrimRight(baseURL, "/"),
		tokens:     tokens,
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d, nil
}

// Upload uploads a single DAR file. It reads the file from disk, posts the
// bytes to /v2/packages, and returns nil on success. The KnownPackageVersion
// path is reported as success.
func (d *DarUploader) Upload(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		return errors.New("dar: path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("dar: read %s: %w", path, err)
	}
	return d.uploadBytes(ctx, path, data)
}

// UploadAll uploads the supplied DAR paths sequentially. The first failure
// stops the run and is returned; successfully-uploaded files prior to the
// failure are not rolled back.
func (d *DarUploader) UploadAll(ctx context.Context, paths ...string) error {
	for _, p := range paths {
		if err := d.Upload(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

func (d *DarUploader) uploadBytes(ctx context.Context, path string, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/v2/packages", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("dar: build POST /v2/packages: %w", err)
	}
	token, err := d.tokens.Token(ctx)
	if err != nil {
		return fmt.Errorf("dar: acquire token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Accept", "application/json")
	req.ContentLength = int64(len(data))

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("dar: POST /v2/packages (%s): %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("dar: read response for %s: %w", path, err)
	}
	if resp.StatusCode == http.StatusBadRequest && bytes.Contains(body, []byte(KnownPackageVersionMarker)) {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("dar: POST /v2/packages (%s) returned HTTP %d: %s", path, resp.StatusCode, truncateBodyForError(body))
	}
	return nil
}
