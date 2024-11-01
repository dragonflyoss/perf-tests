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

package dragonfly

import (
	"context"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/dragonflyoss/perf-tests/pkg/backend"
	"github.com/dragonflyoss/perf-tests/pkg/config"
	"github.com/dragonflyoss/perf-tests/pkg/stats"
	"github.com/dragonflyoss/perf-tests/pkg/util"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	OutputDir = "/tmp"
)

// Dragonfly represents a benchmark runner for Dragonfly.
type Dragonfly interface {
	// Run runs all benchmarks.
	Run(context.Context, string) error

	// RunByFileSizes runs benchmarks by file sizes.
	RunByFileSizes(context.Context, string, backend.FileSizeLevel) error

	// DownloadFileByDfget downloads file by dfget.
	DownloadFileByDfget(context.Context, backend.FileSizeLevel) error

	// DownloadFileByProxy downloads file by proxy.
	DownloadFileByProxy(context.Context, backend.FileSizeLevel) error

	// Cleanup cleans up the downloaded files.
	Cleanup(context.Context) error
}

// dragonfly implements the Dragonfly interface.
type dragonfly struct {
	// namespace is the namespace of the benchmark.
	namespace string

	// fileServer is the file server of the benchmark.
	fileServer backend.FileServer

	// stats is the statistics of the benchmark.
	stats stats.Stats
}

// New creates a new benchmark runner for Dragonfly.
func New(namespace string, fileServer backend.FileServer, stats stats.Stats) Dragonfly {
	return &dragonfly{namespace, fileServer, stats}
}

// Run runs all benchmarks by downloader.
func (d *dragonfly) Run(ctx context.Context, downloader string) error {
	switch downloader {
	case config.DownloaderDfget:
		return d.runByDfget(ctx)
	case config.DownloaderProxy:
		return d.runByProxy(ctx)
	default:
		return errors.New("unknown downloader")
	}
}

// Run runs all benchmarks by dfget.
func (d *dragonfly) runByDfget(ctx context.Context) error {
	if err := d.DownloadFileByDfget(ctx, backend.FileSizeLevelNano); err != nil {
		logrus.Errorf("failed to download %s file by dfget: %v", backend.FileSizeLevelNano, err)
		return err
	}

	if err := d.DownloadFileByDfget(ctx, backend.FileSizeLevelMicro); err != nil {
		logrus.Errorf("failed to download %s file by dfget: %v", backend.FileSizeLevelNano, err)
		return err
	}

	if err := d.DownloadFileByDfget(ctx, backend.FileSizeLevelSmall); err != nil {
		logrus.Errorf("failed to download %s file by dfget: %v", backend.FileSizeLevelNano, err)
		return err
	}

	if err := d.DownloadFileByDfget(ctx, backend.FileSizeLevelMedium); err != nil {
		logrus.Errorf("failed to download %s file by dfget: %v", backend.FileSizeLevelNano, err)
		return err
	}

	if err := d.DownloadFileByDfget(ctx, backend.FileSizeLevelLarge); err != nil {
		logrus.Errorf("failed to download %s file by dfget: %v", backend.FileSizeLevelNano, err)
		return err
	}

	if err := d.DownloadFileByDfget(ctx, backend.FileSizeLevelXLarge); err != nil {
		logrus.Errorf("failed to download %s file by dfget: %v", backend.FileSizeLevelNano, err)
		return err
	}

	if err := d.DownloadFileByDfget(ctx, backend.FileSizeLevelXXLarge); err != nil {
		logrus.Errorf("failed to download %s file by dfget: %v", backend.FileSizeLevelNano, err)
		return err
	}

	return nil
}

// Run runs all benchmarks by proxy.
func (d *dragonfly) runByProxy(ctx context.Context) error {
	if err := d.DownloadFileByProxy(ctx, backend.FileSizeLevelNano); err != nil {
		logrus.Errorf("failed to download %s file by proxy: %v", backend.FileSizeLevelNano, err)
		return err
	}

	if err := d.DownloadFileByProxy(ctx, backend.FileSizeLevelMicro); err != nil {
		logrus.Errorf("failed to download %s file by proxy: %v", backend.FileSizeLevelNano, err)
		return err
	}

	if err := d.DownloadFileByProxy(ctx, backend.FileSizeLevelSmall); err != nil {
		logrus.Errorf("failed to download %s file by proxy: %v", backend.FileSizeLevelNano, err)
		return err
	}

	if err := d.DownloadFileByProxy(ctx, backend.FileSizeLevelMedium); err != nil {
		logrus.Errorf("failed to download %s file by proxy: %v", backend.FileSizeLevelNano, err)
		return err
	}

	if err := d.DownloadFileByProxy(ctx, backend.FileSizeLevelLarge); err != nil {
		logrus.Errorf("failed to download %s file by proxy: %v", backend.FileSizeLevelNano, err)
		return err
	}

	if err := d.DownloadFileByProxy(ctx, backend.FileSizeLevelXLarge); err != nil {
		logrus.Errorf("failed to download %s file by proxy: %v", backend.FileSizeLevelNano, err)
		return err
	}

	if err := d.DownloadFileByProxy(ctx, backend.FileSizeLevelXXLarge); err != nil {
		logrus.Errorf("failed to download %s file by proxy: %v", backend.FileSizeLevelNano, err)
		return err
	}

	return nil
}

// RunByFileSizes runs benchmarks by file sizes.
func (d *dragonfly) RunByFileSizes(ctx context.Context, downloader string, fileSizeLevel backend.FileSizeLevel) error {
	switch downloader {
	case config.DownloaderDfget:
		return d.DownloadFileByDfget(ctx, fileSizeLevel)
	case config.DownloaderProxy:
		return d.DownloadFileByProxy(ctx, fileSizeLevel)
	default:
		return errors.New("unknown downloader")
	}
}

