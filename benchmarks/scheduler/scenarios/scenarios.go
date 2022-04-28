/*
 *     Copyright 2022 The Dragonfly Authors
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

package scenarios

// Scenario is the interface used for performance test
type Scenario interface {
	// Name returs name of scenario
	Name() string

	// Run executes the scenario performance test
	Run() error

	// Print output performance test results to os.Stdout
	Print()
}

// New scenario interfaces
func New(host, protoset string, concurrency uint, insecure bool) []Scenario {
	return []Scenario{
		newRegister(host, protoset, concurrency, insecure),
		newReportPiece(host, protoset, concurrency, insecure),
		newReportPeer(host, protoset, concurrency, insecure),
		newStatTask(host, protoset, concurrency, insecure),
		newAnnounceTask(host, protoset, concurrency, insecure),
		newLeaveTask(host, protoset, concurrency, insecure),
	}
}
