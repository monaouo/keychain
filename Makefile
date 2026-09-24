BIN     := bin/keychain
ADDR    ?= 127.0.0.1:8787
DATA    ?= $(HOME)/.keychain/vault.json
LDFLAGS := -s -w

.PHONY: all run build test fmt vet clean

all: fmt vet test build

run:
	go run ./cmd/keychain -addr $(ADDR) -data $(DATA)

build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/keychain

test:
	go test -race ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -rf bin dist
