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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/dragonflyoss/perf-tests/pkg/fixedpool/contract"
)

const taskHeader = "X-Dragonfly-Task-ID"
const serverHeader = "X-Dragonfly-Server-IP"

const Unsupported = "TARGET_UNSUPPORTED"

func newClient(proxyURL string) (*http.Client, error) {
	proxy, err := contract.ParseProxyURL(proxyURL)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxy)
	transport.DisableCompression = true
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return client, nil
}

func responseIdentity(resp *http.Response) (string, error) {
	task, server := resp.Header.Get(taskHeader), resp.Header.Get(serverHeader)
	if len(resp.Header.Values(taskHeader)) != 1 || task == "" || len(resp.Header.Values(serverHeader)) != 1 || net.ParseIP(server) == nil {
		return task, contract.Tag(Unsupported, fmt.Errorf("HTTP %d: missing/ambiguous Dragonfly task or server IP headers", resp.StatusCode))
	}
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return task, fmt.Errorf("HTTP %d: unexpected Content-Encoding %q", resp.StatusCode, encoding)
	}
	return task, nil
}

func fullRead(ctx context.Context, client *http.Client, objectURL string, o contract.Object, size uint64, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, objectURL, nil)
	if err != nil {
		return "", fmt.Errorf("full GET: %w", err)
	}
	req.Header = contract.FixedHeaders()
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("full GET: %w", err)
	}
	task, err := readFullResponse(ctx, resp, o, size)
	return task, errors.Join(err, resp.Body.Close())
}

func readFullResponse(ctx context.Context, resp *http.Response, o contract.Object, size uint64) (string, error) {
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("full GET requires HTTP 200, got %d", resp.StatusCode)
	}
	task, err := responseIdentity(resp)
	if err != nil {
		return task, err
	}
	hash := sha256.New()
	n, err := io.CopyBuffer(hash, io.LimitReader(resp.Body, int64(size)+1), make([]byte, 64*1024))
	if err != nil {
		return task, fmt.Errorf("HTTP %d full GET after %d bytes: %w", resp.StatusCode, n, err)
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if uint64(n) != size || digest != o.SHA256 {
		return task, fmt.Errorf("HTTP %d full GET body mismatch: received %d, expected %d bytes; SHA-256 expected %s, actual %s", resp.StatusCode, n, size, o.SHA256, digest)
	}
	return task, ctx.Err()
}
