// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

// Package yamlconfig parses the preview canton-localnet.yaml consumer
// config, merges it over built-in defaults, and translates the result
// into the env-var pairs the existing compose pipeline already
// consumes. The YAML schema is documented in
// docs/canton-localnet-yaml-schema.md and is marked preview/unstable
// until compose codegen lands (see docs/adr/0001).
package yamlconfig

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileName is the on-disk name the CLI walks up looking for.
const FileName = "canton-localnet.yaml"

// SchemaVersion is the only schema version this loader accepts.
const SchemaVersion = "preview-1"

// SlotSV is the non-toggleable Super Validator slot. Every topology
// must include exactly one SV.
const SlotSV = "sv-validator-1"

// KnownSlots is the fixed-cardinality set of validator slots baked
// into the compose graph. References to slots outside this set are
// rejected as configuration errors.
var KnownSlots = []string{
	SlotSV,
	"a-validator-1",
	"b-validator-1",
	"c-validator-1",
	"d-validator-1",
}

// Config is the in-memory representation of a fully-merged
// canton-localnet.yaml. The zero value is not valid — use Defaults or
// Load to obtain one.
type Config struct {
	SchemaVersion string
	Modules       Modules
	Validators    map[string]Validator
}

// Modules captures the global feature toggles that gate the
// observability and PQS stacks.
type Modules struct {
	Obs bool
	Pqs bool
}

// Validator captures the per-slot configuration. SlotSV is always
// enabled; for non-SV slots Enabled gates the corresponding compose
// profile.
type Validator struct {
	Enabled   bool
	PartyHint string
	Auth      Auth
	Parties   []Party
}

// Auth captures the OAuth2 client credentials for a validator slot.
// ClientSecret may be set to a literal value or to "${ENV_VAR}", in
// which case the referenced environment variable is resolved at
// Load time.
type Auth struct {
	ClientID     string
	ClientSecret string
}

// Party is a hosted party hint carried in the YAML so consumers can
// later drive bootstrap. v1 only emits env vars — bootstrap is the
// fixture's job.
type Party struct {
	Name    string
	Primary bool
}

type rawConfig struct {
	SchemaVersion string                  `yaml:"schemaVersion"`
	Modules       *rawModules             `yaml:"modules"`
	Validators    map[string]*rawValidator `yaml:"validators"`
}

type rawModules struct {
	Obs *bool `yaml:"obs"`
	Pqs *bool `yaml:"pqs"`
}

type rawValidator struct {
	Enabled   *bool      `yaml:"enabled"`
	PartyHint *string    `yaml:"partyHint"`
	Auth      *rawAuth   `yaml:"auth"`
	Parties   []rawParty `yaml:"parties"`
}

type rawAuth struct {
	ClientID     *string `yaml:"clientId"`
	ClientSecret *string `yaml:"clientSecret"`
}

type rawParty struct {
	Name    string `yaml:"name"`
	Primary bool   `yaml:"primary"`
}

// Defaults returns the built-in fall-back configuration used when no
// canton-localnet.yaml is discovered: all five slots enabled, obs and
// pqs on, party hints equal to slot names.
func Defaults() Config {
	cfg := Config{
		SchemaVersion: SchemaVersion,
		Modules:       Modules{Obs: true, Pqs: true},
		Validators:    map[string]Validator{},
	}
	for _, slot := range KnownSlots {
		cfg.Validators[slot] = Validator{
			Enabled:   true,
			PartyHint: slot,
		}
	}
	return cfg
}

// Discover walks up from start looking for a canton-localnet.yaml in
// each ancestor, matching the discovery semantics of git, docker
// compose and kubectl. It returns ("", nil) when no file is found
// (caller falls back to Defaults). start may be empty, in which case
// the process working directory is used.
func Discover(start string) (string, error) {
	if start == "" {
		var err error
		start, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("yamlconfig: resolving working directory: %w", err)
		}
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("yamlconfig: %w", err)
	}
	dir := abs
	for {
		candidate := filepath.Join(dir, FileName)
		info, err := os.Stat(candidate)
		switch {
		case err == nil && !info.IsDir():
			return candidate, nil
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			return "", fmt.Errorf("yamlconfig: stat %s: %w", candidate, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

// Resolve combines an explicit --config path with walk-up discovery
// and returns a fully-merged Config. When explicitPath is set, that
// path is read directly. Otherwise Discover runs against start, and
// the discovered file (if any) is merged over Defaults. When no file
// is found anywhere, Defaults is returned.
func Resolve(explicitPath, start string) (Config, string, error) {
	if explicitPath != "" {
		cfg, err := Load(explicitPath)
		if err != nil {
			return Config{}, "", err
		}
		return cfg, explicitPath, nil
	}
	path, err := Discover(start)
	if err != nil {
		return Config{}, "", err
	}
	if path == "" {
		return Defaults(), "", nil
	}
	cfg, err := Load(path)
	if err != nil {
		return Config{}, "", err
	}
	return cfg, path, nil
}

// Load reads path, parses it as preview-1 YAML, validates it against
// the known slot set and the supported schema version, and returns
// the result merged over Defaults.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("yamlconfig: reading %s: %w", path, err)
	}
	return Parse(data, path)
}

// Parse is the in-memory counterpart of Load: it decodes data
// (annotated with sourceLabel for error messages), validates, and
// merges. Exposed for tests.
func Parse(data []byte, sourceLabel string) (Config, error) {
	var raw rawConfig
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		return Config{}, fmt.Errorf("yamlconfig: parsing %s: %w", sourceLabel, err)
	}
	if raw.SchemaVersion == "" {
		return Config{}, fmt.Errorf("yamlconfig: %s: schemaVersion is required (expected %q)", sourceLabel, SchemaVersion)
	}
	if raw.SchemaVersion != SchemaVersion {
		return Config{}, fmt.Errorf("yamlconfig: %s: unsupported schemaVersion %q (expected %q)", sourceLabel, raw.SchemaVersion, SchemaVersion)
	}
	for name := range raw.Validators {
		if !isKnownSlot(name) {
			return Config{}, fmt.Errorf("yamlconfig: %s: unknown validator slot %q (known slots: %s)", sourceLabel, name, strings.Join(KnownSlots, ", "))
		}
	}
	cfg := Defaults()
	if raw.Modules != nil {
		if raw.Modules.Obs != nil {
			cfg.Modules.Obs = *raw.Modules.Obs
		}
		if raw.Modules.Pqs != nil {
			cfg.Modules.Pqs = *raw.Modules.Pqs
		}
	}
	for name, rv := range raw.Validators {
		merged, err := mergeValidator(cfg.Validators[name], rv, name, sourceLabel)
		if err != nil {
			return Config{}, err
		}
		cfg.Validators[name] = merged
	}
	return cfg, nil
}

