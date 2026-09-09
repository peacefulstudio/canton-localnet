# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

.DEFAULT_GOAL := help

COMPOSE_DIR := compose
CLI_DIR      := cli
LOCALNET_DIR := $(CURDIR)/$(COMPOSE_DIR)/modules/localnet
KEYCLOAK_DIR := $(CURDIR)/$(COMPOSE_DIR)/modules/keycloak
PQS_DIR      := $(CURDIR)/$(COMPOSE_DIR)/modules/pqs
OBS_DIR      := $(CURDIR)/$(COMPOSE_DIR)/modules/observability
ONBOARD_DIR  := $(CURDIR)/$(COMPOSE_DIR)/modules/splice-onboarding

# Module compose files reference ${MODULES_DIR} / ${LOCALNET_DIR} for volume
# mounts and extension env_files; export absolute defaults so docker compose
# resolves bind-mount paths regardless of the caller's working directory.
export MODULES_DIR  ?= $(CURDIR)/$(COMPOSE_DIR)/modules
export LOCALNET_DIR

# AUTH_MODE selects the auth profile applied to the stack:
#   oauth2  — Keycloak realms issue tokens (matches shared VM and production)
#   secret  — Canton's shared-secret JWT (undocumented escape hatch; not CI-tested)
AUTH_MODE ?= oauth2

# RES=true (default) applies per-module mem_limit / JVM heap caps so the stack
# fits comfortably on a 16 GB dev machine. Set RES=false to remove caps.
RES ?= true

SLOT ?= a

YES ?=

# Base stack: always present.
COMPOSE_FILES := -f $(LOCALNET_DIR)/compose.yaml \
                 -f $(ONBOARD_DIR)/compose.yaml
ENV_FILES     := --env-file $(COMPOSE_DIR)/.env.defaults \
                 --env-file $(LOCALNET_DIR)/compose.env \
                 --env-file $(LOCALNET_DIR)/env/common.env
PROFILES      := --profile a-validator-1 --profile b-validator-1 --profile c-validator-1 --profile sv-validator-1 --profile d-validator-1

ifeq ($(RES),true)
  COMPOSE_FILES += -f $(LOCALNET_DIR)/resource-constraints.yaml \
                   -f $(ONBOARD_DIR)/resource-constraints.yaml
endif

ifeq ($(AUTH_MODE),oauth2)
  COMPOSE_FILES += -f $(KEYCLOAK_DIR)/compose.yaml
  ENV_FILES    += --env-file $(KEYCLOAK_DIR)/compose.env
  PROFILES     += --profile keycloak
  ifeq ($(RES),true)
    COMPOSE_FILES += -f $(KEYCLOAK_DIR)/resource-constraints.yaml
  endif
endif

# Optional PQS layer — opt in via `make up PQS=true`.
ifeq ($(PQS),true)
  COMPOSE_FILES += -f $(PQS_DIR)/compose.yaml
  ENV_FILES    += --env-file $(PQS_DIR)/compose.env
  PROFILES     += --profile pqs-a-validator-1
  ifeq ($(RES),true)
    COMPOSE_FILES += -f $(PQS_DIR)/resource-constraints.yaml
  endif
endif

MULTI_SYNC ?=
ifeq ($(MULTI_SYNC),true)
PROFILES += --profile multi-sync
endif

# `DOCKER_COMPOSE_APP` is the application stack alone — used by stop-app /
# clean-app to leave the observability stack running across iterations.
DOCKER_COMPOSE_APP := docker compose $(COMPOSE_FILES) $(ENV_FILES) $(PROFILES)

# Optional observability layer (Grafana / Prometheus / Loki / Tempo / cAdvisor)
# — opt in via `make up OBS=true`. Grafana lands on http://localhost:3030.
OBS_COMPOSE_FILES :=
OBS_ENV_FILES     :=
OBS_PROFILES      :=
ifeq ($(OBS),true)
  OBS_COMPOSE_FILES += -f $(OBS_DIR)/compose.yaml \
                       -f $(OBS_DIR)/observability.yaml
  ifeq ($(shell uname -s),Darwin)
    OBS_COMPOSE_FILES += -f $(OBS_DIR)/cadvisor-darwin.yaml
  else
    OBS_COMPOSE_FILES += -f $(OBS_DIR)/cadvisor-linux.yaml
  endif
  OBS_ENV_FILES += --env-file $(OBS_DIR)/compose.env
  OBS_PROFILES  += --profile observability
  ifeq ($(PQS),true)
    OBS_COMPOSE_FILES += -f $(PQS_DIR)/observability.yaml
  endif
