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
	"regexp"
	"time"

	"github.com/dragonflyoss/perf-tests/pkg/util"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

const (
	// pollInterval is the interval between two checks of the pull pods.
	pollInterval = 2 * time.Second

	// runLabel is the label key of the run id on the pods created by the benchmark.
	runLabel = "image-bench-run"

	// pullComponent is the component label of the pull pods.
	pullComponent = "image-bench-pull"

	// pullLabel is the label selector of the pull pods.
	pullLabel = "component=" + pullComponent

	// pullCommand is the command of the pull pods. The image-bench images hold nothing
	// but random data, see build/images/image-bench, so the container is expected to
	// fail to start once the image is pulled, only the pull is measured.
	pullCommand = "/image-bench-pull-only"
)

// pulledMessage matches the kubelet Pulled event and captures the pull cost, e.g.
// `Successfully pulled image "x" in 1.5s (2s including waiting). Image size: 42 bytes.`.
var pulledMessage = regexp.MustCompile(`^Successfully pulled image "[^"]*" in (\S+)`)

// pullFailureReasons are the container waiting reasons of a failed pull.
var pullFailureReasons = map[string]bool{
	"ErrImagePull":      true,
	"ImagePullBackOff":  true,
	"InvalidImageName":  true,
	"ErrImageNeverPull": true,
}

// newID returns a short random id for the pods of a run.
func newID() string {
	return uuid.New().String()[:8]
}

// pull creates one pod per peer pinned to its node, so every node pulls the image
// by containerd at the same time, waits for the pulls and deletes the pods.
func (b *imageBench) pull(ctx context.Context, image string, peers []util.Peer) (util.Downloads, error) {
	runID := newID()
	label := runLabel + "=" + runID

	downloads := make(util.Downloads, 0, len(peers))
	pending := make(map[string]*util.Download, len(peers))
	pods := make([]any, 0, len(peers))
	for i, p := range peers {
		name := fmt.Sprintf("image-bench-%s-%d", runID, i)
		download := &util.Download{Peer: p.Node}
		downloads = append(downloads, download)
		pending[name] = download
		pods = append(pods, pullPod(b.config.Namespace, name, runID, image, p.Node))
	}

	// Delete the pods on a fresh context, so they do not leak when the benchmark timed out.
	defer func() {
		if err := util.DeletePods(context.WithoutCancel(ctx), b.config.Namespace, label); err != nil {
			logrus.Errorf("failed to delete pods: %v", err)
		}
	}()

	if err := util.CreatePods(ctx, pods); err != nil {
		logrus.Errorf("failed to create pods: %v", err)
		return nil, err
	}

	if err := b.waitForPulls(ctx, label, pending); err != nil {
		logrus.Errorf("failed to wait for pulls: %v", err)
		return nil, err
	}

	return downloads, nil
}

// waitForPulls polls the pull pods and their events until every pending pod pulled
// the image or failed to.
func (b *imageBench) waitForPulls(ctx context.Context, label string, pending map[string]*util.Download) error {
	for len(pending) > 0 {
		pods, err := util.ListPods(ctx, b.config.Namespace, label)
		if err != nil {
			return err
		}

		events, err := util.ListPodEvents(ctx, b.config.Namespace)
		if err != nil {
			return err
		}

		for _, p := range pods {
			download, ok := pending[p.Metadata.Name]
			if !ok {
				continue
			}

			if cost, ok := pulledCost(p, events[p.Metadata.UID]); ok {
				download.Cost = cost
				delete(pending, p.Metadata.Name)
				logrus.Debugf("pulled image on %s in %s", download.Peer, cost)
				continue
			}

			if err := pullError(p); err != nil {
				download.Err = err
				delete(pending, p.Metadata.Name)
				logrus.Errorf("failed to pull image on %s: %v", download.Peer, err)
			}
		}

		if len(pending) == 0 {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	return nil
}

// pulledCost returns the pull cost from the Pulled event of the pod, false until the image is pulled.
func pulledCost(p util.Pod, events []util.Event) (time.Duration, bool) {
	for _, e := range events {
		if e.Reason != "Pulled" {
			continue
		}

		if m := pulledMessage.FindStringSubmatch(e.Message); m != nil {
			if cost, err := time.ParseDuration(m[1]); err == nil {
				return cost, true
			}
		}

		// The image was already present on the node, fall back to the coarse timestamps.
		if t := e.Timestamp(); !t.IsZero() && !p.Metadata.CreationTimestamp.IsZero() {
			return max(t.Sub(p.Metadata.CreationTimestamp), 0), true
		}

		return 0, true
	}

	return 0, false
}

// pullError returns why the pod cannot pull the image, nil while the pull is running or once it succeeded.
func pullError(p util.Pod) error {
	// The kubelet rejected the pod, e.g. out of resources. A failed pod without a
	// reason ran the container after the pull, which is expected to fail, see pullCommand.
	if p.Status.Phase == util.PodFailed && p.Status.Reason != "" {
		return fmt.Errorf("%s: %s", p.Status.Reason, p.Status.Message)
	}

	for _, cs := range p.Status.ContainerStatuses {
		if w := cs.State.Waiting; w != nil && pullFailureReasons[w.Reason] {
			return fmt.Errorf("%s: %s", w.Reason, w.Message)
		}
	}

	return nil
}

// pullPod returns the manifest of a pod pinned to the node that pulls the image.
func pullPod(namespace string, name string, runID string, image string, node string) map[string]any {
	return map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels": map[string]string{
				"app":       "dragonfly",
				"component": pullComponent,
				runLabel:    runID,
			},
		},
		"spec": map[string]any{
			"nodeName":                      node,
			"restartPolicy":                 "Never",
			"terminationGracePeriodSeconds": 0,
			"tolerations":                   []any{map[string]any{"operator": "Exists"}},
			"containers": []any{map[string]any{
				"name":            "image-bench",
				"image":           image,
				"imagePullPolicy": "Always",
				"command":         []string{pullCommand},
			}},
		},
	}
}
