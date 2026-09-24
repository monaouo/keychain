BIN     := bin/keychain
ADDR    ?= 127.0.0.1:8787
DATA    ?= $(HOME)/.keychain/vault.json
LDFLAGS := -s -w

IMAGE     ?= ghcr.io/monaouo/keychain
VERSION   ?= 0.1.0
PLATFORMS ?= linux/amd64,linux/arm64

.PHONY: all run build test fmt vet clean docker-build docker-run docker-push

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

# 本機建置映像
docker-build:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

docker-run:
	docker run --rm -p 127.0.0.1:8787:8787 -v keychain-data:/data $(IMAGE):$(VERSION)

# 多架構建置並推送
docker-push:
	docker buildx build --platform $(PLATFORMS) --build-arg VERSION=$(VERSION) \
		-t $(IMAGE):$(VERSION) -t $(IMAGE):latest --push .
