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
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/dragonflyoss/perf-tests/pkg/config"
	"github.com/sirupsen/logrus"
)

const (
	// peerLabel is the label selector of the peer pods, one per node.
	peerLabel = "component=client"

	// peerContainer is the dfdaemon container name of the peer pods.
	peerContainer = "client"

	// seedLabel is the label selector of the seed peer pods.
	seedLabel = "component=seed-client"

	// seedContainer is the dfdaemon container name of the seed peer pods.
	seedContainer = "seed-client"
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

// peer represents a dfdaemon container in a pod and the node it runs on.
type peer struct {
	// pod is the name of the pod.
	pod string

	// container is the name of the dfdaemon container.
	container string

	// node is the name of the node.
	node string
}

// New creates a new benchmark runner for concurrent image pulls.
func New(config *config.ImageBenchConfig, stats Stats) ImageBench {
	return &imageBench{config, stats}
}

// Run pulls the image on all peer nodes at the same time.
func (b *imageBench) Run(ctx context.Context) error {
	peers, err := b.getPeers(ctx)
	if err != nil {
		return err
	}

	seeds, err := b.getSeeds(ctx)
	if err != nil {
		return err
	}

	image := b.config.Image

	// Seed peers serve the peers and may back to source, so their traffic counts too.
	members := slices.Concat(peers, seeds)
	before, err := collectTraffic(ctx, b.config.Namespace, members)
	if err != nil {
		logrus.Errorf("failed to collect client metrics: %v", err)
		return err
	}

	fmt.Printf("Pulling %s on %d peers ...\n", image, len(peers))
	start := time.Now()
	downloads, err := b.pull(ctx, image, peers)
	if err != nil {
		return err
	}

	after, err := collectTraffic(ctx, b.config.Namespace, members)
	if err != nil {
		logrus.Errorf("failed to collect client metrics: %v", err)
		return err
	}

	var traffic Traffic
	for i := range members {
		traffic = traffic.Add(after[i].Sub(before[i]))
	}

	result := &Result{Image: image, Downloads: downloads, Traffic: traffic, Elapsed: time.Since(start)}
	b.stats.SetResult(result)

	fmt.Printf("Pulled %s: %d/%d succeeded in %s\n", image, result.Succeeded(), len(downloads), result.Elapsed.Round(time.Millisecond))
	return nil
}

// getPeers returns the peers to pull on, one per node sorted by node name, limited to the configured number.
func (b *imageBench) getPeers(ctx context.Context) ([]peer, error) {
	pods, err := listPods(ctx, b.config.Namespace, peerLabel)
	if err != nil {
		logrus.Errorf("failed to get pods: %v", err)
		return nil, err
	}

	peers := make([]peer, 0, len(pods))
	for _, p := range pods {
		if p.Spec.NodeName == "" {
			logrus.Warnf("client pod %s is not scheduled, skipping", p.Metadata.Name)
			continue
		}

		peers = append(peers, peer{pod: p.Metadata.Name, container: peerContainer, node: p.Spec.NodeName})
	}

	if len(peers) == 0 {
		logrus.Errorf("no client pod found")
		return nil, errors.New("no client pod found")
	}
	slices.SortFunc(peers, func(x, y peer) int { return strings.Compare(x.node, y.node) })

	if n := int(b.config.Peers); n > len(peers) {
		logrus.Warnf("only %d client pods found, less than the requested %d", len(peers), n)
	} else if n > 0 {
		peers = peers[:n]
	}

	return peers, nil
}

// getSeeds returns the seed peers to collect metrics from.
func (b *imageBench) getSeeds(ctx context.Context) ([]peer, error) {
	pods, err := listPods(ctx, b.config.Namespace, seedLabel)
	if err != nil {
		logrus.Errorf("failed to get pods: %v", err)
		return nil, err
	}

	if len(pods) == 0 {
		logrus.Warnf("no seed client pod found")
	}

	seeds := make([]peer, 0, len(pods))
	for _, p := range pods {
		seeds = append(seeds, peer{pod: p.Metadata.Name, container: seedContainer, node: p.Spec.NodeName})
	}

	return seeds, nil
}
