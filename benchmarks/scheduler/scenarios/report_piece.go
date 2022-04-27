/*
 *     Copyright 2022 The Dragonfly Authors
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

package scenarios

import (
	"fmt"
	"os"
	"sync"
	"time"

	"d7y.io/dragonfly/v2/pkg/idgen"
	"d7y.io/dragonfly/v2/pkg/rpc/base"
	"d7y.io/dragonfly/v2/pkg/rpc/base/common"
	"d7y.io/dragonfly/v2/pkg/rpc/scheduler"
	"github.com/bojand/ghz/runner"
	"github.com/google/uuid"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/olekukonko/tablewriter"
)

type reportPiece struct {
	host                  string
	protoset              string
	concurrency           uint
	insecure              bool
	peerIDs               []string
	runnerConcurrency     uint
	runnerTotalRequests   uint
	url                   string
	urlMeta               *base.UrlMeta
	reports               []*runner.Report
	errs                  []error
	recvCodeDistributions map[string]int
}

func newReportPiece(host, protoset string, concurrency uint, insecure bool) Scenario {
	var peerIDs []string
	for i := 0; i < int(concurrency); i++ {
		peerIDs = append(peerIDs, idgen.PeerID("127.0.0.1"))
	}

	return &reportPiece{
		host:                  host,
		protoset:              protoset,
		concurrency:           concurrency,
		insecure:              insecure,
		runnerConcurrency:     1,
		runnerTotalRequests:   1,
		peerIDs:               peerIDs,
		url:                   "https://report-piece",
		urlMeta:               &base.UrlMeta{},
		recvCodeDistributions: map[string]int{},
	}
}

func (r *reportPiece) Name() string {
	return "ReportPiece"
}

func (r *reportPiece) Run() error {
	if _, err := runner.Run(
		RegisterMethod,
		r.host,
		runner.WithProtoset(r.protoset),
		runner.WithData(r.registerData()),
		runner.WithInsecure(r.insecure),
		runner.WithConcurrency(r.concurrency),
		runner.WithTotalRequests(r.concurrency),
	); err != nil {
		return err
	}

	var wg sync.WaitGroup
	var errs []error
	var reports []*runner.Report
	for _, peerID := range r.peerIDs {
		wg.Add(1)
		go func(peerID string, reports []*runner.Report, reportPiece *reportPiece, errs []error) {
			defer wg.Done()
			report, err := runner.Run(
				ReportPieceMethod,
				reportPiece.host,
				runner.WithProtoset(reportPiece.protoset),
				runner.WithData(r.reportPieceData(peerID)),
				runner.WithInsecure(reportPiece.insecure),
				runner.WithConcurrency(reportPiece.runnerConcurrency),
				runner.WithTotalRequests(reportPiece.runnerTotalRequests),
				runner.WithStreamRecvMsgIntercept(func(msg *dynamic.Message, err error) error {
					if err != nil {
						return err
					}

					r.recvCodeDistributions[fmt.Sprintf("%v", msg.GetFieldByName("code"))]++
					return nil
				}),
			)
			if err != nil {
				r.errs = append(r.errs, err)
				return
			}

			r.reports = append(r.reports, report)
		}(peerID, reports, r, errs)
	}

	wg.Wait()
	return nil
}

func (r *reportPiece) Print() {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Scenario", "Name", "Description", "Value"})

	// Merge reports
	reports := r.reports
	if len(reports) == 0 {
		panic("reports is empty")
	}

	var count uint64
	var totalAverage time.Duration
	slowest := reports[0].Slowest
	fastest := reports[0].Fastest
	statusCodeDistributions := map[string]int{}
	errorDistributions := map[string]int{}
	for _, r := range reports {
		count += r.Count
		totalAverage += r.Average

		if slowest < r.Slowest {
			slowest = r.Slowest
		}

		if fastest > r.Fastest {
			fastest = r.Fastest
		}

		for k, v := range r.StatusCodeDist {
			statusCodeDistributions[k] += v
		}

		for k, v := range r.ErrorDist {
			errorDistributions[k] += v
		}
	}
	average := time.Duration(int64(totalAverage) / int64(count))

	// Summary stat
	data := [][]string{
		{r.Name(), "Summary", "Count", fmt.Sprintf("%d %s", count, "times")},
		{r.Name(), "Summary", "Slowest", fmt.Sprintf("%d %s", int(slowest.Milliseconds()), "ms")},
		{r.Name(), "Summary", "Fastest", fmt.Sprintf("%d %s", int(fastest.Milliseconds()), "ms")},
		{r.Name(), "Summary", "Average", fmt.Sprintf("%d %s", int(average.Milliseconds()), "ms")},
	}

	// Status code distribution stat
	var statusCodeDistributionData [][]string
	for k, v := range statusCodeDistributions {
		statusCodeDistributionData = append(statusCodeDistributionData, []string{
			r.Name(),
			"Status Code Distribution",
			k,
			fmt.Sprintf("%d %s", v, "times"),
		})
	}
	data = append(data, statusCodeDistributionData...)

	// Error distribution stat
	var errorDistributionData [][]string
	for k, v := range errorDistributions {
		errorDistributionData = append(errorDistributionData, []string{
			r.Name(),
			"Error Distribution",
			k,
			fmt.Sprintf("%d %s", v, "times"),
		})
	}
	data = append(data, errorDistributionData...)

	// Runner error distribution stat
	runnerErrorDistributions := map[string]int{}
	for _, v := range r.errs {
		runnerErrorDistributions[v.Error()]++
	}

	var runnerErrorDistributionData [][]string
	for k, v := range runnerErrorDistributions {
		runnerErrorDistributionData = append(runnerErrorDistributionData, []string{
			r.Name(),
			"Runner Error Distribution",
			k,
			fmt.Sprintf("%d %s", v, "times"),
		})
	}
	data = append(data, runnerErrorDistributionData...)

	// Recv status code distribution stat
	var recvStatusCodeDistributionData [][]string
	for k, v := range r.recvCodeDistributions {
		recvStatusCodeDistributionData = append(recvStatusCodeDistributionData, []string{
			r.Name(),
			"Recv Status Code Distribution",
			k,
			fmt.Sprintf("%d %s", v, "times"),
		})
	}
	data = append(data, recvStatusCodeDistributionData...)

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetAutoMergeCells(true)
	table.SetRowLine(true)
	table.AppendBulk(data)
	table.Render()
}

func (r *reportPiece) registerData() interface{} {
	var data []*scheduler.PeerTaskRequest
	for _, peerID := range r.peerIDs {
		data = append(data, &scheduler.PeerTaskRequest{
			Url:     r.url,
			UrlMeta: r.urlMeta,
			PeerId:  peerID,
			PeerHost: &scheduler.PeerHost{
				Uuid:     uuid.NewString(),
				Ip:       "127.0.0.1",
				RpcPort:  8080,
				DownPort: 8081,
				HostName: "localhost",
			},
		})
	}

	return data
}

func (r *reportPiece) reportPieceData(peerID string) interface{} {
	taskID := idgen.TaskID(r.url, r.urlMeta)
	return &scheduler.PieceResult{
		TaskId: taskID,
		SrcPid: peerID,
		DstPid: idgen.PeerID("127.0.0.1"),
		PieceInfo: &base.PieceInfo{
			PieceNum: common.BeginOfPiece,
		},
		Success: true,
	}
}
