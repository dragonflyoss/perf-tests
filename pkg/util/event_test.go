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
	"encoding/json"
	"testing"
	"time"
)

func TestDecodeEvents(t *testing.T) {
	data := `[
		{"involvedObject":{"kind":"Pod","name":"a","uid":"u1"},"reason":"Pulled","message":"m","lastTimestamp":"2026-09-10T12:00:03Z","eventTime":null},
		{"involvedObject":{"kind":"Pod","name":"a","uid":"u2"},"reason":"Failed","message":"n","lastTimestamp":null,"eventTime":"2026-09-10T12:00:04.123456Z"}
	]`

	var events []Event
	if err := json.Unmarshal([]byte(data), &events); err != nil {
		t.Fatalf("unmarshal events: %v", err)
	}

	if len(events) != 2 || events[0].InvolvedObject.UID != "u1" || events[1].InvolvedObject.UID != "u2" {
		t.Fatalf("unexpected events: %+v", events)
	}

	if got := events[0].Timestamp(); got != time.Date(2026, 9, 10, 12, 0, 3, 0, time.UTC) {
		t.Errorf("Timestamp() of legacy event = %v", got)
	}

	if got := events[1].Timestamp(); got != time.Date(2026, 9, 10, 12, 0, 4, 123456000, time.UTC) {
		t.Errorf("Timestamp() of new event = %v", got)
	}
}
