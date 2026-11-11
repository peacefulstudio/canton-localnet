// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package slot

import (
	"regexp"
	"strings"
	"testing"
)

const (
	csharpEndpointDiscovery = "csharp/src/Peaceful.Canton.Localnet.Testing/EndpointDiscovery.cs"
	csharpLocalnetFixture   = "csharp/src/Peaceful.Canton.Localnet.Testing/LocalnetFixture.cs"
	goEndpointDiscovery     = "go/fixture/endpoint_discovery.go"
)

func csharpProfileName(s Slot) string {
	parts := strings.Split(s.Canonical, "-")
	for i, part := range parts {
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, "")
}

func csharpMemberBody(t *testing.T, source, name string) string {
	t.Helper()
	pattern := regexp.MustCompile(`(?ms)^    (?:public|private) static [^\n]*\b` + regexp.QuoteMeta(name) + `\b[^\n]*\n(.*?)^    \}`)
	match := pattern.FindStringSubmatch(source)
	if match == nil {
		t.Fatalf("C# member %s not found", name)
	}
	return match[1]
}

func csharpProfileArms(t *testing.T, source, member string, valueArm string) map[string]string {
	t.Helper()
	arms := regexp.MustCompile(`LocalnetProfile\.(\w+) => `+valueArm).FindAllStringSubmatch(csharpMemberBody(t, source, member), -1)
	return tableFromMatches(arms)
}

func csharpProfileTable(field func(Slot) string) map[string]string {
	table := map[string]string{}
	for _, s := range All() {
		if value := field(s); value != "" {
			table[csharpProfileName(s)] = value
		}
	}
	return table
}

func nonSvSlots() []Slot {
	return slotsWhere(func(s Slot) bool { return s.AuthKind == AuthKindOAuth2 })
}

func nonSvTable(field func(Slot) string) map[string]string {
	table := map[string]string{}
	for _, s := range nonSvSlots() {
		table[csharpProfileName(s)] = field(s)
	}
	return table
}

func TestCSharpLocalnetProfileEnumCoversEverySlot(t *testing.T) {
	t.Parallel()
	enum := regexp.MustCompile(`(?ms)public enum LocalnetProfile\n\{(.*?)^\}`).FindStringSubmatch(readRepoFile(t, csharpEndpointDiscovery))
	if enum == nil {
		t.Fatal("C# LocalnetProfile enum not found")
	}
	members := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^    (\w+),$`).FindAllStringSubmatch(enum[1], -1) {
		members[m[1]] = m[1]
	}
	want := map[string]string{}
	for _, s := range All() {
		want[csharpProfileName(s)] = csharpProfileName(s)
	}
	assertTableMatches(t, "C# LocalnetProfile enum", members, want)
}

func TestCSharpSlotNameTablesMatchSlot(t *testing.T) {
	t.Parallel()
	fixture := readRepoFile(t, csharpLocalnetFixture)
	canonicalByProfile := csharpProfileTable(func(s Slot) string { return s.Canonical })

	assertTableMatches(t, "C# LocalnetFixture.SlotName",
		csharpProfileArms(t, fixture, "SlotName", `"([\w-]+)"`), canonicalByProfile)

	parseArms := regexp.MustCompile(`(?m)^\s+"([\w-]+)"(?: or "[\w-]+")* => LocalnetProfile\.(\w+),$`).
		FindAllStringSubmatch(csharpMemberBody(t, fixture, "ParseSlot"), -1)
	profileByCanonical := map[string]string{}
	for _, m := range parseArms {
		profileByCanonical[m[2]] = m[1]
	}
	assertTableMatches(t, "C# LocalnetFixture.ParseSlot", profileByCanonical, canonicalByProfile)

	profileByCanonicalEndpointDiscovery := map[string]string{}
	resolveArms := regexp.MustCompile(`(?m)^\s+"([\w-]+)"(?: or "[\w-]+")* => LocalnetProfile\.(\w+),$`).
		FindAllStringSubmatch(readRepoFile(t, csharpEndpointDiscovery), -1)
	for _, m := range resolveArms {
		profileByCanonicalEndpointDiscovery[m[2]] = m[1]
	}
	assertTableMatches(t, "C# EndpointDiscovery.ResolveProfile", profileByCanonicalEndpointDiscovery, canonicalByProfile)
}

func TestCSharpKnownSlotsListsEverySlotInCanonicalOrder(t *testing.T) {
	t.Parallel()
	body := csharpMemberBody(t, readRepoFile(t, csharpLocalnetFixture), "KnownSlots")
	var got []string
	for _, m := range regexp.MustCompile(`(?m)^\s+"([\w-]+)",$`).FindAllStringSubmatch(body, -1) {
		got = append(got, m[1])
	}
	if strings.Join(got, ",") != strings.Join(Canonicals(), ",") {
		t.Errorf("C# LocalnetFixture.KnownSlots disagrees with cli/internal/slot:\n  have: %v\n  want: %v", got, Canonicals())
	}
}

func TestCSharpEndpointDiscoveryDefaultsMatchSlot(t *testing.T) {
	t.Parallel()
	source := readRepoFile(t, csharpEndpointDiscovery)

	assertTableMatches(t, "C# EndpointDiscovery.DefaultJsonApiUrl",
		csharpProfileArms(t, source, "DefaultJsonApiUrl", `"([^"]+)"`),
		csharpProfileTable(func(s Slot) string { return "http://localhost:" + s.JSONLedgerPort() }))
	assertTableMatches(t, "C# EndpointDiscovery.DefaultGrpcLedgerApiUrl",
		csharpProfileArms(t, source, "DefaultGrpcLedgerApiUrl", `"([^"]+)"`),
		csharpProfileTable(func(s Slot) string { return "http://localhost:" + s.LedgerGrpcPort() }))
	assertTableMatches(t, "C# EndpointDiscovery.DefaultTokenUrl realms",
		csharpProfileArms(t, source, "DefaultTokenUrl", `"(\w+)"`),
		csharpProfileTable(func(s Slot) string { return s.Realm }))
	assertTableMatches(t, "C# EndpointDiscovery.DefaultClientId",
		csharpProfileArms(t, source, "DefaultClientId", `"([\w-]+)"`),
		nonSvTable(func(s Slot) string { return s.Canonical + "-validator" }))
	assertTableMatches(t, "C# EndpointDiscovery.SlotEnvPrefix",
		csharpProfileArms(t, source, "SlotEnvPrefix", `"(\w+)"`),
		csharpProfileTable(func(s Slot) string { return s.EnvPrefix() }))
}

