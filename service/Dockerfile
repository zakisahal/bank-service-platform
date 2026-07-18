# syntax=docker/dockerfile:1
FROM golang:1.26-bookworm@sha256:1ecb7edf62a0408027bd5729dfd6b1b8766e578e8df93995b225dfd0944eb651 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/bank-service ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/bank-consumer ./cmd/consumer

# distroless "nonroot" runs as uid 65532 with no shell, no package manager -
# nothing an attacker who gets code exec could use to pivot.
#
# Two final stages share this one pinned base. "api" is the default target
# (docker build with no --target) since it's the primary artifact; the
# consumer is built with `--target consumer`.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:aef9602f8710ec12bde19d593fed1f76c708531bb7aba205110f1029786ead7b AS consumer
WORKDIR /
COPY --from=build /out/bank-consumer /bank-consumer
USER nonroot:nonroot
ENTRYPOINT ["/bank-consumer"]

FROM gcr.io/distroless/static-debian12:nonroot@sha256:aef9602f8710ec12bde19d593fed1f76c708531bb7aba205110f1029786ead7b AS api
WORKDIR /
COPY --from=build /out/bank-service /bank-service
USER nonroot:nonroot
EXPOSE 8080
# The binary probes its own /healthz (distroless has no shell/curl for a
# CMD-shell-form healthcheck to call out to).
HEALTHCHECK --interval=10s --timeout=2s --start-period=5s --retries=3 \
    CMD ["/bank-service", "-healthcheck"]
ENTRYPOINT ["/bank-service"]
