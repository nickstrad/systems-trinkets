# Lessons and the local backing services they use. Start only what a lesson needs.
#
#   make run-<lesson>     run the lesson from its directory; writes measurements.csv
#   make analyze-<lesson> run its analyze.sql in DuckDB over measurements.csv
#   make lab-<lesson>     both, in order
#   make serve-<lesson>   HTTP adapter on localhost:8080; Ctrl-C stops it
#   make k6-<lesson>      test a running adapter (PROFILE=smoke by default)
#   make analyze-k6-<lesson> analyze the latest performance CSV with DuckDB
#   make lab-k6-<lesson>  start adapter, run k6, analyze, and stop adapter
#   make up-<service>     start in the background and wait until healthy
#   make down-<service>   stop and remove containers, keep data
#   make clean-<service>  stop and delete containers and data volumes
#   make logs-<service>   follow logs
#   make ps               show running trinkets services
#   make check            compile-check every lesson: go vet ./... and deno check
#
# Services: postgres, valkey, seaweedfs (see services/index.md)

SERVICES     := postgres valkey seaweedfs
GO_LESSONS   := $(patsubst lessons/go/%/main.go,%,$(wildcard lessons/go/*/main.go))
DENO_LESSONS := $(patsubst lessons/deno/%/main.ts,%,$(wildcard lessons/deno/*/main.ts))
LESSONS      := $(GO_LESSONS) $(DENO_LESSONS)

compose_file_postgres  := services/postgres.compose.yaml
compose_file_valkey    := services/valkey.compose.yaml
compose_file_seaweedfs := services/seaweedfs/compose.yaml

compose = docker compose -f $(compose_file_$(1))

.PHONY: help ps check

help:
	@sed -n 's/^#   /  /p' Makefile
	@echo "  lessons:  $(LESSONS)"
	@echo "  k6:       $(notdir $(PERF_LESSONS))"
	@echo "  services: $(SERVICES)"

# Explicit per-lesson rules, for the same reason as the service rules below.
# A lesson is lessons/go/<name>/main.go or lessons/deno/<name>/main.ts plus an
# analyze.sql; the directory picks the runtime. Both run from inside that
# directory because measurements.csv and analyze.sql use relative paths.
run_go   := go run .
run_deno := deno run -A main.ts

define lesson_rules
.PHONY: run-$(1) analyze-$(1) lab-$(1)
run-$(1):
	cd lessons/$(2)/$(1) && $(run_$(2))
analyze-$(1):
	cd lessons/$(2)/$(1) && duckdb < analyze.sql
lab-$(1): run-$(1) analyze-$(1)
endef

$(foreach l,$(GO_LESSONS),$(eval $(call lesson_rules,$(l),go)))
$(foreach l,$(DENO_LESSONS),$(eval $(call lesson_rules,$(l),deno)))

# Only lessons with a workload gain performance targets; run.ts picks the
# runtime from the lesson directory. Settings such as PROFILE=load or MODE=blocking
# reach the recipe from the make command line or the environment as-is.
PERF_LESSONS := $(patsubst %/perf/k6.ts,%,$(wildcard lessons/*/*/perf/k6.ts))

define perf_rules
.PHONY: serve-$(2) k6-$(2) analyze-k6-$(2) lab-k6-$(2)
serve-$(2):
	deno run -A scripts/perf/run.ts serve $(1)
k6-$(2):
	deno run -A scripts/perf/run.ts run $(1)
analyze-k6-$(2):
	deno run -A scripts/perf/run.ts analyze $(1)
lab-k6-$(2):
	deno run -A scripts/perf/run.ts lab $(1)
endef

$(foreach p,$(PERF_LESSONS),$(eval $(call perf_rules,$(p),$(notdir $(p)))))

check:
	go vet ./...
	deno check .

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
