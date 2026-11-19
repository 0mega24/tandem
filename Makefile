# tandem — Makefile
# Run `make` or `make help` to see the targets.

# ---- Shell and make behaviour ------------------------------------------------
SHELL         := bash
.SHELLFLAGS   := -euo pipefail -c
.ONESHELL:
.DELETE_ON_ERROR:
.DEFAULT_GOAL := help
MAKEFLAGS     += --warn-undefined-variables --no-builtin-rules --no-print-directory
.SUFFIXES:

# Every build and test runs at low CPU and IO priority, so the desktop stays usable while
# worktrees build. LOWPRIO=0 turns it off (benchmarks). demo is off by default: vhs takes a
# screenshot per frame, and at low priority it drops frames, so the gifs play too fast.
LOWPRIO ?= $(if $(filter demo,$(MAKECMDGOALS)),0,1)
ifeq ($(LOWPRIO),1)
$(shell renice -n 19 -p $$PPID >/dev/null 2>&1; ionice -c 3 -p $$PPID >/dev/null 2>&1)
endif
# Use the dev shell when not in it already.
IN_NIX_SHELL ?=
ifeq ($(IN_NIX_SHELL),)
DEV ?= nix develop -c
else
DEV ?=
endif

# ---- Project ----------------------------------------------------------------
BINARY   := tdm
MODULE   := $(shell awk '/^module /{print $$2; exit}' go.mod)
PKG      := ./cmd/$(BINARY)
BIN_DIR  := bin
DIST_DIR := dist
PREFIX   ?= $(HOME)/.local
DESTDIR  ?=

# ---- Version (from git; overridable: make VERSION=1.2.3 build) ---------------
# No leading v, as in the goreleaser and Nix builds: tag v0.1.0 is version 0.1.0.
VERSION ?= $(patsubst v%,%,$(shell git describe --tags --always --dirty 2>/dev/null || echo dev))
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(MODULE)/internal/version.Version=$(VERSION) \
	-X $(MODULE)/internal/version.Commit=$(COMMIT) \
	-X $(MODULE)/internal/version.Date=$(DATE)

# ---- Tools --------------------------------------------------------------------
GO          ?= go
BUILD_FLAGS ?= -trimpath
export CGO_ENABLED ?= 0
# GOWORK=off: -modfile does not work in workspace mode, for example in a tandem space with a go.work.
GOTOOL      := $(DEV) env GOWORK=off $(GO) tool -modfile=go.tool.mod
GOTESTSUM   := $(GOTOOL) gotestsum
GOLANGCI    ?= golangci-lint
NIXFMT      ?= nixfmt
TEST_FLAGS  ?=
RUN         ?=
ARGS        ?=
ID          ?=
MANIFEST    ?=

# Fail early with a clear message if a required tool is missing. Skipped when DEV enters the shell.
need = $(if $(DEV),,@command -v $(1) >/dev/null 2>&1 || { echo "error: '$(1)' not found. Run 'nix develop'."; exit 1; })

# Run a command and fail if it changed the working tree. Works on a dirty tree too: it compares
# the tree before and after, not against HEAD.
define unchanged
	before="$$(git status --porcelain -uall; git diff)"
	$(1)
	after="$$(git status --porcelain -uall; git diff)"
	if [[ "$$before" != "$$after" ]]; then
	  echo "error: $(2)"
	  git status --short
	  exit 1
	fi
endef

