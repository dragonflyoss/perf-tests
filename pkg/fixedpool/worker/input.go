/*
 *     Copyright 2026 The Dragonfly Authors
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

package worker

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dragonflyoss/perf-tests/pkg/fixedpool/contract"
	"github.com/dragonflyoss/perf-tests/pkg/fixedpool/plan"
)

type Spec struct {
	BaseURL          string            `json:"base_url"`
	RunID            string            `json:"run_id,omitempty"`
	WorkerID         uint64            `json:"worker_id"`
	PlanID           string            `json:"plan_id"`
	ManifestID       string            `json:"manifest_id"`
	Manifest         string            `json:"manifest"`
	Shard            string            `json:"shard"`
	RegionCount      uint64            `json:"region_count"`
	RangeBytes       uint64            `json:"range_bytes"`
	RegionsPerObject uint64            `json:"regions_per_object"`
	Proxy            string            `json:"proxy"`
	Headers          map[string]string `json:"headers"`
	Rate             uint64            `json:"rate"`
	DurationMS       int64             `json:"duration_ms"`
	TimeoutMS        int64             `json:"timeout_ms"`
	GracefulStopMS   int64             `json:"graceful_stop_ms"`
	PreAllocatedVUs  uint64            `json:"pre_allocated_vus"`
	MaxVUs           uint64            `json:"max_vus"`
}

type Environment struct {
	Manifest, Plan, SourceBaseURL, Proxy, RunID string
	WorkerID, Rate, PreAllocatedVUs, MaxVUs     uint64
	Duration, Timeout, GracefulStop             time.Duration
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func ReadEnvironment(environ []string) (Environment, error) {
	var e Environment
	values := map[string]string{}
	for _, pair := range environ {
		key, value, _ := strings.Cut(pair, "=")
		values[key] = value
	}
	text := func(target *string) func(string) error {
		return func(raw string) error { *target = raw; return nil }
	}
	integer := func(target *uint64, min, max uint64) func(string) error {
		return func(raw string) error {
			n, err := strconv.ParseUint(raw, 10, 64)
			if err == nil && (n < min || n > max) {
				return fmt.Errorf("expected %d..%d, actual %d", min, max, n)
			}
			*target = n
			return err
		}
	}
	duration := func(target *time.Duration, min, max time.Duration) func(string) error {
		return func(raw string) error {
			d, err := time.ParseDuration(raw)
			if err == nil && (d < min || d > max || d%time.Millisecond != 0) {
				return fmt.Errorf("expected %s..%s in whole milliseconds, actual %s", min, max, d)
			}
			*target = d
			return err
		}
	}
	const maxDuration = time.Duration(1<<63 - 1)
	options := []struct {
		name     string
		required bool
		fallback string
		parse    func(string) error
	}{
		{"MANIFEST", true, "", text(&e.Manifest)},
		{"PLAN", true, "", text(&e.Plan)},
		{"SOURCE_BASE_URL", true, "", func(raw string) error {
			value, err := contract.NormalizeBaseURL(raw)
			e.SourceBaseURL = value
			return err
		}},
		{"HTTP_PROXY", true, "", func(raw string) error {
			_, err := contract.ParseProxyURL(raw)
			e.Proxy = raw
			return err
		}},
		{"RATE", true, "", integer(&e.Rate, 1, contract.MaxRate)},
		{"WORKER_ID", false, "0", integer(&e.WorkerID, 0, contract.MaxRegions)},
		{"PRE_ALLOCATED_VUS", false, "64", integer(&e.PreAllocatedVUs, 1, contract.MaxRate)},
		{"MAX_VUS", false, "512", integer(&e.MaxVUs, 1, contract.MaxRate)},
		{"DURATION", false, "300s", duration(&e.Duration, time.Second, maxDuration)},
		{"TIMEOUT", false, "30s", duration(&e.Timeout, time.Millisecond, maxDuration-5*time.Second)},
		{"GRACEFUL_STOP", false, "", duration(&e.GracefulStop, time.Millisecond, maxDuration)},
		{"RUN_ID", false, "", func(raw string) error {
			if raw != "" && !identifier.MatchString(raw) {
				return fmt.Errorf("expected 1..128 ASCII letters/digits/._-, starting with a letter or digit")
			}
			e.RunID = raw
			return nil
		}},
	}
	for _, option := range options {
		raw, exists := values[option.name]
		if option.required && (!exists || raw == "") {
			return e, fmt.Errorf("%s is required", option.name)
		}
		if !exists {
			if option.fallback == "" {
				continue
			}
			raw = option.fallback
		}
		if err := option.parse(raw); err != nil {
			return e, fmt.Errorf("%s: %w", option.name, err)
		}
	}
	// Explicit values must be positive; zero therefore means this input was omitted.
	if e.GracefulStop == 0 {
		e.GracefulStop = e.Timeout + 5*time.Second
	}
	if e.MaxVUs < e.PreAllocatedVUs {
		return e, fmt.Errorf("MAX_VUS: expected >= PRE_ALLOCATED_VUS=%d, actual %d", e.PreAllocatedVUs, e.MaxVUs)
	}
	if e.GracefulStop <= e.Timeout {
		return e, fmt.Errorf("GRACEFUL_STOP: expected > TIMEOUT=%s, actual %s", e.Timeout, e.GracefulStop)
	}
	return e, nil
}

func PrepareInput(ctx context.Context, e Environment) (Spec, error) {
	manifestPath, err := filepath.Abs(e.Manifest)
	if err != nil {
		return Spec{}, err
	}
	m, manifestRaw, err := contract.ReadManifest(manifestPath)
	if err != nil {
		return Spec{}, err
	}
	manifestID := contract.Digest(manifestRaw)
	p, planRaw, err := plan.LoadWorker(ctx, e.Plan, m, manifestID, e.WorkerID)
	if err != nil {
		return Spec{}, err
	}
	shard := p.Shards[e.WorkerID]
	shardPath, err := filepath.Abs(filepath.Join(filepath.Dir(e.Plan), shard.Path))
	if err != nil {
		return Spec{}, err
	}
	counts := make([]uint64, len(p.Shards))
	for i, s := range p.Shards {
		counts[i] = s.RegionCount
	}
	rates, err := allocateRates(counts, e.Rate)
	if err != nil {
		return Spec{}, contract.Tag(contract.InputInvalid, err)
	}
	headers := map[string]string{}
	for key, value := range contract.FixedHeaders() {
		headers[key] = value[0]
	}
	spec := Spec{
		BaseURL:          e.SourceBaseURL,
		RunID:            e.RunID,
		WorkerID:         e.WorkerID,
		PlanID:           contract.Digest(planRaw),
		ManifestID:       manifestID,
		Manifest:         manifestPath,
		Shard:            shardPath,
		RegionCount:      shard.RegionCount,
		RangeBytes:       p.RangeBytes,
		RegionsPerObject: p.RegionsPerObject,
		Proxy:            e.Proxy,
		Headers:          headers,
		Rate:             rates[e.WorkerID],
		DurationMS:       e.Duration.Milliseconds(),
		TimeoutMS:        e.Timeout.Milliseconds(),
		GracefulStopMS:   e.GracefulStop.Milliseconds(),
		PreAllocatedVUs:  e.PreAllocatedVUs,
		MaxVUs:           e.MaxVUs,
	}
	_, _ = fmt.Printf("fixed-pool inputs checked\n"+
		"  Dataset: path=%q sha256=%s B=%d N=%d source=%q\n"+
		"  Plan: path=%q sha256=%s R=%d W=%d regions=%d\n"+
		"  Target: proxy=%q\n"+
		"  Worker: id=%d shard=%q sha256=%s regions=%d\n"+
		"  Load: total_rate=%d local_rate=%d duration=%s timeout=%s graceful_stop=%s pre_allocated_vus=%d max_vus=%d\n",
		manifestPath, manifestID, m.ObjectSize, m.ObjectCount, e.SourceBaseURL,
		e.Plan, spec.PlanID, p.RangeBytes, p.WorkerCount, p.RegionCount,
		e.Proxy, e.WorkerID, shardPath, shard.SHA256, spec.RegionCount,
		e.Rate, spec.Rate, e.Duration, e.Timeout, e.GracefulStop, e.PreAllocatedVUs, e.MaxVUs,
	)
	if e.RunID != "" {
		_, _ = fmt.Printf("  Run ID: %s\n", e.RunID)
	}
	return spec, nil
}

// allocateRates uses inputs checked by ReadEnvironment and plan.LoadWorker.
// Assign remaining units before rejecting a zero rate.
// Premature rejection after floor() would incorrectly reject M=3,W=2,RATE=2.
func allocateRates(counts []uint64, totalRate uint64) ([]uint64, error) {
	if totalRate < uint64(len(counts)) {
		return nil, fmt.Errorf("total RATE: expected at least worker_count=%d, actual %d", len(counts), totalRate)
	}
	var total uint64
	for _, count := range counts {
		total += count
	}
	type remainder struct {
		id    int
		value uint64
	}
	order := make([]remainder, len(counts))
	rates := make([]uint64, len(counts))
	left := totalRate
	for i, count := range counts {
		product := totalRate * count
		rates[i] = product / total
		left -= rates[i]
		order[i] = remainder{id: i, value: product % total}
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].value == order[j].value {
			return order[i].id < order[j].id
		}
		return order[i].value > order[j].value
	})
	for i := uint64(0); i < left; i++ {
		rates[order[i].id]++
	}
	for _, rate := range rates {
		if rate == 0 {
			return nil, fmt.Errorf("RATE allocation leaves a worker below 1 req/s")
		}
	}
	return rates, nil
}
