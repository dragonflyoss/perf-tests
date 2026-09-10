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
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/dragonflyoss/perf-tests/pkg/config"
	"github.com/dragonflyoss/perf-tests/pkg/util"
	"github.com/sirupsen/logrus"
)

// ImageBench represents a benchmark runner for concurrent image pulls.
type ImageBench interface {
	// Run pulls the image on all peer nodes at the same time.
	Run(context.Context) error

	// Cleanup removes the image from the peer nodes and clears the cache of the peers and seed peers.
	Cleanup(context.Context) error
}

// imageBench implements the ImageBench interface.
type imageBench struct {
	// config is the configuration of the benchmark.
	config *config.ImageBenchConfig

	// stats is the statistics of the benchmark.
	stats Stats
}

// New creates a new benchmark runner for concurrent image pulls.
func New(config *config.ImageBenchConfig, stats Stats) ImageBench {
	return &imageBench{config, stats}
}

// Run pulls the image on all peer nodes at the same time.
func (b *imageBench) Run(ctx context.Context) error {
	peers, err := util.GetPeers(ctx, b.config.Namespace, b.config.PeerLabel, b.config.PeerContainer, int(b.config.Peers))
	if err != nil {
		return err
	}

	seeds, err := util.GetSeeds(ctx, b.config.Namespace, b.config.SeedPeerLabel, b.config.SeedPeerContainer)
	if err != nil {
		return err
	}

	image := b.config.Image

	// Seed peers serve the peers and may back to source, so their traffic counts too.
	members := slices.Concat(peers, seeds)
	before := util.CollectTraffic(ctx, b.config.Namespace, b.config.MetricsPort, members)

	fmt.Printf("Pulling %s on %d peers ...\n", image, len(peers))
	start := time.Now()
	downloads, err := b.pull(ctx, image, peers)
	if err != nil {
		return err
	}

	after := util.CollectTraffic(ctx, b.config.Namespace, b.config.MetricsPort, members)
	traffic, sampled := util.TrafficBetween(before, after)
	if sampled < len(members) {
		logrus.Warnf("read the metrics of %d of %d peers, the traffic leaves out the rest", sampled, len(members))
	}

	result := &Result{Image: image, Downloads: downloads, Traffic: traffic, Sampled: sampled, Members: len(members), Elapsed: time.Since(start)}
	b.stats.SetResult(result)

	fmt.Printf("Pulled %s: %d/%d succeeded in %s\n", image, downloads.Succeeded(), len(downloads), result.Elapsed.Round(time.Millisecond))
	return nil
}
