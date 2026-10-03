SHELL := /bin/sh

APP_NAME ?= pocketfuse
BIN_DIR ?= bin
FRONTEND_DIR ?= web
GO ?= go
PNPM ?= pnpm
DOCKER ?= docker

POCKETFUSE_DB_PATH ?= ./data/pocketfuse.db
POCKETFUSE_HOST ?= 127.0.0.1
POCKETFUSE_PORT ?= 7625

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
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags='-s -w' -o "$(BIN_DIR)/$(APP_NAME)" ./cmd/pocketfuse

run: frontend-build
	POCKETFUSE_DB_PATH="$(POCKETFUSE_DB_PATH)" \
	POCKETFUSE_HOST="$(POCKETFUSE_HOST)" \
	POCKETFUSE_PORT="$(POCKETFUSE_PORT)" \
	CGO_ENABLED=0 $(GO) run ./cmd/pocketfuse

dev:
	air -c .air.toml

test: frontend-build
	$(GO) test ./...

vet: frontend-build
	$(GO) vet ./...

# Host arch for the local Linux binary staged into the Docker context
# (GoReleaser layout: linux/<arch>/pocketfuse).
HOST_ARCH := $(shell uname -m | sed -e s/aarch64/arm64/ -e s/x86_64/amd64/)

docker-build: frontend-build
	@mkdir -p build/docker/linux/$(HOST_ARCH)
	CGO_ENABLED=0 GOOS=linux GOARCH=$(HOST_ARCH) $(GO) build \
	    -trimpath -ldflags='-s -w' \
	    -o build/docker/linux/$(HOST_ARCH)/$(APP_NAME) ./cmd/$(APP_NAME)
	cp Dockerfile build/docker/Dockerfile
	$(DOCKER) build -t $(APP_NAME):local build/docker

docker-up: docker-build
	POCKETFUSE_IMAGE=$(APP_NAME):local $(DOCKER) compose up

docker-down:
	$(DOCKER) compose down

docs-dev:
	@cd docs && $(PNPM) install && $(PNPM) run dev

docs-build:
	@cd docs && $(PNPM) install --frozen-lockfile && $(PNPM) run build

clean:
	rm -rf "$(BIN_DIR)" build

