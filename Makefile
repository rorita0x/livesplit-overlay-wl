LSC_DIR      := third_party/livesplit-core
LSC_FEATURES := software-rendering,livesplit-core/font-loading
LSC_LIB      := $(LSC_DIR)/target/release/liblivesplit_core.a
LSC_HEADER   := build/bindings/livesplit_core.h

.PHONY: all clean
all: livesplit-overlay

$(LSC_LIB): $(LSC_DIR)/Cargo.toml
	cd $(LSC_DIR) && cargo rustc --release -p livesplit-core-capi --crate-type staticlib --features $(LSC_FEATURES)

$(LSC_HEADER): $(LSC_DIR)/Cargo.toml
	cd $(LSC_DIR)/capi/bind_gen && cargo run --release -q -- --features software-rendering --output-dir $(CURDIR)/build/bindings

livesplit-overlay: $(LSC_LIB) $(LSC_HEADER) $(shell find cmd internal -name '*.go') go.mod go.sum
	go build -o $@ ./cmd/livesplit-overlay

clean:
	rm -rf build livesplit-overlay
	cd $(LSC_DIR) && cargo clean
