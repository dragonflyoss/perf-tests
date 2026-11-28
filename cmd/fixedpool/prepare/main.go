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
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/dragonflyoss/perf-tests/pkg/fixedpool/contract"
	"github.com/dragonflyoss/perf-tests/pkg/fixedpool/dataset"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		_, _ = fmt.Fprintln(os.Stderr, "Usage: fixed-pool-prepare data [options]; each action runs independently. Use <action> --help for options.")
		return 0
	}
	var err error
	switch args[0] {
	case "data":
		err = runData(ctx, args[1:])
	default:
		err := contract.Tag(contract.InputInvalid, fmt.Errorf("unknown preparation action %q; use data", args[0]))
		return finish(ctx, "prepare", err)
	}
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	return finish(ctx, args[0], err)
}

func finish(ctx context.Context, action string, err error) int {
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s: %s: %v\n", action, contract.ErrorCode(err), err)
	}
	return contract.ExitCode(ctx, err)
}

func parseArgs(flags *flag.FlagSet, args []string) error {
	// The CLI prints failures once; retain the standard help output on request.
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			flags.SetOutput(os.Stderr)
			flags.Usage()
		}
		return contract.Tag(contract.InputInvalid, err)
	}
	if flags.NArg() != 0 {
		return contract.Tag(contract.InputInvalid, fmt.Errorf("unexpected arguments: %q", flags.Args()))
	}
	return nil
}

func runData(ctx context.Context, args []string) error {
	started := time.Now()
	flags := flag.NewFlagSet("fixed-pool-prepare data", flag.ContinueOnError)
	var spec dataset.Spec
	flags.StringVar(&spec.Root, "root", "", "required: source directory; one writer, existing owned outputs are not replaced")
	decimalFlag(flags, &spec.Size, "size", 0, "required: positive decimal object size in bytes")
	decimalFlag(flags, &spec.Count, "count", 0, "required: positive decimal object count")
	decimalFlag(flags, &spec.GenerationWorkers, "generation-workers", 1, "optional: positive generation concurrency (default 1)")
	if err := parseArgs(flags, args); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(os.Stdout, "Preparing a new fixed dataset.")
	result, err := dataset.Prepare(ctx, spec)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(os.Stdout, "Data generated: B=%d N=%d manifest=%q sha256=%s elapsed_ms=%d\n", spec.Size, spec.Count, result.ManifestPath, result.ManifestSHA256, time.Since(started).Milliseconds())
	return nil
}

func decimalFlag(flags *flag.FlagSet, value *uint64, name string, initial uint64, description string) {
	*value = initial
	flags.Func(name, description, func(text string) error {
		n, err := strconv.ParseUint(text, 10, 64)
		if err != nil {
			return err
		}
		*value = n
		return nil
	})
}
