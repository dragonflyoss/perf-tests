// Shared presentation only: native k6 metrics and thresholds retain their meaning.
export const summaryTrendStats = ['min', 'avg', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'];

// Formatting for the report.
const fmt = {
  int: (n) => (Number.isFinite(n) ? String(Math.round(n)).replace(/\B(?=(\d{3})+(?!\d))/g, ',') : 'n/a'),
  ms: (n) => (Number.isFinite(n) ? n.toFixed(2) : 'n/a'),
  gbps: (bytesPerSecond) =>
    (Number.isFinite(bytesPerSecond) ? ((bytesPerSecond * 8) / 1e9).toFixed(2) : 'n/a'),
  pct: (part, total) =>
    (Number.isFinite(part) && total > 0 ? `${((100 * part) / total).toFixed(2)}%` : 'n/a'),
  seconds: (s) => (s < 120 ? `${s.toFixed(1)}s` : `${Math.floor(s / 60)}m${Math.round(s % 60)}s`),
  bytes: (n) => {
    if (!Number.isFinite(n)) {
      return 'n/a';
    }
    const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
    let i = 0;
    for (; n >= 1024 && i < units.length - 1; i++) {
      n /= 1024;
    }

    return `${Number.isInteger(n) ? n : n.toFixed(1)} ${units[i]}`;
  },
};

// Keep existing workload details in the same units as the shared report.
export const formatBytes = fmt.bytes;

// Latency rows of the report and the k6 metric behind each one.
const LATENCY_ROWS = [
  ['request', 'http_req_duration'],
  ['blocked', 'http_req_blocked'],
  ['connect', 'http_req_connecting'],
  ['sending', 'http_req_sending'],
  ['first byte', 'http_req_waiting'],
  ['download', 'http_req_receiving'],
  ['iteration', 'iteration_duration'],
];

// k6 omits dropped_iterations when none were dropped. Other missing
// samples remain unavailable in the display; native output is unchanged.
function metric(data, name, key) {
  const m = data.metrics[name];
  return m && m.values[key] !== undefined ? m.values[key] : name === 'dropped_iterations' ? 0 : NaN;
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
function cpuCost(bytesPerSecond, seedClientCPUs) {
  if (!Number.isFinite(bytesPerSecond)) {
    return 'n/a';
  }
  const gbps = (bytesPerSecond * 8) / 1e9;
  return gbps > 0
    ? `${seedClientCPUs} cores / ${gbps.toFixed(2)} Gbps = ${(seedClientCPUs / gbps).toFixed(3)} core/Gbps`
    : 'n/a, nothing received';
}

// The end-of-test report, printed in place of k6's default summary: run
// parameters, request counts, latency percentiles, throughput and, with
// SEED_CLIENT_CPUS, the CPU cost. Thresholds still decide the exit code, this
// only decides what is printed.
export function textSummary(data, context) {
  const {
    mode, proxy, target, rate, preAllocatedVUs, maxVUs, expectedStatus, details = [], seedClientCPUs = 0,
  } = context;
  const value = (name, key) => metric(data, name, key);
  const paint = painter(data);
  const mark = (ok) => paint(ok, ok ? '✓' : '✗');
  const row = (label, text) => `  ${label.padEnd(14)}${text}`;
  const cells = (texts) => texts.map((t) => t.padStart(9)).join('');

  const stats = data.options.summaryTrendStats;
  const verdicts = thresholds(data);
  const passed = verdicts.every((t) => t.ok);

  const elapsed = fmt.seconds(data.state.testRunDurationMs / 1000);
  const requests = value('http_reqs', 'count');
  const failed = value('http_req_failed', 'passes'); // The rate metric counts failures as passes.
  const dropped = value('dropped_iterations', 'count');
  const received = value('data_received', 'count');
  const receiveRate = value('data_received', 'rate');
  const load = `${rate} req/s target, constant-arrival-rate, ${preAllocatedVUs} to ${maxVUs} VUs`;

  const lines = [
    '',
    `proxy-bench (${mode})`,
    '',
    row('Run', `${mode} via ${proxy}, ${elapsed}`),
    row('Target', target),
    row('Load', load),
    ...details.map(([label, text]) => row(label, text)),
    '',
    row(
      'Requests',
      `${fmt.int(requests)} total, ${Number.isFinite(value('http_reqs', 'rate')) ? value('http_reqs', 'rate').toFixed(3) : 'n/a'} req/s, ${fmt.int(dropped)} dropped`,
    ),
    row('Iterations', `${fmt.int(value('iterations', 'count'))} complete`),
    row('VUs observed', `${fmt.int(value('vus', 'max'))} active peak, ${fmt.int(value('vus_max', 'max'))} allocated peak`),
    row(
      'Failed',
      `${fmt.int(failed)} of ${fmt.int(requests)} (${fmt.pct(failed, requests)}), ` +
        `status other than ${expectedStatus} or transport error`,
    ),
    '',
    row('Latency (ms)', cells(stats)),
    ...LATENCY_ROWS.map(([label, name]) => row(`  ${label}`, cells(stats.map((s) => fmt.ms(value(name, s)))))),
    '',
    row('Throughput', `${fmt.bytes(received)} received, ${fmt.gbps(receiveRate)} Gbps`),
    ...(seedClientCPUs > 0 ? [row('CPU cost', cpuCost(receiveRate, seedClientCPUs))] : []),
    '',
    row(
      'Result',
      verdicts.length
        ? `${paint(passed, passed ? 'PASSED' : 'FAILED')}, ${verdicts.map((t) => `${mark(t.ok)} ${t.name}`).join(', ')}`
        : 'NOT EVALUATED (no thresholds configured)',
    ),
    '',
  ];

  return lines.join('\n') + '\n';
}
