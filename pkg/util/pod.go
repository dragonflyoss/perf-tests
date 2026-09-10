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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	// pollInterval is the interval between two checks of the pods.
	pollInterval = 2 * time.Second

	// deleteTimeout is the timeout for deleting pods.
	deleteTimeout = 2 * time.Minute

	// PodSucceeded is the phase of a pod whose containers exited successfully.
	PodSucceeded = "Succeeded"

	// PodFailed is the phase of a pod whose containers failed or that the kubelet rejected.
	PodFailed = "Failed"
)

// Pod is the subset of the Kubernetes Pod read by the benchmarks.
type Pod struct {
	Metadata struct {
		Name              string    `json:"name"`
		UID               string    `json:"uid"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
	} `json:"metadata"`
	Spec struct {
		NodeName string `json:"nodeName"`
	} `json:"spec"`
	Status struct {
		Phase             string            `json:"phase"`
		Reason            string            `json:"reason"`
		Message           string            `json:"message"`
		ContainerStatuses []ContainerStatus `json:"containerStatuses"`
	} `json:"status"`
}

// ContainerStatus is the subset of the Kubernetes ContainerStatus read by the benchmarks.
type ContainerStatus struct {
	State struct {
		Waiting *struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"waiting"`
	} `json:"state"`
}

// ListPods returns the pods matching the label.
func ListPods(ctx context.Context, namespace string, label string) ([]Pod, error) {
	var list struct {
		Items []Pod `json:"items"`
	}
	if err := getJSON(ctx, &list, "pods", "-n", namespace, "-l", label); err != nil {
		return nil, err
	}

	return list.Items, nil
}

// CreatePods creates the pods in one kubectl call, so they start at the same time.
func CreatePods(ctx context.Context, pods []any) error {
	manifest, err := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "List", "items": pods})
	if err != nil {
		return err
	}

	cmd := KubeCtlCommand(ctx, "create", "-f", "-")
	cmd.Stdin = bytes.NewReader(manifest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create pods: %w \nmessage: %s", err, string(output))
	}

	logrus.Debugf("kubectl create output: %s", string(output))
	return nil
}

// DeletePods deletes the pods matching the label and waits until they are gone.
func DeletePods(ctx context.Context, namespace string, label string) error {
	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	output, err := KubeCtlCommand(ctx, "delete", "pods", "-n", namespace, "-l", label, "--ignore-not-found").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete pods: %w \nmessage: %s", err, string(output))
	}

	logrus.Debugf("kubectl delete output: %s", string(output))
	return nil
}

// PodLogs returns the logs of the pod.
func PodLogs(ctx context.Context, namespace string, name string) (string, error) {
	output, err := KubeCtlCommand(ctx, "logs", "-n", namespace, name).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get logs of %s: %w \nmessage: %s", name, err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}

// WaitForPods polls the pods matching the label until done returns true for
// expected pods, and returns the pods of the last poll.
func WaitForPods(ctx context.Context, namespace string, label string, expected int, done func(Pod) bool) ([]Pod, error) {
	for {
		pods, err := ListPods(ctx, namespace, label)
		if err != nil {
			return nil, err
		}

		var n int
		for _, p := range pods {
			if done(p) {
				n++
			}
		}

		if n >= expected {
			return pods, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}
