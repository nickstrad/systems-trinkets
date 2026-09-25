# Lessons and the local backing services they use. Start only what a lesson needs.
#
#   make run-<lesson>     run the lesson from its directory; writes measurements.csv
#   make analyze-<lesson> run its analyze.sql in DuckDB over measurements.csv
#   make lab-<lesson>     both, in order
#   make up-<service>     start in the background and wait until healthy
#   make down-<service>   stop and remove containers, keep data
#   make clean-<service>  stop and delete containers and data volumes
#   make logs-<service>   follow logs
#   make ps               show running trinkets services
#
# Services: postgres, valkey, seaweedfs (see services/index.md)

SERVICES := postgres valkey seaweedfs
LESSONS  := $(patsubst lessons/%/,%,$(wildcard lessons/*/))

compose_file_postgres  := services/postgres.compose.yaml
compose_file_valkey    := services/valkey.compose.yaml
compose_file_seaweedfs := services/seaweedfs/compose.yaml

compose = docker compose -f $(compose_file_$(1))

.PHONY: help ps

help:
	@sed -n 's/^#   /  /p' Makefile
	@echo "  lessons:  $(LESSONS)"
	@echo "  services: $(SERVICES)"

# Explicit per-lesson rules, for the same reason as the service rules below.
# A lesson is a directory under lessons/ with a main.go (or main.ts for Deno)
# and an analyze.sql; both run from inside that directory.
define lesson_rules
.PHONY: run-$(1) analyze-$(1) lab-$(1)
run-$(1):
	cd lessons/$(1) && $(if $(wildcard lessons/$(1)/main.ts),deno run -A main.ts,go run .)
analyze-$(1):
	cd lessons/$(1) && duckdb < analyze.sql
lab-$(1): run-$(1) analyze-$(1)
endef

$(foreach l,$(LESSONS),$(eval $(call lesson_rules,$(l))))

ps:
	@docker ps --filter "label=com.docker.compose.project" --format '{{.Label "com.docker.compose.project"}}\t{{.Status}}\t{{.Ports}}' | grep '^trinkets-' || echo "no trinkets services running"

# Explicit per-service rules (pattern rules are ignored for .PHONY targets).
define service_rules
.PHONY: up-$(1) down-$(1) clean-$(1) logs-$(1)
up-$(1):
	$$(call compose,$(1)) up -d --wait
down-$(1):
	$$(call compose,$(1)) down
clean-$(1):
	$$(call compose,$(1)) down -v --remove-orphans
logs-$(1):
	$$(call compose,$(1)) logs -f
endef

$(foreach s,$(SERVICES),$(eval $(call service_rules,$(s))))
