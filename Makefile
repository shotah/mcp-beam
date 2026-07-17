# mcp-beam Makefile — daily loop + release wrappers
#
# Run `make` or `make help` to see everything.

.DEFAULT_GOAL := help

.PHONY: help fmt vet lint test test-race test-short coverage check \
	build cli install tidy deps clean self-test \
	release-pack release-snapshot release version tools install-hooks

# Build-time version stamp (git describe). Release tags are tracked in ./VERSION.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || cat VERSION 2>/dev/null || echo dev)
LDFLAGS := -s -w -X go2tv.app/mcp-beam/internal/buildinfo.Version=$(VERSION)

# Release bump: patch (default), minor, or major. Or set TAG=v0.2.0 explicitly.
BUMP ?= patch

# Optional: `make test PKG=./internal/beam/...`
PKG ?= ./...

##@ Getting oriented

help: ## Show this help
	@echo.
	@echo Usage:  make ^<target^>
	@echo.
	@echo Getting oriented
	@echo   help                   Show this help
	@echo.
	@echo Daily loop (format -^> lint -^> test)
	@echo   fmt                    Format imports/code (goimports-reviser)
	@echo   vet                    Static analysis (go vet)
	@echo   lint                   Full lint suite (golangci-lint)
	@echo   test                   Unit tests (PKG=./path/... for one package)
	@echo   test-short             Unit tests with -short
	@echo   test-race              Unit tests with the race detector
	@echo   coverage               Coverage report for packages
	@echo   check                  Autofix, lint, and unit tests
	@echo.
	@echo Build ^& run
	@echo   build                  Compile all packages (sanity check)
	@echo   cli                    Build mcp-beam into ./bin/mcp-beam
	@echo   install                Install mcp-beam into GOPATH/bin
	@echo   self-test              Run mcp-beam --self-test
	@echo.
	@echo Modules ^& cleanup
	@echo   tidy                   Sync go.mod / go.sum with imports
	@echo   deps                   Download module deps
	@echo   clean                  Remove binaries and coverage artifacts
	@echo.
	@echo Release
	@echo   version                Show VERSION file + latest git tag / next patch
	@echo   release                Bump tag, update VERSION, push (BUMP=patch^|minor^|major)
	@echo   release-pack           Multi-arch archives via release-packager -^> ./dist
	@echo   release-snapshot       Local GoReleaser snapshot (no publish)
	@echo.
	@echo Tooling
	@echo   tools                  Install goimports-reviser + golangci-lint v2
	@echo   install-hooks          Install git pre-commit (autofix + lint + test)
	@echo.

##@ Daily loop (format → lint → test)

fmt: ## Autofix imports/code (goimports-reviser + golangci-lint fmt/fix)
	goimports-reviser -format -recursive .
	-golangci-lint fmt ./...
	-golangci-lint run --fix ./...

vet: ## Static analysis (go vet)
	go vet ./...

lint: ## Full lint suite (golangci-lint; no write)
	golangci-lint run ./...

test: ## Unit tests (PKG=./path/... for one package)
	go test $(PKG)

test-short: ## Unit tests with -short
	go test -short $(PKG)

test-race: ## Unit tests with the race detector
	go test -race $(PKG)

coverage: ## Tests + coverage report (writes coverage.out)
	go test -cover "-coverprofile=coverage.out" $(PKG)
	go tool cover "-func=coverage.out"

check: fmt lint test ## Autofix, lint, test

##@ Build & run

build: ## Compile all packages (sanity check; no binary kept)
	go build ./...

cli: ## Build mcp-beam into ./bin/mcp-beam
	mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/mcp-beam$(if $(filter Windows_NT,$(OS)),.exe,) .

install: ## Install mcp-beam into $$GOPATH/bin (or $$GOBIN)
	go install -ldflags "$(LDFLAGS)" .

self-test: ## Run wiring / dependency self-test
	go run -ldflags "$(LDFLAGS)" . --self-test

##@ Modules & cleanup

tidy: ## Sync go.mod / go.sum with imports
	go mod tidy

deps: ## Download module deps into the module cache
	go mod download

clean: ## Remove built binaries and coverage artifacts
	go clean ./...
ifeq ($(OS),Windows_NT)
	-cmd /C "rmdir /S /Q bin 2>NUL & rmdir /S /Q dist 2>NUL & del /Q coverage.out coverage.txt 2>NUL"
else
	rm -rf bin dist
	rm -f coverage.out coverage.txt
endif

##@ Release

version: ## Show VERSION file and latest git tag / next patch
	@go run ./cmd/release -dry-run

# Bump semver, commit VERSION, annotated-tag, push HEAD + tag (triggers GoReleaser).
# Examples:
#   make release
#   make release BUMP=minor
#   make release BUMP=major
#   make release TAG=v0.2.0
#   make release DRY_RUN=1
release: ## Bump version tag, update VERSION, push (BUMP=patch|minor|major)
	go run ./cmd/release \
		$(if $(TAG),-version=$(TAG),-bump=$(BUMP)) \
		$(if $(DRY_RUN),-dry-run,) \
		$(if $(SKIP_PUSH),-skip-push,) \
		$(if $(ALLOW_DIRTY),-allow-dirty,)

release-pack: ## Multi-arch archives via cmd/release-packager → ./dist
	go run ./cmd/release-packager -out dist

release-snapshot: ## Local GoReleaser snapshot build (no publish)
	goreleaser release --snapshot --clean

##@ Tooling

tools: ## Install goimports-reviser + golangci-lint v2 into $$GOBIN
	go install github.com/incu6us/goimports-reviser/v3@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@echo Installed tools. Ensure GOPATH/bin is on PATH, then: golangci-lint version

install-hooks: ## Install git pre-commit hook (autofix + lint + test)
ifeq ($(OS),Windows_NT)
	copy /Y scripts\pre-commit .git\hooks\pre-commit
else
	cp scripts/pre-commit .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit
endif
	@echo Installed .git/hooks/pre-commit
