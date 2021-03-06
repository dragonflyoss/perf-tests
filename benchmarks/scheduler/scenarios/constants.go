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

const (
	// RegisterMethod is grpc method of RegisterPeerTask
	RegisterMethod = "scheduler.Scheduler.RegisterPeerTask"

	// ReportPieceMethod is grpc method of ReportPieceResult
	ReportPieceMethod = "scheduler.Scheduler.ReportPieceResult"

	// ReportPeerMethod is grpc method of ReportPeerResult
	ReportPeerMethod = "scheduler.Scheduler.ReportPeerResult"

	// StatTaskMethod is grpc method of StatTask
	StatTaskMethod = "scheduler.Scheduler.StatTask"

	// AnnounceTaskMethod is grpc method of AnnounceTask
	AnnounceTaskMethod = "scheduler.Scheduler.AnnounceTask"

	// LeaveTaskMethod is grpc method of LeaveTask
	LeaveTaskMethod = "scheduler.Scheduler.LeaveTask"
)
