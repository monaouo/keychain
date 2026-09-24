# syntax=docker/dockerfile:1

# 編譯階段
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w" -o /out/keychain ./cmd/keychain \
 && mkdir -p /out/data

# 執行階段
FROM gcr.io/distroless/static-debian12:nonroot
ARG VERSION=dev
LABEL org.opencontainers.image.title="keychain" \
      org.opencontainers.image.description="本機加密帳號密碼管理 Web 應用" \
      org.opencontainers.image.source="https://github.com/monaouo/keychain" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.licenses="MIT"

COPY --from=build /out/keychain /usr/local/bin/keychain
COPY --from=build --chown=65532:65532 /out/data /data

ENV KEYCHAIN_ADDR=0.0.0.0:8787 \
    KEYCHAIN_DATA=/data/vault.json
VOLUME ["/data"]
EXPOSE 8787
USER nonroot:nonroot

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD ["/usr/local/bin/keychain", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/keychain"]
