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

type FileSizeLevel string

func (f FileSizeLevel) String() string {
	switch f {
	case FileSizeLevelNano:
		return "Nano(1B)"
	case FileSizeLevelMicro:
		return "Micro(1KB)"
	case FileSizeLevelSmall:
		return "Small(1MB)"
	case FileSizeLevelMedium:
		return "Medium(10MB)"
	case FileSizeLevelLarge:
		return "Large(1GB)"
	case FileSizeLevelXLarge:
		return "XLarge(10GB)"
	case FileSizeLevelXXLarge:
		return "XXLarge(30GB)"
	default:
		return "Unknow"
	}
}

func (f FileSizeLevel) TaskSizeLevel() string {
	switch f {
	case FileSizeLevelNano:
		return "1"
	case FileSizeLevelMicro:
		return "1"
	case FileSizeLevelSmall:
		return "2"
	case FileSizeLevelMedium:
		return "4"
	case FileSizeLevelLarge:
		return "11"
	case FileSizeLevelXLarge:
		return "13"
	case FileSizeLevelXXLarge:
		return "14"
	default:
		return "unknown"
	}
}

const (
	FileSizeLevelNano    FileSizeLevel = "nano"
	FileSizeLevelMicro   FileSizeLevel = "micro"
	FileSizeLevelSmall   FileSizeLevel = "small"
	FileSizeLevelMedium  FileSizeLevel = "medium"
	FileSizeLevelLarge   FileSizeLevel = "large"
	FileSizeLevelXLarge  FileSizeLevel = "xlarge"
	FileSizeLevelXXLarge FileSizeLevel = "xxlarge"
)

var FileSizeLevels = []FileSizeLevel{
	FileSizeLevelNano,
	FileSizeLevelMicro,
	FileSizeLevelSmall,
	FileSizeLevelMedium,
	FileSizeLevelLarge,
	FileSizeLevelXLarge,
	FileSizeLevelXXLarge,
}

type FileServer interface {
	GetFileURL(FileSizeLevel, string) (*url.URL, error)
}

type fileServer struct {
	namespace string
}

func NewFileServer(namespace string) FileServer {
	return &fileServer{namespace}
}

func (f *fileServer) GetFileURL(fileSizeLevel FileSizeLevel, tag string) (*url.URL, error) {
	baseURL := fmt.Sprintf("http://file-server.%s.svc", f.namespace)

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, string(fileSizeLevel))

	// Add tag query parameter.
	query := u.Query()
	query.Set("tag", tag)
	query.Set("uuid", uuid.New().String())
	u.RawQuery = query.Encode()
	return u, nil
}
