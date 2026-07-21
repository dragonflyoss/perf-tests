#!/bin/bash
# Benchmark the Dragonfly dfdaemon proxy with vegeta (https://github.com/tsenart/vegeta).
#
# Modes:
#   repeat     - download the same URL repeatedly, every request hits the same
#                Dragonfly task, so the P2P cache serves most of the traffic.
#   random     - append a unique query string to every request, so every request
#                maps to a distinct Dragonfly task and goes back-to-source.
#   sequential - append a unique query string per pass, then read the object
#                (FILE_SIZE bytes) from head to tail in CHUNK_SIZE ranged
#                requests; once a pass completes, switch to a new query string,
#                i.e. a new Dragonfly task. STREAMS passes are interleaved
#                round-robin so several tasks stay in flight at once.
#
# Usage:
#   ./tools/proxy-bench/proxy-bench.sh repeat
#   ./tools/proxy-bench/proxy-bench.sh random
#   ./tools/proxy-bench/proxy-bench.sh sequential
#   RATE=200 DURATION=120s ./tools/proxy-bench/proxy-bench.sh repeat
#   RANGE=0-1023 ./tools/proxy-bench/proxy-bench.sh random
#   FILE_SIZE=$((1 << 30)) CHUNK_SIZE=$((1 << 20)) ./tools/proxy-bench/proxy-bench.sh sequential
#   # RATE=0 means attack as fast as possible, concurrency ramps up to MAX_WORKERS.
#   STREAMS=128 RATE=0 MAX_WORKERS=256 ./tools/proxy-bench/proxy-bench.sh sequential
#
# Install vegeta:
#   go install github.com/tsenart/vegeta/v12@latest

set -eu

MODE="${1:-repeat}"
PROXY="${PROXY:-http://127.0.0.1:4001}"
# sequential walks FILE_SIZE bytes, so it defaults to the 1GiB object (/large,
# see tools/file-server/Dockerfile); the other modes default to the 1MiB /small.
if [[ "${MODE}" == "sequential" ]]; then
    TARGET_URL="${TARGET_URL:-http://file-server/large}"
else
    TARGET_URL="${TARGET_URL:-http://file-server/small}"
fi
RATE="${RATE:-100}"
DURATION="${DURATION:-60s}"
TIMEOUT="${TIMEOUT:-30s}"
MAX_WORKERS="${MAX_WORKERS:-64}"
MAX_BODY="${MAX_BODY:-0}"
OUTPUT_DIR="${OUTPUT_DIR:-bench-results}"
# Byte range sent with every request, e.g. RANGE=0-1023 or RANGE=bytes=0-1023.
# Empty means full downloads. Not supported in sequential mode, which generates
# its own Range header per chunk.
RANGE="${RANGE:-}"
# sequential mode: total object size and per-request chunk size, in bytes.
FILE_SIZE="${FILE_SIZE:-$((1 << 30))}"   # 1GiB
CHUNK_SIZE="${CHUNK_SIZE:-$((1 << 20))}" # 1MiB
# sequential mode: number of passes interleaved round-robin in the target
# stream, i.e. how many Dragonfly tasks are read concurrently.
STREAMS="${STREAMS:-128}"

if [[ "${MODE}" != "repeat" && "${MODE}" != "random" && "${MODE}" != "sequential" ]]; then
    echo "unknown mode: ${MODE} (expected 'repeat', 'random' or 'sequential')" >&2
    exit 1
fi

if [[ "${MODE}" == "sequential" && -n "${RANGE}" ]]; then
    echo "RANGE is not supported in sequential mode, it generates a Range header per chunk" >&2
    exit 1
fi

if ! [[ "${STREAMS}" =~ ^[1-9][0-9]*$ ]]; then
    echo "STREAMS must be a positive integer, got: ${STREAMS}" >&2
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

# Emit vegeta http-format targets, consumed lazily by vegeta so no mode ever
# needs to materialize the full target list up front. sequential emits blank
# line separated targets with a per-chunk Range header, round-robining across
# STREAMS concurrent passes.
targets() {
    # Query strings must be unique across pods too: in-container PIDs are
    # always 1 and same-second starts can seed RANDOM identically, so include
    # the hostname.
    local run_id
    run_id="$(hostname)-$(date +%s)-$$"

    local i=0
    if [[ "${MODE}" == "sequential" ]]; then
        local -a urls offsets
        local s end
        # All streams start together at offset 0 and advance in lockstep, one
        # chunk per round-robin turn. Every pass reads the object from 0 to
        # EOF, and each pass boundary creates STREAMS new tasks at once.
        for ((s = 0; s < STREAMS; s++)); do
            offsets[s]="${FILE_SIZE}" # force a fresh query string on the first turn
        done
        while true; do
            for ((s = 0; s < STREAMS; s++)); do
                if ((offsets[s] >= FILE_SIZE)); then
                    i=$((i + 1))
                    urls[s]="${TARGET_URL}?r=${run_id}-${i}-${RANDOM}"
                    offsets[s]=0
                fi
                end=$((offsets[s] + CHUNK_SIZE - 1))
                if ((end >= FILE_SIZE)); then
                    end=$((FILE_SIZE - 1))
                fi
                printf 'GET %s\nRange: bytes=%d-%d\n\n' "${urls[s]}" "${offsets[s]}" "${end}"
                offsets[s]=$((offsets[s] + CHUNK_SIZE))
            done
        done
    fi

    while true; do
        if [[ "${MODE}" == "random" ]]; then
            i=$((i + 1))
            echo "GET ${TARGET_URL}?r=${run_id}-${i}-${RANDOM}"
        else
            echo "GET ${TARGET_URL}"
        fi
    done
}

echo "mode=${MODE} proxy=${PROXY} url=${TARGET_URL} rate=${RATE}/s duration=${DURATION} range=${RANGE:-none}"
if [[ "${MODE}" == "sequential" ]]; then
    echo "file_size=${FILE_SIZE} chunk_size=${CHUNK_SIZE} chunks_per_pass=$(((FILE_SIZE + CHUNK_SIZE - 1) / CHUNK_SIZE)) streams=${STREAMS}"
fi

targets | vegeta attack \
    -lazy \
    -rate="${RATE}" \
    -duration="${DURATION}" \
    -timeout="${TIMEOUT}" \
    -max-workers="${MAX_WORKERS}" \
    -max-body="${MAX_BODY}" \
    ${HEADER_ARGS[@]+"${HEADER_ARGS[@]}"} \
    >"${RESULT}"

vegeta report "${RESULT}"
vegeta report -type='hist[0,10ms,25ms,50ms,100ms,250ms,500ms,1s,2s,5s]' "${RESULT}"

echo
echo "raw results: ${RESULT}"
echo "render a latency plot with: vegeta plot ${RESULT} > plot.html"
