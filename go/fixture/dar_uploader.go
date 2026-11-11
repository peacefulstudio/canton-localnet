// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// KnownPackageVersionMarker is the error code the Canton JSON Ledger API
// returns (with HTTP 400) when an upload names a package whose name and
// version are already vetted — either the same package or a different package
// id built from the same name and version. DarUploader accepts it only when
// the DAR's own main package is vetted on that synchronizer.
const KnownPackageVersionMarker = "KNOWN_PACKAGE_VERSION"

const (
	darUploadPath   = "/v2/dars"
	packageIDLength = 64
)

var hexRunPattern = regexp.MustCompile(`[0-9a-f]{64,}`)

// DarNotVettedError is returned by DarUploader when a synchronizer answered
// KNOWN_PACKAGE_VERSION but the DAR's main package is not vetted there:
// Canton rejected a different package id that shares the name and version of
// an already vetted package, so the upload changed nothing and the first
// command against the new package would fail with
// NO_SYNCHRONIZER_FOR_SUBMISSION.
type DarNotVettedError struct {
	// Source is the path the DAR was uploaded from.
	Source string
	// MainPackageID is the main package id read from the DAR's manifest.
	MainPackageID string
	// SynchronizerID is the synchronizer that answered KNOWN_PACKAGE_VERSION;
	// empty when the upload named none.
	SynchronizerID string
	// ConflictingPackageIDs are the other package ids Canton's error names,
	// usually the vetted package that blocks this one.
	ConflictingPackageIDs []string
	// ResponseBody is the raw KNOWN_PACKAGE_VERSION response body.
	ResponseBody []byte
}

func (e *DarNotVettedError) Error() string {
	conflicting := "none named"
	if len(e.ConflictingPackageIDs) > 0 {
		conflicting = strings.Join(e.ConflictingPackageIDs, ", ")
	}
	synchronizer := e.SynchronizerID
	if synchronizer == "" {
		synchronizer = "(default)"
	}
	return fmt.Sprintf(
		"dar: %s reported %s on synchronizer %s, but its main package %q is not vetted there; "+
			"Canton usually answers this when another package with the same name and version is already vetted (other ids named: %s); bump the package version; Canton said: %s",
		e.Source, KnownPackageVersionMarker, synchronizer, e.MainPackageID, conflicting, truncateBodyForError(e.ResponseBody))
}

// DarUploader uploads DAR files to a participant via POST /v2/dars.
//
// Behaviour:
//
//   - Single DAR: Upload posts the raw bytes with content-type
//     application/octet-stream and a bearer token, once per connected
//     synchronizer with ?synchronizerId=, so it works on single-sync and
//     multi-sync stacks alike. A stack that reports no connected synchronizer
//     gets one upload without synchronizerId.
//   - Transient 503, 504 or 409 REQUEST_ALREADY_IN_FLIGHT: each post is retried
//     with bounded exponential backoff.
//   - Known version: an HTTP 400 whose body contains "KNOWN_PACKAGE_VERSION"
//     is success only when the DAR's main package (read from its manifest) is
//     vetted on that synchronizer; otherwise Upload returns *DarNotVettedError.
//   - Multi-DAR: UploadAll uploads paths sequentially and stops at the first
//     genuine failure.
type DarUploader struct {
	baseURL    string
	tokens     TokenProvider
	httpClient *http.Client
	retry      retryOptions
	admin      *JsonLedgerAdminClient
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

func withDarRetry(retry retryOptions) DarUploaderOption {
	return func(d *DarUploader) {
		d.retry = retry
	}
}

// NewDarUploader constructs a DarUploader. baseURL is the JSON Ledger API
// origin (e.g. "http://localhost:11975"); trailing slashes are tolerated.
func NewDarUploader(baseURL string, tokens TokenProvider, opts ...DarUploaderOption) (*DarUploader, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("dar: baseURL is required")
	}
	if tokens == nil {
		return nil, errors.New("dar: TokenProvider is required")
	}
	d := &DarUploader{
		baseURL:    normalizeBaseURL(baseURL),
		tokens:     tokens,
		httpClient: newDefaultHTTPClient(),
		retry:      defaultRetryOptions(),
	}
	for _, opt := range opts {
		opt(d)
	}
	if err := d.retry.validate(); err != nil {
		return nil, fmt.Errorf("dar: %w", err)
	}
	admin, err := NewJsonLedgerAdminClient(d.baseURL, d.tokens, WithHTTPClient(d.httpClient))
	if err != nil {
		return nil, err
	}
	d.admin = admin
	return d, nil
}

