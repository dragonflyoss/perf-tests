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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dragonflyoss/perf-tests/pkg/backend"
	"github.com/dragonflyoss/perf-tests/pkg/config"
	"github.com/dragonflyoss/perf-tests/pkg/util"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// dfgetScript downloads $2 to $3 in the directory $1 by dfget and prints the start and end
// time in nanoseconds, so the cost is measured in the pod without the kubectl exec overhead.
// The dfget output goes to stderr, it is read when the download fails.
const dfgetScript = `mkdir -p "$1" || exit $?
start=$(date +%s%N)
dfget "$2" --output "$3" 1>&2
rc=$?
end=$(date +%s%N)
echo "$start $end"
exit $rc`

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
			downloads[i] = f.downloadByDfget(ctx, p, downloadURL, file)
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

// downloadByDfget downloads the file on the peer by dfget and removes the output afterwards.
func (f *fileBench) downloadByDfget(ctx context.Context, p util.Peer, downloadURL *url.URL, file string) *util.Download {
	podExec := util.NewPodExec(f.config.Namespace, p.Pod, p.Container)
	outputPath := path.Join(f.config.OutputDir, fmt.Sprintf("%s-%s-%s", path.Base(file), config.DownloaderDfget, uuid.New().String()))

	start := time.Now()
	// Read stdout only, kubectl prints warnings to stderr.
	output, err := podExec.Command(ctx, "sh", "-c", dfgetScript, "sh", f.config.OutputDir, downloadURL.String(), outputPath).Output()
	cost := time.Since(start)

	if rmOutput, rmErr := podExec.Command(ctx, "sh", "-c", fmt.Sprintf("rm -f %s", outputPath)).CombinedOutput(); rmErr != nil {
		logrus.Errorf("failed to cleanup: %v \nmessage: %s", rmErr, string(rmOutput))
	}

	if err != nil {
		logrus.Errorf("failed to download file on %s: %v \nmessage: %s", p.Pod, err, util.Stderr(err))
		return &util.Download{Peer: p.Pod, Cost: cost, Err: err}
	}

	// Prefer the cost measured in the pod, fall back to the wall-clock cost with the kubectl exec overhead.
	if podCost, err := parseCost(output); err != nil {
		logrus.Warnf("failed to parse the cost on %s, using the wall-clock cost: %v", p.Pod, err)
	} else {
		cost = podCost
	}

	return &util.Download{Peer: p.Pod, Cost: cost}
}

// parseCost parses the start and end time in nanoseconds printed by dfgetScript into the cost.
func parseCost(output []byte) (time.Duration, error) {
	fields := strings.Fields(string(output))
	if len(fields) != 2 {
		return 0, fmt.Errorf("expected the start and end time, got %q", string(output))
	}

	start, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, err
	}

	end, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0, err
	}

	if end < start {
		return 0, fmt.Errorf("end time %d is before start time %d", end, start)
	}

	return time.Duration(end - start), nil
}
