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

const (
	FileSizeLevelNano    FileSizeLevel = "nano"
	FileSizeLevelMicro   FileSizeLevel = "micro"
	FileSizeLevelSmall   FileSizeLevel = "small"
	FileSizeLevelMedium  FileSizeLevel = "medium"
	FileSizeLevelLarge   FileSizeLevel = "large"
	FileSizeLevelXLarge  FileSizeLevel = "xlarge"
	FileSizeLevelXXLarge FileSizeLevel = "xxlarge"
)

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