endif

DOCKER_COMPOSE := docker compose $(COMPOSE_FILES) $(OBS_COMPOSE_FILES) \
                  $(ENV_FILES) $(OBS_ENV_FILES) $(PROFILES) $(OBS_PROFILES)

.PHONY: help
help: ## Show this help
	@awk 'BEGIN{FS=":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} \
	      /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: up
up: ## Start LocalNet (auth mode: $(AUTH_MODE))
	$(DOCKER_COMPOSE) up -d

.PHONY: down
down: ## Stop LocalNet and remove containers
	$(DOCKER_COMPOSE) down --remove-orphans

.PHONY: stop-app
stop-app: ## Stop the app stack but leave observability containers running (only meaningful with OBS=true)
	$(DOCKER_COMPOSE_APP) down

.PHONY: clean
clean: ## Stop LocalNet and remove containers + volumes
	$(DOCKER_COMPOSE) down -v --remove-orphans

.PHONY: clean-app
clean-app: ## Like `clean`, but leave observability running
	$(DOCKER_COMPOSE_APP) down -v

.PHONY: status
status: ## Show container status
	$(DOCKER_COMPOSE) ps

.PHONY: logs
logs: ## Tail logs
	$(DOCKER_COMPOSE) logs -f

.PHONY: wait-ready
wait-ready: ## Poll JSON Ledger API until participant accepts requests
	$(COMPOSE_DIR)/scripts/wait-ready.sh

.PHONY: vendor
vendor: ## Re-fetch splice modules pinned in compose/links.csv
	cd $(COMPOSE_DIR) && ./scripts/vendor.sh

.PHONY: config
config: ## Print the resolved compose configuration (debugging)
	$(DOCKER_COMPOSE) config

.PHONY: check-party-hints
check-party-hints: ## Assert every resolved slot party hint equals its slot name
	@bash -eo pipefail -c '$(DOCKER_COMPOSE) config | $(COMPOSE_DIR)/scripts/check-party-hints.sh'

.PHONY: prune-rights
prune-rights: ## Revoke the leaked ledger rights on SLOT's validator user. Destructive, and SLOT defaults to a: prints the plan and asks before revoking, unless YES is set to anything other than 0
	@cd $(CLI_DIR) && go build -o canton-localnet ./cmd/canton-localnet
	@$(CLI_DIR)/canton-localnet rights prune --slot $(SLOT) --repo-root $(CURDIR) $(if $(filter-out 0,$(YES)),--yes,)

.PHONY: list-rights
list-rights: ## List the ledger rights held by SLOT's validator user (read-only; SLOT defaults to a)
	@cd $(CLI_DIR) && go build -o canton-localnet ./cmd/canton-localnet
	@$(CLI_DIR)/canton-localnet rights list --slot $(SLOT) --repo-root $(CURDIR)

.PHONY: test-restart-survival
test-restart-survival: ## Onboarding-client restart-survival acceptance test (drives a full down/up cycle; ~5 min). Assumes the stack is already up.
	@command -v jq > /dev/null || { echo "::error::jq is required for this target" >&2; exit 2; }
	tests/acceptance/restart-survival.sh create
	$(MAKE) down
	$(MAKE) up
	$(COMPOSE_DIR)/scripts/wait-ready.sh
	tests/acceptance/restart-survival.sh verify

HETZNER_DIR := terraform/hetzner

.PHONY: hetzner-up
hetzner-up: ## Create the Hetzner LocalNet server (re-attaches the persistent volume)
	terraform -chdir=$(HETZNER_DIR) apply

.PHONY: hetzner-down
hetzner-down: ## Delete only the Hetzner server; keep volume, primary IP, SSH key, firewall
	terraform -chdir=$(HETZNER_DIR) apply -var server_enabled=false
