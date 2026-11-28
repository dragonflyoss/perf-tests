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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/dragonflyoss/perf-tests/pkg/fixedpool/contract"
)

// LoadWorker validates common metadata and only the selected shard's bytes.
// Other workers' files need not be mounted in this process.
func LoadWorker(ctx context.Context, name string, manifest contract.Manifest, manifestSHA string, selected uint64) (p contract.Plan, raw []byte, err error) {
	defer func() {
		if err != nil {
			err = contract.Input("PLAN", name, err)
		}
	}()
	raw, err = contract.ReadJSON(name, &p)
	if err != nil {
		return p, nil, err
	}
	if p.SchemaVersion != contract.SchemaVersion || p.ManifestSHA256 != manifestSHA {
		return p, nil, contract.Tag(contract.ContractMismatch, fmt.Errorf(
			"schema_version/manifest_sha256: expected %d/%s, actual %d/%s",
			contract.SchemaVersion, manifestSHA, p.SchemaVersion, p.ManifestSHA256,
		))
	}
	perObject, total, err := calculateRegions(manifest.ObjectSize, manifest.ObjectCount, p.RangeBytes, p.WorkerCount)
	if err != nil {
		return p, nil, contract.Tag(contract.ContractMismatch, err)
	}
	if p.RegionsPerObject != perObject || p.RegionCount != total || uint64(len(p.Shards)) != p.WorkerCount {
		return p, nil, contract.Tag(contract.ContractMismatch, fmt.Errorf(
			"region metadata mismatch: expected regions_per_object=%d region_count=%d worker_count=%d, actual %d/%d/%d",
			perObject, total, p.WorkerCount, p.RegionsPerObject, p.RegionCount, len(p.Shards),
		))
	}
	if selected >= p.WorkerCount {
		return p, nil, contract.Tag(contract.InputInvalid, fmt.Errorf("WORKER_ID %d outside worker_count %d", selected, p.WorkerCount))
	}
	for worker, shard := range p.Shards {
		if shard.WorkerID != uint64(worker) || shard.RegionCount != shardCount(total, p.WorkerCount, uint64(worker)) || shard.Path != "shards/"+strconv.Itoa(worker)+".json" || !contract.ValidDigest(shard.SHA256) {
			return p, nil, contract.Tag(contract.ContractMismatch, fmt.Errorf(
				"shards[%d] metadata: expected worker_id=%d region_count=%d path=%q and a lowercase SHA-256, actual %+v",
				worker, worker, shardCount(total, p.WorkerCount, uint64(worker)), "shards/"+strconv.Itoa(worker)+".json", shard,
			))
		}
	}
	shard := p.Shards[selected]
	path := filepath.Join(filepath.Dir(name), filepath.FromSlash(shard.Path))
	if err := checkShard(ctx, path, shard, total); err != nil {
		return p, nil, contract.Input(fmt.Sprintf("PLAN.shards[%d]", selected), path, err)
	}
	return p, raw, nil
}

func checkShard(ctx context.Context, name string, shard contract.Shard, total uint64) (err error) {
	info, err := os.Lstat(name)
	if err != nil {
		return contract.Tag(contract.InputInvalid, err)
	}
	if !info.Mode().IsRegular() {
		return contract.Tag(contract.ContractMismatch, fmt.Errorf("shard must be a regular file"))
	}
	f, err := os.Open(name)
	if err != nil {
		return contract.Tag(contract.InputInvalid, err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	hash := sha256.New()
	decoder := json.NewDecoder(io.TeeReader(f, hash))
	decoder.UseNumber()
	start, err := decoder.Token()
	if err != nil || start != json.Delim('[') {
		return contract.Tag(contract.ContractMismatch, fmt.Errorf("expected shard JSON array"))
	}
	var count uint64
	for decoder.More() {
		if err := ctx.Err(); err != nil {
			return err
		}
		token, err := decoder.Token()
		if err != nil {
			return contract.Tag(contract.ContractMismatch, err)
		}
		number, ok := token.(json.Number)
		if !ok {
			return contract.Tag(contract.ContractMismatch, fmt.Errorf("region must be an integer"))
		}
		g, err := strconv.ParseUint(string(number), 10, 32)
		if err != nil || g >= total {
			return contract.Tag(contract.ContractMismatch, fmt.Errorf("region[%d]: expected integer in [0,%d), actual %s", count, total, number))
		}
		if count >= shard.RegionCount {
			return contract.Tag(contract.ContractMismatch, fmt.Errorf("region[%d]: unexpected extra entry; expected %d regions", count, shard.RegionCount))
		}
		count++
	}
	if _, err := decoder.Token(); err != nil {
		return contract.Tag(contract.ContractMismatch, err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return contract.Tag(contract.ContractMismatch, fmt.Errorf("unexpected data after shard array"))
	}
	if count != shard.RegionCount || hex.EncodeToString(hash.Sum(nil)) != shard.SHA256 {
		return contract.Tag(contract.ContractMismatch, fmt.Errorf(
			"shard count/sha256 mismatch: expected count=%d sha256=%s, actual count=%d sha256=%s",
			shard.RegionCount, shard.SHA256, count, hex.EncodeToString(hash.Sum(nil)),
		))
	}
	return nil
}
