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

package dfgetbench

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dragonflyoss/perf-tests/pkg/backend"
	"github.com/dragonflyoss/perf-tests/pkg/config"
	"github.com/dragonflyoss/perf-tests/pkg/util"
	"github.com/sirupsen/logrus"
)

const (
	// ModeRepeat downloads the same URL by every dfget, so they all share one Dragonfly task.
	ModeRepeat = "repeat"

	// ModeRandom downloads a unique URL by every dfget, so each of them downloads its own Dragonfly task.
	ModeRandom = "random"

	// ModeFixed downloads the URL with r=<i> and no uuid by the i-th dfget, so every run downloads the same
	// Dragonfly tasks, and a run on another peer downloads them from the peers of the runs before.
	ModeFixed = "fixed"
)

// Modes are the modes of the benchmark.
var Modes = []string{ModeRepeat, ModeRandom, ModeFixed}

// DfgetBench represents a benchmark runner for concurrent dfget downloads on one peer.
type DfgetBench interface {
	// Run downloads the file by all dfget on the peer at the same time.
	Run(context.Context) error

	// Cleanup clears the cache of the peers and seed peers.
	Cleanup(context.Context) error
}

// dfgetBench implements the DfgetBench interface.
type dfgetBench struct {
	// config is the configuration of the benchmark.
	config *config.DfgetBenchConfig

	// fileServer is the file server of the benchmark.
	fileServer backend.FileServer

	// stats is the statistics of the benchmark.
	stats Stats
}

// New creates a new benchmark runner for concurrent dfget downloads on one peer.
func New(config *config.DfgetBenchConfig, fileServer backend.FileServer, stats Stats) DfgetBench {
	return &dfgetBench{config, fileServer, stats}
}

// Run downloads the file by all dfget on the peer at the same time.
func (d *dfgetBench) Run(ctx context.Context) error {
	if d.config.Concurrency == 0 {
		return errors.New("concurrency must be greater than 0")
	}

	peer, err := d.getPeer(ctx)
	if err != nil {
		return err
	}

	file := d.config.File
	downloadURLs, err := d.getURLs(file)
	if err != nil {
		logrus.Errorf("failed to get file URLs: %v", err)
		return err
	}

	peers := []util.Peer{peer}
	before := util.CollectTraffic(ctx, d.config.Namespace, d.config.MetricsPort, peers)

	fmt.Printf("Downloading %s by %d dfget on %s ...\n", downloadURLs[0], len(downloadURLs), peer.Pod)
	start := time.Now()
	downloads := make(util.Downloads, len(downloadURLs))
	var wg sync.WaitGroup
	for i, downloadURL := range downloadURLs {
		wg.Go(func() {
			downloads[i] = util.DownloadByDfget(ctx, d.config.Namespace, peer, downloadURL, d.config.OutputDir, file)
		})
	}
	wg.Wait()

	after := util.CollectTraffic(ctx, d.config.Namespace, d.config.MetricsPort, peers)
	traffic, sampled := util.TrafficBetween(before, after)
	if sampled == 0 {
		logrus.Warnf("failed to read the metrics of %s, the traffic is left out", peer.Pod)
	}

	result := &Result{File: file, URL: downloadURLs[0].String(), Pod: peer.Pod, Mode: d.config.Mode, Downloads: downloads, Traffic: traffic, Sampled: sampled, Elapsed: time.Since(start)}
	d.stats.SetResult(result)

	fmt.Printf("Downloaded %s: %d/%d succeeded in %s\n", file, downloads.Succeeded(), len(downloads), result.Elapsed.Round(time.Millisecond))
	return nil
}

// Cleanup clears the cache of the peers and seed peers.
func (d *dfgetBench) Cleanup(ctx context.Context) error {
	peer := util.Workload{Label: d.config.PeerLabel, ConfigMap: d.config.PeerConfigMap}
	seed := util.Workload{Label: d.config.SeedPeerLabel, ConfigMap: d.config.SeedPeerConfigMap}
	return util.CleanupWorkloads(ctx, d.config.Namespace, peer, seed)
}

// getPeer returns the peer to download on, the configured pod or the first pod found by the peer label.
func (d *dfgetBench) getPeer(ctx context.Context) (util.Peer, error) {
	if d.config.Pod != "" {
		return util.Peer{Pod: d.config.Pod, Container: d.config.PeerContainer}, nil
	}

	peers, err := util.GetPeers(ctx, d.config.Namespace, d.config.PeerLabel, d.config.PeerContainer, 1)
	if err != nil {
		return util.Peer{}, err
	}

	return peers[0], nil
}

// getURLs returns the URL of every dfget, its query string decides the Dragonfly task. The file server
// puts a fresh uuid in every URL: repeat shares the first one between all dfget, random gives each of
// them its own, and fixed replaces the uuid by the index of the dfget, so every run downloads the same tasks.
func (d *dfgetBench) getURLs(file string) ([]*url.URL, error) {
	if !slices.Contains(Modes, d.config.Mode) {
		return nil, fmt.Errorf("unknown mode %q, expected one of %s", d.config.Mode, strings.Join(Modes, ", "))
	}

	downloadURLs := make([]*url.URL, d.config.Concurrency)
	for i := range downloadURLs {
		if i > 0 && d.config.Mode == ModeRepeat {
			downloadURLs[i] = downloadURLs[0]
			continue
		}

		downloadURL, err := d.fileServer.GetURL(file, config.DownloaderDfget)
		if err != nil {
			return nil, err
		}

		if d.config.Mode == ModeFixed {
			query := downloadURL.Query()
			query.Del("uuid")
			query.Set("r", strconv.Itoa(i))
			downloadURL.RawQuery = query.Encode()
		}

		downloadURLs[i] = downloadURL
	}

	return downloadURLs, nil
}
