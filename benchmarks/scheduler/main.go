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

package main

import (
	"os"
	"strconv"

	"github.com/dragonflyoss/perf-tests/benchmarks/scheduler/scenarios"
)

var (
	host             = "localhost:8002"
	protoset         = "../bundle.pb"
	concurrency uint = 100
	insecure         = true
)

func init() {
	if h := os.Getenv("DRAGONFLY_TEST_SCHEDULER_HOST"); h != "" {
		host = h
	}

	if p := os.Getenv("DRAGONFLY_TEST_SCHEDULER_PROTOSET"); p != "" {
		protoset = p
	}

	if c := os.Getenv("DRAGONFLY_TEST_SCHEDULER_CONCURRENCY"); c != "" {
		c, err := strconv.Atoi(c)
		if err != nil {
			panic(err)
		}

		concurrency = uint(c)
	}

	if i := os.Getenv("DRAGONFLY_TEST_SCHEDULER_INSECURE"); i != "" {
		i, err := strconv.ParseBool(i)
		if err != nil {
			panic(err)
		}

		insecure = i
	}
}

func main() {
	scenarios := scenarios.New(host, protoset, concurrency, insecure)
	for _, scenario := range scenarios {
		if err := scenario.Run(); err != nil {
			panic(err)
		}
	}

	for _, scenario := range scenarios {
		scenario.Print()
	}
}
