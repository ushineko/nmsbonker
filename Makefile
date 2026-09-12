default: help

.PHONY: help
help: ## Show this help
	@echo
	@echo "Available commands:"
	@echo
	@awk -F ':|##' '/^[^\t].+?:.*?##/ {printf "\033[36m%-30s\033[0m %s\n", $$1, $$NF}' $(MAKEFILE_LIST)

BINDIR=$(shell go env GOPATH)
MODULE=github.com/ushineko/nmsbonker
VERSION?=$(shell cat VERSION 2>/dev/null || echo dev)
COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS=-w -s -X $(MODULE)/internal/buildinfo.Version=$(VERSION) -X $(MODULE)/internal/buildinfo.Commit=$(COMMIT)

LINT_NAME?=golangci-lint
LINT_VERSION?=v2.12.2
LINT_PROGRAM=$(LINT_NAME)-$(LINT_VERSION)

# Release asset coordinates for the pinned linter version.
# (The upstream install.sh is not used: its checksum extraction matches the
# .sbom.json asset line and fails verification on recent releases.)
LINT_VERSION_NUM=$(LINT_VERSION:v%=%)
LINT_BASE_URL=https://github.com/golangci/golangci-lint/releases/download/$(LINT_VERSION)

.PHONY: install-lint
install-lint: $(BINDIR)/bin/$(LINT_PROGRAM) ## Install linter

$(BINDIR)/bin/$(LINT_PROGRAM):
	@echo "Setting up $(LINT_PROGRAM) ..."
	@set -e; \
	os=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
	arch=$$(uname -m); \
	case "$$arch" in x86_64) arch=amd64;; aarch64|arm64) arch=arm64;; esac; \
	dist="$(LINT_NAME)-$(LINT_VERSION_NUM)-$$os-$$arch"; \
	tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	curl -fsSL "$(LINT_BASE_URL)/$$dist.tar.gz" -o "$$tmp/$$dist.tar.gz"; \
	curl -fsSL "$(LINT_BASE_URL)/$(LINT_NAME)-$(LINT_VERSION_NUM)-checksums.txt" -o "$$tmp/checksums.txt"; \
	want=$$(awk -v f="$$dist.tar.gz" '$$2 == f {print $$1}' "$$tmp/checksums.txt"); \
	got=$$( (sha256sum "$$tmp/$$dist.tar.gz" 2>/dev/null || shasum -a 256 "$$tmp/$$dist.tar.gz") | awk '{print $$1}'); \
	if [ -z "$$want" ] || [ "$$want" != "$$got" ]; then echo "checksum mismatch for $$dist.tar.gz: want '$$want' got '$$got'"; exit 1; fi; \
	tar -C "$$tmp" -xzf "$$tmp/$$dist.tar.gz"; \
	mkdir -p "$(BINDIR)/bin"; \
	mv -v "$$tmp/$$dist/$(LINT_NAME)" "$(BINDIR)/bin/$(LINT_PROGRAM)"

.PHONY: setup
setup: install-lint ## Setup system for local development
	@echo "Make sure your system path includes GOPATH/bin. See README.md for details."

# golangci-lint type-checks against the standard library sources of whichever Go
# it finds, using a go/types built into the linter binary. A linter built with
# Go 1.26 panics outright ("file requires newer Go version go1.27") on a machine
# whose GOROOT is 1.27. go.mod deliberately carries no `toolchain` line (R1.1),
# so the pin lives here instead, matching the Go that this linter release was
# built with. Bump it together with LINT_VERSION.
LINT_GO_TOOLCHAIN?=go1.26.0

.PHONY: lint
lint: export GOTOOLCHAIN = $(LINT_GO_TOOLCHAIN)
lint: install-lint ## Lint files
	@go version
	$(BINDIR)/bin/$(LINT_PROGRAM) run --timeout 5m0s --config config/.golangci-$(LINT_VERSION).yml ./...

# The parity test carries a build tag so that a plain `go test ./...` does not
# need it, and `make test` always does: it is the guard that the CLI and the GUI
# expose the same operations, and a guard that runs only when someone remembers
# to ask for it is not a guard. It needs no display.
.PHONY: test
test: ## Run unit tests with the race detector (fast, no build)
	@go test -race -tags parity ./...

.PHONY: coverage
coverage: ## Run all tests and open a coverage report in the default browser
	@go test -coverprofile coverage.out ./...
	@go tool cover -html=coverage.out
	@rm -f coverage.out

.PHONY: build
build: ## Build the CLI for the host platform (no CGO: the CLI needs no display)
	CGO_ENABLED=0 go build -ldflags='$(LDFLAGS)' -trimpath -o nmsbonker ./cmd/nmsbonker

.PHONY: build-gui
build-gui: ## Build the desktop front end for the host platform (requires CGO)
	CGO_ENABLED=1 go build -ldflags='$(LDFLAGS)' -trimpath -o nmsbonker-gui ./cmd/nmsbonker-gui

.PHONY: build-all
build-all: ## Build static CLI binaries for every platform, plus the host's GUI
	@set -e; for p in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do \
		os=$${p%/*}; arch=$${p#*/}; \
		echo "building $$os/$$arch ..."; \
		mkdir -p dist; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
			go build -ldflags='$(LDFLAGS)' -trimpath -o dist/nmsbonker-$$os-$$arch ./cmd/nmsbonker; \
	done
	@# The GUI is built for this machine only. It needs CGO, so cross-compiling
	@# it would need a C toolchain per target. Nothing about the tool's job
	@# depends on the difference: the CLI builds and deploys mods on its own.
	@echo "building the GUI for this host ..."; \
	if CGO_ENABLED=1 go build -ldflags='$(LDFLAGS)' -trimpath \
		-o dist/nmsbonker-gui-$$(go env GOOS)-$$(go env GOARCH) ./cmd/nmsbonker-gui; then \
		echo "  built dist/nmsbonker-gui-$$(go env GOOS)-$$(go env GOARCH)"; \
	else \
		echo "  the GUI did not build; the CLI binaries are unaffected" >&2; \
	fi
	@ls -l dist/

.PHONY: clean
clean: ## Remove build artifacts
	@rm -rf nmsbonker nmsbonker-gui dist/ coverage.out count.out
