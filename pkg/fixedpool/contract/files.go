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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
)

// PrepareOutput requires an absent output or an empty directory skeleton.
// Callers guarantee one writer; individual files must still be created without replacement.
func PrepareOutput(ctx context.Context, directory string, subdirectories []string) (string, error) {
	if directory == "" {
		return "", Tag(InputInvalid, fmt.Errorf("output directory is required"))
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	directories := append([]string{""}, subdirectories...)
	if err := prepareSkeleton(ctx, directory, directories); err != nil {
		return "", err
	}
	return directory, nil
}

func prepareSkeleton(ctx context.Context, directory string, directories []string) error {
	for _, name := range directories {
		path := filepath.Join(directory, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return OutputError(path, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("output %q must be a real directory", path)
		}
		f, err := os.Open(path)
		if err != nil {
			return OutputError(path, err)
		}
		for {
			entries, readErr := f.Readdirnames(1)
			if len(entries) != 0 && !slices.Contains(directories, filepath.Join(name, entries[0])) {
				err = OutputError(filepath.Join(path, entries[0]), os.ErrExist)
				break
			}
			if readErr != nil {
				if readErr != io.EOF {
					err = OutputError(path, readErr)
				}
				break
			}
		}
		if err := errors.Join(err, f.Close()); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, name := range directories {
		path := filepath.Join(directory, name)
		if err := os.MkdirAll(path, 0755); err != nil {
			return OutputError(path, err)
		}
	}
	return nil
}

func OutputError(name string, err error) error {
	if err == nil {
		return nil
	}
	if os.IsExist(err) {
		return Tag(DataConflict, fmt.Errorf("output %q already exists (earlier output, incomplete work, or unsupported concurrent writer); use one preparer per output location; reuse a successful artifact downstream, explicitly clean conflicting items, or choose another location: %w", name, err))
	}
	return fmt.Errorf("output %q: %w", name, err)
}

// PublishJSON writes compact JSON with one final LF and publishes it without replacement.
func PublishJSON(ctx context.Context, name string, value any) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	data, err := Encode(value)
	if err != nil {
		return "", err
	}
	temporary := filepath.Join(filepath.Dir(name), "."+filepath.Base(name)+".tmp")
	if err := writeFile(temporary, data); err != nil {
		return "", err
	}
	if err := Publish(ctx, temporary, name); err != nil {
		return "", err
	}
	return Digest(data), nil
}

// Publish links a complete, synced temporary without replacing an existing result.
func Publish(ctx context.Context, temporary, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Link(temporary, destination); err != nil {
		return OutputError(destination, err)
	}
	if err := SyncDir(filepath.Dir(destination)); err != nil {
		return OutputError(destination, errors.Join(err, os.Remove(destination)))
	}
	if err := os.Remove(temporary); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "publish: warning: published %q; temporary cleanup %q failed: %v\n", destination, temporary, err)
	}
	return nil
}

func SyncDir(directory string) error {
	f, err := os.Open(directory)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

// writeFile retains a failed partial file so the next invocation cannot silently
// adopt it. Callers publish only after this function and all validation succeed.
func writeFile(name string, data []byte) (err error) {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0444)
	if err != nil {
		return OutputError(name, err)
	}
	defer func() { err = OutputError(name, errors.Join(err, f.Close())) }()
	if _, err = f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}
