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

// leaveTask provides scenario function
type leaveTask struct {
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

// New leaveTask instance
func newLeaveTask(host, protoset string, concurrency uint, insecure bool) Scenario {
	var peerIDs []string
	for i := 0; i < int(concurrency); i++ {
		peerIDs = append(peerIDs, idgen.PeerID("127.0.0.1"))
	}

	return &leaveTask{
		host:                host,
		protoset:            protoset,
		concurrency:         concurrency,
		insecure:            insecure,
		runnerConcurrency:   concurrency,
		runnerTotalRequests: concurrency,
		peerIDs:             peerIDs,
		url:                 "https://leave-task",
		urlMeta:             &base.UrlMeta{},
	}
}

// Name returs name of leaveTask
func (l *leaveTask) Name() string {
	return "LeaveTask"
}

// Run executes the leaveTask performance test
func (l *leaveTask) Run() error {
	if _, err := runner.Run(
		RegisterMethod,
		l.host,
		runner.WithProtoset(l.protoset),
		runner.WithData(l.registerData()),
		runner.WithInsecure(l.insecure),
		runner.WithConcurrency(l.runnerConcurrency),
		runner.WithTotalRequests(l.runnerTotalRequests),
	); err != nil {
		return err
	}

	if _, err := runner.Run(
		ReportPeerMethod,
		l.host,
		runner.WithProtoset(l.protoset),
		runner.WithData(l.reportPeerData()),
		runner.WithInsecure(l.insecure),
		runner.WithConcurrency(l.runnerConcurrency),
		runner.WithTotalRequests(l.runnerTotalRequests),
	); err != nil {
		return err
	}

	report, err := runner.Run(
		LeaveTaskMethod,
		l.host,
		runner.WithProtoset(l.protoset),
		runner.WithData(l.leaveTaskData()),
		runner.WithInsecure(l.insecure),
		runner.WithConcurrency(l.runnerConcurrency),
		runner.WithTotalRequests(l.runnerTotalRequests),
	)
	if err != nil {
		return err
	}

	l.report = report
	return nil
}

// Print output leaveTask performance test results to os.Stdout
func (l *leaveTask) Print() {
	report := l.report
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Scenario", "Name", "Description", "Value"})

	// Summary stat
	data := [][]string{
		{l.Name(), "Summary", "Count", fmt.Sprintf("%d %s", report.Count, "times")},
		{l.Name(), "Summary", "Slowest", fmt.Sprintf("%d %s", int(report.Slowest.Milliseconds()), "ms")},
		{l.Name(), "Summary", "Fastest", fmt.Sprintf("%d %s", int(report.Fastest.Milliseconds()), "ms")},
		{l.Name(), "Summary", "Average", fmt.Sprintf("%d %s", int(report.Average.Milliseconds()), "ms")},
	}

	// Status code distribution stat
	var statusCodeDistributionData [][]string
	for k, v := range report.StatusCodeDist {
		statusCodeDistributionData = append(statusCodeDistributionData, []string{
			l.Name(),
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
			l.Name(),
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

// registerData mocks register requests
func (l *leaveTask) registerData() interface{} {
	var data []*scheduler.PeerTaskRequest
	for _, peerID := range l.peerIDs {
		data = append(data, &scheduler.PeerTaskRequest{
			Url:     l.url,
			UrlMeta: l.urlMeta,
			PeerId:  peerID,
			PeerHost: &scheduler.PeerHost{
				Id:       uuid.NewString(),
				Ip:       "127.0.0.1",
				RpcPort:  8080,
				DownPort: 8081,
				HostName: "localhost",
			},
		})
	}

	return data
}

// reportPeerData mocks report peer requests
func (l *leaveTask) reportPeerData() interface{} {
	taskID := idgen.TaskID(l.url, l.urlMeta)
	var data []*scheduler.PeerResult
	for _, peerID := range l.peerIDs {
		data = append(data, &scheduler.PeerResult{
			TaskId:          taskID,
			PeerId:          peerID,
			SrcIp:           "127.0.0.1",
			Url:             l.url,
			ContentLength:   100,
			Traffic:         100,
			Cost:            10,
			TotalPieceCount: 10,
			Success:         true,
		})
	}

	return data
}

// leaveTaskData mocks leave task requests
func (l *leaveTask) leaveTaskData() interface{} {
	taskID := idgen.TaskID(l.url, l.urlMeta)
	var data []*scheduler.PeerTarget
	for _, peerID := range l.peerIDs {
		data = append(data, &scheduler.PeerTarget{
			TaskId: taskID,
			PeerId: peerID,
		})
	}

	return data
}
