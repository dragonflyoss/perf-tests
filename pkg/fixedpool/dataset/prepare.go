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

package dataset

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/dragonflyoss/perf-tests/pkg/fixedpool/contract"
)

const (
	contentSeed      = "nydus-range-content-v1"
	writeBufferBytes = 1 << 20
)

type Spec struct {
	Root              string
	Size              uint64
	Count             uint64
	GenerationWorkers uint64
}

type Result struct {
	ManifestPath   string
	ManifestSHA256 string
}

func Prepare(ctx context.Context, spec Spec) (result Result, err error) {
	if spec.GenerationWorkers == 0 {
		return result, contract.Tag(contract.InputInvalid, fmt.Errorf("generation-workers: expected a positive integer, actual 0"))
	}
	if spec.Size == 0 || spec.Size > contract.MaxSafeInteger || spec.Count == 0 || spec.Count > contract.MaxSafeInteger {
		return result, contract.Tag(contract.InputInvalid, fmt.Errorf("positive JavaScript-safe size/count are required"))
	}
	if spec.Count > math.MaxUint64/spec.Size {
		return result, contract.Tag(contract.InputInvalid, fmt.Errorf("dataset byte count overflows uint64"))
	}
	requiredBytes := spec.Count * spec.Size
	root, err := contract.PrepareOutput(ctx, spec.Root,
		[]string{"objects", "objects/sha256"})
	if err != nil {
		return result, err
	}
	var filesystem syscall.Statfs_t
	if err := syscall.Statfs(root, &filesystem); err != nil {
		return result, contract.OutputError(root, err)
	}
	available := filesystem.Bavail * uint64(filesystem.Bsize)
	if requiredBytes > available {
		return result, fmt.Errorf("insufficient free space at %s: need %d bytes, available %d", root, requiredBytes, available)
	}
	for _, directory := range []string{filepath.Join(root, "objects"), root} {
		if err := contract.SyncDir(directory); err != nil {
			return result, err
		}
	}
	manifest := contract.Manifest{
		SchemaVersion: contract.SchemaVersion,
		ObjectSize:    spec.Size,
		Objects:       make([]contract.Object, int(spec.Count)),
	}
	if err := generateObjects(ctx, root, spec.GenerationWorkers, &manifest); err != nil {
		return result, err
	}
	manifest.ObjectCount = spec.Count
	name := filepath.Join(root, "dataset-manifest.json")
	digest, err := contract.PublishJSON(ctx, name, manifest)
	if err != nil {
		return result, err
	}
	result.ManifestPath, result.ManifestSHA256 = name, digest
	return result, nil
}

func generateObjects(ctx context.Context, root string, workers uint64, manifest *contract.Manifest) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan uint64)
	var group sync.WaitGroup
	var once sync.Once
	var firstErr error
	for range min(workers, uint64(len(manifest.Objects))) {
		group.Go(func() {
			for id := range jobs {
				object, err := writeObject(ctx, root, id, manifest.ObjectSize)
				if err != nil {
					once.Do(func() { firstErr = err; cancel() })
					return
				}
				manifest.Objects[id] = object
			}
		})
	}
dispatch:
	for id := uint64(0); id < uint64(len(manifest.Objects)); id++ {
		select {
		case jobs <- id:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(jobs)
	group.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

func writeObject(ctx context.Context, root string, id, size uint64) (contract.Object, error) {
	name := filepath.Join(root, "objects", fmt.Sprintf(".object-%016x.part", id))
	sha, err := writeObjectFile(ctx, name, id, size)
	if err != nil {
		return contract.Object{}, err
	}
	destination := filepath.Join(root, "objects", "sha256", sha)
	if err := contract.Publish(ctx, name, destination); err != nil {
		return contract.Object{}, err
	}
	return contract.Object{ID: id, SHA256: sha}, nil
}

func writeObjectFile(ctx context.Context, name string, id, size uint64) (digest string, err error) {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", contract.OutputError(name, err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	hash := sha256.New()
	if err := generateContent(ctx, io.MultiWriter(f, hash), id, size); err != nil {
		return "", err
	}
	if err := f.Chmod(0444); err != nil {
		return "", err
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// generateContent streams reproducible bytes; request geometry never enters the key,
// counter, or write buffer. The public fixed key is not a confidentiality feature.
func generateContent(ctx context.Context, writer io.Writer, objectID, size uint64) error {
	key := sha256.Sum256([]byte(contentSeed))
	blockCipher, err := aes.NewCipher(key[:])
	if err != nil {
		return err
	}
	buffer := make([]byte, writeBufferBytes)
	var counter [aes.BlockSize]byte
	binary.BigEndian.PutUint64(counter[:8], objectID)
	stream := cipher.NewCTR(blockCipher, counter[:])
	for remaining := size; remaining > 0; {
		if err := ctx.Err(); err != nil {
			return err
		}
		length := min(uint64(len(buffer)), remaining)
		// CTR over zero bytes reproduces the fixed AES counter stream.
		clear(buffer[:length])
		stream.XORKeyStream(buffer[:length], buffer[:length])
		written, err := writer.Write(buffer[:length])
		if err != nil {
			return err
		}
		if uint64(written) != length {
			return io.ErrShortWrite
		}
		remaining -= length
	}
	return nil
}
