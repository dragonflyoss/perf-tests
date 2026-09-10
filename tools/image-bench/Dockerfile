FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS="${TARGETOS:-linux}" GOARCH="${TARGETARCH:-}" go build -o /out/dfbench ./cmd/dfbench

FROM alpine:3.22

ARG TARGETARCH
ARG CRICTL_VERSION=v1.37.0

# crictl is run by the cleanup pods, which use this image on every peer node
# to remove the pulled image from containerd.
RUN apk add --no-cache curl kubectl \
    && curl -fsSL "https://github.com/kubernetes-sigs/cri-tools/releases/download/${CRICTL_VERSION}/crictl-${CRICTL_VERSION}-linux-${TARGETARCH:-amd64}.tar.gz" \
    | tar -xz -C /usr/local/bin crictl

COPY --from=builder /out/dfbench /usr/local/bin/dfbench

# kubectl writes its discovery cache under HOME, which must be writable for
# the non-root user, mount an emptyDir at /tmp when the root filesystem is
# read-only.
USER 65534:65534
ENV HOME=/tmp

ENTRYPOINT ["/usr/local/bin/dfbench", "image-bench"]
