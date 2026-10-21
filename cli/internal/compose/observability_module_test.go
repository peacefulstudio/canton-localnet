// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package compose

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const observabilityModuleDir = "../../../compose/modules/observability"

type observabilityCompose struct {
	Services map[string]struct {
		MemLimit    string            `yaml:"mem_limit"`
		Ports       []string          `yaml:"ports"`
		Environment map[string]string `yaml:"environment"`
	} `yaml:"services"`
}

type collectorConfig struct {
	Receivers struct {
		Otlp struct {
			Protocols map[string]struct {
				Endpoint string `yaml:"endpoint"`
			} `yaml:"protocols"`
		} `yaml:"otlp"`
	} `yaml:"receivers"`
}

func readYAML[T any](t *testing.T, relativePath string) T {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(observabilityModuleDir, relativePath))
	if err != nil {
		t.Fatal(err)
	}
	var parsed T
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	return parsed
}

func readEnvFile(t *testing.T, relativePath string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(observabilityModuleDir, relativePath))
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		name, value, found := strings.Cut(line, "=")
		if found && !strings.HasPrefix(name, "#") {
			values[name] = value
		}
	}
	return values
}

func TestObservabilityCollectorPublishesOtlpGrpcOnTheHostBindIp(t *testing.T) {
	t.Parallel()

	collector := readYAML[observabilityCompose](t, "compose.yaml").Services["otel-collector"]

	want := "${HOST_BIND_IP:-127.0.0.1}:${OTEL_COLLECTOR_OTLP_PORT}:${OTEL_COLLECTOR_OTLP_PORT}"
	if !slices.Contains(collector.Ports, want) {
		t.Errorf("otel-collector ports = %v, want %q among them", collector.Ports, want)
	}
	if got := readEnvFile(t, "compose.env")["OTEL_COLLECTOR_OTLP_PORT"]; got != "14001" {
		t.Errorf("OTEL_COLLECTOR_OTLP_PORT = %q, want 14001", got)
	}
}

func TestObservabilityCollectorPublishesOtlpHttpOnTheHostBindIp(t *testing.T) {
	t.Parallel()

	collector := readYAML[observabilityCompose](t, "compose.yaml").Services["otel-collector"]

	want := "${HOST_BIND_IP:-127.0.0.1}:${OTEL_COLLECTOR_OTLP_HTTP_PORT}:${OTEL_COLLECTOR_OTLP_HTTP_PORT}"
	if !slices.Contains(collector.Ports, want) {
		t.Errorf("otel-collector ports = %v, want %q among them", collector.Ports, want)
	}
	if got := readEnvFile(t, "compose.env")["OTEL_COLLECTOR_OTLP_HTTP_PORT"]; got != "14003" {
		t.Errorf("OTEL_COLLECTOR_OTLP_HTTP_PORT = %q, want 14003", got)
	}
}

func TestObservabilityCollectorListensForOtlpHttpOnThePublishedPort(t *testing.T) {
	t.Parallel()

	collector := readYAML[observabilityCompose](t, "compose.yaml").Services["otel-collector"]
	config := readYAML[collectorConfig](t, "conf/otel-collector/collector.yaml")

	if got := collector.Environment["OTLP_HTTP_LISTEN_PORT"]; got != "${OTEL_COLLECTOR_OTLP_HTTP_PORT}" {
		t.Errorf("OTLP_HTTP_LISTEN_PORT = %q, want ${OTEL_COLLECTOR_OTLP_HTTP_PORT}", got)
	}
	if got := config.Receivers.Otlp.Protocols["http"].Endpoint; got != "0.0.0.0:${env:OTLP_HTTP_LISTEN_PORT}" {
		t.Errorf("otlp http endpoint = %q, want 0.0.0.0:${env:OTLP_HTTP_LISTEN_PORT}", got)
	}
}

func TestObservabilityGrafanaMemoryLimitHoldsWhileBrowsingTraces(t *testing.T) {
	t.Parallel()

	grafana := readYAML[observabilityCompose](t, "compose.yaml").Services["grafana"]

	if grafana.MemLimit != "512mb" {
		t.Errorf("grafana mem_limit = %q, want 512mb", grafana.MemLimit)
	}
}
