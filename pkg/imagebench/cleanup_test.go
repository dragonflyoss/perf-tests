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

package imagebench

import (
	"encoding/json"
	"testing"

	"github.com/dragonflyoss/perf-tests/pkg/config"
)

func TestCleanupPod(t *testing.T) {
	b := &imageBench{config: &config.ImageBenchConfig{
		Namespace:        "dragonfly-system",
		Image:            "dragonflyoss/image-bench:v1-1gb-4",
		CleanupImage:     "dragonflyoss/image-bench:latest",
		ContainerdSocket: "/run/k3s/containerd/containerd.sock",
	}}

	data, err := json.Marshal(b.cleanupPod("image-bench-cleanup-abcdef01-0", "abcdef01", "node-1"))
	if err != nil {
		t.Fatalf("marshal pod: %v", err)
	}

	var p struct {
		Spec struct {
			NodeName   string `json:"nodeName"`
			Containers []struct {
				Image string `json:"image"`
				Env   []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"env"`
				VolumeMounts []struct {
					MountPath string `json:"mountPath"`
				} `json:"volumeMounts"`
			} `json:"containers"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatalf("unmarshal pod: %v", err)
	}

	if p.Spec.NodeName != "node-1" || len(p.Spec.Containers) != 1 || p.Spec.Containers[0].Image != "dragonflyoss/image-bench:latest" {
		t.Fatalf("unexpected pod: %s", data)
	}

	env := map[string]string{}
	for _, e := range p.Spec.Containers[0].Env {
		env[e.Name] = e.Value
	}

	if env["IMAGE"] != "dragonflyoss/image-bench:v1-1gb-4" || env["CONTAINER_RUNTIME_ENDPOINT"] != "unix:///run/k3s/containerd/containerd.sock" {
		t.Fatalf("unexpected env: %v", env)
	}

	if len(p.Spec.Containers[0].VolumeMounts) != 1 || p.Spec.Containers[0].VolumeMounts[0].MountPath != "/run/k3s/containerd/containerd.sock" {
		t.Fatalf("unexpected volume mounts: %s", data)
	}
}
