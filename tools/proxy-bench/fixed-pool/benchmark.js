// Fixed-pool execution consumes prepared metadata only. No preparation hooks.
import http from 'k6/http';
import exec from 'k6/execution';
import { SharedArray } from 'k6/data';
import { Counter } from 'k6/metrics';
import { textSummary, summaryTrendStats } from '../lib/summary.js';

const spec = JSON.parse(__ENV.WORKER_SPEC);
const objects = new SharedArray('fixed-pool-objects', () => JSON.parse(open(spec.manifest)).objects.map(object => `${spec.base_url}/objects/sha256/${object.sha256}`));
const shard = new SharedArray('fixed-pool-shard', () => JSON.parse(open(spec.shard)));
if (shard.length !== spec.region_count) throw new Error('shard length mismatch');
const consumed = new Counter('plan_consumed');
http.setResponseCallback(http.expectedStatuses(206));

export const options = {
  scenarios: {
    fixed_pool: {
      executor: 'constant-arrival-rate', rate: spec.rate, timeUnit: '1s',
      duration: `${spec.duration_ms}ms`, gracefulStop: `${spec.graceful_stop_ms}ms`,
      preAllocatedVUs: spec.pre_allocated_vus, maxVUs: spec.max_vus,
    },
  },
  discardResponseBodies: true,
  noConnectionReuse: false,
  noVUConnectionReuse: false,
  maxRedirects: 0,
  summaryTrendStats,
};

// Keep the hot path to one shard lookup, integer geometry and one URL lookup.
function regionRequest(spec, objects, shard, iteration) {
  const g = shard[iteration % spec.region_count];
  const object = Math.floor(g / spec.regions_per_object);
  const start = (g % spec.regions_per_object) * spec.range_bytes;
  const end = start + spec.range_bytes - 1;
  const url = objects[object];
  if (!Number.isSafeInteger(iteration) || iteration < 0 ||
      !Number.isSafeInteger(g) || g < 0 || !Number.isSafeInteger(end) ||
      typeof url !== 'string' || end < start) {
    throw new Error('invalid fixed-pool plan mapping');
  }
  return { url, range: `bytes=${start}-${end}` };
}

export default function () {
  let request;
  try {
    request = regionRequest(spec, objects, shard, exec.scenario.iterationInInstance);
  } catch (error) {
    exec.test.abort(`fixed-pool mapping failed: ${error.message}`);
    return;
  }
  consumed.add(1);
  // One business call per iteration. With connection reuse the native HTTP
  // transport can resend an idempotent GET; this does not consume another item.
  http.get(request.url, {
    headers: { ...spec.headers, Range: request.range },
    timeout: `${spec.timeout_ms}ms`, redirects: 0, responseType: 'none',
    tags: { name: 'fixed-pool' },
  });
}

export function handleSummary(data) {
  const envelope = {
    run_id: spec.run_id, worker_id: spec.worker_id,
    manifest_id: spec.manifest_id, plan_id: spec.plan_id,
    effective_options: exec.test.options,
    native_summary: data,
  };
  const json = JSON.stringify(envelope);
  const consumed = data.metrics.plan_consumed?.values?.count;
  const report = textSummary(data, {
    mode: 'fixed-pool', proxy: spec.proxy,
    target: `dataset ${spec.manifest_id}, Range: ${spec.range_bytes} bytes`,
    rate: spec.rate, preAllocatedVUs: spec.pre_allocated_vus, maxVUs: spec.max_vus,
    expectedStatus: 206,
    details: [
      ...(spec.run_id ? [['Run ID', spec.run_id]] : []), ['Worker', String(spec.worker_id)], ['Plan', spec.plan_id],
      ['Consumed', `${Number.isFinite(consumed) ? consumed : 'n/a'} plan entries; shard ${spec.region_count} regions (repeats included)`],
    ],
  });
  return { stdout: report + `PROXY_BENCH_FIXED_POOL_K6_SUMMARY_V1 ${json}\n` };
}
