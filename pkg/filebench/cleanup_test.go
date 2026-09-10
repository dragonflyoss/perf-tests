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

package filebench

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSetStorageKeep(t *testing.T) {
	tests := []struct {
		name           string
		dfdaemonConfig string
	}{
		{
			name: "keep enabled",
			dfdaemonConfig: `host:
  idc: ""
storage:
  dir: /var/lib/dragonfly/
  keep: true
  writeBufferSize: 524288
gc:
  interval: 900s
`,
		},
		{
			name: "keep missing",
			dfdaemonConfig: `storage:
  dir: /var/lib/dragonfly/
`,
		},
		{
			name:           "storage missing",
			dfdaemonConfig: "host:\n  idc: \"\"\n",
		},
		{
			name:           "empty config",
			dfdaemonConfig: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := setStorageKeep([]byte(tt.dfdaemonConfig), false)
			if err != nil {
				t.Fatalf("setStorageKeep() error = %v", err)
			}

			var config map[string]any
			if err := yaml.Unmarshal(got, &config); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}

			storage, ok := config["storage"].(map[string]any)
			if !ok {
				t.Fatalf("storage = %v, want a map", config["storage"])
			}

			if keep, ok := storage["keep"].(bool); !ok || keep {
				t.Fatalf("storage.keep = %v, want false", storage["keep"])
			}
		})
	}
}

func TestSetStorageKeepPreservesOtherFields(t *testing.T) {
	got, err := setStorageKeep([]byte("storage:\n  dir: /var/lib/dragonfly/\n  keep: true\ngc:\n  interval: 900s\n"), false)
	if err != nil {
		t.Fatalf("setStorageKeep() error = %v", err)
	}

	var config map[string]any
	if err := yaml.Unmarshal(got, &config); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if dir := config["storage"].(map[string]any)["dir"]; dir != "/var/lib/dragonfly/" {
		t.Errorf("storage.dir = %v, want /var/lib/dragonfly/", dir)
	}

	if interval := config["gc"].(map[string]any)["interval"]; interval != "900s" {
		t.Errorf("gc.interval = %v, want 900s", interval)
	}
}

func TestSetStorageKeepInvalid(t *testing.T) {
	if _, err := setStorageKeep([]byte("storage: [unclosed"), false); err == nil {
		t.Fatal("setStorageKeep() expected error for malformed config")
	}
}
