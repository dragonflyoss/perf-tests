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

package target

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/dragonflyoss/perf-tests/pkg/fixedpool/contract"
)

type Result struct {
	ManifestSHA256 string
	ObjectCount    uint64
	SourceBaseURL  string
}

type Spec struct {
	Manifest         string
	SourceBaseURL    string
	Proxy            string
	Concurrency      uint64
	RequestTimeoutMS uint64
}

func Prepare(ctx context.Context, s Spec) (result Result, err error) {
	if s.Concurrency == 0 {
		return result, contract.Tag(contract.InputInvalid, fmt.Errorf("prepare.concurrency: expected a positive integer, actual %d", s.Concurrency))
	}
	if s.RequestTimeoutMS == 0 || s.RequestTimeoutMS > uint64((1<<63-1)/int64(time.Millisecond)) {
		return result, contract.Tag(contract.InputInvalid, fmt.Errorf("prepare.request_timeout_ms: expected positive representable milliseconds, actual %d", s.RequestTimeoutMS))
	}
	m, raw, err := contract.ReadManifest(s.Manifest)
	if err != nil {
		return result, err
	}
	s.SourceBaseURL, err = contract.NormalizeBaseURL(s.SourceBaseURL)
	if err != nil {
		return result, contract.Tag(contract.InputInvalid, fmt.Errorf("source-base-url: %w", err))
	}
	client, err := newClient(s.Proxy)
	if err != nil {
		return result, contract.Tag(contract.InputInvalid, fmt.Errorf("proxy: %w", err))
	}
	defer client.CloseIdleConnections()
	if err := prepareObjects(ctx, client, m, s); err != nil {
		return result, err
	}
	result = Result{ManifestSHA256: contract.Digest(raw), ObjectCount: m.ObjectCount, SourceBaseURL: s.SourceBaseURL}
	return result, nil
}

func prepareObjects(ctx context.Context, client *http.Client, m contract.Manifest, s Spec) error {
	requestTimeout := time.Duration(s.RequestTimeoutMS) * time.Millisecond
	var mu sync.Mutex
	var failure error
	tasks := map[string]uint64{}
	next := 0
	var wait sync.WaitGroup
	for range min(s.Concurrency, m.ObjectCount) {
		wait.Go(func() {
			for {
				mu.Lock()
				if failure != nil || ctx.Err() != nil || next == len(m.Objects) {
					mu.Unlock()
					return
				}
				o := m.Objects[next]
				next++
				mu.Unlock()
				objectURL := s.SourceBaseURL + "/objects/sha256/" + o.SHA256
				task, err := fullRead(ctx, client, objectURL, o, m.ObjectSize, requestTimeout)
				if err == nil {
					mu.Lock()
					if other, exists := tasks[task]; exists {
						err = fmt.Errorf("objects %d and %d share task %s", other, o.ID, task)
					} else {
						tasks[task] = o.ID
					}
					mu.Unlock()
				}
				if err != nil {
					mu.Lock()
					if failure == nil {
						failure = fmt.Errorf(
							"object_id=%d stage=full_get url=%q task_id=%q proxy=%q: %w",
							o.ID, objectURL, task, s.Proxy, err,
						)
					}
					mu.Unlock()
					return
				}
			}
		})
	}
	wait.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return failure
}