func mergeValidator(base Validator, rv *rawValidator, slot, sourceLabel string) (Validator, error) {
	out := base
	if rv == nil {
		return out, nil
	}
	if rv.Enabled != nil {
		if slot == SlotSV && !*rv.Enabled {
			return Validator{}, fmt.Errorf("yamlconfig: %s: slot %q cannot be disabled (SV is non-toggleable)", sourceLabel, slot)
		}
		out.Enabled = *rv.Enabled
	}
	if rv.PartyHint != nil {
		out.PartyHint = *rv.PartyHint
	}
	if rv.Auth != nil {
		if rv.Auth.ClientID != nil {
			out.Auth.ClientID = *rv.Auth.ClientID
		}
		if rv.Auth.ClientSecret != nil {
			resolved, err := resolveSecret(*rv.Auth.ClientSecret)
			if err != nil {
				return Validator{}, fmt.Errorf("yamlconfig: %s: slot %q clientSecret: %w", sourceLabel, slot, err)
			}
			out.Auth.ClientSecret = resolved
		}
	}
	if rv.Parties != nil {
		out.Parties = make([]Party, 0, len(rv.Parties))
		for _, p := range rv.Parties {
			if p.Name == "" {
				return Validator{}, fmt.Errorf("yamlconfig: %s: slot %q parties: name is required", sourceLabel, slot)
			}
			out.Parties = append(out.Parties, Party(p))
		}
	}
	return out, nil
}

func resolveSecret(raw string) (string, error) {
	if !strings.HasPrefix(raw, "${") || !strings.HasSuffix(raw, "}") {
		return raw, nil
	}
	name := strings.TrimSuffix(strings.TrimPrefix(raw, "${"), "}")
	if name == "" {
		return "", fmt.Errorf("empty ${} reference")
	}
	value, ok := os.LookupEnv(name)
	if !ok {
		return "", fmt.Errorf("env var %q is not set", name)
	}
	return value, nil
}

func isKnownSlot(name string) bool {
	for _, s := range KnownSlots {
		if s == name {
			return true
		}
	}
	return false
}

// Env translates a merged Config into the env-var pairs (KEY=value)
// the existing compose pipeline already consumes. Output is sorted so
// the result is deterministic for tests.
func (c Config) Env() []string {
	pairs := map[string]string{
		"OBS_PROFILE": profileFlag(c.Modules.Obs),
		"PQS_PROFILE": profileFlag(c.Modules.Pqs),
	}
	for slot, v := range c.Validators {
		prefix := slotEnvPrefix(slot)
		enabled := v.Enabled || slot == SlotSV
		pairs[prefix+"_PROFILE"] = profileFlag(enabled)
		if v.PartyHint != "" {
			pairs[prefix+"_PARTY_HINT"] = v.PartyHint
		}
		if v.Auth.ClientID != "" {
			pairs[prefix+"_OAUTH_CLIENT_ID"] = v.Auth.ClientID
		}
		if v.Auth.ClientSecret != "" {
			pairs[prefix+"_OAUTH_CLIENT_SECRET"] = v.Auth.ClientSecret
		}
	}
	keys := make([]string, 0, len(pairs))
	for k := range pairs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+pairs[k])
	}
	return out
}

// EnabledSlots returns the slot names whose compose profile should be
// activated, in deterministic order. SV is always included; other
// slots are filtered by their Enabled flag.
func (c Config) EnabledSlots() []string {
	out := make([]string, 0, len(c.Validators))
	for _, slot := range KnownSlots {
		v, ok := c.Validators[slot]
		if !ok {
			continue
		}
		if slot == SlotSV || v.Enabled {
			out = append(out, slot)
		}
	}
	return out
}

func slotEnvPrefix(slot string) string {
	return strings.ReplaceAll(strings.ToUpper(slot), "-", "_")
}

func profileFlag(on bool) string {
	if on {
		return "on"
	}
	return "off"
}
