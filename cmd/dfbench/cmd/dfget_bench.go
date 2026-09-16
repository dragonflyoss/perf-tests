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
	"errors"
	"fmt"

	"github.com/dragonflyoss/perf-tests/pkg/config"
	"github.com/dragonflyoss/perf-tests/pkg/dfgetbench"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// dfgetBenchCmd represents the benchmark command for concurrent dfget downloads on one peer.
var dfgetBenchCmd = &cobra.Command{
	Use:                "dfget-bench [flags]",
	Short:              "A command line tool for benchmarking concurrent dfget downloads from one dfdaemon",
	Args:               cobra.NoArgs,
	DisableAutoGenTag:  true,
	SilenceUsage:       true,
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		defer cancel()

		logrus.Debugf("running dfget benchmark for %s by %d dfget", cfg.DfgetBench.File, cfg.DfgetBench.Concurrency)
		return runDfgetBench(ctx, cfg)
	},
}

// init initializes dfget-bench command.
func init() {
	// Namespace and labels are shared with the cleanup command.
	persistentFlags := dfgetBenchCmd.PersistentFlags()
	persistentFlags.StringVar(&cfg.DfgetBench.Namespace, "namespace", cfg.DfgetBench.Namespace, "Specify the namespace to use for the dfget benchmark")
	persistentFlags.StringVar(&cfg.DfgetBench.PeerLabel, "peer-label", cfg.DfgetBench.PeerLabel, "Specify the label selector of the peer pods for the dfget benchmark, the first pod by name is downloaded on, default is component=client")
	persistentFlags.StringVar(&cfg.DfgetBench.SeedPeerLabel, "seed-peer-label", cfg.DfgetBench.SeedPeerLabel, "Specify the label selector of the seed peer pods to cleanup, default is component=seed-client")

	flags := dfgetBenchCmd.Flags()
	flags.StringVar(&cfg.DfgetBench.PeerContainer, "peer-container", cfg.DfgetBench.PeerContainer, "Specify the dfdaemon container name of the peer pod for the dfget benchmark, default is client")
	flags.StringVar(&cfg.DfgetBench.Pod, "pod", cfg.DfgetBench.Pod, "Specify the peer pod to download on for the dfget benchmark, default is the first pod found by the peer label")
	flags.Uint32VarP(&cfg.DfgetBench.Concurrency, "concurrency", "c", cfg.DfgetBench.Concurrency, "Specify the number of dfget to start at once for the dfget benchmark, default is 10")
	flags.StringVar(&cfg.DfgetBench.Mode, "mode", cfg.DfgetBench.Mode, "Specify the mode for the dfget benchmark [repeat, random, fixed], repeat shares one task between the dfget, random gives each its own, fixed downloads the same tasks on every run, default is repeat")
	flags.StringVar(&cfg.DfgetBench.File, "file", cfg.DfgetBench.File, "Specify the file to download for the dfget benchmark [1b, 1k, 1m, 4m, 10m, 1g, 2g, 4g, 10g, 20g, 30g], default is 1g")
	flags.StringVar(&cfg.DfgetBench.FileServer, "file-server", cfg.DfgetBench.FileServer, "Specify the base URL of the file server for the dfget benchmark, default is http://file-server.<namespace>.svc")
	flags.StringVar(&cfg.DfgetBench.OutputDir, "output-dir", cfg.DfgetBench.OutputDir, "Specify the directory in the peer pod to write the downloaded files to, dfget hard links the file when it is on the dfdaemon storage filesystem, default is /tmp")
	flags.Uint32Var(&cfg.DfgetBench.MetricsPort, "metrics-port", cfg.DfgetBench.MetricsPort, "Specify the metrics port of the dfdaemon to collect the traffic from, default is 4002")

	cleanupFlags := dfgetBenchCleanupCmd.Flags()
	cleanupFlags.StringVar(&cfg.DfgetBench.PeerConfigMap, "peer-configmap", cfg.DfgetBench.PeerConfigMap, "Specify the dfdaemon configmap name of the peers to cleanup, default is found by the peer label")
	cleanupFlags.StringVar(&cfg.DfgetBench.SeedPeerConfigMap, "seed-peer-configmap", cfg.DfgetBench.SeedPeerConfigMap, "Specify the dfdaemon configmap name of the seed peers to cleanup, default is found by the seed peer label")

	if err := viper.BindPFlags(persistentFlags); err != nil {
		panic(fmt.Errorf("bind cache dfget-bench persistent flags to viper: %w", err))
	}

	if err := viper.BindPFlags(flags); err != nil {
		panic(fmt.Errorf("bind cache dfget-bench flags to viper: %w", err))
	}

	if err := viper.BindPFlags(cleanupFlags); err != nil {
		panic(fmt.Errorf("bind cache dfget-bench cleanup flags to viper: %w", err))
	}

	// Add sub command.
	dfgetBenchCmd.AddCommand(dfgetBenchCleanupCmd)
}

// runDfgetBench runs the dfget benchmark.
func runDfgetBench(ctx context.Context, cfg *config.Config) error {
	stats := dfgetbench.NewStats()
	fileServer := newFileServer(cfg.DfgetBench.FileServer, cfg.DfgetBench.Namespace)
	dfgetBench := dfgetbench.New(&cfg.DfgetBench, fileServer, stats)

	fmt.Printf("Running dfget benchmark for %s by %d DFGET ...\n", cfg.DfgetBench.File, cfg.DfgetBench.Concurrency)
	if err := dfgetBench.Run(ctx); err != nil {
		logrus.Errorf("failed to run dfget benchmark: %v", err)
		return err
	}

	if err := stats.PrettyPrint(); err != nil {
		logrus.Errorf("failed to print dfget benchmark statistics: %v", err)
		return err
	}

	// Fail the run, and the Job in Kubernetes, when too many downloads failed.
	if !stats.GetResult().Downloads.Passed() {
		return errors.New("dfget benchmark failed")
	}

	return nil
}

// dfgetBenchCleanupCmd represents the cleanup command for the dfget benchmark.
var dfgetBenchCleanupCmd = &cobra.Command{
	Use:                "cleanup [flags]",
	Short:              "A command line tool for cleaning up the cache of Dragonfly peers and seed peers",
	Args:               cobra.NoArgs,
	DisableAutoGenTag:  true,
	SilenceUsage:       true,
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		defer cancel()

		logrus.Debugf("cleaning up dfget benchmark in namespace %s", cfg.DfgetBench.Namespace)
		return cleanupDfgetBench(ctx, cfg)
	},
}

// cleanupDfgetBench cleans up the cache of the peers and seed peers.
func cleanupDfgetBench(ctx context.Context, cfg *config.Config) error {
	stats := dfgetbench.NewStats()
	fileServer := newFileServer(cfg.DfgetBench.FileServer, cfg.DfgetBench.Namespace)
	dfgetBench := dfgetbench.New(&cfg.DfgetBench, fileServer, stats)

	fmt.Printf("Cleaning up peers and seed peers in %s ...\n", cfg.DfgetBench.Namespace)
	if err := dfgetBench.Cleanup(ctx); err != nil {
		logrus.Errorf("failed to cleanup dfget benchmark: %v", err)
		return err
	}

	return nil
}
