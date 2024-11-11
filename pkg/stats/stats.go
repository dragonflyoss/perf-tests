/*
 *     Copyright 2024 The Dragonfly Authors
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

package stats

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"github.com/dragonflyoss/perf-tests/pkg/backend"
	"github.com/dragonflyoss/perf-tests/pkg/config"
	"github.com/dragonflyoss/perf-tests/pkg/util"
	"github.com/google/uuid"
	"github.com/olekukonko/tablewriter"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/sirupsen/logrus"
)

// Stats represents the statistics of the benchmark.
type Stats interface {
	// GetDownloads returns the download statistics.
	GetDownloads() []*Download

	// CollectClientMetrics collects the client metrics and resets the metrics.
	CollectClientMetrics(ctx context.Context, downloader string, fileSizeLevel backend.FileSizeLevel) error

	// PrettyPrint prints the statistics in a pretty format.
	PrettyPrint() error
}

// stats implements the Stats interface.
type stats struct {
	// downloads stores the download statistics.
	downloads *sync.Map

	// namespace is the namespace of the benchmark.
	namespace string
}

// Download represents the download statistics.
type Download struct {
	// podName is the name of the pod.
	podName string

	// downloader is the downloader used to download the file.
	downloader string

	// fileSizeLevel is the file size level of the file.
	fileSizeLevel backend.FileSizeLevel

	// metricFamilies is the metric families of the download.
	metricFamilies map[string]*dto.MetricFamily
}

// New creates a new Stats instance.
func New(namespace string) Stats {
	return &stats{downloads: &sync.Map{}, namespace: namespace}
}

// GetDownloads returns the download statistics.
func (s *stats) GetDownloads() []*Download {
	downloads := []*Download{}
	s.downloads.Range(func(key, value interface{}) bool {
		downloads = append(downloads, value.(*Download))
		return true
	})

	return downloads
}

// collectClientMetrics collects the client metrics.
func (s *stats) CollectClientMetrics(ctx context.Context, downloader string, fileSizeLevel backend.FileSizeLevel) error {
	clientPods, err := s.getClientPods(ctx)
	if err != nil {
		logrus.Errorf("failed to get client pods: %v", err)
		return err
	}

	for _, pod := range clientPods {
		data, err := s.getClientMetrics(ctx, pod)
		if err != nil {
			logrus.Errorf("failed to get client metrics: %v", err)
			return err
		}
		reader := bytes.NewReader(data)

		parser := expfmt.TextParser{}
		metricFamilies, err := parser.TextToMetricFamilies(reader)
		if err != nil {
			logrus.Errorf("failed to parse metrics: %v", err)
			return err
		}

		s.downloads.Store(uuid.New().String(), &Download{
			podName:        pod,
			downloader:     downloader,
			fileSizeLevel:  fileSizeLevel,
			metricFamilies: metricFamilies,
		})

		if err := s.resetClientMetrics(ctx, pod); err != nil {
			logrus.Errorf("failed to reset client metrics: %v", err)
			return err
		}
	}

	return nil
}

// getClientMetrics collects the client metrics by pod name
func (s *stats) getClientMetrics(ctx context.Context, name string) ([]byte, error) {
	podExec := util.NewPodExec(s.namespace, name, "client")
	output, err := podExec.Command(ctx, "sh", "-c", "curl -s http://127.0.0.1:4002/metrics").CombinedOutput()
	if err != nil {
		logrus.Errorf("failed to cleanup: %v \nmessage: %s", err, string(output))
		return nil, err
	}

	return output, nil
}

// resetClientMetrics resets the client metrics by pod name
func (s *stats) resetClientMetrics(ctx context.Context, name string) error {
	podExec := util.NewPodExec(s.namespace, name, "client")
	output, err := podExec.Command(ctx, "sh", "-c", "curl -s -X DELETE http://127.0.0.1:4002/metrics").CombinedOutput()
	if err != nil {
		logrus.Errorf("failed to cleanup: %v \nmessage: %s", err, string(output))
		return err
	}

	return nil
}

// getClientPods returns the client pods.
func (s *stats) getClientPods(ctx context.Context) ([]string, error) {
	pods, err := util.GetPods(ctx, s.namespace, "component=client")
	if err != nil {
		logrus.Errorf("failed to get pods: %v", err)
		return nil, err
	}

	if len(pods) == 0 {
		logrus.Errorf("no client pod found")
		return nil, errors.New("no client pod found")
	}

	return pods, nil
}

// PrettyPrint prints the statistics in a pretty format.
func (s *stats) PrettyPrint() error {
	downloads := s.GetDownloads()
	proxyDownloads := make(map[backend.FileSizeLevel][]*Download)
	dfgetDownloads := make(map[backend.FileSizeLevel][]*Download)
	for _, download := range downloads {
		switch download.downloader {
		case config.DownloaderDfget:
			dfgetDownloads[download.fileSizeLevel] = append(dfgetDownloads[download.fileSizeLevel], download)
		case config.DownloaderProxy:
			proxyDownloads[download.fileSizeLevel] = append(proxyDownloads[download.fileSizeLevel], download)
		}
	}

	if len(dfgetDownloads) != 0 {
		if err := printTable(dfgetDownloads); err != nil {
			return err
		}
	}

	if len(proxyDownloads) != 0 {
		if err := printTable(proxyDownloads); err != nil {
			return err
		}
	}

	return nil
}

// printTable prints the download statistics in a table format.
func printTable(downloads map[backend.FileSizeLevel][]*Download) error {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"File Size Level", "Times", "Min Cost", "Max Cost", "Avg Cost"})

	rows := map[backend.FileSizeLevel][]string{}
	for fileSizeLevel, records := range downloads {
		maxCost := time.Duration(-math.MaxInt64)
		minCost := time.Duration(math.MaxInt64)

		var (
			totalCost time.Duration
			n         uint64
		)
		for _, record := range records {
			for name, mf := range record.metricFamilies {
				if name == "dragonfly_client_download_task_duration_milliseconds" {
					for _, metrics := range mf.GetMetric() {
						for _, label := range metrics.GetLabel() {
							if *label.Name == "task_size_level" && *label.Value == fileSizeLevel.TaskSizeLevel() {
								if metrics.GetHistogram().GetSampleCount() != 1 {
									return errors.New("invalid sample count")
								}

								cost := time.Duration(int64(metrics.GetHistogram().GetSampleSum()) * int64(time.Millisecond))
								totalCost += cost
								n += metrics.GetHistogram().GetSampleCount()

								if cost < minCost {
									minCost = cost
								}

								if cost > maxCost {
									maxCost = cost
								}
							}
						}
					}
				}
			}
		}

		avgCost := totalCost / time.Duration(n)
		rows[fileSizeLevel] = []string{
			fileSizeLevel.String(),
			fmt.Sprintf("%d", len(records)),
			formatDuration(minCost),
			formatDuration(maxCost),
			formatDuration(avgCost),
		}
	}

	for _, fileSizeLevel := range backend.FileSizeLevels {
		if row, ok := rows[fileSizeLevel]; ok {
			table.Append(row)
			continue
		}
	}

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetRowLine(true)
	table.Render()
	return nil
}

// formatDuration formats the duration to a string.
func formatDuration(d time.Duration) string {
	ms := float64(d) / float64(time.Millisecond)
	return fmt.Sprintf("%.2fms", ms)
}
