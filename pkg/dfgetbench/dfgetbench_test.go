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

package dfgetbench

import (
	"strconv"
	"strings"
	"testing"

	"github.com/dragonflyoss/perf-tests/pkg/backend"
	"github.com/dragonflyoss/perf-tests/pkg/config"
)

func TestGetURLs(t *testing.T) {
	fileServer := backend.NewFileServerURL("http://file-server")

	// repeat shares one URL, and so one task, between all dfget.
	repeat := New(&config.DfgetBenchConfig{Concurrency: 3, Mode: ModeRepeat}, fileServer, NewStats()).(*dfgetBench)
	urls, err := repeat.getURLs("1g")
	if err != nil {
		t.Fatalf("getURLs() error = %v", err)
	}

	if len(urls) != 3 || urls[1] != urls[0] || urls[2] != urls[0] {
		t.Fatalf("getURLs() = %v, want the same URL 3 times", urls)
	}

	if !strings.HasPrefix(urls[0].String(), "http://file-server/1g?") || urls[0].Query().Get("tag") != config.DownloaderDfget {
		t.Fatalf("getURLs()[0] = %s, want http://file-server/1g tagged dfget", urls[0])
	}

	// random gives every dfget its own uuid, and so its own task.
	random := New(&config.DfgetBenchConfig{Concurrency: 3, Mode: ModeRandom}, fileServer, NewStats()).(*dfgetBench)
	if urls, err = random.getURLs("1g"); err != nil {
		t.Fatalf("getURLs() error = %v", err)
	}

	uuids := map[string]bool{}
	for _, u := range urls {
		uuids[u.Query().Get("uuid")] = true
	}

	if len(uuids) != 3 {
		t.Fatalf("getURLs() = %v, want 3 distinct uuids", urls)
	}

	// fixed numbers the dfget instead of the uuid, so the URLs are the same on every run.
	fixed := New(&config.DfgetBenchConfig{Concurrency: 3, Mode: ModeFixed}, fileServer, NewStats()).(*dfgetBench)
	if urls, err = fixed.getURLs("1g"); err != nil {
		t.Fatalf("getURLs() error = %v", err)
	}

	for i, u := range urls {
		if want := "http://file-server/1g?r=" + strconv.Itoa(i) + "&tag=dfget"; u.String() != want {
			t.Fatalf("getURLs()[%d] = %s, want %s", i, u, want)
		}
	}
}

func TestGetURLsUnknownMode(t *testing.T) {
	unknown := New(&config.DfgetBenchConfig{Concurrency: 1, Mode: "sequential"}, backend.NewFileServerURL("http://file-server"), NewStats()).(*dfgetBench)
	if _, err := unknown.getURLs("1g"); err == nil {
		t.Fatal("getURLs() expected error for an unknown mode")
	}
}
