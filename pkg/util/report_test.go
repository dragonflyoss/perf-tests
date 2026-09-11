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
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDownloads(t *testing.T) {
	downloads := Downloads{
		{Peer: "client-2", Cost: 3 * time.Second},
		{Peer: "client-1", Cost: time.Second},
		{Peer: "client-3", Cost: 10 * time.Second, Err: errors.New("timeout")},
	}

	if got := downloads.Succeeded(); got != 2 {
		t.Fatalf("Succeeded() = %d, want 2", got)
	}

	costs := downloads.Costs()
	if len(costs) != 2 || costs[0] != time.Second || costs[1] != 3*time.Second {
		t.Fatalf("Costs() = %v, want [1s 3s]", costs)
	}

	// One of three failed is above the 1% threshold.
	if downloads.Passed() {
		t.Fatal("Passed() = true, want false")
	}
}

func TestDownloadsPassed(t *testing.T) {
	tests := []struct {
		name   string
		total  int
		failed int
		want   bool
	}{
		{name: "all succeeded", total: 10, failed: 0, want: true},
		{name: "one of ten failed", total: 10, failed: 1, want: false},
		{name: "one of two hundred failed", total: 200, failed: 1, want: true},
		{name: "no download", total: 0, failed: 0, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var downloads Downloads
			for i := 0; i < tt.total; i++ {
				download := &Download{Peer: "client", Cost: time.Second}
				if i < tt.failed {
					download.Err = errors.New("timeout")
				}
				downloads = append(downloads, download)
			}

			if got := downloads.Passed(); got != tt.want {
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
			if got := Percentile(tt.sorted, tt.p); got != tt.want {
				t.Fatalf("Percentile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAverage(t *testing.T) {
	if got := Average([]time.Duration{time.Second, 3 * time.Second}); got != 2*time.Second {
		t.Fatalf("Average() = %v, want 2s", got)
	}

	if got := Average(nil); got != 0 {
		t.Fatalf("Average(nil) = %v, want 0", got)
	}
}

func TestLatencies(t *testing.T) {
	got := Latencies([]time.Duration{time.Second, 2 * time.Second, 3 * time.Second})
	want := []string{"1000.00", "2000.00", "2000.00", "3000.00", "3000.00", "3000.00", "3000.00"}
	if !slices.Equal(got, want) {
		t.Fatalf("Latencies() = %v, want %v", got, want)
	}

	if len(got) != len(TrendStats) {
		t.Fatalf("len(Latencies()) = %d, want %d columns", len(got), len(TrendStats))
	}
}

func TestFormat(t *testing.T) {
	if got := FormatMilliseconds(1500 * time.Millisecond); got != "1500.00" {
		t.Errorf("FormatMilliseconds(1.5s) = %q, want 1500.00", got)
	}

	if got := FormatSeconds(12300 * time.Millisecond); got != "12.3s" {
		t.Errorf("FormatSeconds(12.3s) = %q, want 12.3s", got)
	}

	if got := FormatSeconds(3*time.Minute + 5*time.Second); got != "3m5s" {
		t.Errorf("FormatSeconds(3m5s) = %q, want 3m5s", got)
	}

	if got := FormatPercent(1, 0); got != "0.00%" {
		t.Errorf("FormatPercent(1, 0) = %q, want 0.00%%", got)
	}

	if got := FormatPercent(uint64(1), uint64(10)); got != "10.00%" {
		t.Errorf("FormatPercent(1, 10) = %q, want 10.00%%", got)
	}

	if got := FormatResult(true, "download"); got != "PASSED, ✓ download failed rate<0.01" {
		t.Errorf("FormatResult(true) = %q", got)
	}

	if got := FormatResult(false, "pull"); got != "FAILED, ✗ pull failed rate<0.01" {
		t.Errorf("FormatResult(false) = %q", got)
	}
}

func TestReport(t *testing.T) {
	report := NewReport("file-bench")
	report.Row("Run", "1g on 10 peers")
	report.Cells("Latency (ms)", []string{"min", "max"})
	report.Blank()

	want := "\nfile-bench\n\n  Run             1g on 10 peers\n  Latency (ms)           min       max\n\n"
	if got := report.String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}

	if !strings.HasPrefix(report.String(), "\nfile-bench\n") {
		t.Fatal("report should start with the benchmark name")
	}
}
