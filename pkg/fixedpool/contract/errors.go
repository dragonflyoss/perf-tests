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

package contract

import (
	"context"
	"errors"
)

const (
	InputInvalid    = "INPUT_INVALID"
	DataConflict    = "DATA_CONFLICT"
	ExecutionFailed = "EXECUTION_FAILED"
)

const (
	exitSuccess    = 0
	exitInputError = 2
	ExitFailure    = 3
	ExitCanceled   = 130
)

// failure preserves an error's category through wrapping.
type failure struct {
	Code  string
	Cause error
}

func (f *failure) Error() string { return f.Cause.Error() }
func (f *failure) Unwrap() error { return f.Cause }
func Tag(code string, err error) error {
	if err == nil {
		return nil
	}
	return &failure{Code: code, Cause: err}
}
func ErrorCode(err error) string {
	var tagged *failure
	if errors.As(err, &tagged) {
		return tagged.Code
	}
	return ExecutionFailed
}

// ExitCode classifies shared preparation and worker-input failures.
func ExitCode(ctx context.Context, err error) int {
	if ctx.Err() != nil {
		return ExitCanceled
	}
	if err == nil {
		return exitSuccess
	}
	switch ErrorCode(err) {
	case InputInvalid, DataConflict:
		return exitInputError
	default:
		return ExitFailure
	}
}
