.PHONY: build test race bench lint clean help setup setup-clean

BIN_DIR     := bin
PKGS        := ./...
THIRD_PARTY := third_party
LLAMA_DIR   := $(THIRD_PARTY)/llama.cpp
LLAMA_TAG   := b4567

help:
	@echo "llmserve — common targets"
	@echo "  make setup       fetch and build pinned llama.cpp (required for model package)"
	@echo "  make setup-clean wipe third_party/ to redo setup from scratch"
	@echo "  make build       build binaries into ./$(BIN_DIR)/"
	@echo "  make test        go test $(PKGS)"
	@echo "  make race        go test -race $(PKGS)"
	@echo "  make bench       go test -bench=. -benchmem $(PKGS)"
	@echo "  make lint        gofmt + go vet"
	@echo "  make clean       remove build artifacts"

CXX_HEADER_PATH := /Library/Developer/CommandLineTools/SDKs/MacOSX15.5.sdk/usr/include/c++/v1

setup:
	@mkdir -p $(THIRD_PARTY)
	@if [ ! -d $(LLAMA_DIR) ]; then \
		echo "==> Cloning llama.cpp @ $(LLAMA_TAG)"; \
		git clone --depth=1 --branch=$(LLAMA_TAG) https://github.com/ggerganov/llama.cpp $(LLAMA_DIR) || \
		git clone --depth=1 https://github.com/ggerganov/llama.cpp $(LLAMA_DIR); \
	else \
		echo "==> llama.cpp already cloned at $(LLAMA_DIR)"; \
	fi
	@echo "==> Wiping any prior failed build"
	@rm -rf $(LLAMA_DIR)/build
	@echo "==> Building llama.cpp (this takes a few minutes the first time)"
	@cd $(LLAMA_DIR) && \
		cmake -B build \
			-DBUILD_SHARED_LIBS=OFF \
			-DLLAMA_CURL=OFF \
			-DCMAKE_BUILD_TYPE=Release \
			-DGGML_METAL=ON \
			-DCMAKE_CXX_FLAGS="-I$(CXX_HEADER_PATH)" && \
		cmake --build build --config Release -j
	@echo ""
	@echo "==> llama.cpp built. Static libs in $(LLAMA_DIR)/build/"
	@echo "==> Next: place a GGUF model under ./models/ (Phi-3-mini-4k-instruct Q4 recommended)"

setup-clean:
	rm -rf $(THIRD_PARTY)

build:
	@mkdir -p $(BIN_DIR)
	go build -tags llamacpp -o $(BIN_DIR)/llmserve ./cmd/llmserve
	go build -tags llamacpp -o $(BIN_DIR)/llmctl ./cmd/llmctl

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
