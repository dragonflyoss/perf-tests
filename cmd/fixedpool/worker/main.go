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

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dragonflyoss/perf-tests/pkg/fixedpool/contract"
	"github.com/dragonflyoss/perf-tests/pkg/fixedpool/worker"
)

const (
	exitCannotExecute = 126
	exitNotFound      = 127
	exitSignalBase    = 128
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}

func finish(ctx context.Context, err error) int {
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "worker: %s: %v\n", contract.ErrorCode(err), err)
	}
	return contract.ExitCode(ctx, err)
}

func run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		return finish(ctx, contract.Tag(contract.InputInvalid, fmt.Errorf("internal: benchmark script required")))
	}
	env, err := worker.ReadEnvironment(os.Environ())
	if err != nil {
		return finish(ctx, contract.Tag(contract.InputInvalid, err))
	}
	spec, err := worker.PrepareInput(ctx, env)
	if err != nil {
		return finish(ctx, err)
	}
	child, err := newK6Command(args, spec)
	if err != nil {
		return finish(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		return finish(ctx, err)
	}
	if value := os.Getenv("RANGE"); value != "" {
		fmt.Fprintf(os.Stderr,
			"worker: warning: RANGE=%q is ignored; PLAN=%q uses range_bytes=%d. "+
				"To change request size, generate another plan using fixed-pool-prepare plan --range.\n",
			value, env.Plan, spec.RangeBytes,
		)
	}
	return runK6(ctx, child)
}

func newK6Command(args []string, spec worker.Spec) (*exec.Cmd, error) {
	raw, err := contract.Encode(spec)
	if err != nil {
		return nil, err
	}
	child := exec.Command("k6", append([]string{"run", "--no-color", args[0]}, args[1:]...)...)
	for _, pair := range os.Environ() {
		key, _, _ := strings.Cut(pair, "=")
		switch key {
		case "HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy", "WORKER_SPEC", "K6_NO_USAGE_REPORT":
		default:
			child.Env = append(child.Env, pair)
		}
	}
	child.Env = append(child.Env, "HTTP_PROXY="+spec.Proxy, "WORKER_SPEC="+string(raw), "K6_NO_USAGE_REPORT=true")
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
	return child, nil
}

func runK6(ctx context.Context, child *exec.Cmd) int {
	started := time.Now().UTC()
	code, interrupted := 0, false
	if err := child.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "worker: %s: k6 start: %v\n", contract.ErrorCode(err), err)
		code = exitCannotExecute
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			code = exitNotFound
		}
	} else {
		done := make(chan error, 1)
		go func() { done <- child.Wait() }()
		var waitErr error
		select {
		case waitErr = <-done:
		case <-ctx.Done():
			interrupted = true
			_ = child.Process.Signal(syscall.SIGTERM)
			waitErr = <-done
		}
		if waitErr != nil {
			var exited *exec.ExitError
			if errors.As(waitErr, &exited) {
				code = exited.ExitCode()
				if status, ok := exited.Sys().(syscall.WaitStatus); ok && status.Signaled() {
					code = exitSignalBase + int(status.Signal())
				}
			} else {
				code = contract.ExitFailure
				fmt.Fprintf(os.Stderr, "worker: %s: k6 wait: %v\n", contract.ErrorCode(waitErr), waitErr)
			}
		}
	}
	interrupted = interrupted || ctx.Err() != nil
	raw, err := contract.Encode(struct {
		Started     string `json:"process_started_at"`
		Ended       string `json:"process_ended_at"`
		Exit        int    `json:"exit_code"`
		Interrupted bool   `json:"interrupted"`
	}{started.Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), code, interrupted})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "worker: warning: cannot encode exit record: %v\n", err)
	} else {
		_, _ = fmt.Fprintf(os.Stdout, "PROXY_BENCH_FIXED_POOL_WORKER_EXIT_V1 %s", raw)
	}
	if interrupted {
		return contract.ExitCanceled
	}
	return code
}
