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
	"time"
)

func TestPulledCost(t *testing.T) {
	created := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	var p pod
	p.Metadata.Name = "image-bench-abcdef01-0"
	p.Metadata.CreationTimestamp = created

	tests := []struct {
		name     string
		events   []event
		wantCost time.Duration
		wantOK   bool
	}{
		{
			name:     "not pulled yet",
			events:   []event{{Reason: "Pulling", Message: `Pulling image "dragonflyoss/image-bench:v1-1gb-4"`}},
			wantCost: 0,
			wantOK:   false,
		},
		{
			name: "pulled with waiting time",
			events: []event{
				{Reason: "Pulling", Message: `Pulling image "dragonflyoss/image-bench:v1-1gb-4"`},
				{Reason: "Pulled", Message: `Successfully pulled image "dragonflyoss/image-bench:v1-1gb-4" in 1m2.345s (1m10.5s including waiting). Image size: 1073741824 bytes.`},
			},
			wantCost: time.Minute + 2345*time.Millisecond,
			wantOK:   true,
		},
		{
			name:     "pulled legacy message",
			events:   []event{{Reason: "Pulled", Message: `Successfully pulled image "dragonflyoss/image-bench:v1-1gb-4" in 12.5s`}},
			wantCost: 12500 * time.Millisecond,
			wantOK:   true,
		},
		{
			name:     "already present falls back to timestamps",
			events:   []event{{Reason: "Pulled", Message: `Container image "dragonflyoss/image-bench:v1-1gb-4" already present on machine`, LastTimestamp: created.Add(3 * time.Second)}},
			wantCost: 3 * time.Second,
			wantOK:   true,
		},
		{
			name:     "already present without timestamps",
			events:   []event{{Reason: "Pulled", Message: `Container image "dragonflyoss/image-bench:v1-1gb-4" already present on machine`}},
			wantCost: 0,
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost, ok := pulledCost(p, tt.events)
			if ok != tt.wantOK || cost != tt.wantCost {
				t.Fatalf("pulledCost() = (%v, %v), want (%v, %v)", cost, ok, tt.wantCost, tt.wantOK)
			}
		})
	}
}

func TestPullError(t *testing.T) {
	tests := []struct {
		name    string
		pod     string
		wantErr bool
	}{
		{
			name:    "pending",
			pod:     `{"status":{"phase":"Pending","containerStatuses":[{"state":{"waiting":{"reason":"ContainerCreating"}}}]}}`,
			wantErr: false,
		},
		{
			name:    "image pull error",
			pod:     `{"status":{"phase":"Pending","containerStatuses":[{"state":{"waiting":{"reason":"ErrImagePull","message":"rpc error: code = NotFound"}}}]}}`,
			wantErr: true,
		},
		{
			name:    "image pull back-off",
			pod:     `{"status":{"phase":"Pending","containerStatuses":[{"state":{"waiting":{"reason":"ImagePullBackOff","message":"Back-off pulling image"}}}]}}`,
			wantErr: true,
		},
		{
			name:    "rejected by kubelet",
			pod:     `{"status":{"phase":"Failed","reason":"OutOfcpu","message":"Pod was rejected"}}`,
			wantErr: true,
		},
		{
			name:    "container failed to start after the pull",
			pod:     `{"status":{"phase":"Failed","containerStatuses":[{"state":{"terminated":{"reason":"StartError","exitCode":128}}}]}}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p pod
			if err := json.Unmarshal([]byte(tt.pod), &p); err != nil {
				t.Fatalf("unmarshal pod: %v", err)
			}

			if err := pullError(p); (err != nil) != tt.wantErr {
				t.Fatalf("pullError() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDecodeEvents(t *testing.T) {
	data := `{"items":[
		{"involvedObject":{"kind":"Pod","name":"a","uid":"u1"},"reason":"Pulled","message":"m","lastTimestamp":"2026-09-10T12:00:03Z","eventTime":null},
		{"involvedObject":{"kind":"Pod","name":"a","uid":"u2"},"reason":"Failed","message":"n","lastTimestamp":null,"eventTime":"2026-09-10T12:00:04.123456Z"}
	]}`

	var list eventList
	if err := json.Unmarshal([]byte(data), &list); err != nil {
		t.Fatalf("unmarshal events: %v", err)
	}

	if len(list.Items) != 2 || list.Items[0].InvolvedObject.UID != "u1" || list.Items[1].InvolvedObject.UID != "u2" {
		t.Fatalf("unexpected items: %+v", list.Items)
	}

	if got := list.Items[0].timestamp(); got != time.Date(2026, 9, 10, 12, 0, 3, 0, time.UTC) {
		t.Errorf("timestamp() of legacy event = %v", got)
	}

	if got := list.Items[1].timestamp(); got != time.Date(2026, 9, 10, 12, 0, 4, 123456000, time.UTC) {
		t.Errorf("timestamp() of new event = %v", got)
	}
}

func TestPullPod(t *testing.T) {
	manifest := pullPod("dragonfly-system", "image-bench-abcdef01-0", "abcdef01", "ghcr.io/dragonflyoss/image-bench:v1-1gb-4", "node-1")
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal pod: %v", err)
	}

	var p struct {
		Metadata struct {
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Spec struct {
			NodeName      string `json:"nodeName"`
			RestartPolicy string `json:"restartPolicy"`
			Containers    []struct {
				Image           string `json:"image"`
				ImagePullPolicy string `json:"imagePullPolicy"`
			} `json:"containers"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatalf("unmarshal pod: %v", err)
	}

	if p.Metadata.Labels[runLabel] != "abcdef01" || p.Spec.NodeName != "node-1" || p.Spec.RestartPolicy != "Never" {
		t.Fatalf("unexpected pod: %s", data)
	}

	if len(p.Spec.Containers) != 1 || p.Spec.Containers[0].Image != "ghcr.io/dragonflyoss/image-bench:v1-1gb-4" || p.Spec.Containers[0].ImagePullPolicy != "Always" {
		t.Fatalf("unexpected containers: %s", data)
	}
}
