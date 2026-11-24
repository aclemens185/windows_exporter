// SPDX-License-Identifier: Apache-2.0
//
// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build windows

package terminal_services

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/prometheus-community/windows_exporter/internal/pdh"
	"github.com/prometheus-community/windows_exporter/internal/scrapecache"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCollectorWithoutSessionObject covers hosts without the Terminal Services
// Session performance object, e.g. Server Core installations: the collector
// must build and scrape without errors, and skip only the per-session
// process metrics.
func TestCollectorWithoutSessionObject(t *testing.T) {
	original := terminalServicesSessionObject
	terminalServicesSessionObject = "windows_exporter missing object"

	t.Cleanup(func() { terminalServicesSessionObject = original })

	c := New(nil)

	t.Cleanup(func() { assert.NoError(t, c.Close()) })

	require.NoError(t, c.Build(slog.New(slog.DiscardHandler), nil))
	require.Nil(t, c.perfDataCollectorTerminalServicesSession)

	for range 2 {
		ch := make(chan prometheus.Metric, 1024)

		cacheScrape := scrapecache.Begin(c)
		err := c.Collect(ch, time.Second)
		cacheScrape.End(err)
		require.NoError(t, err)
		close(ch)

		for metric := range ch {
			desc := metric.Desc().String()
			assert.NotContains(t, desc, `"windows_terminal_services_handles"`)
			assert.True(t,
				strings.Contains(desc, `"windows_terminal_services_session_info"`) ||
					strings.Contains(desc, `"windows_terminal_services_connection_broker_`),
				"unexpected metric %s", desc)
		}
	}
}

// TestNewQueryCollectorAnyInstanceType covers single-instance v2 counter sets
// such as the Remote Desktop Connection Broker Counterset, whose wildcard
// instance path PDH rejects. Event Tracing for Windows is such a counter set
// on every host.
func TestNewQueryCollectorAnyInstanceType(t *testing.T) {
	t.Parallel()

	type eventTracing struct {
		ActiveSessions float64 `perfdata:"Total Number of Active Sessions"`
	}

	type process struct {
		Name string

		ThreadCount float64 `perfdata:"Thread Count"`
	}

	logger := slog.New(slog.DiscardHandler)

	query, err := pdh.NewQuery()
	require.NoError(t, err)

	t.Cleanup(query.Close)

	_, err = pdh.NewQueryCollector[eventTracing](logger, query, pdh.CounterTypeRaw, "Event Tracing for Windows", pdh.InstancesAll)
	require.Error(t, err, "PDH accepted the wildcard path of a single-instance counter set")

	singleInstance, err := newQueryCollectorAnyInstanceType[eventTracing](logger, query, "Event Tracing for Windows")
	require.NoError(t, err)

	t.Cleanup(singleInstance.Close)

	multiInstance, err := newQueryCollectorAnyInstanceType[process](logger, query, "Process")
	require.NoError(t, err)

	t.Cleanup(multiInstance.Close)

	_, err = newQueryCollectorAnyInstanceType[process](logger, query, "windows_exporter missing object")
	require.Error(t, err)

	require.NoError(t, query.Collect())

	var eventTracingData []eventTracing

	require.NoError(t, singleInstance.Collect(&eventTracingData))
	require.Len(t, eventTracingData, 1)
	require.Positive(t, eventTracingData[0].ActiveSessions)

	var processData []process

	require.NoError(t, multiInstance.Collect(&processData))
	require.Greater(t, len(processData), 1, "the wildcard path must collect every process")
}
