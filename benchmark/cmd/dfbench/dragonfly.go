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

// dragonflyCmd represents the benchmark command for dragonfly.
var dragonflyCmd = &cobra.Command{
	Use:                "dragonfly [flags]",
	Short:              "A command line tool for benchmarking Dragonfly",
	Args:               cobra.NoArgs,
	DisableAutoGenTag:  true,
	SilenceUsage:       true,
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		defer cancel()

		logrus.Infof("running dragonfly benchmark %d times", cfg.Dragonfly.Number)
		return runDragonfly(ctx, cfg)
	},
}

// init initializes dragonfly command.
func init() {
	flags := dragonflyCmd.Flags()
	flags.Uint32VarP(&cfg.Dragonfly.Number, "number", "n", cfg.Dragonfly.Number, "Specify the number of times to run the dragonfly benchmark")

	if err := viper.BindPFlags(flags); err != nil {
		panic(fmt.Errorf("bind cache dragonfly flags to viper: %w", err))
	}
}

// runDragonfly runs the dragonfly benchmark.
func runDragonfly(ctx context.Context, cfg *config.Config) error {
	// TODO: Add dragonfly benchmark logic here.
	return nil
}
