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

package util

import (
	"encoding/json"
	"testing"
)

func TestDecodePod(t *testing.T) {
	data := `{"metadata":{"name":"a","uid":"u1","creationTimestamp":"2026-09-10T12:00:00Z"},"spec":{"nodeName":"node-1"},
		"status":{"phase":"Pending","containerStatuses":[{"state":{"waiting":{"reason":"ErrImagePull","message":"not found"}}}]}}`

	var p Pod
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		t.Fatalf("unmarshal pod: %v", err)
	}

	if p.Metadata.Name != "a" || p.Metadata.UID != "u1" || p.Spec.NodeName != "node-1" || p.Status.Phase != "Pending" {
		t.Fatalf("unexpected pod: %+v", p)
	}

	if len(p.Status.ContainerStatuses) != 1 || p.Status.ContainerStatuses[0].State.Waiting.Reason != "ErrImagePull" {
		t.Fatalf("unexpected container statuses: %+v", p.Status.ContainerStatuses)
	}
}
