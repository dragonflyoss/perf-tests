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
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"time"
)

// FailedRateThreshold is the failed rate that fails the benchmark, same as proxy-bench.
const FailedRateThreshold = 0.01

// TrendStats are the latency columns of the report, same as proxy-bench.
var TrendStats = []string{"min", "avg", "med", "p(90)", "p(95)", "p(99)", "max"}

// Download represents one download on one peer.
type Download struct {
	// Peer is the name of the peer.
	Peer string

	// Cost is the cost of the download.
	Cost time.Duration

	// Err is the download error, nil when the download succeeded.
	Err error
}

// Downloads are the downloads of the benchmark, one per peer.
type Downloads []*Download

// Succeeded returns the number of successful downloads.
func (d Downloads) Succeeded() int {
	var n int
	for _, download := range d {
		if download.Err == nil {
			n++
		}
	}

	return n
}

// Costs returns the costs of the successful downloads sorted ascending.
func (d Downloads) Costs() []time.Duration {
	costs := make([]time.Duration, 0, len(d))
	for _, download := range d {
		if download.Err == nil {
			costs = append(costs, download.Cost)
		}
	}
	slices.Sort(costs)

	return costs
}

// Passed returns whether the failed download rate is below the threshold.
func (d Downloads) Passed() bool {
	total := len(d)
	if total == 0 {
		return false
	}

	return float64(total-d.Succeeded())/float64(total) < FailedRateThreshold
}

// Report builds the text report of a benchmark, same layout as proxy-bench.
type Report struct {
	// b buffers the report.
	b strings.Builder
}

// NewReport creates a new report titled by the benchmark name.
func NewReport(name string) *Report {
	r := &Report{}
	fmt.Fprintf(&r.b, "\n%s\n\n", name)
	return r
}

// Row writes a labeled row.
func (r *Report) Row(label string, text string) {
	fmt.Fprintf(&r.b, "  %-16s%s\n", label, text)
}

// Cells writes a labeled row of right-aligned cells, e.g. the latency columns.
func (r *Report) Cells(label string, texts []string) {
	var line strings.Builder
	for _, text := range texts {
		fmt.Fprintf(&line, "%9s", text)
	}

	r.Row(label, line.String())
}

// Blank writes a blank line.
func (r *Report) Blank() {
	r.b.WriteString("\n")
}

// String returns the report.
func (r *Report) String() string {
	return r.b.String()
}

// Print prints the report to stdout.
func (r *Report) Print() error {
	_, err := fmt.Fprint(os.Stdout, r.String())
	return err
}

// Latencies returns the TrendStats of the sorted costs formatted in milliseconds.
func Latencies(sorted []time.Duration) []string {
	latencies := make([]string, 0, len(TrendStats))
	for _, cost := range []time.Duration{
		Percentile(sorted, 0), Average(sorted), Percentile(sorted, 50),
		Percentile(sorted, 90), Percentile(sorted, 95), Percentile(sorted, 99), Percentile(sorted, 100),
	} {
		latencies = append(latencies, FormatMilliseconds(cost))
	}

	return latencies
}

// Percentile returns the nearest-rank percentile of the sorted costs, zero if empty.
func Percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}

	rank := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	return sorted[max(0, min(rank, len(sorted)-1))]
}

// Average returns the average of the costs, zero if empty.
func Average(costs []time.Duration) time.Duration {
	if len(costs) == 0 {
		return 0
	}

	var total time.Duration
	for _, cost := range costs {
		total += cost
	}

	return total / time.Duration(len(costs))
}

// FormatMilliseconds formats the duration in milliseconds.
func FormatMilliseconds(d time.Duration) string {
	return fmt.Sprintf("%.2f", float64(d)/float64(time.Millisecond))
}

// FormatSeconds formats the duration in seconds, or minutes and seconds from two minutes on.
func FormatSeconds(d time.Duration) string {
	if d < 2*time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}

	return d.Round(time.Second).String()
}

// FormatPercent formats part of total as a percentage, 0.00% if total is zero.
func FormatPercent[T int | uint64](part, total T) string {
	if total == 0 {
		return "0.00%"
	}

	return fmt.Sprintf("%.2f%%", float64(part)/float64(total)*100)
}

// FormatResult formats the result row of the report, e.g. "PASSED, ✓ download failed rate<0.01".
func FormatResult(passed bool, subject string) string {
	if passed {
		return fmt.Sprintf("PASSED, ✓ %s failed rate<%.2f", subject, FailedRateThreshold)
	}

	return fmt.Sprintf("FAILED, ✗ %s failed rate<%.2f", subject, FailedRateThreshold)
}
