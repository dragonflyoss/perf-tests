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
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"time"

	humanize "github.com/dustin/go-humanize"
)

// failedRateThreshold is the failed pull rate that fails the benchmark, same as proxy-bench.
const failedRateThreshold = 0.01

// trendStats are the latency columns of the report, same as proxy-bench.
var trendStats = []string{"min", "avg", "med", "p(90)", "p(95)", "p(99)", "max"}

// Stats represents the statistics of the benchmark.
type Stats interface {
	// SetResult records the result of the benchmark.
	SetResult(*Result)

	// GetResult returns the result of the benchmark, nil before it ran.
	GetResult() *Result

	// PrettyPrint prints the report of the benchmark.
	PrettyPrint() error
}

// stats implements the Stats interface.
type stats struct {
	// result stores the result of the benchmark.
	result *Result
}

// Result represents the pulls of the image on all peer nodes.
type Result struct {
	// Image is the pulled image.
	Image string

	// Downloads is the pull of every peer node.
	Downloads []*Download

	// Traffic is the traffic of the peers and seed peers during the benchmark.
	Traffic Traffic

	// Elapsed is the wall-clock time of the benchmark.
	Elapsed time.Duration
}

// Download represents one image pull on one peer node.
type Download struct {
	// Node is the name of the peer node.
	Node string

	// Pod is the name of the pod that pulled the image.
	Pod string

	// Cost is the pull cost reported by the kubelet.
	Cost time.Duration

	// Err is the pull error, nil when the pull succeeded.
	Err error
}

// NewStats creates a new Stats instance.
func NewStats() Stats {
	return &stats{}
}

// SetResult records the result of the benchmark.
func (s *stats) SetResult(result *Result) {
	s.result = result
}

// GetResult returns the result of the benchmark, nil before it ran.
func (s *stats) GetResult() *Result {
	return s.result
}

// PrettyPrint prints the report of the benchmark.
func (s *stats) PrettyPrint() error {
	result := s.result
	if result == nil {
		return errors.New("no result")
	}

	costs := result.Costs()
	total, succeeded := len(result.Downloads), result.Succeeded()
	failed := total - succeeded
	traffic := result.Traffic

	latencies := make([]string, 0, len(trendStats))
	for _, cost := range []time.Duration{
		percentile(costs, 0), average(costs), percentile(costs, 50),
		percentile(costs, 90), percentile(costs, 95), percentile(costs, 99), percentile(costs, 100),
	} {
		latencies = append(latencies, formatMilliseconds(cost))
	}

	var b strings.Builder
	row := func(label string, text string) { fmt.Fprintf(&b, "  %-16s%s\n", label, text) }
	cells := func(texts []string) string {
		var line strings.Builder
		for _, text := range texts {
			fmt.Fprintf(&line, "%9s", text)
		}

		return line.String()
	}

	b.WriteString("\nimage-bench\n\n")
	row("Run", fmt.Sprintf("%d peers by containerd, %s", total, formatSeconds(result.Elapsed)))
	row("Target", result.Image)
	b.WriteString("\n")
	row("Pulls", fmt.Sprintf("%d total, %d succeeded", total, succeeded))
	row("Failed", fmt.Sprintf("%d of %d (%s)", failed, total, formatPercent(failed, total)))
	b.WriteString("\n")
	row("Latency (ms)", cells(trendStats))
	row("  pull", cells(latencies))
	b.WriteString("\n")
	row("Traffic", fmt.Sprintf("%s total, %s back-to-source, %s remote peer, %s local peer",
		humanize.IBytes(traffic.Total()), humanize.IBytes(traffic.BackToSource), humanize.IBytes(traffic.RemotePeer), humanize.IBytes(traffic.LocalPeer)))
	row("Back to source", formatPercent(traffic.BackToSource, traffic.Total()))
	b.WriteString("\n")
	if result.Passed() {
		row("Result", fmt.Sprintf("PASSED, ✓ pull failed rate<%.2f", failedRateThreshold))
	} else {
		row("Result", fmt.Sprintf("FAILED, ✗ pull failed rate<%.2f", failedRateThreshold))
	}
	b.WriteString("\n")

	_, err := fmt.Fprint(os.Stdout, b.String())
	return err
}

// Succeeded returns the number of successful pulls.
func (r *Result) Succeeded() int {
	var n int
	for _, download := range r.Downloads {
		if download.Err == nil {
			n++
		}
	}

	return n
}

// Costs returns the costs of the successful pulls sorted ascending.
func (r *Result) Costs() []time.Duration {
	costs := make([]time.Duration, 0, len(r.Downloads))
	for _, download := range r.Downloads {
		if download.Err == nil {
			costs = append(costs, download.Cost)
		}
	}
	slices.Sort(costs)

	return costs
}

// Passed returns whether the failed pull rate is below the threshold.
func (r *Result) Passed() bool {
	total := len(r.Downloads)
	if total == 0 {
		return false
	}

	return float64(total-r.Succeeded())/float64(total) < failedRateThreshold
}

// percentile returns the nearest-rank percentile of the sorted costs, zero if empty.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}

	rank := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	return sorted[max(0, min(rank, len(sorted)-1))]
}

// average returns the average of the costs, zero if empty.
func average(costs []time.Duration) time.Duration {
	if len(costs) == 0 {
		return 0
	}

	var total time.Duration
	for _, cost := range costs {
		total += cost
	}

	return total / time.Duration(len(costs))
}

// formatMilliseconds formats the duration in milliseconds.
func formatMilliseconds(d time.Duration) string {
	return fmt.Sprintf("%.2f", float64(d)/float64(time.Millisecond))
}

// formatSeconds formats the duration in seconds, or minutes and seconds from two minutes on.
func formatSeconds(d time.Duration) string {
	if d < 2*time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}

	return d.Round(time.Second).String()
}

// formatPercent formats part of total as a percentage, 0.00% if total is zero.
func formatPercent[T int | uint64](part, total T) string {
	if total == 0 {
		return "0.00%"
	}

	return fmt.Sprintf("%.2f%%", float64(part)/float64(total)*100)
}