// DownloadFileByDfget downloads file by dfget.
func (d *dragonfly) DownloadFileByDfget(ctx context.Context, fileSizeLevel backend.FileSizeLevel) error {
	pods, err := d.getClientPods(ctx)
	if err != nil {
		return err
	}

	var eg errgroup.Group
	for _, pod := range pods {
		podExec := util.NewPodExec(d.namespace, pod, "client")
		eg.Go(func(podExec *util.PodExec) func() error {
			return func() error {
				if err := d.downloadFileByDfget(ctx, podExec, fileSizeLevel); err != nil {
					return err
				}
				return nil
			}
		}(podExec))
	}

	if err := eg.Wait(); err != nil {
		logrus.Errorf("error processing pods: %v", err)
		return err
	}

	return nil
}

// downloadFileByDfget downloads file by dfget.
func (d *dragonfly) downloadFileByDfget(ctx context.Context, podExec *util.PodExec, fileSizeLevel backend.FileSizeLevel) error {
	downloadURL, err := d.fileServer.GetFileURL(fileSizeLevel, "dfget")
	if err != nil {
		logrus.Errorf("failed to get file URL: %v", err)
		return err
	}

	outputPath, err := d.getOutput(fileSizeLevel, "dfget")
	if err != nil {
		logrus.Errorf("failed to get output path: %v", err)
		return err
	}

	createdAt := time.Now()
	output, err := podExec.Command(ctx, "sh", "-c", fmt.Sprintf("dfget '%s' --output %s", downloadURL.String(), outputPath)).CombinedOutput()
	if err != nil {
		logrus.Errorf("failed to download file: %v \nmessage: %s", err, string(output))
		return err
	}

	d.stats.AddDownload(downloadURL, config.DownloaderDfget, fileSizeLevel, createdAt, time.Now())
	logrus.Debugf("dfget output: %s", string(output))
	return nil
}

// DownloadFileByProxy downloads file by proxy.
func (d *dragonfly) DownloadFileByProxy(ctx context.Context, fileSizeLevel backend.FileSizeLevel) error {
	pods, err := d.getClientPods(ctx)
	if err != nil {
		return err
	}

	var eg errgroup.Group
	for _, pod := range pods {
		podExec := util.NewPodExec(d.namespace, pod, "client")
		eg.Go(func(ctx context.Context, podExec *util.PodExec) func() error {
			return func() error {
				if err := d.downloadFileByProxy(ctx, podExec, fileSizeLevel); err != nil {
					return err
				}
				return nil
			}
		}(ctx, podExec))
	}

	if err := eg.Wait(); err != nil {
		logrus.Errorf("error processing pods: %v", err)
		return err
	}

	return nil
}

// downloadFileByProxy downloads file by proxy.
func (d *dragonfly) downloadFileByProxy(ctx context.Context, podExec *util.PodExec, fileSizeLevel backend.FileSizeLevel) error {
	downloadURL, err := d.fileServer.GetFileURL(fileSizeLevel, "proxy")
	if err != nil {
		logrus.Errorf("failed to get file URL: %v", err)
		return err
	}

	outputPath, err := d.getOutput(fileSizeLevel, "proxy")
	if err != nil {
		logrus.Errorf("failed to get output path: %v", err)
		return err
	}

	createdAt := time.Now()
	output, err := podExec.Command(ctx, "sh", "-c", fmt.Sprintf("curl -x %s '%s' --output %s", "http://127.0.0.1:4001", downloadURL.String(), outputPath)).CombinedOutput()
	if err != nil {
		logrus.Errorf("failed to download file: %v \nmessage: %s", err, string(output))
		return err
	}
	d.stats.AddDownload(downloadURL, config.DownloaderProxy, fileSizeLevel, createdAt, time.Now())

	logrus.Debugf("curl output: %s", string(output))
	return nil
}

// Cleanup cleans up the downloaded files.
func (d *dragonfly) Cleanup(ctx context.Context) error {
	pods, err := d.getClientPods(ctx)
	if err != nil {
		return err
	}

	var eg errgroup.Group
	for _, pod := range pods {
		podExec := util.NewPodExec(d.namespace, pod, "client")
		eg.Go(func(podExec *util.PodExec) func() error {
			return func() error {
				output, err := podExec.Command(ctx, "sh", "-c", fmt.Sprintf("rm -rf %s/*", OutputDir)).CombinedOutput()
				if err != nil {
					logrus.Errorf("failed to cleanup: %v \nmessage: %s", err, string(output))
					return err
				}

				return nil
			}
		}(podExec))
	}

	if err := eg.Wait(); err != nil {
		logrus.Errorf("error processing pods: %v", err)
		return err
	}

	return nil
}

// getClientPods returns the client pods.
func (d *dragonfly) getClientPods(ctx context.Context) ([]string, error) {
	pods, err := util.GetPods(ctx, d.namespace, "component=client")
	if err != nil {
		logrus.Errorf("failed to get pods: %v", err)
		return nil, err
	}

	if len(pods) == 0 {
		logrus.Errorf("no client pod found")
		return nil, errors.New("no client pod found")
	}
	return pods, nil
}

// getOutput returns the output path.
func (d *dragonfly) getOutput(fileSizeLevel backend.FileSizeLevel, tag string) (string, error) {
	return path.Join(OutputDir, fmt.Sprintf("%s-%s-%s", string(fileSizeLevel), tag, uuid.New().String())), nil
}
