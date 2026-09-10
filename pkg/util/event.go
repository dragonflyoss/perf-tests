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
	"time"
)

// Event is the subset of the Kubernetes Event read by the benchmarks.
type Event struct {
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

// Timestamp returns the time of the event, the last timestamp if set, otherwise the event time.
func (e Event) Timestamp() time.Time {
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp
	}

	return e.EventTime
}

// ListPodEvents returns the events of the pods indexed by pod UID. Events outlive
// their pod, the UID keeps out the events of a deleted pod with the same name.
func ListPodEvents(ctx context.Context, namespace string) (map[string][]Event, error) {
	var list struct {
		Items []Event `json:"items"`
	}
	if err := getJSON(ctx, &list, "events", "-n", namespace, "--field-selector", "involvedObject.kind=Pod"); err != nil {
		return nil, err
	}

	events := make(map[string][]Event)
	for _, e := range list.Items {
		events[e.InvolvedObject.UID] = append(events[e.InvolvedObject.UID], e)
	}

	return events, nil
}
