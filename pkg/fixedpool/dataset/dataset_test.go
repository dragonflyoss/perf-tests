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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/dragonflyoss/perf-tests/pkg/fixedpool/contract"
)

func TestContentVectorsAndTails(t *testing.T) {
	fixture := struct {
		size   uint64
		prefix []byte
		sha256 []string
	}{
		size:   64,
		prefix: []byte{0x4a, 0x8d, 0x62, 0x96, 0x79, 0x67, 0x33, 0x6b, 0x3c, 0xd8, 0xfd, 0x5d, 0x7b, 0x06, 0xb4, 0x07},
		sha256: []string{
			"66895a8785a1dee4951313348a79a5ee9def315d6728a9d303c8b6fe74a0fb13",
			"11bfb09936c1359bcafb5c3cc56fd25892f014397659ed786eeb2fa32bb60550",
		},
	}
	for id, want := range fixture.sha256 {
		var body bytes.Buffer
		if err := generateContent(context.Background(), &body, uint64(id), fixture.size); err != nil {
			t.Fatal(err)
		}
		if uint64(body.Len()) != fixture.size || id == 0 && !bytes.Equal(body.Bytes()[:len(fixture.prefix)], fixture.prefix) || contract.Digest(body.Bytes()) != want {
			t.Fatalf("object %d vector mismatch", id)
		}
	}
	var whole bytes.Buffer
	if err := generateContent(context.Background(), &whole, 0, writeBufferBytes+17); err != nil {
		t.Fatal(err)
	}
	// Independent AES-CTR vector covers counter continuity across the write buffer.
	if got := contract.Digest(whole.Bytes()); got != "84fca477c4845c41005d45d17b0fdbdbfff8d0d7b3a6356e7d2af499cf0c5fc5" {
		t.Fatalf("cross-buffer vector mismatch: %s", got)
	}
	for _, size := range []uint64{1, 15, 16, 17, writeBufferBytes - 1, writeBufferBytes, writeBufferBytes + 1} {
		var body bytes.Buffer
		if err := generateContent(context.Background(), &body, 0, size); err != nil {
			t.Fatal(err)
		}
		if uint64(body.Len()) != size || !bytes.Equal(body.Bytes(), whole.Bytes()[:size]) {
			t.Fatalf("incorrect partial block at size %d", size)
		}
	}
}

type shortWriter struct{}

func (shortWriter) Write(data []byte) (int, error) { return len(data) - 1, nil }

type cancelWriter struct {
	cancel context.CancelFunc
	bytes  int
}

func (w *cancelWriter) Write(data []byte) (int, error) {
	w.bytes += len(data)
	w.cancel()
	return len(data), nil
}

func TestGeneratorWriteFailureAndCancellation(t *testing.T) {
	if err := generateContent(context.Background(), shortWriter{}, 0, 64); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write error=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &cancelWriter{cancel: cancel}
	if err := generateContent(ctx, w, 0, writeBufferBytes*2); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
	if w.bytes != writeBufferBytes {
		t.Fatalf("wrote %d bytes after cancellation", w.bytes)
	}
}

func newPool(t *testing.T, count, workers uint64) (string, Result) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "source")
	result, err := Prepare(context.Background(), Spec{Root: root, Size: 64, Count: count, GenerationWorkers: workers})
	if err != nil {
		t.Fatal(err)
	}
	return root, result
}

func read(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestIndependentDatasetContentAndConcurrency(t *testing.T) {
	root, original := newPool(t, 2, 1)
	other, parallel := newPool(t, 2, 4)
	third, larger := newPool(t, 3, 2)
	before := read(t, original.ManifestPath)
	var manifest, largerManifest contract.Manifest
	if err := json.Unmarshal(before, &manifest); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(read(t, larger.ManifestPath), &largerManifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 1 || manifest.ObjectSize != 64 || manifest.ObjectCount != 2 || len(manifest.Objects) != 2 {
		t.Fatalf("incorrect generated manifest: %+v", manifest)
	}
	if !bytes.Equal(before, read(t, parallel.ManifestPath)) {
		t.Fatal("manifest changes with concurrency")
	}
	if largerManifest.ObjectCount != 3 || len(largerManifest.Objects) != 3 {
		t.Fatalf("incorrect larger dataset manifest: %+v", largerManifest)
	}
	for i, object := range manifest.Objects {
		if object.ID != uint64(i) {
			t.Fatalf("object[%d]: incorrect ID %d", i, object.ID)
		}
		for _, pool := range []string{other, third} {
			if !bytes.Equal(read(t, filepath.Join(root, "objects/sha256/"+object.SHA256)), read(t, filepath.Join(pool, "objects/sha256/"+object.SHA256))) {
				t.Fatal("pool count or concurrency changes common bytes")
			}
		}
		if object != largerManifest.Objects[i] {
			t.Fatal("N changes common object identity")
		}
	}
}

func TestExistingDatasetIsNotReplaced(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "objects", "sha256"), 0755); err != nil {
		t.Fatal(err)
	}
	old, err := Prepare(context.Background(), Spec{Root: root, Size: 64, Count: 2, GenerationWorkers: 1})
	if err != nil {
		t.Fatal(err)
	}
	before := read(t, old.ManifestPath)
	if _, err := Prepare(context.Background(), Spec{Root: root, Size: 64, Count: 3, GenerationWorkers: 1}); contract.ErrorCode(err) != contract.DataConflict {
		t.Fatalf("accepted replacement dataset: %v", err)
	}
	if !bytes.Equal(before, read(t, old.ManifestPath)) {
		t.Fatal("rejected command changed old manifest")
	}
}

func TestObjectConflictDoesNotOverwrite(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "objects/sha256")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(directory, "66895a8785a1dee4951313348a79a5ee9def315d6728a9d303c8b6fe74a0fb13")
	if err := os.WriteFile(name, []byte("sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := writeObject(context.Background(), root, 0, 64); contract.ErrorCode(err) != contract.DataConflict {
		t.Fatalf("object conflict: %v", err)
	}
	if string(read(t, name)) != "sentinel" {
		t.Fatal("overwrote conflicting object")
	}
}

func TestCancelledAndIncompleteDatasetsAreRejected(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	spec := Spec{Root: root, Size: 64, Count: 2, GenerationWorkers: 2}
	result, err := Prepare(ctx, spec)
	if !errors.Is(err, context.Canceled) || result.ManifestPath != "" {
		t.Fatalf("interrupted publication: %+v %v", result, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("cancelled preparation created output: %v", err)
	}
	spec.Root = t.TempDir()
	if err := os.Mkdir(filepath.Join(spec.Root, "objects"), 0755); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(spec.Root, "objects", ".object-0000000000000001.part")
	if err := os.WriteFile(partial, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(context.Background(), spec); contract.ErrorCode(err) != contract.DataConflict {
		t.Fatalf("adopted incomplete dataset: %v", err)
	}
	if string(read(t, partial)) != "partial" {
		t.Fatal("overwrote incomplete object")
	}
	if _, err := os.Stat(filepath.Join(spec.Root, "dataset-manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("incomplete dataset published a manifest: %v", err)
	}
}
