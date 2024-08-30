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
	"fmt"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
)

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
	cmd := KubeCtlCommand(ctx, "get", "pods", "-n", namespace, "-l", label, "-o", "jsonpath={.items[*].metadata.name}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get pods: %w", err)
	}

	return strings.Fields(string(output)), nil
}

// KubeCtlCommand returns a kubectl command.
func KubeCtlCommand(ctx context.Context, arg ...string) *exec.Cmd {
	logrus.Debug(fmt.Sprintf(`kubectl command: "kubectl" "%s"`, strings.Join(arg, `" "`)))
	return exec.CommandContext(ctx, "kubectl", arg...)
}
