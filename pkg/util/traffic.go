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

package util

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/sirupsen/logrus"
)

// downloadTrafficMetric is the dfdaemon counter of the download traffic by type.
const downloadTrafficMetric = "dragonfly_client_download_traffic"

// Traffic represents the download traffic by type.
type Traffic struct {
	// BackToSource is the bytes downloaded from the source.
	BackToSource uint64

	// RemotePeer is the bytes downloaded from the remote peers.
	RemotePeer uint64

	// LocalPeer is the bytes hit in the local cache.
	LocalPeer uint64
}

// Add returns the sum of the traffics.
func (t Traffic) Add(o Traffic) Traffic {
	return Traffic{
		BackToSource: t.BackToSource + o.BackToSource,
		RemotePeer:   t.RemotePeer + o.RemotePeer,
		LocalPeer:    t.LocalPeer + o.LocalPeer,
	}
}

// Sub returns the difference of the traffics, clamped at zero in case the dfdaemon restarted.
func (t Traffic) Sub(o Traffic) Traffic {
	return Traffic{
		BackToSource: subClamped(t.BackToSource, o.BackToSource),
		RemotePeer:   subClamped(t.RemotePeer, o.RemotePeer),
		LocalPeer:    subClamped(t.LocalPeer, o.LocalPeer),
	}
}

// Total returns the total traffic.
func (t Traffic) Total() uint64 {
	return t.BackToSource + t.RemotePeer + t.LocalPeer
}

// subClamped returns a-b, or zero if b is greater than a.
func subClamped(a, b uint64) uint64 {
	if a < b {
		return 0
	}

	return a - b
}

// CollectTraffic collects the traffic of the peers from their dfdaemon metrics port, keyed by pod name.
// A peer whose metrics cannot be read is logged and left out, so one failed scrape does not fail the run.
func CollectTraffic(ctx context.Context, namespace string, metricsPort uint32, peers []Peer) map[string]Traffic {
	traffics := make([]*Traffic, len(peers))
	var wg sync.WaitGroup
	for i, p := range peers {
		wg.Go(func() {
			traffic, err := getTraffic(ctx, namespace, metricsPort, p)
			if err != nil {
				return
			}

			traffics[i] = &traffic
		})
	}
	wg.Wait()

	collected := make(map[string]Traffic, len(peers))
	for i, p := range peers {
		if traffics[i] != nil {
			collected[p.Pod] = *traffics[i]
		}
	}

	return collected
}

// TrafficBetween returns the traffic of the peers read both before and after, and how many they are.
// A peer missing from either side is left out, its counters could not be compared.
func TrafficBetween(before map[string]Traffic, after map[string]Traffic) (Traffic, int) {
	var traffic Traffic
	var n int
	for pod, a := range after {
		b, ok := before[pod]
		if !ok {
			continue
		}

		traffic = traffic.Add(a.Sub(b))
		n++
	}

	return traffic, n
}

// getTraffic collects the traffic of the peer from the client metrics.
func getTraffic(ctx context.Context, namespace string, metricsPort uint32, p Peer) (Traffic, error) {
	podExec := NewPodExec(namespace, p.Pod, p.Container)
	// Read stdout only, kubectl prints warnings to stderr.
	output, err := podExec.Command(ctx, "sh", "-c", fmt.Sprintf("curl -s http://127.0.0.1:%d/metrics", metricsPort)).Output()
	if err != nil {
		logrus.Errorf("failed to get client metrics on %s: %v \nmessage: %s", p.Pod, err, Stderr(err))
		return Traffic{}, err
	}

	traffic, err := parseTraffic(output)
	if err != nil {
		logrus.Errorf("failed to parse metrics on %s: %v", p.Pod, err)
		return Traffic{}, err
	}

	return traffic, nil
}

// parseTraffic parses the traffic from the client metrics, missing series count as zero.
func parseTraffic(metrics []byte) (Traffic, error) {
	parser := expfmt.NewTextParser(model.UTF8Validation)
	metricFamilies, err := parser.TextToMetricFamilies(bytes.NewReader(metrics))
	if err != nil {
		return Traffic{}, err
	}

	var traffic Traffic
	mf, ok := metricFamilies[downloadTrafficMetric]
	if !ok {
		return traffic, nil
	}

	for _, metric := range mf.GetMetric() {
		for _, label := range metric.GetLabel() {
			if label.GetName() != "type" {
				continue
			}

			value := uint64(metric.GetCounter().GetValue())
			switch label.GetValue() {
			case "BACK_TO_SOURCE":
				traffic.BackToSource += value
			case "REMOTE_PEER":
				traffic.RemotePeer += value
			case "LOCAL_PEER":
				traffic.LocalPeer += value
			default:
				logrus.Debugf("invalid traffic type: %s", label.GetValue())
			}
		}
	}

	return traffic, nil
}
