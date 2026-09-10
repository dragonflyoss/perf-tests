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

package cmd

import (
	"context"
	"fmt"

	"github.com/dragonflyoss/perf-tests/pkg/backend"
	"github.com/dragonflyoss/perf-tests/pkg/config"
	"github.com/dragonflyoss/perf-tests/pkg/filebench"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// fileBenchCmd represents the benchmark command for concurrent file downloads.
var fileBenchCmd = &cobra.Command{
	Use:                "file-bench [flags]",
	Short:              "A command line tool for benchmarking concurrent file downloads by Dragonfly",
	Args:               cobra.NoArgs,
	DisableAutoGenTag:  true,
	SilenceUsage:       true,
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		defer cancel()

		logrus.Debugf("running file benchmark for %s on %d peers", cfg.FileBench.File, cfg.FileBench.Peers)
		return runFileBench(ctx, cfg)
	},
}

// fileBenchCleanupCmd represents the cleanup command for the file benchmark.
var fileBenchCleanupCmd = &cobra.Command{
	Use:                "cleanup [flags]",
	Short:              "A command line tool for cleaning up the cache of Dragonfly peers and seed peers",
	Args:               cobra.NoArgs,
	DisableAutoGenTag:  true,
	SilenceUsage:       true,
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		defer cancel()

		logrus.Debugf("cleaning up file benchmark in namespace %s", cfg.FileBench.Namespace)
		return cleanupFileBench(ctx, cfg)
	},
}

// init initializes file-bench command.
func init() {
	// Namespace is shared with the cleanup command.
	persistentFlags := fileBenchCmd.PersistentFlags()
	persistentFlags.StringVar(&cfg.FileBench.Namespace, "namespace", cfg.FileBench.Namespace, "Specify the namespace to use for the file benchmark")

	flags := fileBenchCmd.Flags()
	flags.Uint32VarP(&cfg.FileBench.Peers, "peers", "p", cfg.FileBench.Peers, "Specify the number of peers to download on for the file benchmark, default is all peers")
	flags.StringVar(&cfg.FileBench.File, "file", cfg.FileBench.File, "Specify the file to download for the file benchmark [1g, 2g, 4g, 10g, 20g], default is 1g")

	if err := viper.BindPFlags(persistentFlags); err != nil {
		panic(fmt.Errorf("bind cache file-bench persistent flags to viper: %w", err))
	}

	if err := viper.BindPFlags(flags); err != nil {
		panic(fmt.Errorf("bind cache file-bench flags to viper: %w", err))
	}

	// Add sub command.
	fileBenchCmd.AddCommand(fileBenchCleanupCmd)
}

// runFileBench runs the file benchmark.
func runFileBench(ctx context.Context, cfg *config.Config) error {
	stats := filebench.NewStats()
	fileServer := backend.NewFileServer(cfg.FileBench.Namespace)
	fileBench := filebench.New(&cfg.FileBench, fileServer, stats)

	fmt.Printf("Running file benchmark for %s by DFGET ...\n", cfg.FileBench.File)
	if err := fileBench.Run(ctx); err != nil {
		logrus.Errorf("failed to run file benchmark: %v", err)
		return err
	}

	if err := stats.PrettyPrint(); err != nil {
		logrus.Errorf("failed to print file benchmark statistics: %v", err)
		return err
	}

	return nil
}

// cleanupFileBench cleans up the cache of the peers and seed peers.
func cleanupFileBench(ctx context.Context, cfg *config.Config) error {
	stats := filebench.NewStats()
	fileServer := backend.NewFileServer(cfg.FileBench.Namespace)
	fileBench := filebench.New(&cfg.FileBench, fileServer, stats)

	fmt.Printf("Cleaning up peers and seed peers in %s ...\n", cfg.FileBench.Namespace)
	if err := fileBench.Cleanup(ctx); err != nil {
		logrus.Errorf("failed to cleanup file benchmark: %v", err)
		return err
	}

	return nil
}
