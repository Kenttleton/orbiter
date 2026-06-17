# Orbiter build tasks

build:
    go build -o bin/orbiter ./cmd/orbiter

install:
    go install ./cmd/orbiter

test:
    go test ./...

test-verbose:
    go test -v ./...

lint:
    golangci-lint run

clean:
    rm -f bin/orbiter

# ── Integration WASM builds ────────────────────────────────────────────────────

# Build all integration WASM plugins
build-integrations: build-integration-git build-integration-golang build-integration-node build-integration-make build-integration-dotenv build-integration-python build-integration-rust build-integration-brew build-integration-uv build-integration-rustup build-integration-docker build-integration-macos build-integration-onepassword build-integration-ssh build-integration-nvm build-integration-just build-integration-shell build-integration-asdf build-integration-local

build-integration-git:
    cd integrations/git && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/git.wasm .

build-integration-golang:
    cd integrations/golang && tinygo build -o golang.wasm -target=wasm-unknown ./guest/

build-integration-node:
    cd integrations/node && tinygo build -o node.wasm -target=wasm-unknown ./guest/

build-integration-make:
    cd integrations/make && tinygo build -o make.wasm -target=wasm-unknown ./guest/

build-integration-dotenv:
    cd integrations/dotenv && tinygo build -o dotenv.wasm -target=wasm-unknown ./guest/

build-integration-python:
    cd integrations/python && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/python.wasm .

build-integration-rust:
    cd integrations/rust && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/rust.wasm .

build-integration-brew:
    cd integrations/brew && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/brew.wasm .

build-integration-uv:
    cd integrations/uv && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/uv.wasm .

build-integration-rustup:
    cd integrations/rustup && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/rustup.wasm .

build-integration-docker:
    cd integrations/docker && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/docker.wasm .

build-integration-macos:
    cd integrations/macos && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/macos.wasm .

build-integration-onepassword:
    cd integrations/onepassword && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/onepassword.wasm .

build-integration-ssh:
    cd integrations/ssh && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/ssh.wasm .

build-integration-nvm:
    cd integrations/nvm && npm install && ./node_modules/.bin/asc assembly/index.ts --target release

build-integration-just:
    cd integrations/just && npm install && ./node_modules/.bin/asc assembly/index.ts --target release

build-integration-shell:
    cd integrations/shell && npm install && ./node_modules/.bin/asc assembly/index.ts --target release

build-integration-asdf:
    cd integrations/asdf && zig build-exe src/main.zig -target wasm32-freestanding -O ReleaseSmall -fno-entry --export=detect --export=initialize --export=scan --export=calibrate -femit-bin=asdf.wasm

build-integration-local:
    cd integrations/local && zig build-exe src/main.zig -target wasm32-freestanding -O ReleaseSmall -fno-entry --export=detect --export=initialize --export=scan --export=calibrate -femit-bin=local.wasm

# Cross-compilation target for CI release builds.
# Usage: just build-release orbiter linux amd64 v1.2.3
build-release binary goos goarch version:
    #!/usr/bin/env bash
    set -euo pipefail
    EXT=""
    if [ "{{goos}}" = "windows" ]; then EXT=".exe"; fi
    mkdir -p dist
    CGO_ENABLED=0 GOOS={{goos}} GOARCH={{goarch}} go build \
        -ldflags="-s -w -X main.version={{version}}" \
        -o "dist/{{binary}}-{{goos}}-{{goarch}}${EXT}" \
        ./cmd/{{binary}}
