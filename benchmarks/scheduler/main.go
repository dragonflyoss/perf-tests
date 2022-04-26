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
	"sync"

	"d7y.io/dragonfly/v2/pkg/idgen"
	"d7y.io/dragonfly/v2/pkg/rpc/base"
	"d7y.io/dragonfly/v2/pkg/rpc/base/common"
	"d7y.io/dragonfly/v2/pkg/rpc/scheduler"
	"github.com/bojand/ghz/runner"
	"github.com/google/uuid"
)

var TaskID = idgen.TaskID("https://foo", &base.UrlMeta{})

const Concurrency = 10

func main() {
	var peerIDs []string
	for i := 0; i < Concurrency; i++ {
		peerIDs = append(peerIDs, idgen.PeerID("127.0.0.1"))
	}

	if err := register(peerIDs); err != nil {
		panic(err)
	}

	if errs := reportPiece(peerIDs); len(errs) > 0 {
		panic(errs)
	}
}

func register(peerIDs []string) error {
	var req []*scheduler.PeerTaskRequest
	for _, peerID := range peerIDs {
		req = append(req, &scheduler.PeerTaskRequest{
			Url:     "https://foo",
			UrlMeta: &base.UrlMeta{},
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

	_, err := runner.Run(
		"scheduler.Scheduler.RegisterPeerTask",
		"127.0.0.1:8002",
		runner.WithProtoset("../bundle.pb"),
		runner.WithData(req),
		runner.WithInsecure(true),
		runner.WithConcurrency(uint(len(peerIDs))),
		runner.WithTotalRequests(uint(len(peerIDs))),
	)

	return err
}

func reportPiece(peerIDs []string) []error {
	var wg sync.WaitGroup
	var errs []error
	for _, peerID := range peerIDs {
		wg.Add(1)
		go func(peerID string) {
			_, err := runner.Run(
				"scheduler.Scheduler.ReportPieceResult",
				"127.0.0.1:8002",
				runner.WithProtoset("../bundle.pb"),
				runner.WithData(&scheduler.PieceResult{
					TaskId: idgen.TaskID("https://foo", &base.UrlMeta{}),
					SrcPid: peerID,
					DstPid: idgen.PeerID("127.0.0.1"),
					PieceInfo: &base.PieceInfo{
						PieceNum: common.BeginOfPiece,
					},
					Success: true,
				}),
				runner.WithInsecure(true),
				runner.WithConcurrency(uint(1)),
				runner.WithTotalRequests(uint(1)),
			)
			wg.Done()
			if err != nil {
				errs = append(errs, err)
			}
		}(peerID)
	}
	wg.Wait()
	return errs
}
