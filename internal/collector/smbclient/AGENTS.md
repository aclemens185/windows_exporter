# `smbclient` collector

The collector reads the `SMB Client Shares` PDH object with
`pdh.CounterTypeRaw`. It is multi-instance, with one instance per share the
host has connected to. These notes were recorded on a Windows Server 2025
(build 26100) Hyper-V guest that had mapped one share of its own SMB server.

- Instance names have a single leading backslash: `\SERVER2025\SmbClientTest`.
  The collector splits the name at `\` and uses the two parts as the `server`
  and `share` labels. The aggregate `_Total` is dropped by `internal/pdh`
  because the collector asks for `pdh.InstancesAll`.
- The mapped share was idle, so every counter of the sample instance was 0.
- `Current Data Queue Length` is the only gauge ("the current number of read
  or write requests outstanding"). The `/sec` counters are cumulative counts.
- `Avg. Data/Read/Write Queue Length` are `PERF_COUNTER_100NS_QUEUELEN_TYPE`:
  the raw value is accumulated queue time in 100 ns ticks, which
  `internal/pdh` converts to seconds.
- `Avg. sec/Read`, `Avg. sec/Write` and `Avg. sec/Data Request` are
  `PERF_AVERAGE_TIMER`. The first value is the summed latency, which
  `internal/pdh` converts to seconds with the counter time base, and the base
  is the request count; the collector reads only the first value. The raw
  sum is 32 bits, so the collector converts it back to ticks with
  `pdh.Collector.Frequency`, extends it past the 2^32 wrap with
  `utils.Counter32` and divides again. Keep that round trip: extending the
  seconds directly truncates them and adds 2^32 seconds per wrap.
- `Data Bytes/sec`, `Read Bytes/sec` and `Write Bytes/sec` are bytes
  (`PERF_COUNTER_BULK_COUNT`).
- Windows Server 2016 (build 14393) has 18 counters in `SMB Client Shares`.
  It lacks the four `... transmitted via SMB Direct/sec` counters and
  `Turbo I/O Reads/sec` and `Turbo I/O Writes/sec`, which Windows Server 2019
  (build 17763) has. Their fields carry `perfdata_min_build:"17763"`, and
  the collector publishes their metrics only if `pdh.Collector.MetricType`
  finds all six, so the zeros of skipped counters are never published.
- Windows Server 2016 also registers `SMB Client Shares` as a single-instance
  counter set (`Get-Counter -ListSet` reports `SingleInstance`; 2019 reports
  `MultiInstance`), although `PdhEnumObjectItems` lists the shares. The raw
  array of a `(*)` counter then has only the items `*` and `_Total`, so
  `internal/pdh` returns one row named `------` (`pdh.InstanceEmpty`), which
  `parseInstanceName` rejects. Paths with an explicit instance, such as
  `\SMB Client Shares(\SERVER\share)\Data Bytes/sec`, work; `(\*)` and
  `(\SERVER\*)` return no data.

## Counter reference

Recorded on Windows Server 2025 (build 26100). `Raw` and `Base` are the first and second
values that `pdh.CounterTypeRaw` returns for the sample instance.

### `SMB Client Shares`

Multi-instance. Instances: `\SERVER2025\SmbClientTest`, `_Total`. Sample instance: `\SERVER2025\SmbClientTest`.

Counters the collector reads:

| Counter | Type | Raw | Base |
|---|---|---|---|
| `Avg. Data Queue Length` | `PERF_COUNTER_100NS_QUEUELEN_TYPE` | 0 |  |
| `Avg. Read Queue Length` | `PERF_COUNTER_100NS_QUEUELEN_TYPE` | 0 |  |
| `Avg. sec/Data Request` | `PERF_AVERAGE_TIMER` | 0 | 0 |
| `Avg. sec/Read` | `PERF_AVERAGE_TIMER` | 0 | 0 |
| `Avg. sec/Write` | `PERF_AVERAGE_TIMER` | 0 | 0 |
| `Avg. Write Queue Length` | `PERF_COUNTER_100NS_QUEUELEN_TYPE` | 0 |  |
| `Credit Stalls/sec` | `PERF_COUNTER_COUNTER` | 0 |  |
| `Current Data Queue Length` | `PERF_COUNTER_RAWCOUNT` | 0 |  |
| `Data Bytes/sec` | `PERF_COUNTER_BULK_COUNT` | 0 |  |
| `Data Requests/sec` | `PERF_COUNTER_COUNTER` | 0 |  |
| `Metadata Requests/sec` | `PERF_COUNTER_COUNTER` | 0 |  |
| `Read Bytes transmitted via SMB Direct/sec` | `PERF_COUNTER_BULK_COUNT` | 0 |  |
| `Read Bytes/sec` | `PERF_COUNTER_BULK_COUNT` | 0 |  |
| `Read Requests transmitted via SMB Direct/sec` | `PERF_COUNTER_COUNTER` | 0 |  |
| `Read Requests/sec` | `PERF_COUNTER_COUNTER` | 0 |  |
| `Turbo I/O Reads/sec` | `PERF_COUNTER_COUNTER` | 0 |  |
| `Turbo I/O Writes/sec` | `PERF_COUNTER_COUNTER` | 0 |  |
| `Write Bytes transmitted via SMB Direct/sec` | `PERF_COUNTER_BULK_COUNT` | 0 |  |
| `Write Bytes/sec` | `PERF_COUNTER_BULK_COUNT` | 0 |  |
| `Write Requests transmitted via SMB Direct/sec` | `PERF_COUNTER_COUNTER` | 0 |  |
| `Write Requests/sec` | `PERF_COUNTER_COUNTER` | 0 |  |

Not read by the collector:

- `PERF_AVERAGE_BULK`: `Avg. Bytes/Read`, `Avg. Bytes/Write`, `Avg. Data Bytes/Request`
- `PERF_COUNTER_COUNTER`: `Attempted Compressed Requests/sec`, `Compressed Bytes Sent/sec`, `Compressed Responses/sec`, `Successful Compressed Requests /sec`
