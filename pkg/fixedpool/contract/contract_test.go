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
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestStrictManifest(t *testing.T) {
	sha := "66895a8785a1dee4951313348a79a5ee9def315d6728a9d303c8b6fe74a0fb13"
	manifest := Manifest{SchemaVersion: 1, ObjectSize: 64, ObjectCount: 1, Objects: []Object{{ID: 0, SHA256: sha}}}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	valid := string(data)
	cases := map[string]string{
		"unknown root":     strings.Replace(valid, `{"schema_version":1`, `{"extra":0,"schema_version":1`, 1),
		"unknown nested":   strings.Replace(valid, `"object_id":0`, `"extra":0,"object_id":0`, 1),
		"duplicate":        strings.Replace(valid, `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1),
		"duplicate nested": strings.Replace(valid, `"object_id":0`, `"object_id":0,"object_id":0`, 1),
		"missing zero ID":  strings.Replace(valid, `"object_id":0,`, "", 1),
		"wrong case":       strings.Replace(valid, `"schema_version"`, `"SCHEMA_VERSION"`, 1),
		"case alias root":  strings.Replace(valid, `"schema_version":1`, `"schema_version":1,"SCHEMA_VERSION":1`, 1),
		"case alias inner": strings.Replace(valid, `"object_id":0`, `"object_id":0,"OBJECT_ID":0`, 1),
		"null":             strings.Replace(valid, `"object_id":0`, `"object_id":null`, 1),
		"schema":           strings.Replace(valid, `"schema_version":1`, `"schema_version":2`, 1),
		"zero size":        strings.Replace(valid, `"object_size_bytes":64`, `"object_size_bytes":0`, 1),
		"unsafe size":      strings.Replace(valid, `"object_size_bytes":64`, `"object_size_bytes":9007199254740992`, 1),
		"empty dataset":    `{"schema_version":1,"object_size_bytes":64,"object_count":0,"objects":[]}`,
		"wrong count":      strings.Replace(valid, `"object_count":1`, `"object_count":2`, 1),
		"wrong ID":         strings.Replace(valid, `"object_id":0`, `"object_id":1`, 1),
		"bad digest":       strings.Replace(valid, sha, "bad", 1),
		"second value":     valid + "{}",
	}
	manifestPath := filepath.Join(t.TempDir(), "dataset-manifest.json")
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(manifestPath, []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := ReadManifest(manifestPath); err == nil || ErrorCode(err) != ContractMismatch {
				t.Fatalf("expected contract rejection: %v", err)
			} else if !strings.Contains(err.Error(), manifestPath) || strings.Count(err.Error(), "MANIFEST ") != 1 {
				t.Fatalf("expected one manifest path context: %v", err)
			}
		})
	}
}
