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
	"fmt"
	"math"
	"os"
	"slices"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
)

// Stats represents the statistics of the benchmark.
type Stats interface {
	// SetResult records the result of the benchmark.
	SetResult(*Result)

	// GetResult returns the result of the benchmark, nil before it ran.
	GetResult() *Result

	// PrettyPrint prints the statistics in a pretty format.
	PrettyPrint() error
}

// stats implements the Stats interface.
type stats struct {
	// result stores the result of the benchmark.
	result *Result
}

// Result represents the downloads of the file on all peers.
type Result struct {
	// File is the file server path of the downloaded file.
	File string

	// Downloads is the download of every peer.
	Downloads []*Download

	// Traffic is the traffic of the peers and seed peers during the benchmark.
	Traffic Traffic
}

// Download represents one dfget download on one peer.
type Download struct {
	// Peer is the name of the peer pod.
	Peer string

	// Cost is the wall-clock cost of the download.
	Cost time.Duration

	// Err is the download error, nil when the download succeeded.
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

// PrettyPrint prints the statistics in a pretty format.
func (s *stats) PrettyPrint() error {
	table := tablewriter.NewWriter(os.Stdout)
	table.Header("File", "Peers", "Success Rate", "Avg Cost", "P50 Cost", "P90 Cost", "P99 Cost", "Max Cost", "Back To Source Traffic", "Remote Peer Traffic", "Local Peer Traffic", "Back To Source Rate")

	if result := s.result; result != nil {
		costs := result.Costs()
		if err := table.Append(
			result.File,
			fmt.Sprintf("%d", len(result.Downloads)),
			fmt.Sprintf("%s (%d/%d)", formatRate(uint64(result.Succeeded()), uint64(len(result.Downloads))), result.Succeeded(), len(result.Downloads)),
			formatDuration(average(costs)),
			formatDuration(percentile(costs, 50)),
			formatDuration(percentile(costs, 90)),
			formatDuration(percentile(costs, 99)),
			formatDuration(percentile(costs, 100)),
			humanize.IBytes(result.Traffic.BackToSource),
			humanize.IBytes(result.Traffic.RemotePeer),
			humanize.IBytes(result.Traffic.LocalPeer),
			formatRate(result.Traffic.BackToSource, result.Traffic.Total()),
		); err != nil {
			return err
		}
	}

	return table.Render()
}

// Succeeded returns the number of successful downloads.
func (r *Result) Succeeded() int {
	var n int
	for _, download := range r.Downloads {
		if download.Err == nil {
			n++
		}
	}

	return n
}

// Costs returns the costs of the successful downloads sorted ascending.
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

// formatDuration formats the duration to a string, "-" when there is no sample.
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "-"
	}

	ms := float64(d) / float64(time.Millisecond)
	return fmt.Sprintf("%.2fms", ms)
}

// formatRate formats part of total as a percentage, "-" when total is zero.
func formatRate(part, total uint64) string {
	if total == 0 {
		return "-"
	}

	return fmt.Sprintf("%.2f%%", float64(part)/float64(total)*100)
}
