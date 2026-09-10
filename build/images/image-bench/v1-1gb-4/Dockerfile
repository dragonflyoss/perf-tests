# image-bench:v1-1gb-4: 1 GiB image, 256 MiB x 4 layers.
FROM --platform=$BUILDPLATFORM alpine:3.22 AS builder

RUN mkdir -p /data \
    && dd if=/dev/urandom of=/data/layer1 bs=1M count=256 \
    && dd if=/dev/urandom of=/data/layer2 bs=1M count=256 \
    && dd if=/dev/urandom of=/data/layer3 bs=1M count=256 \
    && dd if=/dev/urandom of=/data/layer4 bs=1M count=256

FROM scratch

COPY --from=builder /data/layer1 /layer1
COPY --from=builder /data/layer2 /layer2
COPY --from=builder /data/layer3 /layer3
COPY --from=builder /data/layer4 /layer4
