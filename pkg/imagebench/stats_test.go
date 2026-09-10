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
	"errors"
	"testing"
	"time"
)

func TestResult(t *testing.T) {
	result := &Result{
		Image: "dragonflyoss/image-bench:v1-1gb-4",
		Downloads: []*Download{
			{Node: "node-2", Cost: 3 * time.Second},
			{Node: "node-1", Cost: time.Second},
			{Node: "node-3", Cost: 10 * time.Second, Err: errors.New("ErrImagePull")},
		},
	}

	if got := result.Succeeded(); got != 2 {
		t.Fatalf("Succeeded() = %d, want 2", got)
	}

	costs := result.Costs()
	if len(costs) != 2 || costs[0] != time.Second || costs[1] != 3*time.Second {
		t.Fatalf("Costs() = %v, want [1s 3s]", costs)
	}

	// One of three failed is above the 1% threshold.
	if result.Passed() {
		t.Fatal("Passed() = true, want false")
	}
}

func TestResultPassed(t *testing.T) {
	tests := []struct {
		name   string
		total  int
		failed int
		want   bool
	}{
		{name: "all succeeded", total: 10, failed: 0, want: true},
		{name: "one of ten failed", total: 10, failed: 1, want: false},
		{name: "one of two hundred failed", total: 200, failed: 1, want: true},
		{name: "no pull", total: 0, failed: 0, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &Result{}
			for i := 0; i < tt.total; i++ {
				download := &Download{Node: "node", Cost: time.Second}
				if i < tt.failed {
					download.Err = errors.New("ErrImagePull")
				}
				result.Downloads = append(result.Downloads, download)
			}

			if got := result.Passed(); got != tt.want {
				t.Fatalf("Passed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPercentile(t *testing.T) {
	costs := make([]time.Duration, 0, 100)
	for i := 1; i <= 100; i++ {
		costs = append(costs, time.Duration(i)*time.Millisecond)
	}

	tests := []struct {
		name   string
		sorted []time.Duration
		p      float64
		want   time.Duration
	}{
		{name: "min", sorted: costs, p: 0, want: time.Millisecond},
		{name: "p50", sorted: costs, p: 50, want: 50 * time.Millisecond},
		{name: "p90", sorted: costs, p: 90, want: 90 * time.Millisecond},
		{name: "p99", sorted: costs, p: 99, want: 99 * time.Millisecond},
		{name: "max", sorted: costs, p: 100, want: 100 * time.Millisecond},
		{name: "single sample", sorted: []time.Duration{7 * time.Second}, p: 99, want: 7 * time.Second},
		{name: "no sample", sorted: nil, p: 50, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := percentile(tt.sorted, tt.p); got != tt.want {
				t.Fatalf("percentile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAverage(t *testing.T) {
	if got := average([]time.Duration{time.Second, 3 * time.Second}); got != 2*time.Second {
		t.Fatalf("average() = %v, want 2s", got)
	}

	if got := average(nil); got != 0 {
		t.Fatalf("average(nil) = %v, want 0", got)
	}
}

func TestFormat(t *testing.T) {
	if got := formatMilliseconds(1500 * time.Millisecond); got != "1500.00" {
		t.Errorf("formatMilliseconds(1.5s) = %q, want 1500.00", got)
	}

	if got := formatSeconds(12300 * time.Millisecond); got != "12.3s" {
		t.Errorf("formatSeconds(12.3s) = %q, want 12.3s", got)
	}

	if got := formatSeconds(3*time.Minute + 5*time.Second); got != "3m5s" {
		t.Errorf("formatSeconds(3m5s) = %q, want 3m5s", got)
	}

	if got := formatPercent(1, 0); got != "0.00%" {
		t.Errorf("formatPercent(1, 0) = %q, want 0.00%%", got)
	}

	if got := formatPercent(uint64(1), uint64(10)); got != "10.00%" {
		t.Errorf("formatPercent(1, 10) = %q, want 10.00%%", got)
	}
}

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
		Downloads: []*Download{{Node: "node-1", Pod: "image-bench-abcdef01-0", Cost: time.Second}},
		Traffic:   Traffic{BackToSource: 1 << 30},
		Elapsed:   2 * time.Second,
	})
	if got := stats.GetResult(); got == nil || got.Image != "dragonflyoss/image-bench:v1-1gb-4" {
		t.Fatalf("GetResult() = %v, want the v1-1gb-4 result", got)
	}

	if err := stats.PrettyPrint(); err != nil {
		t.Fatalf("PrettyPrint() error = %v", err)
	}
}
