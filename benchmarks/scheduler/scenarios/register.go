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

// register provides scenario function
type register struct {
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

// New register instance
func newRegister(host, protoset string, concurrency uint, insecure bool) Scenario {
	var peerIDs []string
	for i := 0; i < int(concurrency); i++ {
		peerIDs = append(peerIDs, idgen.PeerID("127.0.0.1"))
	}

	return &register{
		host:                host,
		protoset:            protoset,
		concurrency:         concurrency,
		insecure:            insecure,
		runnerConcurrency:   concurrency,
		runnerTotalRequests: concurrency,
		peerIDs:             peerIDs,
		url:                 "https://register",
		urlMeta:             &base.UrlMeta{},
	}
}

// Name returs name of register
func (r *register) Name() string {
	return "Register"
}

// Run executes the register performance test
func (r *register) Run() error {
	report, err := runner.Run(
		RegisterMethod,
		r.host,
		runner.WithProtoset(r.protoset),
		runner.WithData(r.data()),
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

// Print output register performance test results to os.Stdout
func (r *register) Print() {
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

// data mocks register requests
func (r *register) data() interface{} {
	var data []*scheduler.PeerTaskRequest
	for _, peerID := range r.peerIDs {
		data = append(data, &scheduler.PeerTaskRequest{
			TaskId:  idgen.TaskID(r.url, r.urlMeta),
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
