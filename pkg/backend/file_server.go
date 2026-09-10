/*
 *     Copyright 2024 The Dragonfly Authors
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

package backend

import (
	"fmt"
	"net/url"
	"path"

	"github.com/google/uuid"
)

// FileSizeLevel is the file served by the file server, named by size.
type FileSizeLevel string

func (f FileSizeLevel) String() string {
	switch f {
	case FileSizeLevel1B:
		return "1B"
	case FileSizeLevel1K:
		return "1KiB"
	case FileSizeLevel1M:
		return "1MiB"
	case FileSizeLevel10M:
		return "10MiB"
	case FileSizeLevel1G:
		return "1GiB"
	case FileSizeLevel10G:
		return "10GiB"
	case FileSizeLevel30G:
		return "30GiB"
	default:
		return "Unknow"
	}
}

func (f FileSizeLevel) TaskSizeLevel() string {
	switch f {
	case FileSizeLevel1B:
		return "1"
	case FileSizeLevel1K:
		return "1"
	case FileSizeLevel1M:
		return "2"
	case FileSizeLevel10M:
		return "4"
	case FileSizeLevel1G:
		return "11"
	case FileSizeLevel10G:
		return "13"
	case FileSizeLevel30G:
		return "14"
	default:
		return "unknown"
	}
}

const (
	FileSizeLevel1B  FileSizeLevel = "1b"
	FileSizeLevel1K  FileSizeLevel = "1k"
	FileSizeLevel1M  FileSizeLevel = "1m"
	FileSizeLevel10M FileSizeLevel = "10m"
	FileSizeLevel1G  FileSizeLevel = "1g"
	FileSizeLevel10G FileSizeLevel = "10g"
	FileSizeLevel30G FileSizeLevel = "30g"
)

var FileSizeLevels = []FileSizeLevel{
	FileSizeLevel1B,
	FileSizeLevel1K,
	FileSizeLevel1M,
	FileSizeLevel10M,
	FileSizeLevel1G,
	FileSizeLevel10G,
	FileSizeLevel30G,
}

type FileServer interface {
	// GetFileURL returns the URL of the file by size level.
	GetFileURL(FileSizeLevel, string) (*url.URL, error)

	// GetURL returns the URL of the file by path.
	GetURL(string, string) (*url.URL, error)
}

type fileServer struct {
	namespace string
}

func NewFileServer(namespace string) FileServer {
	return &fileServer{namespace}
}

func (f *fileServer) GetFileURL(fileSizeLevel FileSizeLevel, tag string) (*url.URL, error) {
	return f.GetURL(string(fileSizeLevel), tag)
}

func (f *fileServer) GetURL(filePath string, tag string) (*url.URL, error) {
	baseURL := fmt.Sprintf("http://file-server.%s.svc", f.namespace)

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, filePath)

	// Add tag query parameter.
	query := u.Query()
	query.Set("tag", tag)
	query.Set("uuid", uuid.New().String())
	u.RawQuery = query.Encode()
	return u, nil
}
