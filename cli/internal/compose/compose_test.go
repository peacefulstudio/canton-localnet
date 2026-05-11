// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestRepoRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "compose", "modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestBuild(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		opts           Options
		wantFiles      []string
		wantNotFiles   []string
		wantEnvFiles   []string
		wantProfiles   []string
		wantNotProfile []string
	}{
		{
			name: "default oauth2 with resource limits",
			opts: Options{AuthMode: AuthOAuth2},
			wantFiles: []string{
				"compose/modules/localnet/compose.yaml",
				"compose/modules/splice-onboarding/compose.yaml",
				"compose/modules/localnet/resource-constraints.yaml",
				"compose/modules/splice-onboarding/resource-constraints.yaml",
				"compose/modules/keycloak/compose.yaml",
				"compose/modules/keycloak/resource-constraints.yaml",
			},
			wantNotFiles: []string{
				"compose/modules/pqs/compose.yaml",
				"compose/modules/observability/compose.yaml",
			},
			wantEnvFiles: []string{
				"compose/.env.defaults",
				"compose/modules/localnet/compose.env",
				"compose/modules/keycloak/compose.env",
			},
			wantProfiles:   []string{"app-provider", "app-user", "sv", "keycloak"},
			wantNotProfile: []string{"pqs-app-provider", "observability"},
		},
		{
			name: "pqs and obs enabled on linux",
			opts: Options{AuthMode: AuthOAuth2, Pqs: true, Obs: true, HostOS: "linux"},
			wantFiles: []string{
				"compose/modules/pqs/compose.yaml",
				"compose/modules/pqs/resource-constraints.yaml",
				"compose/modules/observability/compose.yaml",
				"compose/modules/observability/observability.yaml",
				"compose/modules/observability/cadvisor-linux.yaml",
				"compose/modules/pqs/observability.yaml",
			},
			wantNotFiles: []string{
				"compose/modules/observability/cadvisor-darwin.yaml",
			},
			wantProfiles: []string{"app-provider", "app-user", "sv", "keycloak", "pqs-app-provider", "observability"},
		},
		{
			name: "obs enabled on darwin selects darwin cadvisor",
			opts: Options{AuthMode: AuthOAuth2, Obs: true, HostOS: "darwin"},
			wantFiles: []string{
				"compose/modules/observability/cadvisor-darwin.yaml",
			},
			wantNotFiles: []string{
				"compose/modules/observability/cadvisor-linux.yaml",
				"compose/modules/pqs/observability.yaml",
			},
		},
		{
			name: "no resource limits",
			opts: Options{AuthMode: AuthOAuth2, NoResource: true},
			wantFiles: []string{
				"compose/modules/localnet/compose.yaml",
				"compose/modules/keycloak/compose.yaml",
			},
			wantNotFiles: []string{
				"compose/modules/localnet/resource-constraints.yaml",
				"compose/modules/keycloak/resource-constraints.yaml",
			},
		},
		{
			name: "secret auth skips keycloak",
			opts: Options{AuthMode: AuthSecret},
			wantFiles: []string{
				"compose/modules/localnet/compose.yaml",
			},
			wantNotFiles: []string{
				"compose/modules/keycloak/compose.yaml",
				"compose/modules/keycloak/resource-constraints.yaml",
			},
			wantNotProfile: []string{"keycloak"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := newTestRepoRoot(t)
			opts := tc.opts
			opts.RepoRoot = root
			plan, err := Build(opts)
			if err != nil {
				t.Fatalf("Build returned error: %v", err)
			}
			if len(plan.Args) == 0 || plan.Args[0] != "compose" {
				t.Errorf("plan must start with 'compose' subcommand, got %v", plan.Args)
			}
			for _, f := range tc.wantFiles {
				if !containsPair(plan.Args, "-f", filepath.Join(root, f)) {
					t.Errorf("expected -f %q in args, full args: %v", f, plan.Args)
				}
			}
			for _, f := range tc.wantNotFiles {
				if containsPair(plan.Args, "-f", filepath.Join(root, f)) {
					t.Errorf("did not expect -f %q in args, full args: %v", f, plan.Args)
				}
			}
			for _, f := range tc.wantEnvFiles {
				if !containsPair(plan.Args, "--env-file", filepath.Join(root, f)) {
					t.Errorf("expected --env-file %q in args, full args: %v", f, plan.Args)
				}
			}
			for _, p := range tc.wantProfiles {
				if !containsPair(plan.Args, "--profile", p) {
					t.Errorf("expected --profile %q in args, full args: %v", p, plan.Args)
				}
			}
			for _, p := range tc.wantNotProfile {
				if containsPair(plan.Args, "--profile", p) {
					t.Errorf("did not expect --profile %q in args, full args: %v", p, plan.Args)
				}
			}
		})
	}
}

