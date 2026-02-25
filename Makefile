MODULE   := github.com/user/tls-client
BINARY   := tls-client
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE     := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build test lint clean cross fpserver validate bench fmt tidy docker

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o dist/$(BINARY) ./cmd/tls-client/

fpserver:
	CGO_ENABLED=0 go build -trimpath -o dist/fpserver ./tools/fpserver/

test:
	go test -v -race -count=1 ./...

lint:
	golangci-lint run ./...

bench:
	go test -bench=. -benchmem ./pkg/fingerprint/ ./internal/h2/ ./pkg/transport/

validate:
	@echo "==> Validating all fingerprint profiles..."
	go run ./tools/validate-fingerprints/

fmt:
	gofmt -s -w .
	goimports -w .

tidy:
	go mod tidy
	go mod verify

clean:
	rm -rf dist/

cross:
	@echo ">>> linux/amd64"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o dist/$(BINARY)-linux-amd64 ./cmd/tls-client/
	@echo ">>> linux/arm64"
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="$(LDFLAGS)" -o dist/$(BINARY)-linux-arm64 ./cmd/tls-client/
	@echo ">>> darwin/amd64"
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o dist/$(BINARY)-darwin-amd64 ./cmd/tls-client/
	@echo ">>> darwin/arm64"
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="$(LDFLAGS)" -o dist/$(BINARY)-darwin-arm64 ./cmd/tls-client/
	@echo ">>> windows/amd64"
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o dist/$(BINARY)-windows-amd64.exe ./cmd/tls-client/
	@echo ">>> linux/mipsle"
	CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -trimpath -ldflags="$(LDFLAGS)" -o dist/$(BINARY)-linux-mipsle ./cmd/tls-client/

docker:
	docker build -t tls-client:$(VERSION) .
	@echo "Built tls-client:$(VERSION)"
