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
	return &PodExec{
		namespace: namespace,
		name:      name,
		container: container,
	}
}

// Command returns a pod exec command.
func (p *PodExec) Command(arg ...string) *exec.Cmd {
	extArgs := []string{"-n", p.namespace, "exec", p.name, "--"}
	if p.container != "" {
		extArgs = []string{"-n", p.namespace, "exec", "-c", p.container, p.name, "--"}
	}

	extArgs = append(extArgs, arg...)
	return KubeCtlCommand(extArgs...)
}

// KubeCtlCommand returns a kubectl command.
func KubeCtlCommand(arg ...string) *exec.Cmd {
	logrus.Debug(fmt.Sprintf(`kubectl command: "kubectl" "%s"`, strings.Join(arg, `" "`)))
	return exec.Command("kubectl", arg...)
}
