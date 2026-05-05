## iris — KumoMTA operator console / dev + test orchestration
##
## Day-to-day:
##   make up        # base stack only — timescaledb, redis, kumomta, admin-service, frontend
##   make down      # stop the base stack but keep volumes
##   make ps        # show what's running + port mappings
##   make logs s=admin-service   # tail one service (s= name)
##
## Test harness:
##   make test      # one-shot: spins up mocks + base stack, runs loadgen, returns its exit code
##                  # the base stack STAYS UP after loadgen exits — so the UI keeps working
##   make test-clean   # tear down the mocks (keeps the base stack)
##
## Rebuilds:
##   make rebuild s=admin-service   # rebuild + restart one image without touching the rest
##   make rebuild-all                # rebuild every image (slow)
##
## Host-native artefacts (for systemd install — see docs/host-native-deploy.md):
##   make host-build       # build admin-service binary with embedded SPA into ./out/
##   make host-package     # tar the binary + configs + unit file into ./out/iris-host.tar.gz

COMPOSE      := docker compose -f docker-compose.yaml
COMPOSE_TEST := docker compose -f docker-compose.yaml -f docker-compose.test.yaml

.DEFAULT_GOAL := help
.PHONY: help up down ps logs restart rebuild rebuild-all test test-clean nuke host-build host-package

help:
	@grep -E '^##' $(MAKEFILE_LIST) | sed -e 's/^## \{0,1\}//'

up:
	$(COMPOSE) up -d

down:
	$(COMPOSE) down

ps:
	$(COMPOSE) ps

logs:
ifndef s
	$(COMPOSE) logs --tail 100 -f
else
	$(COMPOSE) logs --tail 200 -f $(s)
endif

restart:
ifndef s
	$(error pass s=<service>, e.g. make restart s=kumomta)
endif
	$(COMPOSE) restart $(s)

rebuild:
ifndef s
	$(error pass s=<service>, e.g. make rebuild s=admin-service)
endif
	$(COMPOSE) build $(s)
	$(COMPOSE) up -d --no-deps $(s)

rebuild-all:
	$(COMPOSE) build
	$(COMPOSE) up -d

## test: bring up the full stack including 5 SMTP mocks, then run loadgen ONCE.
## loadgen exits with the assertion verdict (0 = green); the rest of the stack
## stays up so you can poke at /observability/logs after.
test:
	$(COMPOSE_TEST) --profile test up -d --build \
	    timescaledb redis kumomta admin-service \
	    mock-mta-accept mock-mta-bounce mock-mta-defer mock-mta-slow mock-mta-fbl
	$(COMPOSE_TEST) --profile test run --rm loadgen

test-clean:
	$(COMPOSE_TEST) --profile test rm -sf \
	    mock-mta-accept mock-mta-bounce mock-mta-defer mock-mta-slow mock-mta-fbl loadgen

## nuke: stop everything AND drop volumes — wipes timescaledb + spool + dkim keys.
## Use when migrations have drifted past what's safe to incrementally re-apply.
nuke:
	$(COMPOSE_TEST) --profile test down -v

# ── host-native build ───────────────────────────────────────────────
# Produces a static admin-service binary with the Vue SPA embedded,
# ready for systemd install per docs/host-native-deploy.md. Runs the
# frontend build first so pkg/spa/dist/ is populated before the Go
# build picks it up. Output lands in ./out/.
host-build:
	@echo "→ pnpm install + build (frontend)"
	cd ../frontend && pnpm install --frozen-lockfile && pnpm build:antd
	@echo "→ copying frontend dist into pkg/spa/dist"
	rm -rf ../backend/pkg/spa/dist
	mkdir -p ../backend/pkg/spa/dist
	cp -r ../frontend/apps/admin/dist/. ../backend/pkg/spa/dist/
	@echo "→ go build (static, embedded SPA)"
	mkdir -p out
	cd ../backend && CGO_ENABLED=0 GOOS=linux \
	    go build -trimpath \
	    -ldflags "-s -w -X main.version=$$(date +%Y%m%d-%H%M%S)" \
	    -o $$PWD/../deploy/out/iris ./app/admin/service/cmd/server
	@echo "→ artefacts in ./out/"
	@ls -lh out/iris

# ── host-native packaging ───────────────────────────────────────────
# Bundles the binary, sample systemd unit, env file template, and
# host-targeted config dir into a single tarball. Operators extract
# under / and follow docs/host-native-deploy.md from step 4.
host-package: host-build
	@echo "→ packaging tarball"
	mkdir -p out/pkg/usr/local/bin out/pkg/etc/systemd/system out/pkg/etc/iris
	cp out/iris out/pkg/usr/local/bin/
	cp systemd/iris.service out/pkg/etc/systemd/system/
	cp systemd/iris.env.example out/pkg/etc/iris/
	cp -r systemd/configs out/pkg/etc/iris/
	cp -r ../backend/sql out/pkg/etc/iris/sql
	tar -czf out/iris-host.tar.gz -C out/pkg .
	rm -rf out/pkg
	@ls -lh out/iris-host.tar.gz
