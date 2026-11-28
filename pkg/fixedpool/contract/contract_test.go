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

package contract

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareOutput(t *testing.T) {
	ctx := context.Background()
	directory := filepath.Join(t.TempDir(), "new", "output")
	subdirectories := []string{"nested", "nested/child"}
	prepared, err := PrepareOutput(ctx, directory, subdirectories)
	if err != nil || prepared != directory {
		t.Fatalf("prepare output: %q %v", prepared, err)
	}
	if info, err := os.Stat(filepath.Join(directory, "nested", "child")); err != nil || !info.IsDir() {
		t.Fatalf("output skeleton was not created: %v", err)
	}
	if _, err := PrepareOutput(ctx, directory, subdirectories); err != nil {
		t.Fatalf("rejected an existing empty skeleton: %v", err)
	}
	notes := filepath.Join(directory, "notes")
	if err := os.WriteFile(notes, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareOutput(ctx, directory, subdirectories); ErrorCode(err) != DataConflict {
		t.Fatalf("accepted nonempty output: %v", err)
	}
	if data, err := os.ReadFile(notes); err != nil || string(data) != "keep" {
		t.Fatalf("changed existing file: %q %v", data, err)
	}
	linkedRoot := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(linkedRoot, "nested")); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareOutput(ctx, linkedRoot, []string{"nested"}); err == nil {
		t.Fatal("accepted a symbolic link as an output directory")
	}
}

func TestPublicationDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	temporary, final := filepath.Join(dir, ".result.json.tmp"), filepath.Join(dir, "result.json")
	digest, err := PublishJSON(context.Background(), final, "complete")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(temporary); !os.IsNotExist(err) {
		t.Fatalf("published temporary remains: %v", err)
	}
	if _, err := PublishJSON(context.Background(), final, "replacement"); ErrorCode(err) != DataConflict {
		t.Fatalf("publication replaced a result: %v", err)
	}
	data, err := os.ReadFile(final)
	if err != nil || string(data) != "\"complete\"\n" || Digest(data) != digest {
		t.Fatalf("damaged result %q %v", data, err)
	}
	temporary, final = filepath.Join(dir, ".cancelled.json.tmp"), filepath.Join(dir, "cancelled.json")
	if err := os.WriteFile(temporary, []byte("complete"), 0444); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Publish(ctx, temporary, final); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled publication: %v", err)
	}
	if _, err := os.Stat(final); !os.IsNotExist(err) {
		t.Fatalf("cancelled result was published: %v", err)
	}
	if _, err := os.Stat(temporary); err != nil {
		t.Fatalf("cancelled publication removed its working file: %v", err)
	}
}
