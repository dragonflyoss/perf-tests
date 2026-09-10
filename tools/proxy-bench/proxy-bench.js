// Benchmark the Dragonfly dfdaemon proxy with k6 (https://k6.io).
//
// Modes (MODE):
//   repeat     - download the same URL repeatedly, every request hits the same
//                Dragonfly task, so the P2P cache serves most of the traffic.
//   random     - append a unique query string to every request, so every request
//                maps to a distinct Dragonfly task and goes back-to-source.
//   sequential - pre-generate a fixed pool of URL_COUNT unique query strings
//                (one Dragonfly task each), then read the object (FILE_SIZE
//                bytes) from head to tail in CHUNK_SIZE ranged requests; once
//                a pass completes, move on to the next URL in the pool, so the
//                same URL_COUNT tasks are re-read repeatedly. STREAMS passes
//                are interleaved round-robin so several tasks stay in flight
//                at once.
//
// Load model: RATE>0 uses the constant-arrival-rate executor, RATE requests/s
// are started regardless of latency on up to VUS VUs (k6 reports
// dropped_iterations when the VUs cannot keep up). RATE=0 uses the constant-vus
// executor, VUS VUs loop back-to-back as fast as the proxy answers.
//
// Pass/fail: every response must carry the status the mode expects, 206 for
// ranged requests and 200 otherwise; anything else, or a transport error,
// counts towards http_req_failed. The run fails (exit code 99, a Failed Job in
// Kubernetes) when more than 1% of the requests failed. handleSummary() prints
// the report either way, in place of k6's default summary.
//
// k6 picks the proxy up from the standard HTTP_PROXY/HTTPS_PROXY environment
// variables (real environment variables, -e is not enough). Every other knob is
// read from the environment too, exported or passed with -e:
//   HTTP_PROXY=http://127.0.0.1:4001 k6 run tools/proxy-bench/proxy-bench.js
//   HTTP_PROXY=http://127.0.0.1:4001 k6 run -e MODE=random -e RATE=200 -e DURATION=120s tools/proxy-bench/proxy-bench.js
//   HTTP_PROXY=http://127.0.0.1:4001 k6 run -e MODE=random -e RANGE=0-1023 tools/proxy-bench/proxy-bench.js
//   HTTP_PROXY=http://127.0.0.1:4001 k6 run -e MODE=sequential -e RATE=0 -e VUS=256 -e SEED_CLIENT_CPUS=4 tools/proxy-bench/proxy-bench.js
//
// Knobs (defaults in parentheses):
//   MODE              repeat | random | sequential (repeat)
//   TARGET_URL        object to download (http://file-server/4m, sequential: http://file-server/1g)
//   RATE              requests per second, 0 switches to the constant-vus executor (100)
//   VUS               max concurrent requests, i.e. k6 virtual users (64)
//   DURATION          test duration (60s)
//   TIMEOUT           per-request timeout (30s)
//   RANGE             Range header sent with every repeat/random request, e.g. 0-1023 (none)
//   FILE_SIZE         sequential: object size in bytes (1GiB)
//   CHUNK_SIZE        sequential: bytes per request (4MiB)
//   STREAMS           sequential: passes interleaved round-robin (128)
//   URL_COUNT         sequential: size of the URL pool (32)
//   SEED_CLIENT_CPUS  CPUs of the seed client behind HTTP_PROXY, adds cpu_cost in core/Gbps to the summary (off)
import http from 'k6/http';
import { check } from 'k6';
import exec from 'k6/execution';

const MODE = __ENV.MODE || 'repeat';
if (!['repeat', 'random', 'sequential'].includes(MODE)) {
  throw new Error(`unknown MODE: ${MODE} (expected 'repeat', 'random' or 'sequential')`);
}

// Integer knob >= min; unset or empty falls back to the default.
function envInt(name, def, min) {
  const raw = __ENV[name];
  if (raw === undefined || raw === '') {
    return def;
  }

  if (!/^\d+$/.test(raw) || Number(raw) < min) {
    throw new Error(`${name} must be an integer >= ${min}, got: ${raw}`);
  }

  return Number(raw);
}

