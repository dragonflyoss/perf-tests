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

package plan

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dragonflyoss/perf-tests/pkg/fixedpool/contract"
)

const consumerShardJSON = "[1,2,4]\n"

func consumerFixture(t *testing.T) (contract.Manifest, []byte, string) {
	t.Helper()
	m := contract.Manifest{
		SchemaVersion: 1, ObjectSize: 64, ObjectCount: 2,
		Objects: []contract.Object{{ID: 0, SHA256: strings.Repeat("a", 64)}, {ID: 1, SHA256: strings.Repeat("b", 64)}},
	}
	data, err := contract.Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "shards"), 0755); err != nil {
		t.Fatal(err)
	}
	shard := []byte(consumerShardJSON)
	p := contract.Plan{SchemaVersion: 1, ManifestSHA256: contract.Digest(data), RangeBytes: 16,
		RegionsPerObject: 4, RegionCount: 8, WorkerCount: 3,
		Shards: []contract.Shard{
			{WorkerID: 0, RegionCount: 3, Path: "shards/0.json", SHA256: contract.Digest(shard)},
			{WorkerID: 1, RegionCount: 3, Path: "shards/1.json", SHA256: strings.Repeat("b", 64)},
			{WorkerID: 2, RegionCount: 2, Path: "shards/2.json", SHA256: strings.Repeat("c", 64)},
		},
	}
	raw, err := contract.Encode(p)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(directory, "request-plan.json")
	for name, raw := range map[string][]byte{name: raw, filepath.Join(directory, "shards/0.json"): shard} {
		if err := os.WriteFile(name, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return m, data, name
}

func overwrite(t *testing.T, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCorruptPlanRejected(t *testing.T) {
	const missingShard = "missing shard"
	for _, kind := range []string{"missing plan", missingShard, "manifest mismatch", "worker outside plan", "null worker_id", "missing zero worker_id", "unknown", "duplicate", "out of bounds", "wrong count", "extra region"} {
		t.Run(kind, func(t *testing.T) {
			m, data, name := consumerFixture(t)
			body := readFile(t, name)
			switch kind {
			case "missing plan", missingShard:
				path := name
				if kind == missingShard {
					path = filepath.Join(filepath.Dir(name), "shards/0.json")
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "manifest mismatch":
				overwrite(t, name, bytes.Replace(body, []byte(contract.Digest(data)), []byte(strings.Repeat("f", 64)), 1))
			case "null worker_id":
				overwrite(t, name, bytes.Replace(body, []byte(`"worker_id":0`), []byte(`"worker_id":null`), 1))
			case "missing zero worker_id":
				overwrite(t, name, bytes.Replace(body, []byte(`"worker_id":0,`), nil, 1))
			case "unknown":
				overwrite(t, name, bytes.Replace(body, []byte(`"schema_version":1`), []byte(`"extra":1,"schema_version":1`), 1))
			case "duplicate":
				overwrite(t, name, bytes.Replace(body, []byte(`"schema_version":1`), []byte(`"schema_version":1,"schema_version":1`), 1))
			case "out of bounds", "wrong count", "extra region":
				shard := []byte("[2,1,4]\n")
				if kind == "out of bounds" {
					shard = []byte("[2,1,8]\n")
				}
				if kind == "wrong count" {
					shard = []byte("[2,1]\n")
				}
				if kind == "extra region" {
					shard = []byte("[2,1,4,0]\n")
				}
				overwrite(t, filepath.Join(filepath.Dir(name), "shards/0.json"), shard)
				body = bytes.Replace(body, []byte(contract.Digest([]byte(consumerShardJSON))), []byte(contract.Digest(shard)), 1)
				overwrite(t, name, body)
			}
			selected := uint64(0)
			if kind == "worker outside plan" {
				selected = 3
			}
			_, _, err := LoadWorker(context.Background(), name, m, contract.Digest(data), selected)
			if err == nil {
				t.Fatal("corrupt plan accepted")
			}
			code := contract.ContractMismatch
			if kind == "missing plan" || kind == missingShard {
				code = contract.InputInvalid
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("file error lost its cause: %v", err)
				}
			}
			if kind == "worker outside plan" {
				code = contract.InputInvalid
			}
			if contract.ErrorCode(err) != code {
				t.Fatalf("expected %s, actual %s: %v", code, contract.ErrorCode(err), err)
			}
			if strings.Count(err.Error(), "PLAN \""+name+"\":") != 1 {
				t.Fatalf("expected one root plan context: %v", err)
			}
			want := map[string]string{
				"out of bounds": "region[2]: expected integer in [0,8), actual 8",
				"extra region":  "region[3]: unexpected extra entry; expected 3 regions",
			}[kind]
			if want != "" && (!strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "PLAN.shards[0]") || !strings.Contains(err.Error(), filepath.Join(filepath.Dir(name), "shards/0.json"))) {
				t.Fatalf("missing shard path or region position: %v", err)
			}
		})
	}
}

func TestConsumptionValidation(t *testing.T) {
	m, data, name := consumerFixture(t)
	if _, _, err := LoadWorker(context.Background(), name, m, contract.Digest(data), 0); err != nil {
		t.Fatalf("consumer repeated canonical order validation: %v", err)
	}
	// Changing bytes without updating the published digest remains an error.
	overwrite(t, filepath.Join(filepath.Dir(name), "shards/0.json"), []byte("[2,1,4]\n"))
	if _, _, err := LoadWorker(context.Background(), name, m, contract.Digest(data), 0); err == nil {
		t.Fatal("consumer ignored shard integrity")
	}
}
