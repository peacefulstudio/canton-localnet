// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture_test

import (
	"bytes"
	"context"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"net"
	"net/url"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/peacefulstudio/canton-localnet/go/fixture"
)

const (
	integrationGuidePath = "../../docs/public/integration-testing.md"
	guideSampleFunction  = "TestLedgerRoundTrip"
	sampleFunction       = "ledgerRoundTripSample"
)

func ledgerRoundTripSample(t *testing.T) {
	ctx := context.Background()

	f, err := fixture.New(fixture.Config{Role: fixture.RoleAValidator1})
	if err != nil {
		t.Fatal(err)
	}

	if err := f.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.Teardown(context.Background()); err != nil {
			t.Errorf("teardown: %v", err)
		}
	})

	u, _ := url.Parse(f.Endpoints().JSONLedgerAPIURL)
	if conn, err := net.DialTimeout("tcp", u.Host, 2*time.Second); err != nil {
		t.Skipf("LocalNet %s unreachable; bring it up with `canton-localnet up`", u.Host)
	} else {
		_ = conn.Close()
	}

	if err := f.UploadDar(ctx, "testdata/my-workflow.dar"); err != nil {
		t.Fatal(err)
	}

	party, err := f.AllocateParty(ctx, "globex")
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.CreateUser(ctx, fixture.UserOptions{
		UserID:       "globex-app",
		PrimaryParty: party,
		ActAs:        []string{party},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationGuideSample_SkipsInsteadOfPanickingWhenNothingIsUp(t *testing.T) {
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL", "http://127.0.0.1:1")
	skipped := false

	t.Run("sample", func(t *testing.T) {
		defer func() { skipped = t.Skipped() }()
		ledgerRoundTripSample(t)
	})

	if !skipped {
		t.Error("sample did not skip with nothing listening, want the availability gate to skip it")
	}
}

func TestIntegrationGuideSample_MatchesTheCompiledSample(t *testing.T) {
	guide, err := os.ReadFile(integrationGuidePath)
	if err != nil {
		t.Fatalf("read guide: %v", err)
	}
	block := regexp.MustCompile("(?s)```go\n(//go:build integration.*?)```").FindSubmatch(guide)
	if block == nil {
		t.Fatal("no Go fixture sample with //go:build integration found in the guide")
	}

	fromGuide := functionBody(t, "guide", block[1], guideSampleFunction)
	compiled, err := os.ReadFile("docs_sample_test.go")
	if err != nil {
		t.Fatalf("read compiled sample: %v", err)
	}
	fromTest := functionBody(t, "docs_sample_test.go", compiled, sampleFunction)

	if fromGuide != fromTest {
		t.Errorf("guide sample drifted from the compiled sample.\nguide:\n%s\ncompiled:\n%s", fromGuide, fromTest)
	}
}

func functionBody(t *testing.T, name string, source []byte, function string) string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, source, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == function {
			var out bytes.Buffer
			if err := format.Node(&out, fset, fn.Body); err != nil {
				t.Fatalf("format %s in %s: %v", function, name, err)
			}
			return out.String()
		}
	}
	t.Fatalf("function %s not found in %s", function, name)
	return ""
}