// sequential walks FILE_SIZE bytes, so it defaults to the 1GiB object (/1g,
// see tools/file-server/Dockerfile); the other modes default to the 4MiB /4m.
const TARGET_URL =
  __ENV.TARGET_URL || (MODE === 'sequential' ? 'http://file-server/1g' : 'http://file-server/4m');
const RATE = envInt('RATE', 100, 0);
const VUS = envInt('VUS', 64, 1);
const DURATION = __ENV.DURATION || '60s';
const TIMEOUT = __ENV.TIMEOUT || '30s';
let RANGE = __ENV.RANGE || '';
const FILE_SIZE = envInt('FILE_SIZE', 1 << 30, 1); // 1GiB
const CHUNK_SIZE = envInt('CHUNK_SIZE', 4 << 20, 1); // 4MiB
const STREAMS = envInt('STREAMS', 128, 1);
const URL_COUNT = envInt('URL_COUNT', 32, 1);
const CHUNKS_PER_PASS = Math.ceil(FILE_SIZE / CHUNK_SIZE);

// k6 only sees its own side, so the CPUs of the seed client serving HTTP_PROXY
// are an input: what the operator pinned (cgroup limit) or measured (kubectl
// top, pidstat) while the test ran. Decimals are fine, 0 leaves cpu_cost out.
const SEED_CLIENT_CPUS = Number(__ENV.SEED_CLIENT_CPUS || 0);
if (!Number.isFinite(SEED_CLIENT_CPUS) || SEED_CLIENT_CPUS < 0) {
  throw new Error(`SEED_CLIENT_CPUS must be a number >= 0, got: ${__ENV.SEED_CLIENT_CPUS}`);
}

if (MODE === 'sequential' && RANGE) {
  throw new Error('RANGE is not supported in sequential mode, it generates a Range header per chunk');
}

if (RANGE && !RANGE.startsWith('bytes=')) {
  RANGE = `bytes=${RANGE}`;
}

// sequential sends a Range header with every chunk, repeat/random only when
// RANGE is set, so the expected status is fixed for the whole run. Telling k6
// about it makes http_req_failed, and the threshold on it, catch a proxy that
// ignores the Range header and answers 200 with the whole object.
const EXPECTED_STATUS = MODE === 'sequential' || RANGE ? 206 : 200;
http.setResponseCallback(http.expectedStatuses(EXPECTED_STATUS));
const CHECKS = { [`status is ${EXPECTED_STATUS}`]: (r) => r.status === EXPECTED_STATUS };

