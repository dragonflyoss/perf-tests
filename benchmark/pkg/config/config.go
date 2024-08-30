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

package config

import (
	"errors"
	"time"
)

const (
	// DownloaderDfget is the dfget downloader.
	DownloaderDfget = "dfget"

	// DownloaderProxy is the proxy downloader.
	DownloaderProxy = "proxy"
)

// Config is the root configuration for dfbench.
type Config struct {
	// KubeConfig is the path to the kubeconfig file.
	KubeConfig string `yaml:"kubeconfig,omitempty" mapstructure:"kubeconfig,omitempty"`

	// Timeout specifies the timeout for benchmarking
	Timeout time.Duration `yaml:"timeout,omitempty" mapstructure:"timeout,omitempty"`

	// LogLevel is the level with to log for this config
	LogLevel string `yaml:"log_level,omitempty" mapstructure:"log_level,omitempty"`

	// Dragonfly is the configuration for benchmarking dragonfly.
	Dragonfly DragonflyConfig `yaml:"dragonfly,omitempty" mapstructure:"dragonfly,omitempty"`

	// Nydus is the configuration for benchmarking nydus.
	Nydus NydusConfig `yaml:"nydus,omitempty" mapstructure:"nydus,omitempty"`
}

// DragonflyConfig is the configuration for benchmarking dragonfly.
type DragonflyConfig struct {
	// Namespace is the namespace to use for the benchmark.
	Namespace string `yaml:"namespace,omitempty" mapstructure:"namespace,omitempty"`

	// Number is the number of times to run the benchmark.
	Number uint32 `yaml:"number,omitempty" mapstructure:"number,omitempty"`

	// Downloader is the downloader to use for the benchmark [dfget, proxy], default is dfget.
	Downloader string `yaml:"downloader,omitempty" mapstructure:"downloader,omitempty"`

	// FileSizeLevel is the file size level to use for the benchmark [nano, micro, small, medium, large, xlarge, xxlarge], default is "" to run all levels.
	FileSizeLevel string `yaml:"file_size_level,omitempty" mapstructure:"file_size_level,omitempty"`
}

// NydusConfig is the configuration for benchmarking nydus.
type NydusConfig struct {
	// Namespace is the namespace to use for the benchmark.
	Namespace string `yaml:"namespace,omitempty" mapstructure:"namespace,omitempty"`

	// Number is the number of times to run the benchmark.
	Number uint32 `yaml:"number,omitempty" mapstructure:"number,omitempty"`
}

// New bench configuration.
func New() *Config {
	return &Config{
		KubeConfig: "",
		Timeout:    30 * time.Minute,
		LogLevel:   "info",
		Dragonfly: DragonflyConfig{
			Number:        1,
			Namespace:     "dragonfly-system",
			Downloader:    DownloaderDfget,
			FileSizeLevel: "",
		},
		Nydus: NydusConfig{
			Number:    1,
			Namespace: "nydus-snapshotter",
		},
	}
}

// Validate the configuration.
func (c *Config) Validate() error {
	if c.Timeout <= 1*time.Minute {
		return errors.New("timeout must be greater than 1 minute")
	}

	return nil
}
