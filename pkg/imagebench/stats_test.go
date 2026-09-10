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
	"testing"
	"time"

	"github.com/dragonflyoss/perf-tests/pkg/util"
)

func TestStats(t *testing.T) {
	stats := NewStats()
	if stats.GetResult() != nil {
		t.Fatal("GetResult() = non-nil before the benchmark ran, want nil")
	}

	if err := stats.PrettyPrint(); err == nil {
		t.Fatal("PrettyPrint() expected error without result")
	}

	stats.SetResult(&Result{
		Image:     "dragonflyoss/image-bench:v1-1gb-4",
		Downloads: util.Downloads{{Peer: "node-1", Cost: time.Second}},
		Traffic:   util.Traffic{BackToSource: 1 << 30},
		Elapsed:   2 * time.Second,
	})
	if got := stats.GetResult(); got == nil || got.Image != "dragonflyoss/image-bench:v1-1gb-4" {
		t.Fatalf("GetResult() = %v, want the v1-1gb-4 result", got)
	}

	if err := stats.PrettyPrint(); err != nil {
		t.Fatalf("PrettyPrint() error = %v", err)
	}
}
