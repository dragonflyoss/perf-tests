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
	"encoding/json"
	"fmt"

	"github.com/dragonflyoss/perf-tests/pkg/util"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

const (
	// dfdaemonConfigKey is the key of the dfdaemon config in the ConfigMap.
	dfdaemonConfigKey = "dfdaemon.yaml"

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

// workload represents a dfdaemon controller and its ConfigMap, selected by the same label.
type workload struct {
	// label is the label selector of the controller and its ConfigMap.
	label string

	// kind is the controller kind.
	kind string
}

// workloads are the dfdaemon workloads cleaned up by Cleanup.
var workloads = []workload{
	{label: peerLabel, kind: "daemonset"},
	{label: seedLabel, kind: "statefulset"},
}

// Cleanup removes the image from containerd on the peer nodes, then disables
// storage.keep of the peers and seed peers and restarts them.
func (b *imageBench) Cleanup(ctx context.Context) error {
	if err := b.removeImage(ctx); err != nil {
		logrus.Errorf("failed to remove image: %v", err)
		return err
	}

	for _, w := range workloads {
		if err := b.cleanupWorkload(ctx, w); err != nil {
			logrus.Errorf("failed to cleanup %s: %v", w.kind, err)
			return err
		}
	}

	return nil
}

// removeImage deletes the pull pods left behind, then removes the image from
// containerd on every peer node by a cleanup pod running crictl.
func (b *imageBench) removeImage(ctx context.Context) error {
	// containerd keeps the layers of an image as long as a container uses them,
	// even an exited one, so the pull pods must be gone first.
	fmt.Printf("Deleting pull pods ...\n")
	if err := deletePods(ctx, b.config.Namespace, pullLabel); err != nil {
		return err
	}

	peers, err := b.getPeers(ctx)
	if err != nil {
		return err
	}

	runID := newID()
	label := runLabel + "=" + runID
	pods := make([]any, 0, len(peers))
	for i, p := range peers {
		pods = append(pods, b.cleanupPod(fmt.Sprintf("image-bench-cleanup-%s-%d", runID, i), runID, p.node))
	}

	defer func() {
		if err := deletePods(context.WithoutCancel(ctx), b.config.Namespace, label); err != nil {
			logrus.Errorf("failed to delete pods: %v", err)
		}
	}()

	fmt.Printf("Removing %s on %d nodes ...\n", b.config.Image, len(peers))
	if err := createPods(ctx, pods); err != nil {
		return err
	}

	finished, err := waitForPods(ctx, b.config.Namespace, label, len(peers), func(p pod) bool {
		return p.Status.Phase == podSucceeded || p.Status.Phase == podFailed
	})
	if err != nil {
		return err
	}

	var failed int
	for _, p := range finished {
		if p.Status.Phase == podSucceeded {
			continue
		}

		failed++
		logs, err := podLogs(ctx, b.config.Namespace, p.Metadata.Name)
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

// cleanupWorkload disables storage.keep in the ConfigMap of the workload and restarts its controller.
func (b *imageBench) cleanupWorkload(ctx context.Context, w workload) error {
	configMap, err := b.getResource(ctx, "configmap", w.label)
	if err != nil {
		return err
	}

	controller, err := b.getResource(ctx, w.kind, w.label)
	if err != nil {
		return err
	}

	// Skip the workload if it is not deployed.
	if configMap == "" || controller == "" {
		logrus.Warnf("no %s found by %s", w.kind, w.label)
		return nil
	}

	fmt.Printf("Disabling storage.keep in configmap %s ...\n", configMap)
	if err := b.disableStorageKeep(ctx, configMap); err != nil {
		return err
	}

	fmt.Printf("Restarting %s %s ...\n", w.kind, controller)
	if err := b.restartController(ctx, w.kind, controller); err != nil {
		return err
	}

	return nil
}

// getResource returns the name of the resource by label, empty if not found.
func (b *imageBench) getResource(ctx context.Context, resource string, label string) (string, error) {
	names, err := util.GetResourceNames(ctx, b.config.Namespace, resource, label)
	if err != nil {
		logrus.Errorf("failed to get %s: %v", resource, err)
		return "", err
	}

	switch len(names) {
	case 0:
		return "", nil
	case 1:
		return names[0], nil
	default:
		logrus.Errorf("found %d %s by %s: %v", len(names), resource, label, names)
		return "", fmt.Errorf("found %d %s by %s", len(names), resource, label)
	}
}

// disableStorageKeep sets storage.keep to false in the dfdaemon config of the ConfigMap.
func (b *imageBench) disableStorageKeep(ctx context.Context, configMap string) error {
	// Read stdout only, kubectl prints warnings to stderr.
	output, err := util.KubeCtlCommand(ctx, "get", "configmap", configMap, "-n", b.config.Namespace, "-o", "jsonpath={.data.dfdaemon\\.yaml}").Output()
	if err != nil {
		logrus.Errorf("failed to get configmap: %v \nmessage: %s", err, util.Stderr(err))
		return err
	}

	dfdaemonConfig, err := setStorageKeep(output, false)
	if err != nil {
		logrus.Errorf("failed to set storage.keep: %v", err)
		return err
	}

	patch, err := json.Marshal(map[string]any{"data": map[string]string{dfdaemonConfigKey: string(dfdaemonConfig)}})
	if err != nil {
		logrus.Errorf("failed to marshal patch: %v", err)
		return err
	}

	output, err = util.KubeCtlCommand(ctx, "patch", "configmap", configMap, "-n", b.config.Namespace, "--type", "merge", "-p", string(patch)).CombinedOutput()
	if err != nil {
		logrus.Errorf("failed to patch configmap: %v \nmessage: %s", err, string(output))
		return err
	}

	return nil
}

// restartController restarts the controller and waits for the rollout.
func (b *imageBench) restartController(ctx context.Context, kind string, name string) error {
	output, err := util.KubeCtlCommand(ctx, "rollout", "restart", kind, name, "-n", b.config.Namespace).CombinedOutput()
	if err != nil {
		logrus.Errorf("failed to restart %s: %v \nmessage: %s", kind, err, string(output))
		return err
	}

	output, err = util.KubeCtlCommand(ctx, "rollout", "status", kind, name, "-n", b.config.Namespace).CombinedOutput()
	if err != nil {
		logrus.Errorf("failed to rollout %s: %v \nmessage: %s", kind, err, string(output))
		return err
	}

	logrus.Debugf("rollout output: %s", string(output))
	return nil
}

// setStorageKeep sets storage.keep in the dfdaemon config.
func setStorageKeep(dfdaemonConfig []byte, keep bool) ([]byte, error) {
	var config map[string]any
	if err := yaml.Unmarshal(dfdaemonConfig, &config); err != nil {
		return nil, err
	}

	if config == nil {
		config = map[string]any{}
	}

	storage, ok := config["storage"].(map[string]any)
	if !ok {
		storage = map[string]any{}
	}
	storage["keep"] = keep
	config["storage"] = storage

	return yaml.Marshal(config)
}
