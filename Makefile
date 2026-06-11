# gh-hound — fast, focused GitHub Actions TUI

BINARY      := gh-hound
MODULE      := github.com/indrasvat/gh-hound
BUILD_DIR   := bin
OUT_DIR     := out
CMD_DIR     := ./cmd/gh-hound
INSTALL_DIR := $(HOME)/.local/bin

VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE        := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
GO_VERSION  := $(shell go version | cut -d' ' -f3)
LDFLAGS     := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

GOBIN       := $(shell go env GOPATH)/bin
GOLANGCI    := $(shell command -v golangci-lint 2>/dev/null || echo "$(GOBIN)/golangci-lint")
GOTESTSUM   := $(shell command -v gotestsum 2>/dev/null || echo "$(GOBIN)/gotestsum")
GOIMPORTS   := $(shell command -v goimports 2>/dev/null || echo "$(GOBIN)/goimports")
ACTIONLINT  := $(shell command -v actionlint 2>/dev/null || echo "$(GOBIN)/actionlint")
LEFTHOOK    := $(shell command -v lefthook 2>/dev/null || echo "$(GOBIN)/lefthook")

COLOR_RESET := \033[0m
COLOR_BOLD  := \033[1m
COLOR_GREEN := \033[32m
COLOR_BLUE  := \033[34m
COLOR_RED   := \033[31m
COLOR_DIM   := \033[2m

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show grouped help
	@printf "\n$(COLOR_BOLD)gh-hound$(COLOR_RESET) $(COLOR_DIM)— GitHub Actions CI hound$(COLOR_RESET)\n\n"
	@printf "$(COLOR_BOLD)Usage:$(COLOR_RESET) make $(COLOR_GREEN)<target>$(COLOR_RESET)\n\n"
	@printf "$(COLOR_BOLD)Build & Run$(COLOR_RESET)\n"
	@awk 'BEGIN {FS = ":.*##"} /^(build|install|run|clean):.*?##/ {printf "  $(COLOR_GREEN)%-18s$(COLOR_RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@printf "\n$(COLOR_BOLD)Quality$(COLOR_RESET)\n"
	@awk 'BEGIN {FS = ":.*##"} /^(fmt|fmt-check|gofix|gofix-check|lint|vet|test|coverage|coverage-check|docs-check|check|ci|emoji-check|arch-check|visual-contract-check|workflow-check|shellcheck):.*?##/ {printf "  $(COLOR_GREEN)%-18s$(COLOR_RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@printf "\n$(COLOR_BOLD)Verification$(COLOR_RESET)\n"
	@awk 'BEGIN {FS = ":.*##"} /^(e2e|vqa|vqa-screen|vqa-clean|demo|smoke-test):.*?##/ {printf "  $(COLOR_GREEN)%-18s$(COLOR_RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@printf "\n$(COLOR_BOLD)Tooling & Release$(COLOR_RESET)\n"
	@awk 'BEGIN {FS = ":.*##"} /^(tools|tools-ci|hooks|hooks-run|release-check|snapshot|release-prep):.*?##/ {printf "  $(COLOR_GREEN)%-18s$(COLOR_RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@printf "\n"

.PHONY: build
build: ## Build binary into bin/
	@printf "$(COLOR_BLUE)▶ building $(BINARY)$(COLOR_RESET)\n"
	@mkdir -p $(BUILD_DIR)
	@go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) $(CMD_DIR)
	@printf "$(COLOR_GREEN)✓ built $(BUILD_DIR)/$(BINARY)$(COLOR_RESET)\n"

.PHONY: install
install: build ## Install binary to ~/.local/bin
	@install -d $(INSTALL_DIR)
	@install -m 755 $(BUILD_DIR)/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@printf "$(COLOR_GREEN)✓ installed $(INSTALL_DIR)/$(BINARY)$(COLOR_RESET)\n"

.PHONY: run
run: ## Run the scaffolded CLI
	@go run -ldflags "$(LDFLAGS)" $(CMD_DIR)

.PHONY: fmt
fmt: ## Format Go files
	@gofmt -w .
	@if [ -x "$(GOIMPORTS)" ]; then "$(GOIMPORTS)" -w .; fi

.PHONY: fmt-check
fmt-check: ## Check formatting without writing
	@unformatted="$$(gofmt -l . | grep -v '^.claude/worktrees/' || true)"; \
	if [ -n "$$unformatted" ]; then \
		printf "$(COLOR_RED)gofmt needed:$(COLOR_RESET)\n%s\n" "$$unformatted"; \
		exit 1; \
	fi

.PHONY: gofix
gofix: ## Apply modern Go fixes and tidy modules
	@printf "$(COLOR_BLUE)▶ running go fix$(COLOR_RESET)\n"
	@go fix ./...
	@go mod tidy
	@printf "$(COLOR_GREEN)✓ go fix complete$(COLOR_RESET)\n"

.PHONY: gofix-check
gofix-check: ## Fail if go fix would change tracked or staged Go files
	@./scripts/gofix-check.sh

.PHONY: lint
lint: ## Run golangci-lint
	@[ -x "$(GOLANGCI)" ] || { printf "$(COLOR_RED)missing golangci-lint; run make tools-ci$(COLOR_RESET)\n"; exit 1; }
	@"$(GOLANGCI)" run ./...

.PHONY: vet
vet: ## Run go vet
	@go vet ./...

.PHONY: test
test: ## Run tests with race detector and coverage
	@[ -x "$(GOTESTSUM)" ] || { printf "$(COLOR_RED)missing gotestsum; run make tools-ci$(COLOR_RESET)\n"; exit 1; }
	@"$(GOTESTSUM)" --format=standard-verbose -- -race -shuffle=on -coverprofile=coverage.out -covermode=atomic -count=1 ./...

.PHONY: coverage
coverage: test ## Render coverage report
	@go tool cover -func=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@printf "$(COLOR_GREEN)✓ coverage.html$(COLOR_RESET)\n"

.PHONY: coverage-check
coverage-check: test ## Check minimum total coverage for current scaffold
	@total="$$(go tool cover -func=coverage.out | awk '/^total:/ {gsub("%","",$$3); print $$3}')"; \
	printf "total coverage: %s%%\n" "$$total"; \
	awk -v cov="$$total" 'BEGIN { if (cov < 1.0) exit 1 }'

.PHONY: emoji-check
emoji-check: ## Fail on emoji variation selectors or astral-plane codepoints in Go files
	@./scripts/check-no-emoji.sh

.PHONY: arch-check
arch-check: ## Check current architecture import boundaries
	@./scripts/check-architecture.sh

.PHONY: visual-contract-check
visual-contract-check: ## Check implementation constants against the HTML visual contract
	@./scripts/visual-contract-check.sh

.PHONY: docs-check
docs-check: ## Verify README/docs commands and required sections
	@./scripts/docs-check.sh

.PHONY: workflow-check
workflow-check: ## Validate GitHub Actions workflows with actionlint
	@[ -x "$(ACTIONLINT)" ] || { printf "$(COLOR_RED)missing actionlint; run make tools-ci$(COLOR_RESET)\n"; exit 1; }
	@"$(ACTIONLINT)" .github/workflows/*.yml

.PHONY: shellcheck
shellcheck: ## Lint shell scripts
	@command -v shellcheck >/dev/null 2>&1 || { printf "$(COLOR_RED)missing shellcheck$(COLOR_RESET)\n"; exit 1; }
	@shellcheck install.sh scripts/*.sh

.PHONY: check
check: gofix-check fmt-check lint vet workflow-check shellcheck emoji-check arch-check visual-contract-check docs-check test ## Run local green-bar gate
	@printf "\n$(COLOR_GREEN)$(COLOR_BOLD)✓ check passed$(COLOR_RESET)\n\n"

.PHONY: ci
ci: check build ## Run the CI gate
	@printf "\n$(COLOR_GREEN)$(COLOR_BOLD)✓ ci passed$(COLOR_RESET)\n\n"

.PHONY: e2e
e2e: ## Run end-to-end tests
	@printf "$(COLOR_BLUE)▶ e2e pending Task 090+$(COLOR_RESET)\n"
	@go test -race -count=1 ./...

.PHONY: vqa
vqa: ## Run shux visual-quality audit
	@./.claude/automations/vqa.sh
	@./.claude/automations/interaction_audit.sh

.PHONY: vqa-screen
vqa-screen: ## Run VQA for one screen: make vqa-screen SCREEN=runs
	@SCREEN=$(SCREEN) ./.claude/automations/vqa.sh

.PHONY: vqa-clean
vqa-clean: ## Remove VQA screenshots and captures
	@find .claude/automations/screenshots -mindepth 1 ! -name .gitkeep ! -name .gitignore -exec rm -rf {} +
	@printf "$(COLOR_GREEN)✓ cleaned VQA artifacts$(COLOR_RESET)\n"

.PHONY: demo
demo: build ## Record README demo with VHS
	@command -v vhs >/dev/null 2>&1 || { printf "$(COLOR_RED)missing vhs$(COLOR_RESET)\n"; exit 1; }
	@vhs assets/demo.tape

.PHONY: smoke-test
smoke-test: build ## Smoke test local binary
	@./scripts/smoke-test.sh

.PHONY: tools
tools: tools-ci ## Install all local development tools
	@GOTOOLCHAIN=$(GO_VERSION) go install github.com/evilmartians/lefthook@latest
	@GOTOOLCHAIN=$(GO_VERSION) go install golang.org/x/tools/cmd/goimports@latest

.PHONY: tools-ci
tools-ci: ## Install CI/dev tools required by gates
	@GOTOOLCHAIN=$(GO_VERSION) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.8.0
	@GOTOOLCHAIN=$(GO_VERSION) go install gotest.tools/gotestsum@latest
	@GOTOOLCHAIN=$(GO_VERSION) go install github.com/rhysd/actionlint/cmd/actionlint@latest

.PHONY: hooks
hooks: ## Install lefthook git hooks
	@[ -x "$(LEFTHOOK)" ] || { printf "$(COLOR_RED)missing lefthook; run make tools$(COLOR_RESET)\n"; exit 1; }
	@"$(LEFTHOOK)" install

.PHONY: hooks-run
hooks-run: ## Run lefthook pre-commit and pre-push locally
	@"$(LEFTHOOK)" run pre-commit
	@"$(LEFTHOOK)" run pre-push

.PHONY: release-check
release-check: ## Validate CI/release/install configuration
	@./scripts/release-check.sh

.PHONY: snapshot
snapshot: release-check ## Build local release artifacts into dist/
	@VERSION="$(VERSION)" COMMIT="$(COMMIT)" DATE="$(DATE)" scripts/build-release.sh "$(VERSION)"
	@printf "$(COLOR_GREEN)✓ snapshot in dist/$(COLOR_RESET)\n"

.PHONY: release-prep
release-prep: ci e2e docs-check vqa smoke-test release-check snapshot ## Run release preparation gate
	@printf "$(COLOR_GREEN)$(COLOR_BOLD)✓ release prep passed for current scaffold$(COLOR_RESET)\n"

.PHONY: clean
clean: ## Remove local build artifacts
	@rm -rf $(BUILD_DIR) $(OUT_DIR) coverage.out coverage.html dist/
	@printf "$(COLOR_GREEN)✓ cleaned$(COLOR_RESET)\n"
