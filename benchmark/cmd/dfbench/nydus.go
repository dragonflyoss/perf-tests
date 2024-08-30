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
	"context"
	"fmt"

	"github.com/dragonflyoss/perf-tests/benchmark/pkg/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// nydusCmd represents the benchmark command for nydus.
var nydusCmd = &cobra.Command{
	Use:                "nydus [flags]",
	Short:              "A command line tool for benchmarking Nydus",
	Args:               cobra.NoArgs,
	DisableAutoGenTag:  true,
	SilenceUsage:       true,
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		defer cancel()

		logrus.Infof("running nydus benchmark %d times", cfg.Nydus.Number)
		return runNydus(ctx, cfg)
	},
}

// init initializes nydus command.
func init() {
	flags := nydusCmd.Flags()
	flags.Uint32VarP(&cfg.Nydus.Number, "number", "n", cfg.Nydus.Number, "Specify the number of times to run the nydus benchmark")
	flags.StringVarP(&cfg.Nydus.Namespace, "namespace", "s", cfg.Nydus.Namespace, "Specify the namespace to use for the nydus benchmark")

	if err := viper.BindPFlags(flags); err != nil {
		panic(fmt.Errorf("bind cache nydus flags to viper: %w", err))
	}
}

// runNydus runs the nydus benchmark.
func runNydus(ctx context.Context, cfg *config.Config) error {
	// TODO: Add nydus benchmark logic here.
	return nil
}
