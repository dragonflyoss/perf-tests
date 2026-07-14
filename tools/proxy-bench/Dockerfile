FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

# Cross-compiled binaries land in /go/bin/<os>_<arch>/, native ones in /go/bin/.
RUN if [ -n "${TARGETOS:-}" ] && [ -n "${TARGETARCH:-}" ]; then \
      CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go install github.com/tsenart/vegeta/v12@latest && \
      if [ -f "/go/bin/${TARGETOS}_${TARGETARCH}/vegeta" ]; then mv "/go/bin/${TARGETOS}_${TARGETARCH}/vegeta" /go/bin/vegeta; fi; \
    else \
      CGO_ENABLED=0 go install github.com/tsenart/vegeta/v12@latest; \
    fi
    && if [ -f "/go/bin/${TARGETOS}_${TARGETARCH}/vegeta" ]; then mv "/go/bin/${TARGETOS}_${TARGETARCH}/vegeta" /go/bin/vegeta; fi

FROM alpine:3.22

RUN apk add --no-cache bash

COPY --from=builder /go/bin/vegeta /usr/local/bin/vegeta
COPY tools/proxy-bench/proxy-bench.sh /usr/local/bin/proxy-bench.sh

WORKDIR /bench

ENTRYPOINT ["/usr/local/bin/proxy-bench.sh"]
CMD ["repeat"]