# ---- Help ---------------------------------------------------------------------
.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "; printf "Usage: make <target> [VAR=value]\n\n"} \
	  /^##@/ {printf "\n\033[1m%s\033[0m\n", substr($$0, 5)} \
	  /^[a-zA-Z0-9_.-]+:.*?## / {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

##@ Build
.PHONY: build
build: generate-check-quiet ## Build bin/tdm
	$(DEV) $(GO) build $(BUILD_FLAGS) -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY) $(PKG)

.PHONY: install
install: build ## Install to $(PREFIX)/bin (default ~/.local/bin)
	install -Dm755 $(BIN_DIR)/$(BINARY) $(DESTDIR)$(PREFIX)/bin/$(BINARY)

.PHONY: uninstall
uninstall: ## Remove the installed binary
	rm -f $(DESTDIR)$(PREFIX)/bin/$(BINARY)

.PHONY: run
run: ## Run tdm with ARGS, e.g. make run ARGS="status"
	$(DEV) $(GO) run $(BUILD_FLAGS) -ldflags '$(LDFLAGS)' $(PKG) $(ARGS)

##@ Code generation
.PHONY: generate
generate: ## Run go generate (registry, docs, schema, stringer)
	$(DEV) $(GO) generate ./...

.PHONY: generate-check
generate-check: ## Fail if go generate changes any file
	$(call unchanged,$(DEV) $(GO) generate ./...,generated files changed. Run 'make generate' and commit the result.)

# Used by build: runs generate once if the generated imports file is missing.
.PHONY: generate-check-quiet
generate-check-quiet:
	@if [[ -d internal/gen ]]; then test -f internal/gen/all/zz_generated_imports.go || $(MAKE) generate; fi

.PHONY: newgen
newgen: ## Scaffold a generator: make newgen ID=zig MANIFEST=build.zig.zon
	@test -n "$(ID)" || { echo "error: set ID, e.g. make newgen ID=zig MANIFEST=build.zig.zon"; exit 1; }
	@test -n "$(MANIFEST)" || { echo "error: set MANIFEST"; exit 1; }
	$(DEV) $(GO) run ./tools/newgen -id '$(ID)' -manifest '$(MANIFEST)'
	$(MAKE) generate

##@ Quality
.PHONY: fmt
fmt: ## Format Go (gofumpt, gci) and Nix files
	$(DEV) $(GOLANGCI) fmt
	git ls-files -z '*.nix' | xargs -0r $(DEV) $(NIXFMT)

.PHONY: fmt-check
fmt-check: ## Fail if any file needs formatting
	$(DEV) $(GOLANGCI) fmt --diff
	git ls-files -z '*.nix' | xargs -0r $(DEV) $(NIXFMT) --check

.PHONY: typos
typos: ## Check spelling in code and docs
	$(DEV) typos

.PHONY: lint
lint: lint-go lint-nix lint-actions ## Run all linters (Go, Nix, GitHub Actions)

.PHONY: lint-go
lint-go: ## golangci-lint, with the depguard structure rules
	$(DEV) $(GOLANGCI) run ./...

.PHONY: lint-nix
lint-nix: ## statix (anti-patterns) and deadnix (unused code)
	$(DEV) statix check -o errfmt .
	$(DEV) deadnix --fail .

.PHONY: lint-actions
lint-actions: ## actionlint (syntax) and zizmor (security) on the workflows
	$(DEV) actionlint
	$(DEV) zizmor --offline --quiet .github/workflows

.PHONY: fix
fix: ## Apply every automatic fix: lint fixes, format, spelling, tidy modules
	$(DEV) $(GOLANGCI) run --fix ./...
	$(DEV) $(GOLANGCI) fmt
	$(DEV) statix fix .
	$(DEV) deadnix --edit .
	git ls-files -z '*.nix' | xargs -0r $(DEV) $(NIXFMT)
	$(DEV) typos --write-changes
	$(DEV) $(GO) mod tidy
	$(DEV) $(GO) mod tidy -modfile=go.tool.mod
	@echo "Review the changes with 'git diff' before you commit."

.PHONY: vet
vet: ## Run go vet
	$(DEV) $(GO) vet ./...

.PHONY: vuln
vuln: ## Check dependencies for known vulnerabilities
	$(GOTOOL) govulncheck ./...

.PHONY: audit
audit: vuln ## Supply chain checks (govulncheck)

.PHONY: tidy
tidy: ## Tidy go.mod and go.tool.mod
	$(DEV) $(GO) mod tidy
	$(DEV) $(GO) mod tidy -modfile=go.tool.mod

.PHONY: tidy-check
tidy-check: ## Fail if go.mod or go.tool.mod are not tidy
	$(call unchanged,$(DEV) $(GO) mod tidy && $(DEV) $(GO) mod tidy -modfile=go.tool.mod,modules not tidy. Run 'make tidy' and commit the result.)

##@ Tests
.PHONY: test
test: ## Unit, golden and testscript tests (RUN=regex to filter)
	$(GOTESTSUM) -- $(if $(RUN),-run '$(RUN)') $(TEST_FLAGS) ./...

.PHONY: test-race
test-race: ## Tests with the race detector
	CGO_ENABLED=1 $(GOTESTSUM) -- -race $(TEST_FLAGS) ./...

.PHONY: test-script
test-script: ## End-to-end testscript tests only (needs git only)
	$(GOTESTSUM) -- -run 'TestScript' $(TEST_FLAGS) ./internal/cli/...

.PHONY: test-integration
test-integration: ## Toolchain tests (cargo, pnpm, gradle, ...); skips missing tools
	$(GOTESTSUM) -- -tags integration -run TestIntegration -timeout 30m $(TEST_FLAGS) ./internal/cli/

.PHONY: golden-update
golden-update: ## Rewrite golden files from current output
	$(DEV) $(GO) test ./internal/gen/... -update
	@echo "Review the changes with 'git diff' before you commit."

.PHONY: cover
cover: ## Coverage report in coverage.html
	$(GOTESTSUM) -- -coverprofile=coverage.out -covermode=atomic ./...
	$(DEV) $(GO) tool cover -func=coverage.out | tail -n 1
	$(DEV) $(GO) tool cover -html=coverage.out -o coverage.html

##@ Nix
.PHONY: nix-build
nix-build: ## Build the Nix package (result/bin/tdm)
	nix build .#default -L

.PHONY: nix-check
nix-check: ## Run nix flake check
	nix flake check -L

.PHONY: nix-vendor-hash
nix-vendor-hash: ## Print the steps to get the new vendorHash after go.mod changes
	@echo "Set vendorHash = lib.fakeHash in nix/package.nix, then run:"
	@echo "  nix build .#default 2>&1 | grep 'got:'"

##@ Docs
.PHONY: demo
# The build uses the release version from internal/version/VERSION, so no git hash shows.
demo: ## Record docs/demo/tandem.gif and the README feature gifs (vhs from nixpkgs)
	$(MAKE) build VERSION="$$(cat internal/version/VERSION)"
	PATH="$(CURDIR)/$(BIN_DIR):$$PATH" FONTCONFIG_FILE="$(CURDIR)/docs/demo/fonts.conf" \
	  nix shell nixpkgs#vhs nixpkgs#zsh -c sh -c 'for t in tandem status dryrun tui; do vhs docs/demo/$$t.tape || exit 1; done'
	rm -rf /tmp/tandem-demo

##@ Release
.PHONY: snapshot
snapshot: ## Build archives, .deb, .rpm and pacman packages locally in dist/ (no publish)
	$(call need,goreleaser)
	$(DEV) goreleaser release --snapshot --clean

.PHONY: release
release: ## Publish a draft GitHub release for the current v* tag (the release workflow runs it)
	$(call need,goreleaser)
	$(DEV) goreleaser release --clean

##@ Aggregate
.PHONY: quick
quick: ## No-compile checks: generated files, tidy modules, format, spelling (serial: they compare the tree)
	$(MAKE) -j1 generate-check tidy-check fmt-check typos

.PHONY: check
check: ## Merge gate: quick, then vet, lint, test, build in parallel (what CI's check job runs)
	$(MAKE) quick
	$(MAKE) -j4 --output-sync=target vet lint test build

.PHONY: ci
ci: check test-race vuln ## Full CI set, except integration

.PHONY: check-full
check-full: ci test-integration ## Everything, including the toolchain tests

.PHONY: clean
clean: ## Remove build output
	rm -rf $(BIN_DIR) $(DIST_DIR) coverage.out coverage.html result
