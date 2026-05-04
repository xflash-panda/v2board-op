.PHONY: help build install test test-race fmt vet clean dry-run apply

GOPATH_BIN := $(shell go env GOPATH)/bin
BIN_DIR    := bin

# To add a new CLI:
#   1. Append the binary name to BINARIES.
#   2. Define PKG_<binary-name> with the Go package path.
# That's it — build/install/clean targets pick it up automatically.
BINARIES := sync-ssh-config change-ip-lightsail-cf

PKG_sync-ssh-config        := ./cmd/sync_ssh_config/lightsail
PKG_change-ip-lightsail-cf := ./cmd/change_ip/lightsail_cf

LOCAL_BINS   := $(addprefix $(BIN_DIR)/,$(BINARIES))
INSTALL_BINS := $(addprefix $(GOPATH_BIN)/,$(BINARIES))

help:
	@echo "Targets:"
	@echo "  build              Build all binaries to ./$(BIN_DIR)/"
	@echo "  install            Install all binaries to $(GOPATH_BIN)/"
	@echo "  build-<name>       Build one binary to ./$(BIN_DIR)/"
	@echo "  install-<name>     Install one binary to $(GOPATH_BIN)/"
	@echo "  test               Run all tests"
	@echo "  test-race          Run all tests with -race"
	@echo "  fmt                gofmt ./..."
	@echo "  vet                go vet ./..."
	@echo "  clean              Remove ./$(BIN_DIR)/"
	@echo "  dry-run            Build and run sync-ssh-config (dry-run)"
	@echo "  apply              Build and run sync-ssh-config --apply"
	@echo ""
	@echo "Binaries: $(BINARIES)"
	@echo "  e.g. make install-sync-ssh-config"

build: $(LOCAL_BINS)

install: $(INSTALL_BINS)
	@echo "Installed to $(GOPATH_BIN)/"

# Pattern rule: build any binary listed in BINARIES.
$(BIN_DIR)/%:
	@mkdir -p $(BIN_DIR)
	go build -o $@ $(PKG_$*)

$(GOPATH_BIN)/%:
	go build -o $@ $(PKG_$*)

# Per-binary phony targets: build-<name>, install-<name>.
# Generated automatically for every entry in BINARIES so you can build/install
# a single CLI: e.g. `make install-sync-ssh-config`.
define BINARY_TARGETS
.PHONY: build-$(1) install-$(1)
build-$(1): $(BIN_DIR)/$(1)
install-$(1): $(GOPATH_BIN)/$(1)
	@echo "Installed $(GOPATH_BIN)/$(1)"
endef
$(foreach bin,$(BINARIES),$(eval $(call BINARY_TARGETS,$(bin))))

test:
	go test ./...

test-race:
	go test ./... -race -count=1

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -rf $(BIN_DIR)

dry-run: $(BIN_DIR)/sync-ssh-config
	./$(BIN_DIR)/sync-ssh-config

apply: $(BIN_DIR)/sync-ssh-config
	./$(BIN_DIR)/sync-ssh-config --apply
