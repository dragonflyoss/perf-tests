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

	"d7y.io/dragonfly/v2/pkg/idgen"
	"d7y.io/dragonfly/v2/pkg/rpc/base"
	"d7y.io/dragonfly/v2/pkg/rpc/scheduler"
	"github.com/bojand/ghz/runner"
	"github.com/google/uuid"
	"github.com/olekukonko/tablewriter"
)

// announceTask provides scenario function
type announceTask struct {
	host                string
	protoset            string
	concurrency         uint
	insecure            bool
	peerIDs             []string
	runnerConcurrency   uint
	runnerTotalRequests uint
	url                 string
	urlMeta             *base.UrlMeta
	report              *runner.Report
}

// New announceTask instance
func newAnnounceTask(host, protoset string, concurrency uint, insecure bool) Scenario {
	var peerIDs []string
	for i := 0; i < int(concurrency); i++ {
		peerIDs = append(peerIDs, idgen.PeerID("127.0.0.1"))
	}

	return &announceTask{
		host:                host,
		protoset:            protoset,
		concurrency:         concurrency,
		insecure:            insecure,
		runnerConcurrency:   concurrency,
		runnerTotalRequests: concurrency,
		peerIDs:             peerIDs,
		url:                 "https://announce-task",
		urlMeta:             &base.UrlMeta{},
	}
}

// Name returs name of announceTask
func (a *announceTask) Name() string {
	return "AnnounceTask"
}

// Run executes the announceTask performance test
func (a *announceTask) Run() error {
	report, err := runner.Run(
		AnnounceTaskMethod,
		a.host,
		runner.WithProtoset(a.protoset),
		runner.WithData(a.data()),
		runner.WithInsecure(a.insecure),
		runner.WithConcurrency(a.runnerConcurrency),
		runner.WithTotalRequests(a.runnerTotalRequests),
	)
	if err != nil {
		return err
	}

	a.report = report
	return nil
}

// Print output announceTask performance test results to os.Stdout
func (a *announceTask) Print() {
	report := a.report
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Scenario", "Name", "Description", "Value"})

	// Summary stat
	data := [][]string{
		{a.Name(), "Summary", "Count", fmt.Sprintf("%d %s", report.Count, "times")},
		{a.Name(), "Summary", "Slowest", fmt.Sprintf("%d %s", int(report.Slowest.Milliseconds()), "ms")},
		{a.Name(), "Summary", "Fastest", fmt.Sprintf("%d %s", int(report.Fastest.Milliseconds()), "ms")},
		{a.Name(), "Summary", "Average", fmt.Sprintf("%d %s", int(report.Average.Milliseconds()), "ms")},
	}

	// Status code distribution stat
	var statusCodeDistributionData [][]string
	for k, v := range report.StatusCodeDist {
		statusCodeDistributionData = append(statusCodeDistributionData, []string{
			a.Name(),
			"Status Code Distribution",
			k,
			fmt.Sprintf("%d %s", v, "times"),
		})
	}
	data = append(data, statusCodeDistributionData...)

	// Error distribution stat
	var errorDistributionData [][]string
	for k, v := range report.ErrorDist {
		errorDistributionData = append(errorDistributionData, []string{
			a.Name(),
			"Error Distribution",
			k,
			fmt.Sprintf("%d %s", v, "times"),
		})
	}
	data = append(data, errorDistributionData...)

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetAutoMergeCells(true)
	table.SetRowLine(true)
	table.SetCaption(true, "")
	table.AppendBulk(data)
	table.Render()
}

// data mocks announce task requests
func (a *announceTask) data() interface{} {
	taskID := idgen.TaskID(a.url, a.urlMeta)
	var data []*scheduler.AnnounceTaskRequest
	for _, peerID := range a.peerIDs {
		data = append(data, &scheduler.AnnounceTaskRequest{
			TaskId:  taskID,
			Cid:     a.url,
			UrlMeta: a.urlMeta,
			PeerHost: &scheduler.PeerHost{
				Uuid:     uuid.NewString(),
				Ip:       "127.0.0.1",
				RpcPort:  8080,
				DownPort: 8081,
				HostName: "localhost",
			},
			PiecePacket: &base.PiecePacket{
				TaskId:  taskID,
				DstPid:  peerID,
				DstAddr: "127.0.0.1",
				PieceInfos: []*base.PieceInfo{
					{PieceNum: 1},
					{PieceNum: 2},
				},
				TotalPiece:    2,
				ContentLength: 1000,
			},
		})
	}

	return data
}
