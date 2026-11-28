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
	"bufio"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strconv"

	"github.com/dragonflyoss/perf-tests/pkg/fixedpool/contract"
)

const orderSeed = "nydus-range-order-v1"

type Spec struct {
	ManifestPath string
	Directory    string
	RangeBytes   uint64
	Workers      uint64
}

type Result struct {
	Plan              contract.Plan
	ObjectSize        uint64
	ObjectCount       uint64
	Path              string
	SHA256            string
	PlannedBytes      string
	ExcludedTailBytes string
}

// calculateRegions uses size and count from a validated manifest.
func calculateRegions(size, count, rangeBytes, workers uint64) (perObject, total uint64, err error) {
	if rangeBytes == 0 || rangeBytes > size {
		return 0, 0, fmt.Errorf("R: expected 1..B=%d, actual %d", size, rangeBytes)
	}
	perObject = size / rangeBytes
	if perObject > contract.MaxRegions || count > contract.MaxRegions/perObject {
		return 0, 0, fmt.Errorf("region_count: expected <= %d, actual N=%d regions_per_object=%d", contract.MaxRegions, count, perObject)
	}
	total = count * perObject
	if workers == 0 || workers > total {
		return 0, 0, fmt.Errorf("workers: expected 1..%d, actual %d", total, workers)
	}
	return perObject, total, nil
}

func Prepare(ctx context.Context, spec Spec) (result Result, err error) {
	manifest, manifestData, err := contract.ReadManifest(spec.ManifestPath)
	if err != nil {
		return result, err
	}
	perObject, total, err := calculateRegions(manifest.ObjectSize, manifest.ObjectCount, spec.RangeBytes, spec.Workers)
	if err != nil {
		return result, contract.Tag(contract.InputInvalid, err)
	}
	result.ObjectSize, result.ObjectCount = manifest.ObjectSize, manifest.ObjectCount
	result.Plan = contract.Plan{
		SchemaVersion:    contract.SchemaVersion,
		RangeBytes:       spec.RangeBytes,
		RegionsPerObject: perObject,
		RegionCount:      total,
		WorkerCount:      spec.Workers,
	}
	// Aggregate byte counts may exceed uint64 even when each region is valid.
	planned := new(big.Int).SetUint64(total)
	planned.Mul(planned, new(big.Int).SetUint64(spec.RangeBytes))
	excluded := new(big.Int).SetUint64(manifest.ObjectCount)
	excluded.Mul(excluded, new(big.Int).SetUint64(manifest.ObjectSize%spec.RangeBytes))
	result.PlannedBytes, result.ExcludedTailBytes = planned.String(), excluded.String()
	directory, err := contract.PrepareOutput(ctx, spec.Directory, []string{"shards"})
	if err != nil {
		return result, err
	}
	p := &result.Plan
	p.ManifestSHA256 = contract.Digest(manifestData)
	if err := generateShards(ctx, directory, p); err != nil {
		return result, err
	}
	name := filepath.Join(directory, "request-plan.json")
	digest, err := contract.PublishJSON(ctx, name, p)
	if err != nil {
		return result, err
	}
	result.Path, result.SHA256 = name, digest
	return result, nil
}

func generateShards(ctx context.Context, directory string, p *contract.Plan) error {
	p.Shards = make([]contract.Shard, int(p.WorkerCount))
	values, err := permutation(ctx, p.RegionCount)
	if err != nil {
		return err
	}
	for worker := uint64(0); worker < p.WorkerCount; worker++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := "shards/" + strconv.FormatUint(worker, 10) + ".json"
		digest, err := writeShard(ctx, filepath.Join(directory, filepath.FromSlash(path)), values, worker, p.WorkerCount)
		if err != nil {
			return err
		}
		p.Shards[worker] = contract.Shard{WorkerID: worker, RegionCount: shardCount(p.RegionCount, p.WorkerCount, worker), Path: path, SHA256: digest}
	}
	if err := contract.SyncDir(filepath.Join(directory, "shards")); err != nil {
		return err
	}
	return nil
}

func writeShard(ctx context.Context, name string, values []uint32, worker, workers uint64) (digest string, err error) {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0444)
	if err != nil {
		return "", contract.OutputError(name, err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	hash := sha256.New()
	buffer := bufio.NewWriterSize(io.MultiWriter(f, hash), 64<<10)
	if err := buffer.WriteByte('['); err != nil {
		return "", err
	}
	var encoded [10]byte
	for position := worker; position < uint64(len(values)); position += workers {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if position != worker {
			if err := buffer.WriteByte(','); err != nil {
				return "", err
			}
		}
		if _, err := buffer.Write(strconv.AppendUint(encoded[:0], uint64(values[position]), 10)); err != nil {
			return "", err
		}
	}
	if _, err := buffer.WriteString("]\n"); err != nil {
		return "", err
	}
	if err := buffer.Flush(); err != nil {
		return "", err
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func permutation(ctx context.Context, count uint64) ([]uint32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := sha256.Sum256([]byte(orderSeed))
	blockCipher, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	var counter [aes.BlockSize]byte
	stream := cipher.NewCTR(blockCipher, counter[:])
	var word [4]byte
	next := func() uint64 {
		clear(word[:])
		stream.XORKeyStream(word[:], word[:])
		return uint64(binary.BigEndian.Uint32(word[:]))
	}
	values := make([]uint32, int(count))
	for i := range values {
		if i%65536 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		values[i] = uint32(i)
	}
	for i := count - 1; i > 0; i-- {
		if i%65536 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		n := i + 1
		limit := (uint64(1) << 32) / n * n
		x := next()
		for x >= limit {
			x = next()
		}
		j := x % n
		values[i], values[j] = values[j], values[i]
	}
	return values, nil
}

func shardCount(total, workers, worker uint64) uint64 {
	return (total-1-worker)/workers + 1
}
