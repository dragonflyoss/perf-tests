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

// statTask provides scenario function
type statTask struct {
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

// New statTask instance
func newStatTask(host, protoset string, concurrency uint, insecure bool) Scenario {
	var peerIDs []string
	for i := 0; i < int(concurrency); i++ {
		peerIDs = append(peerIDs, idgen.PeerID("127.0.0.1"))
	}

	return &statTask{
		host:                host,
		protoset:            protoset,
		concurrency:         concurrency,
		insecure:            insecure,
		runnerConcurrency:   concurrency,
		runnerTotalRequests: concurrency,
		peerIDs:             peerIDs,
		url:                 "https://stat-task",
		urlMeta:             &base.UrlMeta{},
	}
}

// Name returs name of statTask
func (s *statTask) Name() string {
	return "StatTask"
}

// Run executes the statTask performance test
func (s *statTask) Run() error {
	if _, err := runner.Run(
		RegisterMethod,
		s.host,
		runner.WithProtoset(s.protoset),
		runner.WithData(s.registerData()),
		runner.WithInsecure(s.insecure),
		runner.WithConcurrency(s.runnerConcurrency),
		runner.WithTotalRequests(s.runnerTotalRequests),
	); err != nil {
		return err
	}

	report, err := runner.Run(
		StatTaskMethod,
		s.host,
		runner.WithProtoset(s.protoset),
		runner.WithData(s.statTaskData()),
		runner.WithInsecure(s.insecure),
		runner.WithConcurrency(s.runnerConcurrency),
		runner.WithTotalRequests(s.runnerTotalRequests),
	)
	if err != nil {
		return err
	}

	s.report = report
	return nil
}

// Print output statTask performance test results to os.Stdout
func (s *statTask) Print() {
	report := s.report
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Scenario", "Name", "Description", "Value"})

	// Summary stat
	data := [][]string{
		{s.Name(), "Summary", "Count", fmt.Sprintf("%d %s", report.Count, "times")},
		{s.Name(), "Summary", "Slowest", fmt.Sprintf("%d %s", int(report.Slowest.Milliseconds()), "ms")},
		{s.Name(), "Summary", "Fastest", fmt.Sprintf("%d %s", int(report.Fastest.Milliseconds()), "ms")},
		{s.Name(), "Summary", "Average", fmt.Sprintf("%d %s", int(report.Average.Milliseconds()), "ms")},
	}

	// Status code distribution stat
	var statusCodeDistributionData [][]string
	for k, v := range report.StatusCodeDist {
		statusCodeDistributionData = append(statusCodeDistributionData, []string{
			s.Name(),
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
			s.Name(),
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
func (s *statTask) registerData() interface{} {
	var data []*scheduler.PeerTaskRequest
	for _, peerID := range s.peerIDs {
		data = append(data, &scheduler.PeerTaskRequest{
			Url:     s.url,
			UrlMeta: s.urlMeta,
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

// statTaskData mocks stat task requests
func (s *statTask) statTaskData() interface{} {
	taskID := idgen.TaskID(s.url, s.urlMeta)
	var data []*scheduler.StatTaskRequest
	for i := 0; i < int(s.concurrency); i++ {
		data = append(data, &scheduler.StatTaskRequest{
			TaskId: taskID,
		})
	}

	return data
}
