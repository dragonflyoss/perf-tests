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
	"testing"
	"time"
)

func TestParseCost(t *testing.T) {
	got, err := parseCost([]byte("1789051377568447313 1789051378368447313\n"))
	if err != nil {
		t.Fatalf("parseCost() error = %v", err)
	}

	if got != 800*time.Millisecond {
		t.Fatalf("parseCost() = %s, want 800ms", got)
	}
}

func TestParseCostInvalid(t *testing.T) {
	for _, output := range []string{
		"",
		"1789051377568447313",
		"1789051377568447313 1789051377568447313 1789051377568447313",
		"1789051377568447313%N 1789051378368447313%N",
		"1789051378368447313 1789051377568447313",
	} {
		if _, err := parseCost([]byte(output)); err == nil {
			t.Fatalf("parseCost(%q) expected error", output)
		}
	}
}