func TestBuildResourceConstraintsFollowBase(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	plan, err := Build(Options{RepoRoot: root, AuthMode: AuthOAuth2})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	base := filepath.Join(root, "compose/modules/localnet/compose.yaml")
	constraints := filepath.Join(root, "compose/modules/localnet/resource-constraints.yaml")
	baseIdx := indexOfPair(plan.Args, "-f", base)
	constraintsIdx := indexOfPair(plan.Args, "-f", constraints)
	if baseIdx < 0 || constraintsIdx < 0 {
		t.Fatalf("expected both base and constraints overlays present (base=%d, constraints=%d) in %v", baseIdx, constraintsIdx, plan.Args)
	}
	if constraintsIdx < baseIdx {
		t.Errorf("resource-constraints overlay must come after base compose.yaml (base=%d, constraints=%d)", baseIdx, constraintsIdx)
	}
}

func TestBuildRequiresRepoRoot(t *testing.T) {
	t.Parallel()
	if _, err := Build(Options{}); err == nil {
		t.Fatal("expected error for empty RepoRoot, got nil")
	}
}

func TestBuildRejectsMissingRepoRoot(t *testing.T) {
	t.Parallel()
	_, err := Build(Options{RepoRoot: "/definitely/not/a/real/path-xyz"})
	if err == nil {
		t.Fatal("expected error for non-existent RepoRoot, got nil")
	}
}

func TestBuildRejectsInvalidAuthMode(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	_, err := Build(Options{RepoRoot: root, AuthMode: AuthMode("garbage")})
	if err == nil {
		t.Fatal("expected error for invalid AuthMode, got nil")
	}
}

func TestParseAuthMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    AuthMode
		wantErr bool
	}{
		{in: "", want: AuthOAuth2},
		{in: "oauth2", want: AuthOAuth2},
		{in: "secret", want: AuthSecret},
		{in: "OAuth2", wantErr: true},
		{in: "bogus", wantErr: true},
	}
	for _, tc := range cases {
		got, err := ParseAuthMode(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseAuthMode(%q): expected error, got %q", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseAuthMode(%q): unexpected error %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("ParseAuthMode(%q): want %q, got %q", tc.in, tc.want, got)
		}
	}
}

func TestBuildExposesEnv(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	plan, err := Build(Options{RepoRoot: root, AuthMode: AuthOAuth2})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	wantEnv := map[string]string{
		"MODULES_DIR":  filepath.Join(root, "compose/modules"),
		"LOCALNET_DIR": filepath.Join(root, "compose/modules/localnet"),
		"AUTH_MODE":    "oauth2",
	}
	for key, value := range wantEnv {
		expected := key + "=" + value
		found := false
		for _, e := range plan.Env {
			if e == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected env %q, got %v", expected, plan.Env)
		}
	}
}

func containsPair(args []string, flag, value string) bool {
	return indexOfPair(args, flag, value) >= 0
}

func indexOfPair(args []string, flag, value string) int {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return i
		}
	}
	return -1
}
