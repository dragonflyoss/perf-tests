#!/bin/sh
set -eu

case "${MODE:-repeat}" in
  repeat|random|sequential)
    exec k6 run /home/k6/proxy-bench.js "$@"
    ;;
  fixed-pool)
    exec fixed-pool-worker /home/k6/fixed-pool/benchmark.js "$@"
    ;;
  *)
    printf 'MODE: expected repeat, random, sequential or fixed-pool; actual %s\n' "$MODE" >&2
    exit 2
    ;;
esac
