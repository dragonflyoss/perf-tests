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
	"encoding/json"
	"fmt"

	"github.com/dragonflyoss/perf-tests/pkg/util"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// dfdaemonConfigKey is the key of the dfdaemon config in the ConfigMap.
const dfdaemonConfigKey = "dfdaemon.yaml"

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

// Cleanup disables storage.keep of the peers and seed peers and restarts them.
func (f *fileBench) Cleanup(ctx context.Context) error {
	for _, w := range workloads {
		if err := f.cleanupWorkload(ctx, w); err != nil {
			logrus.Errorf("failed to cleanup %s: %v", w.kind, err)
			return err
		}
	}

	return nil
}

// cleanupWorkload disables storage.keep in the ConfigMap of the workload and restarts its controller.
func (f *fileBench) cleanupWorkload(ctx context.Context, w workload) error {
	configMap, err := f.getResource(ctx, "configmap", w.label)
	if err != nil {
		return err
	}

	controller, err := f.getResource(ctx, w.kind, w.label)
	if err != nil {
		return err
	}

	// Skip the workload if it is not deployed.
	if configMap == "" || controller == "" {
		logrus.Warnf("no %s found by %s", w.kind, w.label)
		return nil
	}

	fmt.Printf("Disabling storage.keep in configmap %s ...\n", configMap)
	if err := f.disableStorageKeep(ctx, configMap); err != nil {
		return err
	}

	fmt.Printf("Restarting %s %s ...\n", w.kind, controller)
	if err := f.restartController(ctx, w.kind, controller); err != nil {
		return err
	}

	return nil
}

// getResource returns the name of the resource by label, empty if not found.
func (f *fileBench) getResource(ctx context.Context, resource string, label string) (string, error) {
	names, err := util.GetResourceNames(ctx, f.config.Namespace, resource, label)
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
func (f *fileBench) disableStorageKeep(ctx context.Context, configMap string) error {
	// Read stdout only, kubectl prints warnings to stderr.
	output, err := util.KubeCtlCommand(ctx, "get", "configmap", configMap, "-n", f.config.Namespace, "-o", "jsonpath={.data.dfdaemon\\.yaml}").Output()
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

	output, err = util.KubeCtlCommand(ctx, "patch", "configmap", configMap, "-n", f.config.Namespace, "--type", "merge", "-p", string(patch)).CombinedOutput()
	if err != nil {
		logrus.Errorf("failed to patch configmap: %v \nmessage: %s", err, string(output))
		return err
	}

	return nil
}

// restartController restarts the controller and waits for the rollout.
func (f *fileBench) restartController(ctx context.Context, kind string, name string) error {
	output, err := util.KubeCtlCommand(ctx, "rollout", "restart", kind, name, "-n", f.config.Namespace).CombinedOutput()
	if err != nil {
		logrus.Errorf("failed to restart %s: %v \nmessage: %s", kind, err, string(output))
		return err
	}

	output, err = util.KubeCtlCommand(ctx, "rollout", "status", kind, name, "-n", f.config.Namespace).CombinedOutput()
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
