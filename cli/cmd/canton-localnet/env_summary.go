// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"
)

const (
	envKeyPrefix        = "CANTON_LOCALNET_"
	slotJSONAPISuffix   = "_JSON_API_URL"
	slotMarker          = "_VALIDATOR_"
	summaryAbsentValue  = "-"
	summaryPQSEnabled   = "yes"
	summaryPQSDisabled  = "no"
	summaryFilePermBits = 0o600
)

// renderEnvSummary renders the markdown run report for an `env` snapshot:
// the version line, one validators table row per slot, and the shared
// endpoints. It reads only an allowlist of endpoint keys from vars and
// skips every var marked Secret, so a credential, JWT or PQS connection
// string can never reach the report.
func renderEnvSummary(vars []envVar) string {
	public := make(map[string]string, len(vars))
	var slotPrefixes []string
	for _, v := range vars {
		if v.Secret {
			continue
		}
		public[v.Key] = v.Value
		if strings.HasPrefix(v.Key, envKeyPrefix) && strings.Contains(v.Key, slotMarker) && strings.HasSuffix(v.Key, slotJSONAPISuffix) {
			slotPrefixes = append(slotPrefixes, strings.TrimSuffix(v.Key, slotJSONAPISuffix))
		}
	}
	value := func(key string) string {
		if v := public[key]; v != "" {
			return v
		}
		return summaryAbsentValue
	}

	hasParty := false
	for _, prefix := range slotPrefixes {
		if _, ok := public[prefix+"_PARTY"]; ok {
			hasParty = true
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "CLI `%s` · Splice `%s`\n\n", value(envKeyPrefix+"VERSION"), value(envKeyPrefix+"SPLICE_VERSION"))

	b.WriteString("### Validators\n\n")
	head := "| Slot | JSON API | Ledger gRPC | Admin gRPC | Validator API | PQS |"
	divider := "|---|---|---|---|---|---|"
	if hasParty {
		head += " Party |"
		divider += "---|"
	}
	b.WriteString(head + "\n" + divider + "\n")
	for _, prefix := range slotPrefixes {
		pqs := summaryPQSDisabled
		if _, ok := public[prefix+"_PQS_HOST"]; ok {
			pqs = summaryPQSEnabled
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s | %s |",
			slotCanonicalFromEnvPrefix(prefix),
			value(prefix+slotJSONAPISuffix), value(prefix+"_GRPC_URL"), value(prefix+"_ADMIN_GRPC_URL"),
			value(prefix+"_VALIDATOR_API_URL"), pqs)
		if hasParty {
			fmt.Fprintf(&b, " %s |", value(prefix+"_PARTY"))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n### Shared endpoints\n\n| Endpoint | URL |\n|---|---|\n")
	fmt.Fprintf(&b, "| Scan | %s |\n", value(envKeyPrefix+"SCAN_URL"))
	fmt.Fprintf(&b, "| Keycloak | http://%s:%s |\n", value(envKeyPrefix+"KEYCLOAK_HOST"), value(envKeyPrefix+"KEYCLOAK_PORT"))
	if len(slotPrefixes) > 0 {
		fmt.Fprintf(&b, "| OAuth token URL (`%s`) | %s |\n", value(envKeyPrefix+"PROFILE"), value(slotPrefixes[0]+"_TOKEN_URL"))
	}
	return b.String()
}

func slotCanonicalFromEnvPrefix(prefix string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(prefix, envKeyPrefix), "_", "-"))
}

func appendEnvSummary(path string, vars []envVar) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, summaryFilePermBits)
	if err != nil {
		return fmt.Errorf("env: open summary file %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.WriteString(renderEnvSummary(vars)); err != nil {
		return fmt.Errorf("env: write summary file %s: %w", path, err)
	}
	return nil
}
