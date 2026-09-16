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
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/dragonflyoss/perf-tests/pkg/backend"
	"github.com/dragonflyoss/perf-tests/pkg/config"
	"github.com/dragonflyoss/perf-tests/pkg/util"
	"github.com/sirupsen/logrus"
)

// FileBench represents a benchmark runner for concurrent file downloads.
type FileBench interface {
	// Run downloads the file on all peers at the same time.
	Run(context.Context) error

	// Cleanup clears the cache of the peers and seed peers.
	Cleanup(context.Context) error
}

// fileBench implements the FileBench interface.
type fileBench struct {
	// config is the configuration of the benchmark.
	config *config.FileBenchConfig

	// fileServer is the file server of the benchmark.
	fileServer backend.FileServer

	// stats is the statistics of the benchmark.
	stats Stats
}

// New creates a new benchmark runner for concurrent file downloads.
func New(config *config.FileBenchConfig, fileServer backend.FileServer, stats Stats) FileBench {
	return &fileBench{config, fileServer, stats}
}

// Run downloads the file on all peers at the same time.
func (f *fileBench) Run(ctx context.Context) error {
	peers, err := util.GetPeers(ctx, f.config.Namespace, f.config.PeerLabel, f.config.PeerContainer, int(f.config.Peers))
	if err != nil {
		return err
	}

	seeds, err := util.GetSeeds(ctx, f.config.Namespace, f.config.SeedPeerLabel, f.config.SeedPeerContainer)
	if err != nil {
		return err
	}

	file := f.config.File
	downloadURL, err := f.fileServer.GetURL(file, config.DownloaderDfget)
	if err != nil {
		logrus.Errorf("failed to get file URL: %v", err)
		return err
	}

	// Seed peers serve the peers and may back to source, so their traffic counts too.
	members := slices.Concat(peers, seeds)
	before := util.CollectTraffic(ctx, f.config.Namespace, f.config.MetricsPort, members)

	fmt.Printf("Downloading %s on %d peers ...\n", downloadURL, len(peers))
	start := time.Now()
	downloads := make(util.Downloads, len(peers))
	var wg sync.WaitGroup
	for i, p := range peers {
		wg.Go(func() {
			downloads[i] = util.DownloadByDfget(ctx, f.config.Namespace, p, downloadURL, f.config.OutputDir, file)
		})
	}
	wg.Wait()

	after := util.CollectTraffic(ctx, f.config.Namespace, f.config.MetricsPort, members)
	traffic, sampled := util.TrafficBetween(before, after)
	if sampled < len(members) {
		logrus.Warnf("read the metrics of %d of %d peers, the traffic leaves out the rest", sampled, len(members))
	}

	result := &Result{File: file, URL: downloadURL.String(), Downloads: downloads, Traffic: traffic, Sampled: sampled, Members: len(members), Elapsed: time.Since(start)}
	f.stats.SetResult(result)

	fmt.Printf("Downloaded %s: %d/%d succeeded in %s\n", file, downloads.Succeeded(), len(downloads), result.Elapsed.Round(time.Millisecond))
	return nil
}

// Cleanup clears the cache of the peers and seed peers.
func (f *fileBench) Cleanup(ctx context.Context) error {
	peer := util.Workload{Label: f.config.PeerLabel, ConfigMap: f.config.PeerConfigMap}
	seed := util.Workload{Label: f.config.SeedPeerLabel, ConfigMap: f.config.SeedPeerConfigMap}
	return util.CleanupWorkloads(ctx, f.config.Namespace, peer, seed)
}
