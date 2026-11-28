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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dragonflyoss/perf-tests/pkg/fixedpool/contract"
)

func TestPermutationVectors(t *testing.T) {
	want := []uint32{2, 0, 5, 1, 3, 7, 4, 6}
	got, err := permutation(context.Background(), 8)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("permutation=%v want %v", got, want)
	}
	for count := uint64(1); count < 200; count++ {
		got, err := permutation(context.Background(), count)
		if err != nil {
			t.Fatal(err)
		}
		if uint64(len(got)) != count {
			t.Fatalf("permutation at M=%d has %d entries", count, len(got))
		}
		seen := map[uint32]bool{}
		for _, g := range got {
			if uint64(g) >= count || seen[g] {
				t.Fatalf("invalid permutation at M=%d", count)
			}
			seen[g] = true
		}
	}
}

func TestRegionBoundaries(t *testing.T) {
	for _, g := range [][4]uint64{{64, 2, 0, 1}, {64, 2, 65, 1}, {64, 2, 16, 0}, {64, 2, 16, 9}, {contract.MaxSafeInteger, 1, 1, 1}, {1, contract.MaxRegions + 1, 1, 1}, {2, contract.MaxRegions, 1, 1}} {
		if _, _, err := calculateRegions(g[0], g[1], g[2], g[3]); err == nil {
			t.Fatalf("accepted access scope %v", g)
		}
	}
	for _, test := range []struct {
		input      [4]uint64
		per, total uint64
	}{
		{[4]uint64{1, contract.MaxRegions, 1, 1}, 1, contract.MaxRegions},
		{[4]uint64{contract.MaxSafeInteger, 1, contract.MaxSafeInteger, 1}, 1, 1},
		{[4]uint64{64, 2, 3, 1}, 21, 42},
		{[4]uint64{contract.MaxSafeInteger, 8192, 1 << 52, 1}, 1, 8192},
	} {
		g := test.input
		per, total, err := calculateRegions(g[0], g[1], g[2], g[3])
		if err != nil || per != test.per || total != test.total {
			t.Fatalf("regions %v: per_object=%d total=%d %v", g, per, total, err)
		}
	}
	// Byte totals belong to Prepare and may exceed uint64 without large shards.
	_, spec := fixture(t)
	m := contract.Manifest{SchemaVersion: 1, ObjectSize: contract.MaxSafeInteger, ObjectCount: 8192}
	for id := uint64(0); id < m.ObjectCount; id++ {
		m.Objects = append(m.Objects, contract.Object{ID: id, SHA256: fmt.Sprintf("%064x", id)})
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec.ManifestPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	spec.RangeBytes = 1 << 52
	result, err := Prepare(context.Background(), spec)
	if err != nil || result.PlannedBytes != "36893488147419103232" || result.ExcludedTailBytes != "36893488147419095040" {
		t.Fatalf("aggregate byte totals: %+v %v", result, err)
	}
}

func fixture(t *testing.T) ([]byte, Spec) {
	t.Helper()
	m := contract.Manifest{SchemaVersion: 1, ObjectSize: 64, ObjectCount: 2, Objects: []contract.Object{{ID: 0, SHA256: strings.Repeat("a", 64)}, {ID: 1, SHA256: strings.Repeat("b", 64)}}}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	root := t.TempDir()
	manifestPath := filepath.Join(root, "dataset-manifest.json")
	if err := os.WriteFile(manifestPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	return data, Spec{ManifestPath: manifestPath, Directory: filepath.Join(root, "plans"), RangeBytes: 16, Workers: 3}
}

func readFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPlanPublishConflictsAndGeometryChanges(t *testing.T) {
	data, spec := fixture(t)
	if err := os.MkdirAll(filepath.Join(spec.Directory, "shards"), 0755); err != nil {
		t.Fatal(err)
	}
	result, err := Prepare(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	raw := readFile(t, result.Path)
	var p contract.Plan
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if p.SchemaVersion != 1 || p.ManifestSHA256 != contract.Digest(data) || p.RangeBytes != 16 || p.RegionsPerObject != 4 || p.RegionCount != 8 || p.WorkerCount != 3 || len(p.Shards) != 3 {
		t.Fatalf("incorrect generated plan: %+v", p)
	}
	if contract.Digest(raw) != result.SHA256 || result.Plan.ManifestSHA256 != contract.Digest(data) {
		t.Fatal("published result inconsistent")
	}
	expected := [][]uint32{{2, 1, 4}, {0, 3, 6}, {5, 7}}
	for w, shard := range p.Shards {
		shardRaw := readFile(t, filepath.Join(filepath.Dir(result.Path), shard.Path))
		if shard.WorkerID != uint64(w) || shard.RegionCount != uint64(len(expected[w])) || shard.SHA256 != contract.Digest(shardRaw) {
			t.Fatalf("incorrect shard metadata: %+v", shard)
		}
		var got []uint32
		if err := json.Unmarshal(shardRaw, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, expected[w]) {
			t.Fatalf("worker %d=%v", w, got)
		}
	}
	if _, err := Prepare(context.Background(), spec); contract.ErrorCode(err) != contract.DataConflict {
		t.Fatalf("existing plan was reused or overwritten: %v", err)
	}
	occupied := filepath.Join(spec.Directory, p.Shards[1].Path)
	before := readFile(t, occupied)
	if _, err := writeShard(context.Background(), occupied, []uint32{0}, 0, 1); contract.ErrorCode(err) != contract.DataConflict {
		t.Fatalf("overwrote an existing shard: %v", err)
	}
	if !bytes.Equal(before, readFile(t, occupied)) {
		t.Fatal("changed an existing shard")
	}
	spec.Directory = filepath.Join(t.TempDir(), "w2")
	spec.Workers = 2
	changed, err := Prepare(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if changed.SHA256 == result.SHA256 {
		t.Fatal("W did not change plan")
	}
	var reconstructed [8]uint32
	for w := 0; w < 2; w++ {
		var values []uint32
		if err := json.Unmarshal(readFile(t, filepath.Join(filepath.Dir(changed.Path), "shards", string(rune('0'+w))+".json")), &values); err != nil {
			t.Fatal(err)
		}
		for i, g := range values {
			reconstructed[w+i*2] = g
		}
	}
	if reconstructed != [8]uint32{2, 0, 5, 1, 3, 7, 4, 6} {
		t.Fatal("W changed global order")
	}
	for _, test := range []struct {
		rangeBytes, regions uint64
		planned, excluded   string
	}{
		{32, 4, "128", "0"},
		{3, 42, "126", "2"},
	} {
		spec.Directory = filepath.Join(t.TempDir(), "plan")
		spec.RangeBytes = test.rangeBytes
		changedRange, err := Prepare(context.Background(), spec)
		if err != nil {
			t.Fatal(err)
		}
		var published contract.Plan
		if err := json.Unmarshal(readFile(t, changedRange.Path), &published); err != nil {
			t.Fatal(err)
		}
		if published.RegionCount != test.regions || changedRange.SHA256 == changed.SHA256 {
			t.Fatalf("incorrect plan for R=%d: %+v", test.rangeBytes, published)
		}
		if changedRange.PlannedBytes != test.planned || changedRange.ExcludedTailBytes != test.excluded {
			t.Fatalf("incorrect byte totals for R=%d: %+v", test.rangeBytes, changedRange)
		}
		seen := map[uint32]bool{}
		for _, shard := range published.Shards {
			var regions []uint32
			if err := json.Unmarshal(readFile(t, filepath.Join(filepath.Dir(changedRange.Path), shard.Path)), &regions); err != nil {
				t.Fatal(err)
			}
			if uint64(len(regions)) != shard.RegionCount {
				t.Fatalf("R=%d worker=%d: declared %d regions, wrote %d", test.rangeBytes, shard.WorkerID, shard.RegionCount, len(regions))
			}
			for _, region := range regions {
				if uint64(region) >= test.regions || seen[region] {
					t.Fatalf("R=%d: duplicate or out-of-range region %d", test.rangeBytes, region)
				}
				seen[region] = true
			}
		}
		if uint64(len(seen)) != test.regions {
			t.Fatalf("R=%d: expected %d regions, wrote %d", test.rangeBytes, test.regions, len(seen))
		}
	}
}

func TestCancelledPlanIsNotPublished(t *testing.T) {
	_, spec := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := Prepare(ctx, spec)
	if !errors.Is(err, context.Canceled) || result.Path != "" {
		t.Fatalf("cancelled plan: %+v %v", result, err)
	}
	if _, err := os.Stat(spec.Directory); !os.IsNotExist(err) {
		t.Fatalf("cancelled plan created output: %v", err)
	}
}
