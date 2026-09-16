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

package util

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// dfgetScript downloads $2 to $3 in the directory $1 by dfget and prints the start and end
// time in nanoseconds, so the cost is measured in the pod without the kubectl exec overhead.
// The dfget output goes to stderr, it is read when the download fails.
const dfgetScript = `mkdir -p "$1" || exit $?
start=$(date +%s%N)
dfget "$2" --output "$3" 1>&2
rc=$?
end=$(date +%s%N)
echo "$start $end"
exit $rc`

// DownloadByDfget downloads the URL on the peer by dfget into a unique file under outputDir,
// named after file, and removes it afterwards.
func DownloadByDfget(ctx context.Context, namespace string, p Peer, downloadURL *url.URL, outputDir string, file string) *Download {
	podExec := NewPodExec(namespace, p.Pod, p.Container)
	outputPath := path.Join(outputDir, fmt.Sprintf("%s-dfget-%s", path.Base(file), uuid.New().String()))

	start := time.Now()
	// Read stdout only, kubectl prints warnings to stderr.
	output, err := podExec.Command(ctx, "sh", "-c", dfgetScript, "sh", outputDir, downloadURL.String(), outputPath).Output()
	cost := time.Since(start)

	if rmOutput, rmErr := podExec.Command(ctx, "sh", "-c", fmt.Sprintf("rm -f %s", outputPath)).CombinedOutput(); rmErr != nil {
		logrus.Errorf("failed to cleanup: %v \nmessage: %s", rmErr, string(rmOutput))
	}

	if err != nil {
		logrus.Errorf("failed to download file on %s: %v \nmessage: %s", p.Pod, err, Stderr(err))
		return &Download{Peer: p.Pod, Cost: cost, Err: err}
	}

	// Prefer the cost measured in the pod, fall back to the wall-clock cost with the kubectl exec overhead.
	if podCost, err := parseCost(output); err != nil {
		logrus.Warnf("failed to parse the cost on %s, using the wall-clock cost: %v", p.Pod, err)
	} else {
		cost = podCost
	}

	return &Download{Peer: p.Pod, Cost: cost}
}

// parseCost parses the start and end time in nanoseconds printed by dfgetScript into the cost.
func parseCost(output []byte) (time.Duration, error) {
	fields := strings.Fields(string(output))
	if len(fields) != 2 {
		return 0, fmt.Errorf("expected the start and end time, got %q", string(output))
	}

	start, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, err
	}

	end, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0, err
	}

	if end < start {
		return 0, fmt.Errorf("end time %d is before start time %d", end, start)
	}

	return time.Duration(end - start), nil
}
