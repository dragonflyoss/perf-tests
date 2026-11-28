FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY . .
RUN CGO_ENABLED=0 GOOS="${TARGETOS:-linux}" GOARCH="${TARGETARCH:-}" \
    go build -mod=readonly -o /out/fixed-pool-worker ./cmd/fixedpool/worker

FROM grafana/k6:2.3.0

ENV K6_NO_USAGE_REPORT=true

COPY tools/proxy-bench/proxy-bench.js tools/proxy-bench/run-proxy-bench.sh /home/k6/
COPY tools/proxy-bench/lib /home/k6/lib

COPY tools/proxy-bench/fixed-pool/benchmark.js /home/k6/fixed-pool/
COPY --from=builder /out/fixed-pool-worker /usr/local/bin/

ENTRYPOINT ["/bin/sh", "/home/k6/run-proxy-bench.sh"]
