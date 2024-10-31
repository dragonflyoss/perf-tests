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
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/dragonflyoss/perf-tests/benchmark/pkg/backend"
	"github.com/dragonflyoss/perf-tests/benchmark/pkg/config"
	"github.com/google/uuid"
	"github.com/olekukonko/tablewriter"
)

// Stats represents the statistics of the benchmark.
type Stats interface {
	// AddDownload adds a download record to the statistics.
	AddDownload(*url.URL, string, backend.FileSizeLevel, time.Time, time.Time)

	// GetDownloads returns all download records.
	GetDownloads() []*Download

	// PrettyPrint prints the statistics in a pretty format.
	PrettyPrint()
}

// stats implements the Stats interface.
type stats struct {
	// downloads stores the download statistics.
	downloads *sync.Map
}

// Download represents the download statistics.
type Download struct {
	// url is the URL of the file.
	url *url.URL

	// downloader is the downloader used to download the file.
	downloader string

	// fileSizeLevel is the file size level of the file.
	fileSizeLevel backend.FileSizeLevel

	// cost is the time cost of downloading the file.
	cost time.Duration

	// createdAt is the time when the download started.
	createdAt time.Time

	// finishedAt is the time when the download finished.
	finishedAt time.Time
}

// New creates a new Stats instance.
func New() Stats {
	return &stats{downloads: &sync.Map{}}
}

// AddDownload adds a download record to the statistics.
func (s *stats) AddDownload(url *url.URL, downloader string, fileSizeLevel backend.FileSizeLevel, createdAt time.Time, finishedAt time.Time) {
	s.downloads.Store(uuid.New().String(), &Download{
		url:           url,
		downloader:    downloader,
		fileSizeLevel: fileSizeLevel,
		cost:          finishedAt.Sub(createdAt),
		createdAt:     createdAt,
		finishedAt:    finishedAt,
	})
}

func (s *stats) GetDownloads() []*Download {
	downloads := make([]*Download, 0)
	s.downloads.Range(func(key, value interface{}) bool {
		downloads = append(downloads, value.(*Download))
		return true
	})

	return downloads
}

// PrettyPrint prints the statistics in a pretty format.
func (s *stats) PrettyPrint() {
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
		printTable(dfgetDownloads)
	}

	if len(proxyDownloads) != 0 {
		printTable(proxyDownloads)
	}
}

// printTable prints the download statistics in a table format.
func printTable(downloads map[backend.FileSizeLevel][]*Download) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"File Size Level", "Times", "Min Cost", "Max Cost", "Avg Cost"})

	for fileSizeLevel, records := range downloads {
		var minCost, maxCost, totalCost time.Duration
		if len(records) > 0 {
			minCost = records[0].cost
			maxCost = records[0].cost
		}

		for _, record := range records {
			if record.cost < minCost {
				minCost = record.cost
			}

			if record.cost > maxCost {
				maxCost = record.cost
			}

			totalCost += record.cost
		}

		avgCost := totalCost / time.Duration(len(records))
		table.Append([]string{
			fmt.Sprintf("%s", fileSizeLevel),
			fmt.Sprintf("%d", len(records)),
			formatDuration(minCost),
			formatDuration(maxCost),
			formatDuration(avgCost),
		})
	}

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetRowLine(true)
	table.Render()
}

// formatDuration formats the duration to a string.
func formatDuration(d time.Duration) string {
	ms := float64(d) / float64(time.Millisecond)
	return fmt.Sprintf("%.2fms", ms)
}
