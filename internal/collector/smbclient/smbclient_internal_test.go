// SPDX-License-Identifier: MIT
//
// Copyright The Prometheus Authors

//go:build windows

package smbclient

import (
	"log/slog"
	"math"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func TestParseInstanceName(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		server string
		share  string
		ok     bool
	}{
		{name: `\\fileserver\data`, server: "fileserver", share: "data", ok: true},
		// Windows 11 publishes the instances with a single leading backslash.
		{name: `\localhost\C$`, server: "localhost", share: "C$", ok: true},
		{name: `\\domain\dfs\folder`},
		{name: `_Total`},
		{name: ``},
	} {
		server, share, ok := parseInstanceName(tc.name)

		require.Equal(t, tc.ok, ok, tc.name)
		require.Equal(t, tc.server, server, tc.name)
		require.Equal(t, tc.share, share, tc.name)
	}
}

func TestExtendCounters32(t *testing.T) {
	t.Parallel()

	c := &Collector{counters32: make(map[string]*shareCounters32)}

	// internal/pdh delivers the timer sums in seconds of a 10 MHz time base.
	frequencies := [numTimers32]float64{1e7, 1e7, 1e7}

	sample := func(data perfDataCounterValues) perfDataCounterValues {
		c.generation++

		data.Name = `\\server\share`
		c.extendCounters32(&data, frequencies)
		c.pruneCounters32()

		return data
	}

	got := sample(perfDataCounterValues{AvgSecPerRead: (1 << 20) / 1e7, ReadRequestsPerSec: 7, ReadBytesPerSec: 100})
	require.Equal(t, (1<<20)/1e7, got.AvgSecPerRead) //nolint:testifylint
	require.Equal(t, 7.0, got.ReadRequestsPerSec)    //nolint:testifylint

	// A timer sum can wrap from anywhere: 2^32 ticks are about 430 s of
	// summed request time.
	got = sample(perfDataCounterValues{AvgSecPerRead: (1 << 10) / 1e7, ReadRequestsPerSec: 9, ReadBytesPerSec: 200})
	require.Equal(t, (float64(math.MaxUint32)+1+(1<<10))/1e7, got.AvgSecPerRead) //nolint:testifylint
	require.Equal(t, 9.0, got.ReadRequestsPerSec)                                //nolint:testifylint

	// Fractions of a second survive the round trip through ticks.
	got = sample(perfDataCounterValues{AvgSecPerRead: 0.1234567, ReadRequestsPerSec: 10, ReadBytesPerSec: 300})
	require.Equal(t, (float64(math.MaxUint32)+1+1234567)/1e7, got.AvgSecPerRead) //nolint:testifylint

	// A drop of a 64-bit byte counter shows a reconnect, even if DataBytes grew.
	got = sample(perfDataCounterValues{AvgSecPerRead: 5 / 1e7, ReadRequestsPerSec: 1, ReadBytesPerSec: 50, DataBytesPerSec: 1000})
	require.Equal(t, 5/1e7, got.AvgSecPerRead)    //nolint:testifylint
	require.Equal(t, 1.0, got.ReadRequestsPerSec) //nolint:testifylint

	// A metadata-only share has no byte counter to show the reconnect; a
	// request count dropping from below 2^31 is taken as one.
	got = sample(perfDataCounterValues{MetadataRequestsPerSec: 50, ReadBytesPerSec: 50, DataBytesPerSec: 1000})
	require.Equal(t, 50.0, got.MetadataRequestsPerSec) //nolint:testifylint

	got = sample(perfDataCounterValues{MetadataRequestsPerSec: 3, ReadBytesPerSec: 50, DataBytesPerSec: 1000})
	require.Equal(t, 3.0, got.MetadataRequestsPerSec) //nolint:testifylint
}

// publishCollector adapts a publish function to prometheus.Collector, so the
// test checks the metrics as a registry gathers them.
type publishCollector func(ch chan<- prometheus.Metric)

func (publishCollector) Describe(chan<- *prometheus.Desc) {}

func (p publishCollector) Collect(ch chan<- prometheus.Metric) {
	p(ch)
}

// TestCollectSharesSMBDirectTurboIO checks that a host without the SMB Direct
// and Turbo I/O counters, such as Windows Server 2016, publishes the other
// metrics of a share and omits these instead of publishing zeros.
func TestCollectSharesSMBDirectTurboIO(t *testing.T) {
	t.Parallel()

	smbDirectTurboIOMetrics := []string{
		"windows_smbclient_read_bytes_via_smbdirect_total",
		"windows_smbclient_read_requests_via_smbdirect_total",
		"windows_smbclient_turbo_io_reads_total",
		"windows_smbclient_turbo_io_writes_total",
		"windows_smbclient_write_bytes_via_smbdirect_total",
		"windows_smbclient_write_requests_via_smbdirect_total",
	}

	for _, smbDirectTurboIO := range []bool{true, false} {
		c := New(nil)
		c.logger = slog.New(slog.DiscardHandler)
		c.unexpectedInstances = make(map[string]struct{})
		c.counters32 = make(map[string]*shareCounters32)
		c.smbDirectTurboIO = smbDirectTurboIO
		c.buildDescriptors()

		c.perfDataObject = []perfDataCounterValues{{
			Name:                                   `\\fileserver\data`,
			ReadBytesPerSec:                        100,
			ReadBytesTransmittedViaSMBDirectPerSec: 40,
			TurboIOReadsPerSec:                     3,
		}}

		registry := prometheus.NewPedanticRegistry()
		require.NoError(t, registry.Register(publishCollector(c.collectShares)))

		families, err := registry.Gather()
		require.NoError(t, err)

		values := make(map[string]float64, len(families))

		for _, family := range families {
			require.Len(t, family.GetMetric(), 1, family.GetName())

			metric := family.GetMetric()[0]

			labels := make(map[string]string, len(metric.GetLabel()))
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}

			require.Equal(t, map[string]string{"server": "fileserver", "share": "data"}, labels, family.GetName())

			if family.GetType() == dto.MetricType_COUNTER {
				values[family.GetName()] = metric.GetCounter().GetValue()
			} else {
				values[family.GetName()] = metric.GetGauge().GetValue()
			}
		}

		require.InDelta(t, 100, values["windows_smbclient_read_bytes_total"], 0)

		if smbDirectTurboIO {
			require.Len(t, values, 21)
			require.InDelta(t, 40, values["windows_smbclient_read_bytes_via_smbdirect_total"], 0)
			require.InDelta(t, 3, values["windows_smbclient_turbo_io_reads_total"], 0)

			continue
		}

		require.Len(t, values, 21-len(smbDirectTurboIOMetrics))

		for _, name := range smbDirectTurboIOMetrics {
			require.NotContains(t, values, name)
		}
	}
}
