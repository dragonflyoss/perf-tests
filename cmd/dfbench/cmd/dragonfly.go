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

package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/dragonflyoss/perf-tests/pkg/backend"
	"github.com/dragonflyoss/perf-tests/pkg/config"
	"github.com/dragonflyoss/perf-tests/pkg/dragonfly"
	"github.com/dragonflyoss/perf-tests/pkg/stats"
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

		logrus.Debugf("running dragonfly benchmark %d times", cfg.Dragonfly.Number)
		return runDragonfly(ctx, cfg)
	},
}

// init initializes dragonfly command.
func init() {
	flags := dragonflyCmd.Flags()
	flags.Uint32VarP(&cfg.Dragonfly.Number, "number", "n", cfg.Dragonfly.Number, "Specify the number of times to run the dragonfly benchmark")
	flags.StringVarP(&cfg.Dragonfly.Namespace, "namespace", "s", cfg.Dragonfly.Namespace, "Specify the namespace to use for the dragonfly benchmark")
	flags.StringVarP(&cfg.Dragonfly.Downloader, "downloader", "d", cfg.Dragonfly.Downloader, "Specify the downloader to use for the dragonfly benchmark [dfget, proxy], default is dfget")
	flags.StringVar(&cfg.Dragonfly.FileSizeLevel, "file-size-level", cfg.Dragonfly.FileSizeLevel, "Specify the file size level to use for the dragonfly benchmark [nano, micro, small, medium, large, xlarge, xxlarge], default is running all levels")

	if err := viper.BindPFlags(flags); err != nil {
		panic(fmt.Errorf("bind cache dragonfly flags to viper: %w", err))
	}
}

// runDragonfly runs the dragonfly benchmark.
func runDragonfly(ctx context.Context, cfg *config.Config) error {
	stats := stats.New(cfg.Dragonfly.Namespace)
	fileServer := backend.NewFileServer(cfg.Dragonfly.Namespace)
	dragonfly := dragonfly.New(cfg.Dragonfly.Namespace, fileServer, stats)

	// If file size level is not specified, run all file size levels.
	if cfg.Dragonfly.FileSizeLevel == "" {
		fmt.Printf("Running benchmark for all size levels by %s ...\n", strings.ToUpper(cfg.Dragonfly.Downloader))
		if err := dragonfly.Run(ctx, cfg.Dragonfly.Downloader); err != nil {
			logrus.Errorf("failed to run dragonfly benchmark: %v", err)
			return err
		}

		if err := stats.PrettyPrint(); err != nil {
			logrus.Errorf("failed to print dragonfly benchmark statistics: %v", err)
			return err
		}

		if err := dragonfly.Cleanup(ctx); err != nil {
			logrus.Errorf("failed to cleanup dragonfly benchmark: %v", err)
			return err
		}
		return nil
	}

	// Run the benchmark for the specified file size level.
	fmt.Printf("Running benchmark for %s size level by %s ...\n", strings.ToUpper(cfg.Dragonfly.FileSizeLevel), strings.ToUpper(cfg.Dragonfly.Downloader))
	if err := dragonfly.RunByFileSizes(ctx, cfg.Dragonfly.Downloader, backend.FileSizeLevel(cfg.Dragonfly.FileSizeLevel)); err != nil {
		logrus.Errorf("failed to run dragonfly benchmark: %v", err)
		return err
	}

	if err := stats.PrettyPrint(); err != nil {
		logrus.Errorf("failed to print dragonfly benchmark statistics: %v", err)
		return err
	}

	if err := dragonfly.Cleanup(ctx); err != nil {
		logrus.Errorf("failed to cleanup dragonfly benchmark: %v", err)
		return err
	}
	return nil
}
