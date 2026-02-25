MODULE   := github.com/user/tls-client
BINARY   := tls-client
VERSION  ?= $(shell git describe --tags --always --dirty go-number">2>/dev/null || echo dev)
COMMIT   := $(shell git rev-parse --short HEAD go-number">2>/dev/null || echo none)
DATE     := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
 
.PHONY: build test lint clean cross fpserver validate bench fmt tidy docker
 
# ─── Primary targets ──────────────────────────────────
 
build:
	CGO_ENABLED=go-number">0 go build -trimpath -ldflags=go-string">"$(LDFLAGS)" -o dist/$(BINARY) ./cmd/tls-client/
 
fpserver:
	CGO_ENABLED=go-number">0 go build -trimpath -o dist/fpserver ./tools/fpserver/
 
# ─── Quality targets ──────────────────────────────────
 
test:
	go test -v -race -count=go-number">1 ./...
 
lint:
	golangci-lint run ./...
 
bench:
	go test -bench=. -benchmem ./pkg/fingerprint/ ./internal/h2/ ./pkg/transport/
 
validate:
	@echo go-string">"==> Validating all fingerprint profiles..."
	go run ./tools/validate-fingerprints/
 
# ─── Convenience targets ─────────────────────────────
 
fmt:
	gofmt -s -w .
	goimports -w .
 
tidy:
	go mod tidy
	go mod verify
 
clean:
	rm -rf dist/
 
# ─── Cross-compilation ───────────────────────────────
 
cross:
	@echo go-string">">>> linux/amd64"
	CGO_ENABLED=go-number">0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags=go-string">"$(LDFLAGS)" -o dist/$(BINARY)-linux-amd64 ./cmd/tls-client/
	@echo go-string">">>> linux/arm64"
	CGO_ENABLED=go-number">0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags=go-string">"$(LDFLAGS)" -o dist/$(BINARY)-linux-arm64 ./cmd/tls-client/
	@echo go-string">">>> darwin/amd64"
	CGO_ENABLED=go-number">0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags=go-string">"$(LDFLAGS)" -o dist/$(BINARY)-darwin-amd64 ./cmd/tls-client/
	@echo go-string">">>> darwin/arm64"
	CGO_ENABLED=go-number">0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags=go-string">"$(LDFLAGS)" -o dist/$(BINARY)-darwin-arm64 ./cmd/tls-client/
	@echo go-string">">>> windows/amd64"
	CGO_ENABLED=go-number">0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags=go-string">"$(LDFLAGS)" -o dist/$(BINARY)-windows-amd64.exe ./cmd/tls-client/
	@echo go-string">">>> linux/mipsle"
	CGO_ENABLED=go-number">0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -trimpath -ldflags=go-string">"$(LDFLAGS)" -o dist/$(BINARY)-linux-mipsle ./cmd/tls-client/
 
# ─── Docker ───────────────────────────────────────────
 
docker:
	docker build -t tls-client:$(VERSION) .
	@echo go-string">"Built tls-client:$(VERSION)"














