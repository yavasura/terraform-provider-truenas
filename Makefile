NAME=terraform-provider-truenas
VERSION ?= dev
PROVIDER_HOSTNAME ?= registry.terraform.io
PROVIDER_NAMESPACE ?= deevus
PROVIDER_TYPE ?= truenas
PROVIDER_SOURCE ?= $(PROVIDER_NAMESPACE)/$(PROVIDER_TYPE)
PROVIDER_ADDRESS ?= $(PROVIDER_HOSTNAME)/$(PROVIDER_SOURCE)
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
PLUGIN_DIR=$(HOME)/.terraform.d/plugins/$(PROVIDER_HOSTNAME)/$(PROVIDER_NAMESPACE)/$(PROVIDER_TYPE)/$(VERSION)/$(GOOS)_$(GOARCH)

default: build

.PHONY: build
build:
	go build -o $(NAME) .

.PHONY: install
install: build
	mkdir -p $(PLUGIN_DIR)
	cp $(NAME) $(PLUGIN_DIR)/

.PHONY: test
test:
	go test ./...

.PHONY: test-unit
test-unit:
	go test ./...

.PHONY: fmt
fmt:
	gofmt -w .
	go mod tidy

.PHONY: docs
docs:
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs

.PHONY: clean
clean:
	rm -f $(NAME)

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build      - Build the provider binary in the repo root"
	@echo "  install    - Build and install the provider under ~/.terraform.d/plugins"
	@echo "  test       - Run all Go tests"
	@echo "  test-unit  - Run all Go tests"
	@echo "  fmt        - Format code and tidy modules"
	@echo "  docs       - Regenerate provider docs"
	@echo "  clean      - Remove the built provider binary"
	@echo "  help       - Show this help message"
