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

package util

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
)

// remoteCommandWebsocketsEnv is the kubectl feature gate that streams exec over WebSockets, "false" selects SPDY.
const remoteCommandWebsocketsEnv = "KUBECTL_REMOTE_COMMAND_WEBSOCKETS"

// PodExec represents a pod exec information.
type PodExec struct {
	namespace string
	name      string
	container string
}

// NewPodExec creates a new PodExec.
func NewPodExec(namespace string, name string, container string) *PodExec {
	return &PodExec{namespace, name, container}
}

// Command returns a pod exec command.
func (p *PodExec) Command(ctx context.Context, arg ...string) *exec.Cmd {
	extArgs := []string{"-n", p.namespace, "exec", p.name, "--"}
	if p.container != "" {
		extArgs = []string{"-n", p.namespace, "exec", "-c", p.container, p.name, "--"}
	}

	extArgs = append(extArgs, arg...)
	return KubeCtlCommand(ctx, extArgs...)
}

// GetPods returns a list of pods.
func GetPods(ctx context.Context, namespace string, label string) ([]string, error) {
	return GetResourceNames(ctx, namespace, "pods", label)
}

// GetResourceNames returns the names of the resources matching the label.
func GetResourceNames(ctx context.Context, namespace string, resource string, label string) ([]string, error) {
	// Read stdout only, kubectl prints warnings to stderr.
	output, err := KubeCtlCommand(ctx, "get", resource, "-n", namespace, "-l", label, "-o", "jsonpath={.items[*].metadata.name}").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get %s: %w \nmessage: %s", resource, err, Stderr(err))
	}

	return strings.Fields(string(output)), nil
}

// getJSON runs kubectl get with the JSON output and decodes it into out.
func getJSON(ctx context.Context, out any, resource string, args ...string) error {
	// Read stdout only, kubectl prints warnings to stderr.
	output, err := KubeCtlCommand(ctx, append(append([]string{"get", resource}, args...), "-o", "json")...).Output()
	if err != nil {
		return fmt.Errorf("failed to get %s: %w \nmessage: %s", resource, err, Stderr(err))
	}

	return json.Unmarshal(output, out)
}

// KubeCtlCommand returns a kubectl command.
func KubeCtlCommand(ctx context.Context, arg ...string) *exec.Cmd {
	logrus.Debug(fmt.Sprintf(`kubectl command: "kubectl" "%s"`, strings.Join(arg, `" "`)))
	cmd := exec.CommandContext(ctx, "kubectl", arg...)
	cmd.Env = kubectlEnv(os.Environ())
	return cmd
}

// kubectlEnv streams kubectl exec over SPDY unless the environment chooses the protocol.
// kubectl streams over WebSockets since 1.31, and before 1.34 its client races on short
// commands, "Unknown stream id 1, discarding message" then "closed all streams", which
// fails some of the concurrent execs of the benchmarks, see kubernetes/kubernetes#131189.
func kubectlEnv(environ []string) []string {
	for _, kv := range environ {
		if strings.HasPrefix(kv, remoteCommandWebsocketsEnv+"=") {
			return environ
		}
	}

	return append(environ, remoteCommandWebsocketsEnv+"=false")
}

// Stderr returns the stderr of a failed command run with Output, empty otherwise.
func Stderr(err error) string {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return string(exitErr.Stderr)
	}

	return ""
}
