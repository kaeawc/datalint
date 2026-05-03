.PHONY: build build-lsp build-mcp test vet lint fmt complexity security licenses tidy ci clean all

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -s -w -X main.version=$(VERSION)
BIN ?= datalint

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/datalint/

build-lsp:
	go build -ldflags "$(LDFLAGS)" -o datalint-lsp ./cmd/datalint-lsp/

build-mcp:
	go build -ldflags "$(LDFLAGS)" -o datalint-mcp ./cmd/datalint-mcp/

test:
	go test ./... -count=1

vet:
	go vet ./...

fmt:
	gofmt -s -w .

lint:
	golangci-lint run

complexity:
	gocyclo -over 10 -ignore '_test\.go$$' .

security:
	gosec ./...

licenses:
	go-licenses report ./...

tidy:
	go mod tidy

ci: vet test complexity lint security licenses

clean:
	rm -f $(BIN) datalint-lsp datalint-mcp junit-report.xml gosec-report.xml

all: build vet test
