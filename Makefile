.PHONY: all build build-all clean test bench fmt tidy install-hooks

BINARY_NAME := godocstore
CMD_PATH    := ./cmd/godocstore

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

all: build

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) $(CMD_PATH)

# Cross-compile release artifacts into dist/ (same matrix as go-delta).
build-all: clean
	mkdir -p dist
	GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-linux-amd64 $(CMD_PATH)
	GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-linux-arm64 $(CMD_PATH)
	GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-darwin-amd64 $(CMD_PATH)
	GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-darwin-arm64 $(CMD_PATH)
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-windows-amd64.exe $(CMD_PATH)
	cd dist && \
	tar -czf $(BINARY_NAME)-linux-amd64.tar.gz  $(BINARY_NAME)-linux-amd64  && \
	tar -czf $(BINARY_NAME)-linux-arm64.tar.gz  $(BINARY_NAME)-linux-arm64  && \
	tar -czf $(BINARY_NAME)-darwin-amd64.tar.gz $(BINARY_NAME)-darwin-amd64 && \
	tar -czf $(BINARY_NAME)-darwin-arm64.tar.gz $(BINARY_NAME)-darwin-arm64 && \
	zip -q   $(BINARY_NAME)-windows-amd64.zip   $(BINARY_NAME)-windows-amd64.exe && \
	sha256sum *.tar.gz *.zip > checksums.txt

clean:
	rm -rf bin dist

test:
	go test ./... -count=1

bench:
	go test ./... -bench=. -benchmem

fmt:
	gofmt -w .

tidy:
	go mod tidy

install-hooks:
	cp hooks/pre-commit .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit
