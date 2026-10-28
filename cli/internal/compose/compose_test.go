// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package compose

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
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

	allFiveSlots := []string{"sv-validator-1", "a-validator-1", "b-validator-1", "c-validator-1", "d-validator-1"}

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
			opts: Options{AuthMode: AuthOAuth2, EnabledSlots: allFiveSlots},
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
			wantProfiles:   []string{"sv-validator-1", "a-validator-1", "b-validator-1", "c-validator-1", "d-validator-1", "keycloak"},
			wantNotProfile: []string{"pqs-a-validator-1", "observability"},
		},
		{
			name: "pqs and obs enabled on linux",
			opts: Options{AuthMode: AuthOAuth2, Pqs: true, Obs: true, HostOS: "linux", EnabledSlots: allFiveSlots},
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
			wantProfiles: []string{"sv-validator-1", "a-validator-1", "b-validator-1", "c-validator-1", "d-validator-1", "keycloak", "pqs-a-validator-1", "pqs-c-validator-1", "observability"},
		},
		{
			name: "obs enabled on darwin selects darwin cadvisor",
			opts: Options{AuthMode: AuthOAuth2, Obs: true, HostOS: "darwin", EnabledSlots: allFiveSlots},
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
			opts: Options{AuthMode: AuthOAuth2, NoResource: true, EnabledSlots: allFiveSlots},
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
			opts: Options{AuthMode: AuthSecret, EnabledSlots: allFiveSlots},
			wantFiles: []string{
				"compose/modules/localnet/compose.yaml",
			},
			wantNotFiles: []string{
				"compose/modules/keycloak/compose.yaml",
				"compose/modules/keycloak/resource-constraints.yaml",
			},
			wantNotProfile: []string{"keycloak"},
		},
		{
			name:           "yaml-driven slot subset disables c and d",
			opts:           Options{AuthMode: AuthOAuth2, EnabledSlots: []string{"sv-validator-1", "a-validator-1", "b-validator-1"}},
			wantProfiles:   []string{"sv-validator-1", "a-validator-1", "b-validator-1"},
			wantNotProfile: []string{"c-validator-1", "d-validator-1"},
		},
		{
			name:           "pqs without c slot does not add pqs-c-validator-1",
			opts:           Options{AuthMode: AuthOAuth2, Pqs: true, EnabledSlots: []string{"sv-validator-1", "a-validator-1", "b-validator-1", "d-validator-1"}},
			wantProfiles:   []string{"pqs-a-validator-1"},
			wantNotProfile: []string{"pqs-c-validator-1", "c-validator-1"},
		},
		{
			name:           "empty EnabledSlots still emits sv",
			opts:           Options{AuthMode: AuthOAuth2, EnabledSlots: nil},
			wantProfiles:   []string{"sv-validator-1"},
			wantNotProfile: []string{"a-validator-1", "b-validator-1", "c-validator-1", "d-validator-1"},
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
	plan, err := Build(Options{RepoRoot: root, AuthMode: AuthOAuth2, EnabledSlots: []string{"sv-validator-1", "a-validator-1"}})
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

func TestBuildAddsCValidator1ProfileWhenEnabled(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	plan, err := Build(Options{
		RepoRoot:     root,
		AuthMode:     AuthOAuth2,
		EnabledSlots: []string{"sv-validator-1", "a-validator-1", "b-validator-1", "c-validator-1", "d-validator-1"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !containsPair(plan.Args, "--profile", "c-validator-1") {
		t.Errorf("expected --profile c-validator-1 when c slot is enabled, full args: %v", plan.Args)
	}
}

func TestBuildOmitsCValidator1ProfileWhenDisabled(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	plan, err := Build(Options{
		RepoRoot:     root,
		AuthMode:     AuthOAuth2,
		EnabledSlots: []string{"sv-validator-1", "a-validator-1", "b-validator-1", "d-validator-1"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if containsPair(plan.Args, "--profile", "c-validator-1") {
		t.Errorf("did not expect --profile c-validator-1 when c slot is absent from EnabledSlots, full args: %v", plan.Args)
	}
}

func TestBuildAddsPqsCValidator1ProfileWhenCEnabledAndPqsOn(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	plan, err := Build(Options{
		RepoRoot:     root,
		AuthMode:     AuthOAuth2,
		Pqs:          true,
		EnabledSlots: []string{"sv-validator-1", "c-validator-1"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !containsPair(plan.Args, "--profile", "pqs-c-validator-1") {
		t.Errorf("expected --profile pqs-c-validator-1 when c is enabled and Pqs is on, full args: %v", plan.Args)
	}
}

func TestBuildTurnsOnThePqsCValidator1OnboardingProfileWhenCEnabledAndPqsOn(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	plan, err := Build(Options{
		RepoRoot:     root,
		AuthMode:     AuthOAuth2,
		Pqs:          true,
		EnabledSlots: []string{"sv-validator-1", "a-validator-1", "c-validator-1"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !slices.Contains(plan.Env, "PQS_C_VALIDATOR_1_PROFILE=on") {
		t.Errorf("expected PQS_C_VALIDATOR_1_PROFILE=on in env, got %v", plan.Env)
	}
}

func TestBuildLeavesThePqsCValidator1OnboardingProfileOffWhenCDisabled(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	plan, err := Build(Options{
		RepoRoot:     root,
		AuthMode:     AuthOAuth2,
		Pqs:          true,
		EnabledSlots: []string{"sv-validator-1", "a-validator-1"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if slices.Contains(plan.Env, "PQS_C_VALIDATOR_1_PROFILE=on") {
		t.Errorf("did not expect PQS_C_VALIDATOR_1_PROFILE=on when c is disabled, got %v", plan.Env)
	}
}

func TestBuildOmitsPqsCValidator1ProfileWhenCDisabled(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	plan, err := Build(Options{
		RepoRoot:     root,
		AuthMode:     AuthOAuth2,
		Pqs:          true,
		EnabledSlots: []string{"sv-validator-1", "a-validator-1"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if containsPair(plan.Args, "--profile", "pqs-c-validator-1") {
		t.Errorf("did not expect --profile pqs-c-validator-1 when c is disabled, full args: %v", plan.Args)
	}
}

func TestBuildTeardownActivatesEveryPqsProfile(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	plan, err := Build(Options{
		RepoRoot:     root,
		AuthMode:     AuthOAuth2,
		Teardown:     true,
		EnabledSlots: []string{"sv-validator-1", "a-validator-1"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, profile := range []string{"pqs-a-validator-1", "pqs-b-validator-1", "pqs-c-validator-1", "pqs-sv-validator-1"} {
		if !containsPair(plan.Args, "--profile", profile) {
			t.Errorf("expected --profile %s on teardown, full args: %v", profile, plan.Args)
		}
	}
	if !containsPair(plan.Args, "-f", filepath.Join(root, "compose/modules/pqs/compose.yaml")) {
		t.Errorf("expected the PQS compose file on teardown, full args: %v", plan.Args)
	}
}

func TestBuildAlwaysEmitsSvProfile(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	plan, err := Build(Options{
		RepoRoot:     root,
		AuthMode:     AuthOAuth2,
		EnabledSlots: []string{"a-validator-1"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !containsPair(plan.Args, "--profile", "sv-validator-1") {
		t.Errorf("expected --profile sv-validator-1 even when caller forgets to include it, full args: %v", plan.Args)
	}
}

func TestBuildAppendsMultiSyncProfileWhenEnabled(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	opts := DefaultOptions(root)
	opts.MultiSync = true
	plan, err := Build(opts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !containsPair(plan.Args, "--profile", "multi-sync") {
		t.Errorf("expected --profile multi-sync in args, full args: %v", plan.Args)
	}
}

func TestBuildOmitsMultiSyncProfileByDefault(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	plan, err := Build(DefaultOptions(root))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if containsPair(plan.Args, "--profile", "multi-sync") {
		t.Errorf("did not expect --profile multi-sync in args, full args: %v", plan.Args)
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

func TestBuildStartsAScribeOnlyForEachEnabledPqsSlot(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		enabledSlots []string
		wantProfiles []string
		wantToggles  []string
	}{
		{
			name:         "a alone",
			enabledSlots: []string{"sv-validator-1", "a-validator-1"},
			wantProfiles: []string{"pqs-a-validator-1"},
			wantToggles:  []string{"PQS_SV_VALIDATOR_1_PROFILE=off", "PQS_A_VALIDATOR_1_PROFILE=on", "PQS_B_VALIDATOR_1_PROFILE=off", "PQS_C_VALIDATOR_1_PROFILE=off"},
		},
		{
			name:         "c alone starts no slot-a Scribe",
			enabledSlots: []string{"sv-validator-1", "c-validator-1"},
			wantProfiles: []string{"pqs-c-validator-1"},
			wantToggles:  []string{"PQS_SV_VALIDATOR_1_PROFILE=off", "PQS_A_VALIDATOR_1_PROFILE=off", "PQS_B_VALIDATOR_1_PROFILE=off", "PQS_C_VALIDATOR_1_PROFILE=on"},
		},
		{
			name:         "b alone",
			enabledSlots: []string{"sv-validator-1", "b-validator-1"},
			wantProfiles: []string{"pqs-b-validator-1"},
			wantToggles:  []string{"PQS_SV_VALIDATOR_1_PROFILE=off", "PQS_A_VALIDATOR_1_PROFILE=off", "PQS_B_VALIDATOR_1_PROFILE=on", "PQS_C_VALIDATOR_1_PROFILE=off"},
		},
		{
			name:         "a and d",
			enabledSlots: []string{"sv-validator-1", "a-validator-1", "d-validator-1"},
			wantProfiles: []string{"pqs-a-validator-1"},
			wantToggles:  []string{"PQS_SV_VALIDATOR_1_PROFILE=off", "PQS_A_VALIDATOR_1_PROFILE=on", "PQS_B_VALIDATOR_1_PROFILE=off", "PQS_C_VALIDATOR_1_PROFILE=off"},
		},
		{
			name:         "all five never starts the sv Scribe",
			enabledSlots: []string{"sv-validator-1", "a-validator-1", "b-validator-1", "c-validator-1", "d-validator-1"},
			wantProfiles: []string{"pqs-a-validator-1", "pqs-b-validator-1", "pqs-c-validator-1"},
			wantToggles:  []string{"PQS_SV_VALIDATOR_1_PROFILE=off", "PQS_A_VALIDATOR_1_PROFILE=on", "PQS_B_VALIDATOR_1_PROFILE=on", "PQS_C_VALIDATOR_1_PROFILE=on"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			plan, err := Build(Options{RepoRoot: newTestRepoRoot(t), AuthMode: AuthOAuth2, Pqs: true, EnabledSlots: tc.enabledSlots})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if got := pqsProfilesIn(plan.Args); !slices.Equal(got, tc.wantProfiles) {
				t.Errorf("PQS profiles: got %v, want %v", got, tc.wantProfiles)
			}
			if got := pqsTogglesIn(plan.Env); !slices.Equal(got, tc.wantToggles) {
				t.Errorf("PQS toggles: got %v, want %v", got, tc.wantToggles)
			}
		})
	}
}

func TestBuildStartsNoScribeWhenNoEnabledSlotRunsOne(t *testing.T) {
	t.Parallel()
	for _, enabled := range [][]string{
		{"sv-validator-1"},
		{"sv-validator-1", "d-validator-1"},
	} {
		plan, err := Build(Options{RepoRoot: newTestRepoRoot(t), AuthMode: AuthOAuth2, Pqs: true, EnabledSlots: enabled})
		if err != nil {
			t.Fatalf("enabled %v: Build: %v", enabled, err)
		}
		if got := pqsProfilesIn(plan.Args); len(got) != 0 {
			t.Errorf("enabled %v: want no PQS profile, got %v", enabled, got)
		}
		for _, toggle := range pqsTogglesIn(plan.Env) {
			if !strings.HasSuffix(toggle, "=off") {
				t.Errorf("enabled %v: want every toggle off, got %q", enabled, toggle)
			}
		}
	}
}

func TestBuildTeardownAcceptsPqsWithNoPqsSlotEnabled(t *testing.T) {
	t.Parallel()
	_, err := Build(Options{RepoRoot: newTestRepoRoot(t), AuthMode: AuthOAuth2, Pqs: true, Teardown: true, EnabledSlots: []string{"sv-validator-1", "d-validator-1"}})
	if err != nil {
		t.Fatalf("teardown must not reject the PQS topology: %v", err)
	}
}

func pqsProfilesIn(args []string) []string {
	var profiles []string
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--profile" && strings.HasPrefix(args[i+1], "pqs-") {
			profiles = append(profiles, args[i+1])
		}
	}
	return profiles
}

func pqsTogglesIn(env []string) []string {
	var toggles []string
	for _, entry := range env {
		if strings.HasPrefix(entry, "PQS_") && strings.Contains(entry, "_PROFILE=") {
			toggles = append(toggles, entry)
		}
	}
	return toggles
}

func TestPqsProfilesMatchEveryProfileThePqsModuleDefines(t *testing.T) {
	t.Parallel()
	module, err := os.ReadFile(filepath.Join("..", "..", "..", "compose", "modules", "pqs", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var defined []string
	for _, m := range regexp.MustCompile(`(?m)^      - (pqs-[a-z]+-validator-1)$`).FindAllStringSubmatch(string(module), -1) {
		defined = append(defined, m[1])
	}
	want := []string{"pqs-a-validator-1", "pqs-b-validator-1", "pqs-c-validator-1", "pqs-sv-validator-1"}
	if !slices.Equal(defined, want) {
		t.Errorf("compose/modules/pqs/compose.yaml profiles: got %v, want %v", defined, want)
	}
	if got := slices.Sorted(slices.Values(PqsProfiles)); !slices.Equal(got, want) {
		t.Errorf("PqsProfiles: got %v, want %v", got, want)
	}
}
