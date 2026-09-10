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
	"time"

	"github.com/dragonflyoss/perf-tests/pkg/util"
	humanize "github.com/dustin/go-humanize"
)

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

	// Downloads is the image pull of every peer node, the peer is the node name.
	Downloads util.Downloads

	// Traffic is the traffic of the peers and seed peers during the benchmark.
	Traffic util.Traffic

	// Elapsed is the wall-clock time of the benchmark.
	Elapsed time.Duration
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

	downloads, traffic := result.Downloads, result.Traffic
	total, succeeded := len(downloads), downloads.Succeeded()
	failed := total - succeeded

	report := util.NewReport("image-bench")
	report.Row("Run", fmt.Sprintf("%d peers by containerd, %s", total, util.FormatSeconds(result.Elapsed)))
	report.Row("Target", result.Image)
	report.Blank()
	report.Row("Pulls", fmt.Sprintf("%d total, %d succeeded", total, succeeded))
	report.Row("Failed", fmt.Sprintf("%d of %d (%s)", failed, total, util.FormatPercent(failed, total)))
	report.Blank()
	report.Cells("Latency (ms)", util.TrendStats)
	report.Cells("  pull", util.Latencies(downloads.Costs()))
	report.Blank()
	report.Row("Traffic", fmt.Sprintf("%s total, %s back-to-source, %s remote peer, %s local peer",
		humanize.IBytes(traffic.Total()), humanize.IBytes(traffic.BackToSource), humanize.IBytes(traffic.RemotePeer), humanize.IBytes(traffic.LocalPeer)))
	report.Row("Back to source", util.FormatPercent(traffic.BackToSource, traffic.Total()))
	report.Blank()
	report.Row("Result", util.FormatResult(downloads.Passed(), "pull"))
	report.Blank()

	return report.Print()
}
