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
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func Digest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func ValidDigest(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil && len(value) == 64 && value == strings.ToLower(value)
}

// Encode uses compact JSON and one final LF for worker records and descriptors.
func Encode(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// ReadJSON returns the original file bytes after strict decoding.
func ReadJSON(name string, value any) ([]byte, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, Tag(InputInvalid, err)
	}
	if err := decode(data, value); err != nil {
		return nil, Tag(ContractMismatch, err)
	}
	return data, nil
}

// decode uses typed standard-library decoding. The token pass protects against
// duplicate keys and case aliases; value checks belong to each artifact consumer.
func decode(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := checkValue(decoder, "$"); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("expected end of JSON, got %v", err)
	}
	return nil
}

func checkValue(decoder *json.Decoder, location string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token == nil && strings.HasSuffix(location, ".object_id") {
		return fmt.Errorf("%s: null is not allowed", location)
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("%s: duplicate or invalid JSON key %q", location, key)
			}
			// Tool-owned JSON fields are lowercase ASCII. encoding/json folds
			// case (including Unicode aliases), while JavaScript reads exact keys.
			if name != strings.ToLower(name) || strings.IndexFunc(name, func(c rune) bool { return c > 127 }) >= 0 {
				return fmt.Errorf("%s: noncanonical JSON field %q", location, name)
			}
			seen[name] = true
			if err := checkValue(decoder, location+"."+name); err != nil {
				return err
			}
		}
		// Zero is a valid execution identity, so ordinary value validation cannot
		// distinguish its presence. Other required values are checked by consumers.
		field := ""
		if strings.HasPrefix(location, "$.objects[") {
			field = "object_id"
		}
		if field != "" && !seen[field] {
			return fmt.Errorf("%s.%s: required field missing", location, field)
		}
	case '[':
		for index := 0; decoder.More(); index++ {
			if err := checkValue(decoder, fmt.Sprintf("%s[%d]", location, index)); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	_, err = decoder.Token()
	return err
}
