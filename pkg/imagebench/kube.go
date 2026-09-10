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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dragonflyoss/perf-tests/pkg/util"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

const (
	// pollInterval is the interval between two checks of the pods.
	pollInterval = 2 * time.Second

	// deleteTimeout is the timeout for deleting the pods of a run.
	deleteTimeout = 2 * time.Minute

	// runLabel is the label key of the run id on the pods created by the benchmark.
	runLabel = "image-bench-run"

	// podSucceeded is the phase of a pod whose containers exited successfully.
	podSucceeded = "Succeeded"

	// podFailed is the phase of a pod whose containers failed or that the kubelet rejected.
	podFailed = "Failed"
)

// podList is the subset of the Kubernetes PodList read by the benchmark.
type podList struct {
	Items []pod `json:"items"`
}

// pod is the subset of the Kubernetes Pod read by the benchmark.
type pod struct {
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
		ContainerStatuses []containerStatus `json:"containerStatuses"`
	} `json:"status"`
}

// containerStatus is the subset of the Kubernetes ContainerStatus read by the benchmark.
type containerStatus struct {
	State struct {
		Waiting *struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"waiting"`
	} `json:"state"`
}

// eventList is the subset of the Kubernetes EventList read by the benchmark.
type eventList struct {
	Items []event `json:"items"`
}

// event is the subset of the Kubernetes Event read by the benchmark.
type event struct {
	InvolvedObject struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
		UID  string `json:"uid"`
	} `json:"involvedObject"`
	Reason        string    `json:"reason"`
	Message       string    `json:"message"`
	LastTimestamp time.Time `json:"lastTimestamp"`
	EventTime     time.Time `json:"eventTime"`
}

// timestamp returns the time of the event, the last timestamp if set, otherwise the event time.
func (e event) timestamp() time.Time {
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp
	}

	return e.EventTime
}

// newID returns a short random id for the pods of a run.
func newID() string {
	return uuid.New().String()[:8]
}

// getJSON runs kubectl get with the JSON output and decodes it into out.
func getJSON(ctx context.Context, out any, resource string, args ...string) error {
	// Read stdout only, kubectl prints warnings to stderr.
	output, err := util.KubeCtlCommand(ctx, append(append([]string{"get", resource}, args...), "-o", "json")...).Output()
	if err != nil {
		return fmt.Errorf("failed to get %s: %w \nmessage: %s", resource, err, util.Stderr(err))
	}

	return json.Unmarshal(output, out)
}

// listPods returns the pods matching the label.
func listPods(ctx context.Context, namespace string, label string) ([]pod, error) {
	var list podList
	if err := getJSON(ctx, &list, "pods", "-n", namespace, "-l", label); err != nil {
		return nil, err
	}

	return list.Items, nil
}

// listPodEvents returns the events of the pods indexed by pod UID. Events outlive
// their pod, the UID keeps out the events of a deleted pod with the same name.
func listPodEvents(ctx context.Context, namespace string) (map[string][]event, error) {
	var list eventList
	if err := getJSON(ctx, &list, "events", "-n", namespace, "--field-selector", "involvedObject.kind=Pod"); err != nil {
		return nil, err
	}

	events := make(map[string][]event)
	for _, e := range list.Items {
		events[e.InvolvedObject.UID] = append(events[e.InvolvedObject.UID], e)
	}

	return events, nil
}

// createPods creates the pods in one kubectl call, so they start at the same time.
func createPods(ctx context.Context, pods []any) error {
	manifest, err := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "List", "items": pods})
	if err != nil {
		return err
	}

	cmd := util.KubeCtlCommand(ctx, "create", "-f", "-")
	cmd.Stdin = bytes.NewReader(manifest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create pods: %w \nmessage: %s", err, string(output))
	}

	logrus.Debugf("kubectl create output: %s", string(output))
	return nil
}

// deletePods deletes the pods matching the label and waits until they are gone.
func deletePods(ctx context.Context, namespace string, label string) error {
	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	output, err := util.KubeCtlCommand(ctx, "delete", "pods", "-n", namespace, "-l", label, "--ignore-not-found").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete pods: %w \nmessage: %s", err, string(output))
	}

	logrus.Debugf("kubectl delete output: %s", string(output))
	return nil
}

// podLogs returns the logs of the pod.
func podLogs(ctx context.Context, namespace string, name string) (string, error) {
	output, err := util.KubeCtlCommand(ctx, "logs", "-n", namespace, name).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get logs of %s: %w \nmessage: %s", name, err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}

// waitForPods polls the pods matching the label until done returns true for
// expected pods, and returns the pods of the last poll.
func waitForPods(ctx context.Context, namespace string, label string, expected int, done func(pod) bool) ([]pod, error) {
	for {
		pods, err := listPods(ctx, namespace, label)
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
