.PHONY: build clean clean-tools clean-all install test run fmt vet golangci-lint lint install-tools goreleaser-check goreleaser-snapshot help

# Build variables
BINARY_NAME=firectl
BUILD_DIR=bin/build
TOOLS_DIR=bin/tools
GO=go
GOFLAGS=-ldflags="-s -w"

# Tools
GOLANGCI_LINT=$(TOOLS_DIR)/golangci-lint
GORELEASER=$(TOOLS_DIR)/goreleaser

build: ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

clean: ## Remove build artifacts (keeps tools)
	@echo "Cleaning..."
	@rm -f $(BUILD_DIR)/$(BINARY_NAME)*
	@$(GO) clean

clean-tools: ## Remove installed tools
	@echo "Cleaning tools..."
	@rm -rf $(TOOLS_DIR)

clean-all: clean clean-tools ## Remove all build artifacts and tools

test: ## Run tests
	@echo "Running tests..."
	$(GO) test -v ./...

run: build ## Build and run with example file
	@echo "Running $(BINARY_NAME)..."
	@./$(BUILD_DIR)/$(BINARY_NAME) --help

fmt: ## Format code
	@echo "Formatting code..."
	$(GO) fmt ./...

vet: ## Run go vet
	@echo "Running go vet..."
	$(GO) vet ./...

$(TOOLS_DIR):
	@mkdir -p $@

$(GOLANGCI_LINT): | $(TOOLS_DIR)
	@echo "Installing golangci-lint via mise..."
	@mise install golangci-lint
	@ln -sf $$(mise which golangci-lint) $@

$(GORELEASER): | $(TOOLS_DIR)
	@echo "Installing goreleaser via mise..."
	@mise install goreleaser
	@ln -sf $$(mise which goreleaser) $@

golangci-lint: $(GOLANGCI_LINT) ## Run golangci-lint
	@echo "Verifying golangci-lint config..."
	$(GOLANGCI_LINT) config verify
	@echo "Running golangci-lint..."
	$(GOLANGCI_LINT) run --timeout=5m

lint: fmt vet golangci-lint ## Run formatters and linters

goreleaser-check: $(GORELEASER) ## Validate .goreleaser.yml config
	$(GORELEASER) check

goreleaser-snapshot: $(GORELEASER) ## Build a snapshot release locally (no publish)
	$(GORELEASER) release --snapshot --clean

install-tools: $(GOLANGCI_LINT) $(GORELEASER) ## Install development tools via mise into bin/tools
	@echo "Tools installed in $(TOOLS_DIR)!"