// Upload uploads a single DAR file. It reads the file from disk and posts the
// bytes to /v2/dars on every connected synchronizer. It returns nil on
// success, including a KNOWN_PACKAGE_VERSION answer for an already vetted
// main package, and *DarNotVettedError when that answer hides a different,
// unvetted package.
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
	targets, err := d.resolveTargets(ctx)
	if err != nil {
		return err
	}
	mainID := ""
	for _, synchronizerID := range targets {
		err := d.postWithRetry(ctx, path, synchronizerID, data)
		if err == nil {
			continue
		}
		var statusErr *httpStatusError
		if !errors.As(err, &statusErr) || !isKnownPackageVersion(statusErr) {
			return err
		}
		if mainID == "" {
			if mainID, err = mainPackageID(data); err != nil {
				return fmt.Errorf("dar: %s reported %s and its manifest is unreadable: %w", path, KnownPackageVersionMarker, err)
			}
		}
		vetted, err := d.admin.isPackageVetted(ctx, mainID, synchronizerID)
		if err != nil {
			return err
		}
		if !vetted {
			return &DarNotVettedError{
				Source:                path,
				MainPackageID:         mainID,
				SynchronizerID:        synchronizerID,
				ConflictingPackageIDs: conflictingPackageIDs(statusErr.body, mainID),
				ResponseBody:          statusErr.body,
			}
		}
	}
	return nil
}

func (d *DarUploader) resolveTargets(ctx context.Context) ([]string, error) {
	synchronizers, err := d.admin.listConnectedSynchronizers(ctx, "/v2/state/connected-synchronizers")
	if err != nil {
		return nil, err
	}
	if len(synchronizers) == 0 {
		return []string{""}, nil
	}
	targets := make([]string, 0, len(synchronizers))
	for _, s := range synchronizers {
		targets = append(targets, s.ID)
	}
	return targets, nil
}

func (d *DarUploader) postWithRetry(ctx context.Context, source, synchronizerID string, data []byte) error {
	uploadPath := darUploadPath
	if synchronizerID != "" {
		uploadPath += "?synchronizerId=" + url.QueryEscape(synchronizerID)
	}
	for attempt := 1; ; attempt++ {
		_, err := doRequest(ctx, d.httpClient, d.tokens, d.baseURL, httpRequest{
			method:      http.MethodPost,
			path:        uploadPath,
			body:        data,
			contentType: "application/octet-stream",
			errPrefix:   "dar",
			pathLabel:   fmt.Sprintf("%s (%s)", uploadPath, source),
		})
		if err == nil {
			return nil
		}
		var statusErr *httpStatusError
		if ctx.Err() != nil || !errors.As(err, &statusErr) || !isTransientStatus(statusErr) || attempt >= d.retry.MaxAttempts {
			return err
		}
		if sleepErr := d.retry.Sleep(ctx, d.retry.delayForAttempt(attempt)); sleepErr != nil {
			return sleepErr
		}
	}
}

func isKnownPackageVersion(e *httpStatusError) bool {
	return e.statusCode == http.StatusBadRequest && bytes.Contains(e.body, []byte(KnownPackageVersionMarker))
}

func conflictingPackageIDs(knownPackageVersionBody []byte, mainID string) []string {
	var decoded struct {
		Cause string `json:"cause"`
	}
	if err := json.Unmarshal(knownPackageVersionBody, &decoded); err != nil {
		return nil
	}
	var ids []string
	seen := map[string]bool{mainID: true}
	for _, id := range hexRunPattern.FindAllString(decoded.Cause, -1) {
		if len(id) == packageIDLength && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}
