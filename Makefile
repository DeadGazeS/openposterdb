.PHONY: build up down restart logs clean shell env pull

COMPOSE := docker compose
FILES  := -f docker-compose.yml -f docker-compose.local.yml
VERSION := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
APP_VERSION := 1.2.1-dev-$(VERSION)
export APP_VERSION

pull:
	$(COMPOSE) $(FILES) pull

build:
	$(COMPOSE) $(FILES) build --no-cache --build-arg APP_VERSION=$(APP_VERSION)

nuke:
	$(COMPOSE) $(FILES) down --rmi all -v
	docker system prune -af --volumes

rebuild: nuke build

up:
	$(COMPOSE) $(FILES) build --build-arg APP_VERSION=$(APP_VERSION)
	$(COMPOSE) $(FILES) up -d

up-fast:
	$(COMPOSE) $(FILES) up -d

down:
	$(COMPOSE) $(FILES) down

restart: down up

logs:
	$(COMPOSE) $(FILES) logs -f

clean:
	$(COMPOSE) $(FILES) down -v

shell:
	$(COMPOSE) $(FILES) exec openposterdb sh

env:
	@if [ ! -f .env ]; then cp .env.example .env; fi
	@if ! grep -qE '^JWT_SECRET=.+' .env 2>/dev/null; then \
		SEC=$$(openssl rand -hex 32); \
		sed -i "s/^JWT_SECRET=.*/JWT_SECRET=$${SEC}/" .env; \
		echo "JWT_SECRET auto-generated"; \
	fi
	@echo ".env ready — set your TMDB_API_KEY and other secrets in .env"
