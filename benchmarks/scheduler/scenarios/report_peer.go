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

// reportPeer provides scenario function
type reportPeer struct {
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

// New reportPeer instance
func newReportPeer(host, protoset string, concurrency uint, insecure bool) Scenario {
	var peerIDs []string
	for i := 0; i < int(concurrency); i++ {
		peerIDs = append(peerIDs, idgen.PeerID("127.0.0.1"))
	}

	return &reportPeer{
		host:                host,
		protoset:            protoset,
		concurrency:         concurrency,
		insecure:            insecure,
		runnerConcurrency:   concurrency,
		runnerTotalRequests: concurrency,
		peerIDs:             peerIDs,
		url:                 "https://report-peer",
		urlMeta:             &base.UrlMeta{},
	}
}

// Name returs name of reportPeer
func (r *reportPeer) Name() string {
	return "ReportPeer"
}

// Run executes the reportPeer performance test
func (r *reportPeer) Run() error {
	if _, err := runner.Run(
		RegisterMethod,
		r.host,
		runner.WithProtoset(r.protoset),
		runner.WithData(r.registerData()),
		runner.WithInsecure(r.insecure),
		runner.WithConcurrency(r.runnerConcurrency),
		runner.WithTotalRequests(r.runnerTotalRequests),
	); err != nil {
		return err
	}

	report, err := runner.Run(
		ReportPeerMethod,
		r.host,
		runner.WithProtoset(r.protoset),
		runner.WithData(r.reportPeerData()),
		runner.WithInsecure(r.insecure),
		runner.WithConcurrency(r.runnerConcurrency),
		runner.WithTotalRequests(r.runnerTotalRequests),
	)
	if err != nil {
		return err
	}

	r.report = report
	return nil
}

// Print output reportPeer performance test results to os.Stdout
func (r *reportPeer) Print() {
	report := r.report
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Scenario", "Name", "Description", "Value"})

	// Summary stat
	data := [][]string{
		{r.Name(), "Summary", "Count", fmt.Sprintf("%d %s", report.Count, "times")},
		{r.Name(), "Summary", "Slowest", fmt.Sprintf("%d %s", int(report.Slowest.Milliseconds()), "ms")},
		{r.Name(), "Summary", "Fastest", fmt.Sprintf("%d %s", int(report.Fastest.Milliseconds()), "ms")},
		{r.Name(), "Summary", "Average", fmt.Sprintf("%d %s", int(report.Average.Milliseconds()), "ms")},
	}

	// Status code distribution stat
	var statusCodeDistributionData [][]string
	for k, v := range report.StatusCodeDist {
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
	for k, v := range report.ErrorDist {
		errorDistributionData = append(errorDistributionData, []string{
			r.Name(),
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
func (r *reportPeer) registerData() interface{} {
	var data []*scheduler.PeerTaskRequest
	for _, peerID := range r.peerIDs {
		data = append(data, &scheduler.PeerTaskRequest{
			Url:     r.url,
			UrlMeta: r.urlMeta,
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
func (r *reportPeer) reportPeerData() interface{} {
	taskID := idgen.TaskID(r.url, r.urlMeta)
	var data []*scheduler.PeerResult
	for _, peerID := range r.peerIDs {
		data = append(data, &scheduler.PeerResult{
			TaskId:          taskID,
			PeerId:          peerID,
			SrcIp:           "127.0.0.1",
			Url:             r.url,
			ContentLength:   100,
			Traffic:         100,
			Cost:            10,
			TotalPieceCount: 10,
			Success:         true,
		})
	}

	return data
}
