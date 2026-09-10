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

	"github.com/dragonflyoss/perf-tests/pkg/util"
	"github.com/sirupsen/logrus"
)

const (
	// cleanupComponent is the component label of the cleanup pods.
	cleanupComponent = "image-bench-cleanup"

	// cleanupScript removes $IMAGE from containerd by crictl, which deletes every
	// reference of the image and waits for the garbage collection, unlike ctr.
	// A node that does not have the image is skipped.
	cleanupScript = `set -e
if [ -n "$(crictl images -q "$IMAGE")" ]; then
  crictl rmi "$IMAGE"
fi
`
)

// Cleanup removes the image from containerd on the peer nodes, then clears the
// cache of the peers and seed peers.
func (b *imageBench) Cleanup(ctx context.Context) error {
	if err := b.removeImage(ctx); err != nil {
		logrus.Errorf("failed to remove image: %v", err)
		return err
	}

	peer := util.Workload{Label: b.config.PeerLabel, ConfigMap: b.config.PeerConfigMap}
	seed := util.Workload{Label: b.config.SeedPeerLabel, ConfigMap: b.config.SeedPeerConfigMap}
	return util.CleanupWorkloads(ctx, b.config.Namespace, peer, seed)
}

// removeImage deletes the pull pods left behind, then removes the image from
// containerd on every peer node by a cleanup pod running crictl.
func (b *imageBench) removeImage(ctx context.Context) error {
	// containerd keeps the layers of an image as long as a container uses them,
	// even an exited one, so the pull pods must be gone first.
	fmt.Printf("Deleting pull pods ...\n")
	if err := util.DeletePods(ctx, b.config.Namespace, pullLabel); err != nil {
		return err
	}

	peers, err := util.GetPeers(ctx, b.config.Namespace, b.config.PeerLabel, b.config.PeerContainer, int(b.config.Peers))
	if err != nil {
		return err
	}

	runID := newID()
	label := runLabel + "=" + runID
	pods := make([]any, 0, len(peers))
	for i, p := range peers {
		pods = append(pods, b.cleanupPod(fmt.Sprintf("image-bench-cleanup-%s-%d", runID, i), runID, p.Node))
	}

	defer func() {
		if err := util.DeletePods(context.WithoutCancel(ctx), b.config.Namespace, label); err != nil {
			logrus.Errorf("failed to delete pods: %v", err)
		}
	}()

	fmt.Printf("Removing %s on %d nodes ...\n", b.config.Image, len(peers))
	if err := util.CreatePods(ctx, pods); err != nil {
		return err
	}

	finished, err := util.WaitForPods(ctx, b.config.Namespace, label, len(peers), func(p util.Pod) bool {
		return p.Status.Phase == util.PodSucceeded || p.Status.Phase == util.PodFailed
	})
	if err != nil {
		return err
	}

	var failed int
	for _, p := range finished {
		if p.Status.Phase == util.PodSucceeded {
			continue
		}

		failed++
		logs, err := util.PodLogs(ctx, b.config.Namespace, p.Metadata.Name)
		if err != nil {
			logs = err.Error()
		}

		logrus.Errorf("failed to remove image on %s: %s", p.Spec.NodeName, logs)
	}

	if failed > 0 {
		return fmt.Errorf("failed to remove image on %d of %d nodes", failed, len(peers))
	}

	return nil
}

// cleanupPod returns the manifest of a privileged pod pinned to the node that
// removes the image through the containerd socket of the host.
func (b *imageBench) cleanupPod(name string, runID string, node string) map[string]any {
	endpoint := "unix://" + b.config.ContainerdSocket
	return map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      name,
			"namespace": b.config.Namespace,
			"labels": map[string]string{
				"app":       "dragonfly",
				"component": cleanupComponent,
				runLabel:    runID,
			},
		},
		"spec": map[string]any{
			"nodeName":                      node,
			"restartPolicy":                 "Never",
			"terminationGracePeriodSeconds": 0,
			"tolerations":                   []any{map[string]any{"operator": "Exists"}},
			// The image runs as nobody, crictl needs root for the containerd socket.
			"securityContext": map[string]any{"runAsUser": 0, "runAsGroup": 0},
			"volumes": []any{map[string]any{
				"name":     "containerd-socket",
				"hostPath": map[string]any{"path": b.config.ContainerdSocket, "type": "Socket"},
			}},
			"containers": []any{map[string]any{
				"name":            "cleanup",
				"image":           b.config.CleanupImage,
				"imagePullPolicy": "IfNotPresent",
				"command":         []string{"sh", "-c", cleanupScript},
				"env": []any{
					map[string]any{"name": "IMAGE", "value": b.config.Image},
					map[string]any{"name": "CONTAINER_RUNTIME_ENDPOINT", "value": endpoint},
					map[string]any{"name": "IMAGE_SERVICE_ENDPOINT", "value": endpoint},
				},
				"securityContext": map[string]any{"privileged": true},
				"volumeMounts": []any{map[string]any{
					"name":      "containerd-socket",
					"mountPath": b.config.ContainerdSocket,
				}},
			}},
		},
	}
}
