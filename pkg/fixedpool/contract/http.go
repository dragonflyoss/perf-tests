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
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func ParseProxyURL(value string) (*url.URL, error) {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "http" || u.Hostname() == "" || u.User != nil ||
		u.Path != "" || u.RawQuery != "" || u.ForceQuery || strings.Contains(value, "#") {
		return nil, fmt.Errorf("expected an HTTP proxy origin without credentials or path/query/fragment; actual %q", value)
	}
	if u.Port() != "" {
		if n, err := strconv.ParseUint(u.Port(), 10, 16); err != nil || n == 0 {
			return nil, fmt.Errorf("invalid proxy port in %q", value)
		}
	}
	return u, nil
}

func NormalizeBaseURL(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("base_url: %w", err)
	}
	if u.Scheme != "http" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(value, "#") {
		return "", fmt.Errorf("base_url must be HTTP without credentials, query or fragment")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("base_url must have a host")
	}
	port := u.Port()
	if port != "" {
		n, err := strconv.ParseUint(port, 10, 16)
		if err != nil || n == 0 {
			return "", fmt.Errorf("base_url has invalid port")
		}
		port = strconv.FormatUint(n, 10)
		if port == "80" {
			port = ""
		}
	}
	if port != "" {
		u.Host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		u.Host = "[" + host + "]"
	} else {
		u.Host = host
	}
	// Preserve escaped separators and repeated slashes: origin servers may give
	// them different meanings. Only the literal trailing join separator is removed.
	u.RawPath = strings.TrimRight(u.EscapedPath(), "/")
	u.Path, err = url.PathUnescape(u.RawPath)
	if err != nil {
		return "", fmt.Errorf("base_url path: %w", err)
	}
	return u.String(), nil
}

// FixedHeaders keeps preparation and load requests on the same identity rules.
func FixedHeaders() http.Header {
	return http.Header{
		"X-Dragonfly-Use-P2p":                          []string{"true"},
		"X-Dragonfly-Enable-Task-Id-Based-Blob-Digest": []string{"false"},
		"X-Dragonfly-Prefetch":                         []string{"false"},
		"Accept-Encoding":                              []string{"identity"},
	}
}
