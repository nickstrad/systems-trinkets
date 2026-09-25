# Local backing services for lessons. Each service runs on its own; start only what a lesson needs.
#
#   make up-<service>     start in the background and wait until healthy
#   make down-<service>   stop and remove containers, keep data
#   make clean-<service>  stop and delete containers and data volumes
#   make logs-<service>   follow logs
#   make ps               show running trinkets services
#
# Services: postgres, valkey, seaweedfs (see services/index.md)

SERVICES := postgres valkey seaweedfs

compose_file_postgres  := services/postgres.compose.yaml
compose_file_valkey    := services/valkey.compose.yaml
compose_file_seaweedfs := services/seaweedfs/compose.yaml

compose = docker compose -f $(compose_file_$(1))

.PHONY: help ps

help:
	@sed -n 's/^#   /  /p' Makefile
	@echo "  services: $(SERVICES)"

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
