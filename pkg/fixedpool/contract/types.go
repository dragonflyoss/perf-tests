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

const (
	SchemaVersion         = 1
	MaxSafeInteger uint64 = 1<<53 - 1
	MaxRegions     uint64 = 1<<32 - 1
	MaxRate        uint64 = 1<<31 - 1
)

type Object struct {
	ID     uint64 `json:"object_id"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	SchemaVersion int      `json:"schema_version"`
	ObjectSize    uint64   `json:"object_size_bytes"`
	ObjectCount   uint64   `json:"object_count"`
	Objects       []Object `json:"objects"`
}

type Shard struct {
	WorkerID    uint64 `json:"worker_id"`
	RegionCount uint64 `json:"region_count"`
	Path        string `json:"path"`
	SHA256      string `json:"sha256"`
}

type Plan struct {
	SchemaVersion    int     `json:"schema_version"`
	ManifestSHA256   string  `json:"manifest_sha256"`
	RangeBytes       uint64  `json:"range_bytes"`
	RegionsPerObject uint64  `json:"regions_per_object"`
	RegionCount      uint64  `json:"region_count"`
	WorkerCount      uint64  `json:"worker_count"`
	Shards           []Shard `json:"shards"`
}
