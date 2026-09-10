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

package imagebench

import "testing"

func TestParseTraffic(t *testing.T) {
	tests := []struct {
		name       string
		exposition string
		want       Traffic
	}{
		{
			name: "all traffic types",
			exposition: `# HELP dragonfly_client_download_traffic Counter of the number of the download traffic.
# TYPE dragonfly_client_download_traffic counter
dragonfly_client_download_traffic{type="BACK_TO_SOURCE"} 1073741824
dragonfly_client_download_traffic{type="REMOTE_PEER"} 2147483648
dragonfly_client_download_traffic{type="LOCAL_PEER"} 4096
# HELP dragonfly_client_upload_traffic Counter of the number of the upload traffic.
# TYPE dragonfly_client_upload_traffic counter
dragonfly_client_upload_traffic 512
`,
			want: Traffic{BackToSource: 1073741824, RemotePeer: 2147483648, LocalPeer: 4096},
		},
		{
			name: "no traffic metric",
			exposition: `# TYPE dragonfly_client_version gauge
dragonfly_client_version 1
`,
			want: Traffic{},
		},
		{
			name:       "empty exposition",
			exposition: "",
			want:       Traffic{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTraffic([]byte(tt.exposition))
			if err != nil {
				t.Fatalf("parseTraffic() error = %v", err)
			}

			if got != tt.want {
				t.Fatalf("parseTraffic() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseTrafficInvalid(t *testing.T) {
	if _, err := parseTraffic([]byte("dragonfly_client_download_traffic{type=\"BACK_TO_SOURCE\" 1\n")); err == nil {
		t.Fatal("parseTraffic() expected error for malformed exposition")
	}
}

func TestTrafficSub(t *testing.T) {
	before := Traffic{BackToSource: 100, RemotePeer: 200, LocalPeer: 300}
	after := Traffic{BackToSource: 150, RemotePeer: 50, LocalPeer: 300}

	got := after.Sub(before)
	want := Traffic{BackToSource: 50, RemotePeer: 0, LocalPeer: 0}
	if got != want {
		t.Fatalf("Sub() = %+v, want %+v", got, want)
	}
}

func TestTrafficAddTotal(t *testing.T) {
	got := Traffic{BackToSource: 1, RemotePeer: 2, LocalPeer: 3}.Add(Traffic{BackToSource: 10, RemotePeer: 20, LocalPeer: 30})
	want := Traffic{BackToSource: 11, RemotePeer: 22, LocalPeer: 33}
	if got != want {
		t.Fatalf("Add() = %+v, want %+v", got, want)
	}

	if got.Total() != 66 {
		t.Fatalf("Total() = %d, want 66", got.Total())
	}
}
