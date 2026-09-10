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
	"net/url"
	"path"
	"slices"
	"sync"
	"time"

	"github.com/dragonflyoss/perf-tests/pkg/backend"
	"github.com/dragonflyoss/perf-tests/pkg/config"
	"github.com/dragonflyoss/perf-tests/pkg/util"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// OutputDir is the directory in the peer pods to write the downloaded files to.
const OutputDir = "/tmp"

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
	peers, err := util.GetPeers(ctx, f.config.Namespace, int(f.config.Peers))
	if err != nil {
		return err
	}

	seeds, err := util.GetSeeds(ctx, f.config.Namespace)
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
	before, err := util.CollectTraffic(ctx, f.config.Namespace, f.config.MetricsPort, members)
	if err != nil {
		logrus.Errorf("failed to collect client metrics: %v", err)
		return err
	}

	fmt.Printf("Downloading %s on %d peers ...\n", downloadURL, len(peers))
	start := time.Now()
	downloads := make(util.Downloads, len(peers))
	var wg sync.WaitGroup
	for i, p := range peers {
		wg.Go(func() {
			downloads[i] = f.downloadByDfget(ctx, p, downloadURL, file)
		})
	}
	wg.Wait()

	after, err := util.CollectTraffic(ctx, f.config.Namespace, f.config.MetricsPort, members)
	if err != nil {
		logrus.Errorf("failed to collect client metrics: %v", err)
		return err
	}

	var traffic util.Traffic
	for i := range members {
		traffic = traffic.Add(after[i].Sub(before[i]))
	}

	result := &Result{File: file, URL: downloadURL.String(), Downloads: downloads, Traffic: traffic, Elapsed: time.Since(start)}
	f.stats.SetResult(result)

	fmt.Printf("Downloaded %s: %d/%d succeeded in %s\n", file, downloads.Succeeded(), len(downloads), result.Elapsed.Round(time.Millisecond))
	return nil
}

// Cleanup clears the cache of the peers and seed peers.
func (f *fileBench) Cleanup(ctx context.Context) error {
	return util.CleanupWorkloads(ctx, f.config.Namespace)
}

// downloadByDfget downloads the file on the peer by dfget and removes the output afterwards.
func (f *fileBench) downloadByDfget(ctx context.Context, p util.Peer, downloadURL *url.URL, file string) *util.Download {
	podExec := util.NewPodExec(f.config.Namespace, p.Pod, p.Container)
	outputPath := path.Join(OutputDir, fmt.Sprintf("%s-%s-%s", path.Base(file), config.DownloaderDfget, uuid.New().String()))

	start := time.Now()
	output, err := podExec.Command(ctx, "sh", "-c", fmt.Sprintf("dfget '%s' --output %s", downloadURL.String(), outputPath)).CombinedOutput()
	cost := time.Since(start)

	if rmOutput, rmErr := podExec.Command(ctx, "sh", "-c", fmt.Sprintf("rm -f %s", outputPath)).CombinedOutput(); rmErr != nil {
		logrus.Errorf("failed to cleanup: %v \nmessage: %s", rmErr, string(rmOutput))
	}

	if err != nil {
		logrus.Errorf("failed to download file on %s: %v \nmessage: %s", p.Pod, err, string(output))
		return &util.Download{Peer: p.Pod, Cost: cost, Err: err}
	}

	logrus.Debugf("dfget output: %s", string(output))
	return &util.Download{Peer: p.Pod, Cost: cost}
}
