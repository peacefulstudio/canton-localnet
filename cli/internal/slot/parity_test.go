// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package slot

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func readRepoFile(t *testing.T, relativePath string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repoRoot(t), relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	return string(content)
}

func shellFunctionBody(t *testing.T, script, name string) string {
	t.Helper()
	pattern := regexp.MustCompile(`(?ms)^` + regexp.QuoteMeta(name) + `\(\) \{\n(.*?)^\}`)
	match := pattern.FindStringSubmatch(script)
	if match == nil {
		t.Fatalf("shell function %s not found", name)
	}
	return match[1]
}

func tableFromMatches(matches [][]string) map[string]string {
	table := map[string]string{}
	for _, m := range matches {
		table[m[1]] = m[2]
	}
	return table
}

func assertTableMatches(t *testing.T, source string, got, want map[string]string) {
	t.Helper()
	render := func(table map[string]string) string {
		keys := make([]string, 0, len(table))
		for k := range table {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%s", k, table[k]))
		}
		return strings.Join(parts, " ")
	}
	if render(got) != render(want) {
		t.Errorf("%s disagrees with cli/internal/slot:\n  have: %s\n  want: %s", source, render(got), render(want))
	}
}

func slotTable(field func(Slot) string) map[string]string {
	table := map[string]string{}
	for _, s := range All() {
		table[s.Short] = field(s)
	}
	return table
}

func TestActionLibPortPrefixTableMatchesSlot(t *testing.T) {
	t.Parallel()
	body := shellFunctionBody(t, readRepoFile(t, "action/lib.sh"), "canton_localnet_slot_prefix")
	arms := regexp.MustCompile(`(?m)^\s+(\w+)\) echo "(\d+)" ;;$`).FindAllStringSubmatch(body, -1)
	assertTableMatches(t, "action/lib.sh canton_localnet_slot_prefix",
		tableFromMatches(arms), slotTable(func(s Slot) string { return s.PortPrefix }))
}

func TestActionLibCanonicalNameTableMatchesSlot(t *testing.T) {
	t.Parallel()
	body := shellFunctionBody(t, readRepoFile(t, "action/lib.sh"), "canton_localnet_slot_canonical")
	arms := regexp.MustCompile(`(?m)^\s+(\w+)\) echo "([\w-]+)" ;;$`).FindAllStringSubmatch(body, -1)
	assertTableMatches(t, "action/lib.sh canton_localnet_slot_canonical",
		tableFromMatches(arms), slotTable(func(s Slot) string { return s.Canonical }))
}

func TestActionLibValidatorAliasesMatchSlot(t *testing.T) {
	t.Parallel()
	body := shellFunctionBody(t, readRepoFile(t, "action/lib.sh"), "canton_localnet_normalize_validators")
	arms := regexp.MustCompile(`(?m)^\s+(\w+) \| ([\w-]+)\)`).FindAllStringSubmatch(body, -1)
	aliasToCanonical := map[string]string{}
	for _, m := range arms {
		aliasToCanonical[m[1]] = m[2]
	}
	assertTableMatches(t, "action/lib.sh canton_localnet_normalize_validators",
		aliasToCanonical, slotTable(func(s Slot) string { return s.Canonical }))
}

func TestMakefileProfilesCoverEverySlot(t *testing.T) {
	t.Parallel()
	declaration := regexp.MustCompile(`(?m)^PROFILES\s*:=(.*)$`).FindStringSubmatch(readRepoFile(t, "Makefile"))
	if declaration == nil {
		t.Fatal("Makefile PROFILES declaration not found")
	}
	profiles := map[string]string{}
	for _, m := range regexp.MustCompile(`--profile (\S+)`).FindAllStringSubmatch(declaration[1], -1) {
		profiles[m[1]] = m[1]
	}
	want := map[string]string{}
	for _, name := range Canonicals() {
		want[name] = name
	}
	assertTableMatches(t, "Makefile PROFILES", profiles, want)
}

