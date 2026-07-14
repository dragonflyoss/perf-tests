#!/bin/bash
# Benchmark the Dragonfly dfdaemon proxy with vegeta (https://github.com/tsenart/vegeta).
#
# Modes:
#   repeat - download the same URL repeatedly, every request hits the same
#            Dragonfly task, so the P2P cache serves most of the traffic.
#   random - append a unique query string to every request, so every request
#            maps to a distinct Dragonfly task and goes back-to-source.
#
# Usage:
#   ./tools/proxy-bench/proxy-bench.sh repeat
#   ./tools/proxy-bench/proxy-bench.sh random
#   RATE=200 DURATION=120s ./tools/proxy-bench/proxy-bench.sh repeat
#   RANGE=0-1023 ./tools/proxy-bench/proxy-bench.sh random
#
# Install vegeta:
#   go install github.com/tsenart/vegeta/v12@latest

set -eu

MODE="${1:-repeat}"
PROXY="${PROXY:-http://127.0.0.1:4001}"
TARGET_URL="${TARGET_URL:-http://file-server/small}"
RATE="${RATE:-100}"
DURATION="${DURATION:-60s}"
TIMEOUT="${TIMEOUT:-30s}"
MAX_WORKERS="${MAX_WORKERS:-64}"
OUTPUT_DIR="${OUTPUT_DIR:-bench-results}"
# Byte range sent with every request, e.g. RANGE=0-1023 or RANGE=bytes=0-1023.
# Empty means full downloads.
RANGE="${RANGE:-}"

if [[ "${MODE}" != "repeat" && "${MODE}" != "random" ]]; then
    echo "unknown mode: ${MODE} (expected 'repeat' or 'random')" >&2
    exit 1
fi

if ! command -v vegeta >/dev/null 2>&1; then
    echo "vegeta not found, install it with:" >&2
    echo "  go install github.com/tsenart/vegeta/v12@latest" >&2
    exit 1
fi

HEADER_ARGS=()
if [[ -n "${RANGE}" ]]; then
    [[ "${RANGE}" == bytes=* ]] || RANGE="bytes=${RANGE}"
    HEADER_ARGS+=(-header "Range: ${RANGE}")
fi

mkdir -p "${OUTPUT_DIR}"
RESULT="${OUTPUT_DIR}/$(date +%Y%m%d-%H%M%S)-${MODE}.bin"

# Vegeta resolves the proxy from the standard environment variables.
export HTTP_PROXY="${PROXY}"
export HTTPS_PROXY="${PROXY}"

# Emit one "GET <url>" line per request, consumed lazily by vegeta so the
# random mode never needs to materialize the full target list up front.
targets() {
    local start_ts
    start_ts=$(date +%s)

    local i=0
    while true; do
        if [[ "${MODE}" == "random" ]]; then
            i=$((i + 1))
            echo "GET ${TARGET_URL}?r=${start_ts}-$$-${i}-${RANDOM}"
        else
            echo "GET ${TARGET_URL}"
        fi
    done
}

echo "mode=${MODE} proxy=${PROXY} url=${TARGET_URL} rate=${RATE}/s duration=${DURATION} range=${RANGE:-none}"

targets | vegeta attack \
    -lazy \
    -rate="${RATE}" \
    -duration="${DURATION}" \
    -timeout="${TIMEOUT}" \
    -max-workers="${MAX_WORKERS}" \
    ${HEADER_ARGS[@]+"${HEADER_ARGS[@]}"} \
    >"${RESULT}"

vegeta report "${RESULT}"
vegeta report -type='hist[0,10ms,25ms,50ms,100ms,250ms,500ms,1s,2s,5s]' "${RESULT}"

echo
echo "raw results: ${RESULT}"
echo "render a latency plot with: vegeta plot ${RESULT} > plot.html"
