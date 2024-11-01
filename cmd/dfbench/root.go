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

package dfbench

import (
	"os"

	"github.com/dragonflyoss/perf-tests/pkg/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Initialize default dfbench config.
var cfg = config.New()

// rootCmd represents the benchmark command.
var rootCmd = &cobra.Command{
	Use:                "dfbench",
	Short:              "A command line tool for benchmarking Dragonfly",
	Args:               cobra.ExactArgs(1),
	DisableAutoGenTag:  true,
	SilenceUsage:       true,
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		logrus.Debug("dfbench is running")

		// Set the configured log level
		if level, err := logrus.ParseLevel(cfg.LogLevel); err == nil {
			logrus.SetLevel(level)
		}
		logrus.Debug("dfbench log initialized")

		// Set the kubeconfig if it is provided.
		if cfg.KubeConfig != "" {
			os.Setenv("KUBECONFIG", cfg.KubeConfig)
		}

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// Bind more cache specific persistent flags.
	flags := rootCmd.PersistentFlags()
	flags.StringVar(&cfg.KubeConfig, "kubeconfig", cfg.KubeConfig, "Specify the path to the kubeconfig file")
	flags.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "Specify the timeout for benchmarking")
	flags.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "Specify the log level [debug, info, warn, error, fatal, panic], default is info")

	// Bind common flags.
	if err := viper.BindPFlags(flags); err != nil {
		panic(err)
	}

	// Add sub command.
	rootCmd.AddCommand(dragonflyCmd)
	rootCmd.AddCommand(nydusCmd)
}
