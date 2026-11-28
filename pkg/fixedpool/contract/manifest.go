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

import "fmt"

func ReadManifest(name string) (Manifest, []byte, error) {
	var manifest Manifest
	if name == "" {
		return manifest, nil, Tag(InputInvalid, fmt.Errorf("manifest is required"))
	}
	data, err := ReadJSON(name, &manifest)
	if err == nil {
		err = Tag(ContractMismatch, validateManifest(manifest))
	}
	if err != nil {
		return manifest, nil, Input("MANIFEST", name, err)
	}
	return manifest, data, nil
}

func validateManifest(m Manifest) error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("manifest.schema_version: expected %d, actual %d", SchemaVersion, m.SchemaVersion)
	}
	if m.ObjectSize == 0 || m.ObjectSize > MaxSafeInteger {
		return fmt.Errorf("manifest.object_size_bytes: expected 1..%d, actual %d", MaxSafeInteger, m.ObjectSize)
	}
	if m.ObjectCount == 0 || m.ObjectCount > MaxSafeInteger || m.ObjectCount != uint64(len(m.Objects)) {
		return fmt.Errorf("manifest.object_count: expected a positive count <= %d equal to len(objects)=%d, actual %d", MaxSafeInteger, len(m.Objects), m.ObjectCount)
	}
	seen := make(map[string]bool, len(m.Objects))
	for i, object := range m.Objects {
		switch {
		case object.ID != uint64(i):
			return fmt.Errorf("manifest.objects[%d].object_id: expected %d, actual %d", i, i, object.ID)
		case !ValidDigest(object.SHA256):
			return fmt.Errorf("manifest.objects[%d].sha256: expected 64 lowercase hex digits, actual %q", i, object.SHA256)
		}
		if seen[object.SHA256] {
			return fmt.Errorf("manifest.objects[%d].sha256: duplicate %q", i, object.SHA256)
		}
		seen[object.SHA256] = true
	}
	return nil
}
