# Conway — canonical task interface. Self-contained: a fresh clone needs only
# this file. Local conveniences (secrets, shortcuts) live in scripts/local/
# (git-excluded) and just call these targets.
#
# Fastest way to try Conway: `docker compose up` (app + Postgres, seeded with
# a small demo org). The targets below are for local Go development against
# a Postgres you already have running (e.g. `docker compose up -d postgres`).
SHELL := /bin/bash
PORT ?= 8741
APP_DIR := app
RUN := .run
SERVER_PID := $(RUN)/server.pid
DATABASE_URL ?= postgres://conway:conway@localhost:5432/conway?sslmode=disable

.PHONY: help build test server stop status logs clean

help: ## Show this help
	@echo "Conway targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-10s\033[0m %s\n",$$1,$$2}'
	@echo
	@echo "Vars: PORT=$(PORT)  DATABASE_URL=$(DATABASE_URL)  (CONWAY_ADMIN_PASSWORD for 'server')"

$(RUN):
	@mkdir -p $(RUN)

build: ## Build the Go server binary (server/conway)
	cd server && go build -o conway .

test: ## Run engine (JS) and auth (Go) tests
	node --test tests/sim.test.mjs
	cd server && go test ./...

server: build | $(RUN) ## Go server against DATABASE_URL (needs Postgres reachable — see docker-compose.yml)
	@CONWAY_ADDR=:$(PORT) CONWAY_APP_DIR=$(APP_DIR) DATABASE_URL=$(DATABASE_URL) \
	  nohup server/conway >$(RUN)/server.log 2>&1 & echo $$! >$(SERVER_PID)
	@sleep 1; echo "conway server -> http://localhost:$(PORT)  (pid $$(cat $(SERVER_PID)))"
	@grep -i 'admin password' $(RUN)/server.log || true

stop: ## Stop the server started by `make server`
	@if [ -f $(SERVER_PID) ]; then kill $$(cat $(SERVER_PID)) 2>/dev/null && echo "stopped $$(cat $(SERVER_PID))"; rm -f $(SERVER_PID); fi

status: ## Show whether the server is running
	@if [ -f $(SERVER_PID) ] && kill -0 $$(cat $(SERVER_PID)) 2>/dev/null; then echo "up: $(SERVER_PID) ($$(cat $(SERVER_PID)))"; \
	  else echo "down: $(SERVER_PID)"; fi

logs: ## Tail the server log
	@tail -n 40 -f $(RUN)/server.log

clean: stop ## Stop, remove binary, logs, pids, pycaches
	@rm -f server/conway; rm -rf $(RUN)
	@find . -name __pycache__ -type d -prune -exec rm -rf {} + 2>/dev/null || true
	@echo cleaned
