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
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

const (
	// peerKind is the controller kind of the peers.
	peerKind = "daemonset"

	// seedKind is the controller kind of the seed peers.
	seedKind = "statefulset"

	// dfdaemonConfigKey is the key of the dfdaemon config in the ConfigMap.
	dfdaemonConfigKey = "dfdaemon.yaml"
)

// Peer represents a dfdaemon container in a pod.
type Peer struct {
	// Pod is the name of the pod.
	Pod string

	// Container is the name of the dfdaemon container.
	Container string

	// Node is the name of the node the pod runs on.
	Node string
}

// Workload selects a dfdaemon controller and its ConfigMap.
type Workload struct {
	// Label is the label selector of the controller, and of the ConfigMap when ConfigMap is empty.
	Label string

	// ConfigMap is the name of the ConfigMap, empty to find it by Label.
	ConfigMap string
}

// GetPeers returns the peers matching the label to benchmark on, sorted by pod name and limited to n, 0 means all.
func GetPeers(ctx context.Context, namespace string, label string, container string, n int) ([]Peer, error) {
	peers, err := getPeers(ctx, namespace, label, container)
	if err != nil {
		return nil, err
	}

	if len(peers) == 0 {
		logrus.Errorf("no client pod found by %s", label)
		return nil, fmt.Errorf("no client pod found by %s", label)
	}

	if n > len(peers) {
		logrus.Warnf("only %d client pods found, less than the requested %d", len(peers), n)
	} else if n > 0 {
		peers = peers[:n]
	}

	return peers, nil
}

// GetSeeds returns the seed peers matching the label to collect metrics from.
func GetSeeds(ctx context.Context, namespace string, label string, container string) ([]Peer, error) {
	seeds, err := getPeers(ctx, namespace, label, container)
	if err != nil {
		return nil, err
	}

	if len(seeds) == 0 {
		logrus.Warnf("no seed client pod found by %s", label)
	}

	return seeds, nil
}

// getPeers returns the scheduled pods matching the label as peers, sorted by pod name.
func getPeers(ctx context.Context, namespace string, label string, container string) ([]Peer, error) {
	pods, err := ListPods(ctx, namespace, label)
	if err != nil {
		logrus.Errorf("failed to get pods: %v", err)
		return nil, err
	}

	peers := make([]Peer, 0, len(pods))
	for _, pod := range pods {
		if pod.Spec.NodeName == "" {
			logrus.Warnf("pod %s is not scheduled, skipping", pod.Metadata.Name)
			continue
		}

		peers = append(peers, Peer{Pod: pod.Metadata.Name, Container: container, Node: pod.Spec.NodeName})
	}
	slices.SortFunc(peers, func(a, b Peer) int { return strings.Compare(a.Pod, b.Pod) })

	return peers, nil
}

// CleanupWorkloads disables storage.keep of the peer daemonset and the seed peer statefulset
// and restarts them, so they start with an empty cache.
func CleanupWorkloads(ctx context.Context, namespace string, peer Workload, seed Workload) error {
	if err := cleanupWorkload(ctx, namespace, peerKind, peer); err != nil {
		logrus.Errorf("failed to cleanup %s: %v", peerKind, err)
		return err
	}

	if err := cleanupWorkload(ctx, namespace, seedKind, seed); err != nil {
		logrus.Errorf("failed to cleanup %s: %v", seedKind, err)
		return err
	}

	return nil
}

// cleanupWorkload disables storage.keep in the ConfigMap of the workload and restarts its controller.
func cleanupWorkload(ctx context.Context, namespace string, kind string, w Workload) error {
	configMap := w.ConfigMap
	if configMap == "" {
		var err error
		if configMap, err = getResource(ctx, namespace, "configmap", w.Label); err != nil {
			return err
		}
	}

	controller, err := getResource(ctx, namespace, kind, w.Label)
	if err != nil {
		return err
	}

	// Skip the workload if it is not deployed.
	if configMap == "" || controller == "" {
		logrus.Warnf("no %s found by %s", kind, w.Label)
		return nil
	}

	fmt.Printf("Disabling storage.keep in configmap %s ...\n", configMap)
	if err := disableStorageKeep(ctx, namespace, configMap); err != nil {
		return err
	}

	fmt.Printf("Restarting %s %s ...\n", kind, controller)
	if err := restartController(ctx, namespace, kind, controller); err != nil {
		return err
	}

	return nil
}

// getResource returns the name of the resource by label, empty if not found.
func getResource(ctx context.Context, namespace string, resource string, label string) (string, error) {
	names, err := GetResourceNames(ctx, namespace, resource, label)
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
func disableStorageKeep(ctx context.Context, namespace string, configMap string) error {
	// Read stdout only, kubectl prints warnings to stderr.
	output, err := KubeCtlCommand(ctx, "get", "configmap", configMap, "-n", namespace, "-o", "jsonpath={.data.dfdaemon\\.yaml}").Output()
	if err != nil {
		logrus.Errorf("failed to get configmap: %v \nmessage: %s", err, Stderr(err))
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

	output, err = KubeCtlCommand(ctx, "patch", "configmap", configMap, "-n", namespace, "--type", "merge", "-p", string(patch)).CombinedOutput()
	if err != nil {
		logrus.Errorf("failed to patch configmap: %v \nmessage: %s", err, string(output))
		return err
	}

	return nil
}

// restartController restarts the controller and waits for the rollout.
func restartController(ctx context.Context, namespace string, kind string, name string) error {
	output, err := KubeCtlCommand(ctx, "rollout", "restart", kind, name, "-n", namespace).CombinedOutput()
	if err != nil {
		logrus.Errorf("failed to restart %s: %v \nmessage: %s", kind, err, string(output))
		return err
	}

	output, err = KubeCtlCommand(ctx, "rollout", "status", kind, name, "-n", namespace).CombinedOutput()
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
