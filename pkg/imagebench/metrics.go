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
	"bytes"
	"context"
	"fmt"

	"github.com/dragonflyoss/perf-tests/pkg/util"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	// metricsPort is the dfdaemon metrics port.
	metricsPort = 4002

	// downloadTrafficMetric is the dfdaemon counter of the download traffic by type.
	downloadTrafficMetric = "dragonfly_client_download_traffic"
)

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

// collectTraffic collects the traffic of the peers, indexed like peers.
func collectTraffic(ctx context.Context, namespace string, peers []peer) ([]Traffic, error) {
	traffics := make([]Traffic, len(peers))
	var eg errgroup.Group
	for i, p := range peers {
		eg.Go(func() error {
			traffic, err := getTraffic(ctx, namespace, p)
			if err != nil {
				return err
			}

			traffics[i] = traffic
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return traffics, nil
}

// getTraffic collects the traffic of the peer from the client metrics.
func getTraffic(ctx context.Context, namespace string, p peer) (Traffic, error) {
	podExec := util.NewPodExec(namespace, p.pod, p.container)
	output, err := podExec.Command(ctx, "sh", "-c", fmt.Sprintf("curl -s http://127.0.0.1:%d/metrics", metricsPort)).CombinedOutput()
	if err != nil {
		logrus.Errorf("failed to get client metrics on %s: %v \nmessage: %s", p.pod, err, string(output))
		return Traffic{}, err
	}

	traffic, err := parseTraffic(output)
	if err != nil {
		logrus.Errorf("failed to parse metrics on %s: %v", p.pod, err)
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
