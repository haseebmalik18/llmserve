.PHONY: build test race bench lint clean help setup

BIN_DIR := bin
PKGS    := ./...

help:
	@echo "llmserve — common targets"
	@echo "  make setup   fetch and build pinned llama.cpp (Phase 1)"
	@echo "  make build   build binaries into ./$(BIN_DIR)/"
	@echo "  make test    go test $(PKGS)"
	@echo "  make race    go test -race $(PKGS)"
	@echo "  make bench   go test -bench=. -benchmem $(PKGS)"
	@echo "  make lint    gofmt + go vet"
	@echo "  make clean   remove build artifacts"

setup:
	@echo "Phase 1 will populate this target with llama.cpp fetch + build."

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/llmserve ./cmd/llmserve
	go build -o $(BIN_DIR)/llmctl ./cmd/llmctl

test:
	go test $(PKGS)

race:
	go test -race $(PKGS)

bench:
	go test -bench=. -benchmem $(PKGS)

lint:
	@gofmt_out=$$(gofmt -l .); \
	if [ -n "$$gofmt_out" ]; then \
		echo "gofmt found unformatted files:"; \
		echo "$$gofmt_out"; \
		exit 1; \
	fi
	go vet $(PKGS)

clean:
	rm -rf $(BIN_DIR)
	go clean $(PKGS)
