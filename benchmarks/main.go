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

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"d7y.io/dragonfly/v2/pkg/basic"
	"d7y.io/dragonfly/v2/pkg/rpc/base"
	"d7y.io/dragonfly/v2/pkg/rpc/dfdaemon"
	"github.com/bojand/ghz/printer"
	"github.com/bojand/ghz/runner"
	"github.com/google/uuid"
)

func main() {
	report, err := runner.Run(
		"dfdaemon.Daemon.Download",
		"127.0.0.1:65000",
		runner.WithProtoset("./bundle.pb"),
		runner.WithData(GetData()),
		runner.WithInsecure(true),
		runner.WithConcurrency(uint(10)),
		runner.WithTotalRequests(uint(10)),
	)
	if err != nil {
		panic(err)
	}

	printer := printer.ReportPrinter{
		Out:    os.Stdout,
		Report: report,
	}

	printer.Print("summary")
}

func GetData() []*dfdaemon.DownRequest {
	var req []*dfdaemon.DownRequest
	for i := 0; i < 10; i++ {
		output := filepath.Join(os.TempDir(), uuid.New().String())
		fmt.Println(output)
		req = append(req, &dfdaemon.DownRequest{
			Uuid:              uuid.New().String(),
			Url:               fmt.Sprintf("%s?number=%d", "http://foo/1k", i),
			Output:            output,
			DisableBackSource: true,
			UrlMeta:           &base.UrlMeta{},
			Uid:               int64(basic.UserID),
			Gid:               int64(basic.UserGroup),
		})
	}

	return req
}
