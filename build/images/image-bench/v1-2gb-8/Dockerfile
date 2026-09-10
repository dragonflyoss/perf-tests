# image-bench:v1-2gb-8: 2 GiB image, 256 MiB x 8 layers.
FROM --platform=$BUILDPLATFORM alpine:3.22 AS builder

RUN mkdir -p /data \
    && dd if=/dev/urandom of=/data/layer1 bs=1M count=256 \
    && dd if=/dev/urandom of=/data/layer2 bs=1M count=256 \
    && dd if=/dev/urandom of=/data/layer3 bs=1M count=256 \
    && dd if=/dev/urandom of=/data/layer4 bs=1M count=256 \
    && dd if=/dev/urandom of=/data/layer5 bs=1M count=256 \
    && dd if=/dev/urandom of=/data/layer6 bs=1M count=256 \
    && dd if=/dev/urandom of=/data/layer7 bs=1M count=256 \
    && dd if=/dev/urandom of=/data/layer8 bs=1M count=256

FROM scratch

COPY --from=builder /data/layer1 /layer1
COPY --from=builder /data/layer2 /layer2
COPY --from=builder /data/layer3 /layer3
COPY --from=builder /data/layer4 /layer4
COPY --from=builder /data/layer5 /layer5
COPY --from=builder /data/layer6 /layer6
COPY --from=builder /data/layer7 /layer7
COPY --from=builder /data/layer8 /layer8
