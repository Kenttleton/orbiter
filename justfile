# Orbiter build tasks

build-orbit:
    go build -o bin/orbit ./cmd/orbit

build-orbiter:
    go build -o bin/orbiter ./cmd/orbiter

build: build-orbit build-orbiter

install-orbit:
    go install ./cmd/orbit

install-orbiter:
    go install ./cmd/orbiter

install: install-orbit install-orbiter

test:
    go test ./...

test-verbose:
    go test -v ./...

lint:
    golangci-lint run

clean:
    rm -f bin/orbit bin/orbiter

# Cross-compilation target for CI release builds.
# Usage: just build-release orbit linux amd64 v1.2.3
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
