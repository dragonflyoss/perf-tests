/*
 *     Copyright 2026 The Dragonfly Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dragonflyoss/perf-tests/pkg/fixedpool/contract"
)

func baseEnv() []string {
	return []string{"MANIFEST=m", "PLAN=p", "SOURCE_BASE_URL=http://origin.test", "HTTP_PROXY=http://seed.test:4001", "RATE=100"}
}
func TestEnvironmentDefaultsAndRejections(t *testing.T) {
	e, err := ReadEnvironment(baseEnv())
	if err != nil {
		t.Fatal(err)
	}
	if e.RunID != "" || e.WorkerID != 0 || e.Duration != 300*time.Second || e.Timeout != 30*time.Second || e.GracefulStop != 35*time.Second || e.PreAllocatedVUs != 64 || e.MaxVUs != 512 {
		t.Fatal(e)
	}
	for _, extra := range []string{
		"RATE=", "WORKER_ID=", "DURATION=", "GRACEFUL_STOP=",
		"SOURCE_BASE_URL=", "SOURCE_BASE_URL=https://unsupported.test",
		"HTTP_PROXY=", "HTTP_PROXY=abc", "HTTP_PROXY=http://seed.test:0",
		"RATE=0", "RATE=1.5", "RATE=2147483648", "WORKER_ID=4294967296", "RUN_ID=../x",
		"DURATION=0s", "DURATION=1us", "DURATION=999ms", "DURATION=1000001us",
		"TIMEOUT=bad", "TIMEOUT=9223372036854ms",
		"GRACEFUL_STOP=30s", "MAX_VUS=63", "MAX_VUS=2147483648", "PRE_ALLOCATED_VUS=0",
	} {
		t.Run(extra, func(t *testing.T) {
			key, _, _ := strings.Cut(extra, "=")
			if _, err := ReadEnvironment(append(baseEnv(), extra)); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("expected %s rejection, actual %v", key, err)
			}
		})
	}
	for _, key := range []string{"MANIFEST", "PLAN", "SOURCE_BASE_URL", "HTTP_PROXY", "RATE"} {
		var missing []string
		for _, pair := range baseEnv() {
			if !strings.HasPrefix(pair, key+"=") {
				missing = append(missing, pair)
			}
		}
		if _, err := ReadEnvironment(missing); err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("missing %s: %v", key, err)
		}
	}
	for _, extra := range []string{"MODE=fixed-pool", "K6_NO_CONNECTION_REUSE=true", "RUN_ID=", "RUN_ID=custom-1"} {
		if _, err := ReadEnvironment(append(baseEnv(), extra)); err != nil {
			t.Fatalf("valid environment rejected: %v", err)
		}
	}
	e, err = ReadEnvironment(append(baseEnv(), "TIMEOUT=100ms", "DURATION=5s"))
	if err != nil || e.GracefulStop != 5100*time.Millisecond {
		t.Fatalf("%+v %v", e, err)
	}
	e, err = ReadEnvironment(append(baseEnv(), "TIMEOUT=100ms", "GRACEFUL_STOP=200ms"))
	if err != nil || e.GracefulStop != 200*time.Millisecond {
		t.Fatalf("explicit GRACEFUL_STOP was not preserved: %+v %v", e, err)
	}
	for _, value := range []string{"1s", "1001ms"} {
		if _, err := ReadEnvironment(append(baseEnv(), "DURATION="+value)); err != nil {
			t.Fatalf("rejected supported duration %s: %v", value, err)
		}
	}
}
func save(t *testing.T, name string, value any) []byte {
	t.Helper()
	raw, err := contract.Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(name, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestPreparedInputsAreBoundAndLocalShardOnly(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	m := contract.Manifest{SchemaVersion: 1, ObjectSize: 64, ObjectCount: 2}
	for i, ch := range []string{"a", "b"} {
		sha := strings.Repeat(ch, 64)
		m.Objects = append(m.Objects, contract.Object{ID: uint64(i), SHA256: sha})
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	manifestRaw := save(t, manifestPath, m)
	manifestID := contract.Digest(manifestRaw)
	if err := os.Mkdir(filepath.Join(dir, "shards"), 0700); err != nil {
		t.Fatal(err)
	}
	shardPath := filepath.Join(dir, "shards/1.json")
	shardRaw := save(t, shardPath, []uint32{2, 0, 5, 1})
	p := contract.Plan{
		SchemaVersion: 1, ManifestSHA256: manifestID, RangeBytes: 16,
		RegionsPerObject: 4, RegionCount: 8, WorkerCount: 2,
		Shards: []contract.Shard{
			{WorkerID: 0, RegionCount: 4, Path: "shards/0.json", SHA256: strings.Repeat("c", 64)},
			{WorkerID: 1, RegionCount: 4, Path: "shards/1.json", SHA256: contract.Digest(shardRaw)},
		},
	}
	planPath := filepath.Join(dir, "request-plan.json")
	planRaw := save(t, planPath, p)
	env, err := ReadEnvironment(append(baseEnv(), "WORKER_ID=1", "RATE=17"))
	if err != nil {
		t.Fatal(err)
	}
	env.Manifest = manifestPath
	env.Plan = planPath
	spec, err := PrepareInput(ctx, env)
	if err != nil {
		t.Fatal(err)
	}
	if spec.BaseURL != "http://origin.test" || spec.Proxy != "http://seed.test:4001" ||
		spec.Manifest != manifestPath || spec.ManifestID != manifestID ||
		spec.Shard != shardPath || spec.Rate != 8 || spec.RegionCount != 4 ||
		spec.WorkerID != 1 || spec.PlanID != contract.Digest(planRaw) {
		t.Fatalf("%+v", spec)
	}
	if err := os.Remove(planPath); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareInput(ctx, env); contract.ErrorCode(err) != contract.InputInvalid || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worker changed the plan read error: %v", err)
	}
}

func TestAllocateRates(t *testing.T) {
	for _, test := range []struct {
		counts []uint64
		rate   uint64
		want   []uint64
	}{
		{[]uint64{3, 3, 2}, 100, []uint64{38, 37, 25}},
		{[]uint64{2, 1}, 2, []uint64{1, 1}},
		{[]uint64{1, 1, 1}, 5, []uint64{2, 2, 1}},
		{[]uint64{1}, 1, []uint64{1}},
		{[]uint64{1 << 31, 1<<31 - 1}, contract.MaxRate, []uint64{1 << 30, 1<<30 - 1}},
	} {
		got, err := allocateRates(test.counts, test.rate)
		if err != nil || !reflect.DeepEqual(got, test.want) {
			t.Fatalf("counts=%v rate=%d: %v %v", test.counts, test.rate, got, err)
		}
	}
	for _, test := range []struct {
		counts []uint64
		rate   uint64
	}{
		{[]uint64{2, 1}, 1}, {[]uint64{3, 1}, 2},
	} {
		if _, err := allocateRates(test.counts, test.rate); err == nil {
			t.Fatalf("accepted rate %v/%d", test.counts, test.rate)
		}
	}
}
