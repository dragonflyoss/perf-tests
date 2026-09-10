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
	"errors"
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

const (
	// OutputDir is the directory in the peer pods to write the downloaded files to.
	OutputDir = "/tmp"

	// peerLabel is the label selector of the peer pods.
	peerLabel = "component=client"

	// peerContainer is the dfdaemon container name of the peer pods.
	peerContainer = "client"

	// seedLabel is the label selector of the seed peer pods.
	seedLabel = "component=seed-client"

	// seedContainer is the dfdaemon container name of the seed peer pods.
	seedContainer = "seed-client"
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

// peer represents a dfdaemon container in a pod.
type peer struct {
	// pod is the name of the pod.
	pod string

	// container is the name of the dfdaemon container.
	container string
}

// New creates a new benchmark runner for concurrent file downloads.
func New(config *config.FileBenchConfig, fileServer backend.FileServer, stats Stats) FileBench {
	return &fileBench{config, fileServer, stats}
}

// Run downloads the file on all peers at the same time.
func (f *fileBench) Run(ctx context.Context) error {
	peers, err := f.getPeers(ctx)
	if err != nil {
		return err
	}

	seeds, err := f.getSeeds(ctx)
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
	before, err := collectTraffic(ctx, f.config.Namespace, members)
	if err != nil {
		logrus.Errorf("failed to collect client metrics: %v", err)
		return err
	}

	fmt.Printf("Downloading %s on %d peers ...\n", downloadURL, len(peers))
	start := time.Now()
	downloads := make([]*Download, len(peers))
	var wg sync.WaitGroup
	for i, p := range peers {
		wg.Go(func() {
			downloads[i] = f.downloadByDfget(ctx, p, downloadURL, file)
		})
	}
	wg.Wait()

	after, err := collectTraffic(ctx, f.config.Namespace, members)
	if err != nil {
		logrus.Errorf("failed to collect client metrics: %v", err)
		return err
	}

	var traffic Traffic
	for i := range members {
		traffic = traffic.Add(after[i].Sub(before[i]))
	}

	result := &Result{File: file, Downloads: downloads, Traffic: traffic}
	f.stats.SetResult(result)

	fmt.Printf("Downloaded %s: %d/%d succeeded in %s\n", file, result.Succeeded(), len(downloads), time.Since(start).Round(time.Millisecond))
	return nil
}

// downloadByDfget downloads the file on the peer by dfget and removes the output afterwards.
func (f *fileBench) downloadByDfget(ctx context.Context, p peer, downloadURL *url.URL, file string) *Download {
	podExec := util.NewPodExec(f.config.Namespace, p.pod, p.container)
	outputPath := path.Join(OutputDir, fmt.Sprintf("%s-%s-%s", path.Base(file), config.DownloaderDfget, uuid.New().String()))

	start := time.Now()
	output, err := podExec.Command(ctx, "sh", "-c", fmt.Sprintf("dfget '%s' --output %s", downloadURL.String(), outputPath)).CombinedOutput()
	cost := time.Since(start)

	if rmOutput, rmErr := podExec.Command(ctx, "sh", "-c", fmt.Sprintf("rm -f %s", outputPath)).CombinedOutput(); rmErr != nil {
		logrus.Errorf("failed to cleanup: %v \nmessage: %s", rmErr, string(rmOutput))
	}

	if err != nil {
		logrus.Errorf("failed to download file on %s: %v \nmessage: %s", p.pod, err, string(output))
		return &Download{Peer: p.pod, Cost: cost, Err: err}
	}

	logrus.Debugf("dfget output: %s", string(output))
	return &Download{Peer: p.pod, Cost: cost}
}

// getPeers returns the peers to download on, limited to the configured number.
func (f *fileBench) getPeers(ctx context.Context) ([]peer, error) {
	pods, err := util.GetPods(ctx, f.config.Namespace, peerLabel)
	if err != nil {
		logrus.Errorf("failed to get pods: %v", err)
		return nil, err
	}

	if len(pods) == 0 {
		logrus.Errorf("no client pod found")
		return nil, errors.New("no client pod found")
	}
	slices.Sort(pods)

	if n := int(f.config.Peers); n > len(pods) {
		logrus.Warnf("only %d client pods found, less than the requested %d", len(pods), n)
	} else if n > 0 {
		pods = pods[:n]
	}

	return newPeers(pods, peerContainer), nil
}

// getSeeds returns the seed peers to collect metrics from.
func (f *fileBench) getSeeds(ctx context.Context) ([]peer, error) {
	pods, err := util.GetPods(ctx, f.config.Namespace, seedLabel)
	if err != nil {
		logrus.Errorf("failed to get pods: %v", err)
		return nil, err
	}

	if len(pods) == 0 {
		logrus.Warnf("no seed client pod found")
	}

	return newPeers(pods, seedContainer), nil
}

// newPeers pairs the pods with the dfdaemon container name.
func newPeers(pods []string, container string) []peer {
	peers := make([]peer, 0, len(pods))
	for _, pod := range pods {
		peers = append(peers, peer{pod: pod, container: container})
	}

	return peers
}
