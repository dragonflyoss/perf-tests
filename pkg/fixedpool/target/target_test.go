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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dragonflyoss/perf-tests/pkg/fixedpool/contract"
)

const (
	caseCancelled             = "cancelled"
	caseConcurrent            = "concurrent"
	caseConcurrencyAboveCount = "concurrency-above-count"
	caseTimeout               = "timeout"
	caseDuplicateTask         = "duplicate-task"
)

func response(status int, body []byte) *http.Response {
	resp := &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body))}
	resp.Header.Set(taskHeader, "object-task")
	resp.Header.Set(serverHeader, "192.0.2.10")
	return resp
}

func TestFullResponseIntegrity(t *testing.T) {
	// Independent SHA-256 vector for the three bytes "abc".
	object := contract.Object{ID: 0, SHA256: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"}
	for _, name := range []string{"valid", "different-server", "status", "missing-server", "invalid-server", "ambiguous-server", "ambiguous-task", "encoding", "digest", "length", caseCancelled} {
		t.Run(name, func(t *testing.T) {
			resp := response(http.StatusOK, []byte("abc"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch name {
			case "status":
				resp.StatusCode = http.StatusPartialContent
			case "different-server":
				resp.Header.Set(serverHeader, "192.0.2.11")
			case "missing-server":
				resp.Header.Del(serverHeader)
			case "invalid-server":
				resp.Header.Set(serverHeader, "invalid")
			case "ambiguous-server":
				resp.Header.Add(serverHeader, "192.0.2.11")
			case "ambiguous-task":
				resp.Header.Add(taskHeader, "other-task")
			case "encoding":
				resp.Header.Set("Content-Encoding", "gzip")
			case "digest":
				resp.Body = io.NopCloser(strings.NewReader("xyz"))
			case "length":
				resp.Body = io.NopCloser(strings.NewReader("abcd"))
			case caseCancelled:
				cancel()
			}
			task, err := readFullResponse(ctx, resp, object, 3)
			if name == "valid" || name == "different-server" {
				if err != nil || task != "object-task" {
					t.Fatalf("task=%q err=%v", task, err)
				}
			} else if err == nil {
				t.Fatalf("invalid response completed: %q %v", task, err)
			}
		})
	}
}

func TestPreparationRequiresEveryObjectAndActiveContext(t *testing.T) {
	for _, mode := range []string{"success", caseConcurrent, caseConcurrencyAboveCount, "zero-concurrency", "bad-second-object", caseDuplicateTask, caseCancelled, caseTimeout} {
		t.Run(mode, func(t *testing.T) {
			bodies := [][]byte{[]byte("one!"), []byte("two!"), []byte("tri!")}
			concurrency := uint64(1)
			if mode == "zero-concurrency" {
				concurrency = 0
			}
			if mode == caseConcurrent {
				concurrency = 2
			}
			if mode == caseConcurrencyAboveCount {
				concurrency = 5
			}
			limit := uint32(min(concurrency, uint64(len(bodies))))
			var requests atomic.Uint32
			var active atomic.Int32
			firstBatch := make(chan struct{})
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				n := requests.Add(1)
				if req.Header.Get("Range") != "" {
					t.Error("preparation sent a Range request")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if mode == caseTimeout {
					<-req.Context().Done()
					return
				}
				if mode == caseConcurrent || mode == caseConcurrencyAboveCount {
					if inFlight := active.Add(1); inFlight > int32(limit) {
						t.Errorf("in-flight requests=%d exceed limit=%d", inFlight, limit)
					}
					// Hold the first batch until it overlaps; serial execution must time out.
					if n == limit {
						close(firstBatch)
					}
					select {
					case <-firstBatch:
					case <-req.Context().Done():
						active.Add(-1)
						return
					}
					active.Add(-1)
				}
				i := 0
				for j, body := range bodies {
					if strings.HasSuffix(req.URL.Path, contract.Digest(body)) {
						i = j
						break
					}
				}
				w.Header().Set(taskHeader, fmt.Sprintf("task-%d", i))
				if mode == caseDuplicateTask {
					w.Header().Set(taskHeader, "shared-task")
				}
				w.Header().Set(serverHeader, fmt.Sprintf("192.0.2.%d", 10+i))
				w.Header().Set("Content-Length", "4")
				body := bodies[i]
				if mode == "bad-second-object" && i == 1 {
					body = []byte("xxxx")
				}
				_, _ = w.Write(body)
			}))
			defer proxy.Close()
			m := contract.Manifest{SchemaVersion: 1, ObjectSize: 4, ObjectCount: uint64(len(bodies))}
			for i, body := range bodies {
				m.Objects = append(m.Objects, contract.Object{ID: uint64(i), SHA256: contract.Digest(body)})
			}
			raw, _ := json.Marshal(m)
			manifest := filepath.Join(t.TempDir(), "manifest.json")
			if err := os.WriteFile(manifest, raw, 0600); err != nil {
				t.Fatal(err)
			}
			spec := Spec{
				Manifest: manifest, SourceBaseURL: "http://objects.test", Proxy: proxy.URL,
				Concurrency: concurrency, RequestTimeoutMS: 2000,
			}
			if mode == caseTimeout {
				spec.RequestTimeoutMS = 20
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == caseCancelled {
				cancel()
			}
			result, err := Prepare(ctx, spec)
			if mode == "success" || mode == caseConcurrent || mode == caseConcurrencyAboveCount {
				if err != nil || result.ObjectCount != m.ObjectCount || result.ManifestSHA256 != contract.Digest(raw) || result.SourceBaseURL != spec.SourceBaseURL || requests.Load() != uint32(len(bodies)) {
					t.Fatalf("incomplete preparation: %+v requests=%d err=%v", result, requests.Load(), err)
				}
			} else if err == nil || result.ObjectCount != 0 {
				t.Fatalf("failed preparation reported success: %+v %v", result, err)
			}
			if mode == "zero-concurrency" && (contract.ErrorCode(err) != contract.InputInvalid || !strings.Contains(err.Error(), "prepare.concurrency") || requests.Load() != 0) {
				t.Fatalf("zero concurrency was not rejected before requests: requests=%d err=%v", requests.Load(), err)
			}
			if mode == caseDuplicateTask && (contract.ErrorCode(err) != contract.ExecutionFailed || !strings.Contains(err.Error(), "share task")) {
				t.Fatalf("shared task was not rejected: %v", err)
			}
			if (mode == "bad-second-object" || mode == caseDuplicateTask) && requests.Load() != 2 {
				t.Fatalf("preparation dispatched after failure: requests=%d err=%v", requests.Load(), err)
			}
			if mode == caseCancelled && (!errors.Is(err, context.Canceled) || requests.Load() != 0) {
				t.Fatalf("cancelled preparation sent requests: requests=%d err=%v", requests.Load(), err)
			}
			if mode == caseTimeout && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("request timeout lost: %v", err)
			}
		})
	}
}
