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
#   make test             go test ./...: spec and property tests, fuzz seeds; no daemon
#   make fuzz             run every Fuzz target for FUZZTIME (default 10s) each
#   make check-docker     Docker harness tests that need a daemon (TRINKETS_DOCKER=1)
#   make clean-harness    remove containers, volumes, networks and images labelled trinkets.harness=1
#
# Services: postgres, redis, valkey, seaweedfs, nats, etcd, registry, toxiproxy, temporal, openbao, pgbouncer (see software/software.md)

SERVICES     := postgres redis valkey seaweedfs nats etcd registry toxiproxy temporal openbao pgbouncer
LESSONS      := $(patsubst lessons/go/%/main.go,%,$(wildcard lessons/go/*/main.go))

compose_file_postgres  := software/postgres.compose.yaml
compose_file_redis     := software/redis.compose.yaml
compose_file_valkey    := software/valkey.compose.yaml
compose_file_seaweedfs := software/seaweedfs/compose.yaml
compose_file_nats      := software/nats.compose.yaml
compose_file_etcd      := software/etcd.compose.yaml
compose_file_registry  := software/registry.compose.yaml
compose_file_toxiproxy := software/toxiproxy.compose.yaml
compose_file_temporal  := software/temporal.compose.yaml
compose_file_openbao   := software/openbao.compose.yaml
compose_file_pgbouncer := software/pgbouncer.compose.yaml

compose = docker compose -f $(compose_file_$(1))

FUZZTIME     ?= 10s

.PHONY: help ps check test fuzz check-docker clean-harness

help:
	@sed -n 's/^#   /  /p' Makefile
	@echo "  lessons:  $(LESSONS)"
	@echo "  k6:       $(notdir $(PERF_LESSONS))"
	@echo "  services: $(SERVICES)"

# Explicit per-lesson rules, for the same reason as the service rules below.
# A lesson is lessons/go/<name>/main.go plus analyze.sql. Run inside that
# directory because measurements.csv and analyze.sql use relative paths.

define lesson_rules
.PHONY: run-$(1) analyze-$(1) lab-$(1)
run-$(1):
	cd lessons/go/$(1) && go run .
analyze-$(1):
	cd lessons/go/$(1) && duckdb < analyze.sql
lab-$(1): run-$(1) analyze-$(1)
endef

$(foreach l,$(LESSONS),$(eval $(call lesson_rules,$(l))))

# Only Go lessons with a workload gain performance targets.
# Settings such as PROFILE=load or MODE=blocking
# reach the recipe from the make command line or the environment as-is.
PERF_LESSONS := $(patsubst %/perf/k6.ts,%,$(wildcard lessons/go/*/perf/k6.ts))

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

test:
	go test ./...

# go test -fuzz takes exactly one target in one package, so list every
# Fuzz target and run them one at a time. The first failure stops the loop;
# a failing input lands in testdata/fuzz/<Target>/ and should be committed.
fuzz:
	@set -e; pkgs=$$(go list ./...) || exit 1; for pkg in $$pkgs; do \
		out=$$(go test -list '^Fuzz' $$pkg) || { echo "$$out"; exit 1; }; \
		for t in $$(echo "$$out" | grep '^Fuzz' || true); do \
			echo "fuzz $$pkg $$t ($(FUZZTIME))"; \
			go test -run '^$$' -fuzz "^$$t\$$" -fuzztime $(FUZZTIME) $$pkg; \
		done; \
	done

check-docker:
	TRINKETS_DOCKER=1 go test -count=1 ./internal/lab/docker/...

# Only objects the harness labelled (trinkets.harness=1) are touched, never
# the compose services. Each step is a no-op when nothing matches, so a clean
# daemon succeeds. Containers go first because they hold volumes and images.
clean-harness:
	@set -e; l=label=trinkets.harness=1; \
	ids=$$(docker ps -aq --filter $$l); [ -z "$$ids" ] || docker rm -f $$ids >/dev/null; \
	ids=$$(docker network ls -q --filter $$l); [ -z "$$ids" ] || docker network rm $$ids >/dev/null; \
	ids=$$(docker volume ls -q --filter $$l); [ -z "$$ids" ] || docker volume rm -f $$ids >/dev/null; \
	ids=$$(docker image ls -aq --filter $$l); [ -z "$$ids" ] || docker rmi -f $$ids >/dev/null; \
	echo "harness objects removed"

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