func shortSet(slots []Slot) map[string]string {
	table := map[string]string{}
	for _, s := range slots {
		table[s.Short] = s.Short
	}
	return table
}

func shellEchoedWords(t *testing.T, script, function string) map[string]string {
	t.Helper()
	list := regexp.MustCompile(`(?m)^\s+echo "([a-z ]+)"$`).FindStringSubmatch(shellFunctionBody(t, script, function))
	if list == nil {
		t.Fatalf("%s: echo line not found", function)
	}
	return wordSet(list[1])
}

func wordSet(words string) map[string]string {
	set := map[string]string{}
	for _, word := range strings.Fields(words) {
		set[word] = word
	}
	return set
}

func TestActionLibPqsSlotsMatchSlot(t *testing.T) {
	t.Parallel()
	lib := readRepoFile(t, "action/lib.sh")
	assertTableMatches(t, "action/lib.sh canton_localnet_pqs_slots",
		shellEchoedWords(t, lib, "canton_localnet_pqs_slots"), shortSet(PqsSlots()))
	assertTableMatches(t, "action/lib.sh canton_localnet_pqs_default_slots",
		shellEchoedWords(t, lib, "canton_localnet_pqs_default_slots"), shortSet(PqsDefaultSlots()))
}

func TestMakefilePqsSlotNamesMatchSlot(t *testing.T) {
	t.Parallel()
	declaration := regexp.MustCompile(`(?m)^PQS_SLOT_NAMES\s*:=(.*)$`).FindStringSubmatch(readRepoFile(t, "Makefile"))
	if declaration == nil {
		t.Fatal("Makefile PQS_SLOT_NAMES declaration not found")
	}
	assertTableMatches(t, "Makefile PQS_SLOT_NAMES", wordSet(declaration[1]), shortSet(PqsSlots()))
}

func TestPqsModuleDefinesAScribeAndAToggledDatabaseForExactlyThePqsSlots(t *testing.T) {
	t.Parallel()
	module := readRepoFile(t, "compose/modules/pqs/compose.yaml")
	profiles := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^      - pqs-([a-z]+)-validator-1$`).FindAllStringSubmatch(module, -1) {
		profiles[m[1]] = m[1]
	}
	assertTableMatches(t, "compose/modules/pqs/compose.yaml profiles", profiles, shortSet(PqsSlots()))
	toggles := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^PQS_([A-Z]+)_VALIDATOR_1_PROFILE=`).FindAllStringSubmatch(readRepoFile(t, "compose/modules/pqs/compose.env"), -1) {
		toggles[strings.ToLower(m[1])] = strings.ToLower(m[1])
	}
	assertTableMatches(t, "compose/modules/pqs/compose.env toggles", toggles, shortSet(PqsSlots()))
	for _, s := range PqsSlots() {
		service := "\n  pqs-" + s.Canonical + ":\n"
		if !strings.Contains(module, service) {
			t.Errorf("compose/modules/pqs/compose.yaml has no %s service", strings.TrimSpace(service))
		}
		databaseEnv := "${MODULES_DIR}/pqs/env/" + s.Canonical + "/${PQS_" + s.EnvPrefix() + "_PROFILE}/postgres.env"
		if !strings.Contains(module, databaseEnv) {
			t.Errorf("compose/modules/pqs/compose.yaml does not create %s's database through %s", s.Canonical, databaseEnv)
		}
		want := "CREATE_DATABASE_PQS_" + s.EnvPrefix() + "=pqs-" + s.Canonical + "\n"
		if got := readRepoFile(t, "compose/modules/pqs/env/"+s.Canonical+"/on/postgres.env"); !strings.HasSuffix(got, want) {
			t.Errorf("compose/modules/pqs/env/%s/on/postgres.env: want it to end with %q, got %q", s.Canonical, want, got)
		}
	}
}