export const options = {
  scenarios: {
    [MODE]:
      RATE > 0
        ? { executor: 'constant-arrival-rate', rate: RATE, timeUnit: '1s', duration: DURATION, preAllocatedVUs: VUS }
        : { executor: 'constant-vus', vus: VUS, duration: DURATION },
  },

  // Fail the run instead of only printing a summary full of errors, k6 exits
  // with code 99 when a threshold is crossed.
  thresholds: {
    http_req_failed: ['rate<0.01'],
  },

  // Bodies are still downloaded in full, but thrown away as they arrive: never
  // buffered, parsed or written to disk, so the client side stays cheap.
  discardResponseBodies: true,

  // The percentiles handleSummary() prints, the same set for every run so
  // summaries stay comparable column by column.
  summaryTrendStats: ['min', 'avg', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
};

// setup() runs once, so every VU shares the same run id (the init context runs
// once per VU and would hand each VU its own). Query strings must be unique
// across pods too, so include the hostname, the pod name in Kubernetes.
export function setup() {
  const runId = `${__ENV.HOSTNAME || 'k6'}-${Date.now()}-${Math.floor(Math.random() * 1e9)}`;
  console.log(
    `mode=${MODE} proxy=${__ENV.HTTP_PROXY || __ENV.http_proxy || 'none'} url=${TARGET_URL} range=${RANGE || 'none'}`,
  );
  if (MODE === 'sequential') {
    console.log(
      `file_size=${FILE_SIZE} chunk_size=${CHUNK_SIZE} chunks_per_pass=${CHUNKS_PER_PASS} streams=${STREAMS} url_count=${URL_COUNT}`,
    );
  }

  return { runId };
}

export default function ({ runId }) {
  // Tag every request with the bare TARGET_URL so per-request query strings
  // don't explode the cardinality of the name tag in metric outputs.
  const params = { timeout: TIMEOUT, headers: {}, tags: { name: TARGET_URL } };
  let url = TARGET_URL;

  if (MODE === 'random') {
    url = `${TARGET_URL}?r=${runId}-${exec.scenario.iterationInTest}`;
  } else if (MODE === 'sequential') {
    // Iterations are numbered globally across VUs in start order, so derive
    // the request from that number: iteration i is the next chunk of stream
    // i % STREAMS. Stream s starts on pool[s % URL_COUNT] at offset 0 and moves
    // to the next pool entry after each complete pass, cycling through the
    // same URL_COUNT URLs forever. All streams advance in lockstep, one chunk
    // per round-robin turn.
    const iter = exec.scenario.iterationInTest;
    const stream = iter % STREAMS;
    const turn = Math.floor(iter / STREAMS);
    const pass = Math.floor(turn / CHUNKS_PER_PASS);
    const chunk = turn % CHUNKS_PER_PASS;
    const start = chunk * CHUNK_SIZE;
    const end = Math.min(start + CHUNK_SIZE, FILE_SIZE) - 1;
    url = `${TARGET_URL}?r=${runId}-${(stream + pass) % URL_COUNT}`;
    params.headers.Range = `bytes=${start}-${end}`;
  }

  if (RANGE) {
    params.headers.Range = RANGE;
  }

  check(http.get(url, params), CHECKS);
}

// Formatting for the report.
const fmt = {
  int: (n) => String(Math.round(n)).replace(/\B(?=(\d{3})+(?!\d))/g, ','),
  ms: (n) => n.toFixed(2),
  gbps: (bytesPerSecond) => ((bytesPerSecond * 8) / 1e9).toFixed(2),
  pct: (part, total) => `${total > 0 ? ((100 * part) / total).toFixed(2) : '0.00'}%`,
  seconds: (s) => (s < 120 ? `${s.toFixed(1)}s` : `${Math.floor(s / 60)}m${Math.round(s % 60)}s`),
  bytes: (n) => {
    const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
    let i = 0;
    for (; n >= 1024 && i < units.length - 1; i++) {
      n /= 1024;
    }

    return `${Number.isInteger(n) ? n : n.toFixed(1)} ${units[i]}`;
  },
};

// Latency rows of the report and the k6 metric behind each one.
const LATENCY_ROWS = [
  ['request', 'http_req_duration'],
  ['connect', 'http_req_connecting'],
  ['first byte', 'http_req_waiting'],
  ['download', 'http_req_receiving'],
];

// One value of a metric, 0 when the metric has no samples: dropped_iterations,
// for one, only exists once an iteration was dropped.
function metric(data, name, key) {
  const m = data.metrics[name];
  return m && m.values[key] !== undefined ? m.values[key] : 0;
}

// Every threshold of the run with its verdict, e.g. "http_req_failed rate<0.01".
function thresholds(data) {
  return Object.entries(data.metrics).flatMap(([name, m]) =>
    Object.entries(m.thresholds || {}).map(([expr, t]) => ({ name: `${name} ${expr}`, ok: t.ok })),
  );
}

// Green/red on a terminal, plain text in kubectl logs and files.
function painter(data) {
  const enabled = data.state.isStdOutTTY && !data.options.noColor;
  return (ok, text) => (enabled ? `\x1b[${ok ? 32 : 31}m${text}\x1b[0m` : text);
}

// CPU cost of the seed client: cores it burned per Gbps it delivered. Goodput is
// the data_received rate, i.e. wire bytes including headers, within 0.01% of the
// payload at MiB-sized requests.
function cpuCost(bytesPerSecond) {
  const gbps = (bytesPerSecond * 8) / 1e9;
  return gbps > 0
    ? `${SEED_CLIENT_CPUS} cores / ${gbps.toFixed(2)} Gbps = ${(SEED_CLIENT_CPUS / gbps).toFixed(3)} core/Gbps`
    : 'n/a, nothing received';
}

// The end-of-test report, printed in place of k6's default summary: run
// parameters, request counts, latency percentiles, throughput and, with
// SEED_CLIENT_CPUS, the CPU cost. Thresholds still decide the exit code, this
// only decides what is printed.
export function handleSummary(data) {
  const value = (name, key) => metric(data, name, key);
  const paint = painter(data);
  const mark = (ok) => paint(ok, ok ? '✓' : '✗');
  const row = (label, text) => `  ${label.padEnd(14)}${text}`;
  const cells = (texts) => texts.map((t) => t.padStart(9)).join('');

  const stats = data.options.summaryTrendStats;
  const checks = data.root_group.checks || [];
  const verdicts = thresholds(data);
  const passed = verdicts.every((t) => t.ok);

  const proxy = __ENV.HTTP_PROXY || __ENV.http_proxy || 'no proxy';
  const elapsed = fmt.seconds(data.state.testRunDurationMs / 1000);
  const requests = value('http_reqs', 'count');
  const failed = value('http_req_failed', 'passes'); // The rate metric counts failures as passes.
  const dropped = value('dropped_iterations', 'count');
  const received = value('data_received', 'count');
  const receiveRate = value('data_received', 'rate');
  const load =
    RATE > 0
      ? `${RATE} req/s target, constant-arrival-rate, up to ${VUS} VUs, peak ${value('vus', 'max')}`
      : `${VUS} VUs back-to-back, constant-vus`;
  const sequential =
    `${fmt.bytes(FILE_SIZE)} in ${fmt.bytes(CHUNK_SIZE)} chunks, ${CHUNKS_PER_PASS} per pass, ` +
    `${STREAMS} streams over ${URL_COUNT} URLs`;

  const lines = [
    '',
    'proxy-bench',
    '',
    row('Run', `${MODE} via ${proxy}, ${elapsed}`),
    row('Target', RANGE ? `${TARGET_URL}, Range: ${RANGE}` : TARGET_URL),
    row('Load', load),
    ...(MODE === 'sequential' ? [row('Sequential', sequential)] : []),
    '',
    row(
      'Requests',
      `${fmt.int(requests)} total, ${value('http_reqs', 'rate').toFixed(1)} req/s, ${fmt.int(dropped)} dropped`,
    ),
    row('Failed', `${fmt.int(failed)} of ${fmt.int(requests)} (${fmt.pct(failed, requests)})`),
    ...checks.map((c) =>
      row('Check', `${mark(c.fails === 0)} ${c.name}, ${fmt.int(c.passes)} of ${fmt.int(c.passes + c.fails)}`),
    ),
    '',
    row('Latency (ms)', cells(stats)),
    ...LATENCY_ROWS.map(([label, name]) => row(`  ${label}`, cells(stats.map((s) => fmt.ms(value(name, s)))))),
    '',
    row('Throughput', `${fmt.bytes(received)} received, ${fmt.gbps(receiveRate)} Gbps`),
    ...(SEED_CLIENT_CPUS > 0 ? [row('CPU cost', cpuCost(receiveRate))] : []),
    '',
    row(
      'Result',
      `${paint(passed, passed ? 'PASSED' : 'FAILED')}, ${verdicts.map((t) => `${mark(t.ok)} ${t.name}`).join(', ')}`,
    ),
    '',
  ];

  return { stdout: lines.join('\n') + '\n' };
}