func TestCSharpValidatorUserIdDefaultsMatchSlot(t *testing.T) {
	t.Parallel()
	source := readRepoFile(t, csharpEndpointDiscovery)
	constants := tableFromMatches(regexp.MustCompile(`private const string (\w+) = "([\w-]+)";`).FindAllStringSubmatch(source, -1))
	userIdByProfile := map[string]string{}
	for profile, constant := range csharpProfileArms(t, source, "DefaultValidatorUserId", `(\w+Id),`) {
		userIdByProfile[profile] = constants[constant]
	}
	assertTableMatches(t, "C# EndpointDiscovery.DefaultValidatorUserId",
		userIdByProfile, nonSvTable(func(s Slot) string { return s.ValidatorUserID }))
}

func TestGoFixtureSlotDefaultsMatchSlot(t *testing.T) {
	t.Parallel()
	rows := regexp.MustCompile(`(?m)^\s+Role(\w+):\s+\{"(\w+)", "(\d+)", "(\d+)", "(\w*)", "([\w-]*)",`).
		FindAllStringSubmatch(readRepoFile(t, goEndpointDiscovery), -1)
	column := func(index int) map[string]string {
		table := map[string]string{}
		for _, row := range rows {
			table[row[1]] = row[index]
		}
		return table
	}
	assertTableMatches(t, "go/fixture slotDefaultsByRole env prefix", column(2),
		csharpProfileTable(func(s Slot) string { return s.EnvPrefix() }))
	assertTableMatches(t, "go/fixture slotDefaultsByRole JSON port", column(3),
		csharpProfileTable(func(s Slot) string { return s.JSONLedgerPort() }))
	assertTableMatches(t, "go/fixture slotDefaultsByRole gRPC port", column(4),
		csharpProfileTable(func(s Slot) string { return s.LedgerGrpcPort() }))
	assertTableMatches(t, "go/fixture slotDefaultsByRole realm", withoutEmpty(column(5)),
		csharpProfileTable(func(s Slot) string { return s.Realm }))
	assertTableMatches(t, "go/fixture slotDefaultsByRole client id", withoutEmpty(column(6)),
		nonSvTable(func(s Slot) string { return s.Canonical + "-validator" }))
}

func withoutEmpty(table map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range table {
		if value != "" {
			out[key] = value
		}
	}
	return out
}
