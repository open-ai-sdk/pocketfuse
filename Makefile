SHELL := /bin/sh

APP_NAME ?= duckscope
BIN_DIR ?= bin
FRONTEND_DIR ?= web
GO ?= go
PNPM ?= pnpm
DOCKER ?= docker

DUCKSCOPE_DB_PATH ?= ./data/duckscope.db
DUCKSCOPE_HOST ?= 127.0.0.1
DUCKSCOPE_PORT ?= 3825

.PHONY: frontend-install frontend-build build run dev test vet check \
	docker-build docker-up docker-down docs-dev docs-build clean

frontend-install:
	@if [ -f "$(FRONTEND_DIR)/pnpm-lock.yaml" ]; then \
		cd "$(FRONTEND_DIR)" && $(PNPM) install --frozen-lockfile; \
	else \
		cd "$(FRONTEND_DIR)" && $(PNPM) install; \
	fi

frontend-build: frontend-install
	@cd "$(FRONTEND_DIR)" && $(PNPM) run build

build: frontend-build
	@mkdir -p "$(BIN_DIR)"
	CGO_ENABLED=1 $(GO) build -trimpath -ldflags='-s -w' -o "$(BIN_DIR)/$(APP_NAME)" ./cmd/duckscope

run: frontend-build
	DUCKSCOPE_DB_PATH="$(DUCKSCOPE_DB_PATH)" \
	DUCKSCOPE_HOST="$(DUCKSCOPE_HOST)" \
	DUCKSCOPE_PORT="$(DUCKSCOPE_PORT)" \
	CGO_ENABLED=1 $(GO) run ./cmd/duckscope

dev:
	air -c .air.toml

test: frontend-build
	$(GO) test ./...

vet: frontend-build
	$(GO) vet ./...

docker-build: frontend-build
	$(DOCKER) build -t $(APP_NAME):local .

docker-up: frontend-build
	$(DOCKER) compose up --build

docker-down:
	$(DOCKER) compose down

docs-dev:
	@cd docs && $(PNPM) install && $(PNPM) run dev

docs-build:
	@cd docs && $(PNPM) install --frozen-lockfile && $(PNPM) run build

clean:
	rm -rf "$(BIN_DIR)"

