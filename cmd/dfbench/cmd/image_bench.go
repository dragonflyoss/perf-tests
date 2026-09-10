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
	"github.com/dragonflyoss/perf-tests/pkg/imagebench"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// imageBenchCmd represents the benchmark command for concurrent image pulls.
var imageBenchCmd = &cobra.Command{
	Use:                "image-bench [flags]",
	Short:              "A command line tool for benchmarking concurrent image pulls by Dragonfly",
	Args:               cobra.NoArgs,
	DisableAutoGenTag:  true,
	SilenceUsage:       true,
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		defer cancel()

		logrus.Debugf("running image benchmark for %s on %d peers", cfg.ImageBench.Image, cfg.ImageBench.Peers)
		return runImageBench(ctx, cfg)
	},
}

// init initializes image-bench command.
func init() {
	// Namespace, labels, containers, image and peers are shared with the cleanup command.
	persistentFlags := imageBenchCmd.PersistentFlags()
	persistentFlags.StringVar(&cfg.ImageBench.Namespace, "namespace", cfg.ImageBench.Namespace, "Specify the namespace to use for the image benchmark")
	persistentFlags.StringVar(&cfg.ImageBench.PeerLabel, "peer-label", cfg.ImageBench.PeerLabel, "Specify the label selector of the peer pods for the image benchmark, default is component=client")
	persistentFlags.StringVar(&cfg.ImageBench.SeedPeerLabel, "seed-peer-label", cfg.ImageBench.SeedPeerLabel, "Specify the label selector of the seed peer pods for the image benchmark, default is component=seed-client")
	persistentFlags.StringVar(&cfg.ImageBench.PeerContainer, "peer-container", cfg.ImageBench.PeerContainer, "Specify the dfdaemon container name of the peer pods for the image benchmark, default is client")
	persistentFlags.StringVar(&cfg.ImageBench.SeedPeerContainer, "seed-peer-container", cfg.ImageBench.SeedPeerContainer, "Specify the dfdaemon container name of the seed peer pods for the image benchmark, default is seed-client")
	persistentFlags.StringVar(&cfg.ImageBench.Image, "image", cfg.ImageBench.Image, "Specify the image to pull for the image benchmark, e.g. ghcr.io/dragonflyoss/image-bench:v1-10gb-10")
	persistentFlags.Uint32VarP(&cfg.ImageBench.Peers, "peers", "p", cfg.ImageBench.Peers, "Specify the number of peer nodes to pull on for the image benchmark, default is all peer nodes")

	flags := imageBenchCmd.Flags()
	flags.Uint32Var(&cfg.ImageBench.MetricsPort, "metrics-port", cfg.ImageBench.MetricsPort, "Specify the metrics port of the dfdaemon to collect the traffic from, default is 4002")

	cleanupFlags := imageBenchCleanupCmd.Flags()
	cleanupFlags.StringVar(&cfg.ImageBench.PeerConfigMap, "peer-configmap", cfg.ImageBench.PeerConfigMap, "Specify the dfdaemon configmap name of the peers to cleanup, default is found by the peer label")
	cleanupFlags.StringVar(&cfg.ImageBench.SeedPeerConfigMap, "seed-peer-configmap", cfg.ImageBench.SeedPeerConfigMap, "Specify the dfdaemon configmap name of the seed peers to cleanup, default is found by the seed peer label")
	cleanupFlags.StringVar(&cfg.ImageBench.CleanupImage, "cleanup-image", cfg.ImageBench.CleanupImage, "Specify the image of the cleanup pods, it must contain crictl")
	cleanupFlags.StringVar(&cfg.ImageBench.ContainerdSocket, "containerd-socket", cfg.ImageBench.ContainerdSocket, "Specify the containerd socket path on the peer nodes")

	if err := viper.BindPFlags(persistentFlags); err != nil {
		panic(fmt.Errorf("bind cache image-bench persistent flags to viper: %w", err))
	}

	if err := viper.BindPFlags(flags); err != nil {
		panic(fmt.Errorf("bind cache image-bench flags to viper: %w", err))
	}

	if err := viper.BindPFlags(cleanupFlags); err != nil {
		panic(fmt.Errorf("bind cache image-bench cleanup flags to viper: %w", err))
	}

	// Add sub command.
	imageBenchCmd.AddCommand(imageBenchCleanupCmd)
}

// runImageBench runs the image benchmark.
func runImageBench(ctx context.Context, cfg *config.Config) error {
	stats := imagebench.NewStats()
	imageBench := imagebench.New(&cfg.ImageBench, stats)

	fmt.Printf("Running image benchmark for %s by CONTAINERD ...\n", cfg.ImageBench.Image)
	if err := imageBench.Run(ctx); err != nil {
		logrus.Errorf("failed to run image benchmark: %v", err)
		return err
	}

	if err := stats.PrettyPrint(); err != nil {
		logrus.Errorf("failed to print image benchmark statistics: %v", err)
		return err
	}

	// Fail the run, and the Job in Kubernetes, when too many pulls failed.
	if !stats.GetResult().Downloads.Passed() {
		return errors.New("image benchmark failed")
	}

	return nil
}

// imageBenchCleanupCmd represents the cleanup command for the image benchmark.
var imageBenchCleanupCmd = &cobra.Command{
	Use:                "cleanup [flags]",
	Short:              "A command line tool for removing the image from the peer nodes and cleaning up the cache of Dragonfly peers and seed peers",
	Args:               cobra.NoArgs,
	DisableAutoGenTag:  true,
	SilenceUsage:       true,
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		defer cancel()

		logrus.Debugf("cleaning up image benchmark for %s in namespace %s", cfg.ImageBench.Image, cfg.ImageBench.Namespace)
		return cleanupImageBench(ctx, cfg)
	},
}

// cleanupImageBench removes the image from the peer nodes and cleans up the cache of the peers and seed peers.
func cleanupImageBench(ctx context.Context, cfg *config.Config) error {
	stats := imagebench.NewStats()
	imageBench := imagebench.New(&cfg.ImageBench, stats)

	fmt.Printf("Cleaning up %s on peers and seed peers in %s ...\n", cfg.ImageBench.Image, cfg.ImageBench.Namespace)
	if err := imageBench.Cleanup(ctx); err != nil {
		logrus.Errorf("failed to cleanup image benchmark: %v", err)
		return err
	}

	return nil
}
