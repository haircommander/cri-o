package statsserver

//
//import (
//	"fmt"
//	"strconv"
//	"time"
//
//	"github.com/prometheus/client_golang/prometheus"
//	"k8s.io/utils/clock"
//)
//
//// cadvisormetrics.CpuUsageMetrics
//// cadvisormetrics.MemoryUsageMetrics
//// cadvisormetrics.CpuLoadMetrics
//// cadvisormetrics.DiskIOMetrics
//// cadvisormetrics.DiskUsageMetrics
//// cadvisormetrics.NetworkUsageMetrics
//// cadvisormetrics.ProcessMetrics
//
//// PrometheusCollector implements prometheus.Collector.
//type PrometheusCollector struct {
//}
//
//// NewPrometheusCollector returns a new PrometheusCollector. The passed
//// ContainerLabelsFunc specifies which base labels will be attached to all
//// exported metrics. If left to nil, the DefaultContainerLabels function
//// will be used instead.
//func NewPrometheusCollector(now clock.Clock) *PrometheusCollector {
//	c := &PrometheusCollector{
//		errors: prometheus.NewGauge(prometheus.GaugeOpts{
//			Namespace: "container",
//			Name:      "scrape_error",
//			Help:      "1 if there was an error while getting container metrics, 0 otherwise",
//		}),
//		containerMetrics: []containerMetric{
//			{
//				name:      "container_last_seen",
//				help:      "Last time a container was seen by the exporter",
//				valueType: prometheus.GaugeValue,
//				getValues: func(s *info.ContainerStats) metricValues {
//					return metricValues{{
//						value:     float64(now.Now().Unix()),
//						timestamp: now.Now(),
//					}}
//				},
//			},
//		},
//		includedMetrics: includedMetrics,
//		opts:            opts,
//	}
//	// container.CpuUsageMetrics
//	c.containerMetrics = append(c.containerMetrics, []containerMetric{
//		{
//			name:      "container_cpu_user_seconds_total",
//			help:      "Cumulative user cpu time consumed in seconds.",
//			valueType: prometheus.CounterValue,
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{
//					{
//						value:     float64(s.Cpu.Usage.User) / float64(time.Second),
//						timestamp: s.Timestamp,
//					},
//				}
//			},
//		}, {
//			name:      "container_cpu_system_seconds_total",
//			help:      "Cumulative system cpu time consumed in seconds.",
//			valueType: prometheus.CounterValue,
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{
//					{
//						value:     float64(s.Cpu.Usage.System) / float64(time.Second),
//						timestamp: s.Timestamp,
//					},
//				}
//			},
//		}, {
//			name:        "container_cpu_usage_seconds_total",
//			help:        "Cumulative cpu time consumed in seconds.",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"cpu"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				if len(s.Cpu.Usage.PerCpu) == 0 {
//					if s.Cpu.Usage.Total > 0 {
//						return metricValues{{
//							value:     float64(s.Cpu.Usage.Total) / float64(time.Second),
//							labels:    []string{"total"},
//							timestamp: s.Timestamp,
//						}}
//					}
//				}
//				values := make(metricValues, 0, len(s.Cpu.Usage.PerCpu))
//				for i, value := range s.Cpu.Usage.PerCpu {
//					if value > 0 {
//						values = append(values, metricValue{
//							value:     float64(value) / float64(time.Second),
//							labels:    []string{fmt.Sprintf("cpu%02d", i)},
//							timestamp: s.Timestamp,
//						})
//					}
//				}
//				return values
//			},
//		}, {
//			name:      "container_cpu_cfs_periods_total",
//			help:      "Number of elapsed enforcement period intervals.",
//			valueType: prometheus.CounterValue,
//			condition: func(s info.ContainerSpec) bool { return s.Cpu.Quota != 0 },
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{
//					{
//						value:     float64(s.Cpu.CFS.Periods),
//						timestamp: s.Timestamp,
//					}}
//			},
//		}, {
//			name:      "container_cpu_cfs_throttled_periods_total",
//			help:      "Number of throttled period intervals.",
//			valueType: prometheus.CounterValue,
//			condition: func(s info.ContainerSpec) bool { return s.Cpu.Quota != 0 },
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{
//					{
//						value:     float64(s.Cpu.CFS.ThrottledPeriods),
//						timestamp: s.Timestamp,
//					}}
//			},
//		}, {
//			name:      "container_cpu_cfs_throttled_seconds_total",
//			help:      "Total time duration the container has been throttled.",
//			valueType: prometheus.CounterValue,
//			condition: func(s info.ContainerSpec) bool { return s.Cpu.Quota != 0 },
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{
//					{
//						value:     float64(s.Cpu.CFS.ThrottledTime) / float64(time.Second),
//						timestamp: s.Timestamp,
//					}}
//			},
//		},
//	}...)
//	// container.CpuLoadMetrics
//	c.containerMetrics = append(c.containerMetrics, []containerMetric{
//		{
//			name:      "container_cpu_load_average_10s",
//			help:      "Value of container cpu load average over the last 10 seconds.",
//			valueType: prometheus.GaugeValue,
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{{value: float64(s.Cpu.LoadAverage), timestamp: s.Timestamp}}
//			},
//		}, {
//			name:        "container_tasks_state",
//			help:        "Number of tasks in given state",
//			extraLabels: []string{"state"},
//			valueType:   prometheus.GaugeValue,
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{
//					{
//						value:     float64(s.TaskStats.NrSleeping),
//						labels:    []string{"sleeping"},
//						timestamp: s.Timestamp,
//					},
//					{
//						value:     float64(s.TaskStats.NrRunning),
//						labels:    []string{"running"},
//						timestamp: s.Timestamp,
//					},
//					{
//						value:     float64(s.TaskStats.NrStopped),
//						labels:    []string{"stopped"},
//						timestamp: s.Timestamp,
//					},
//					{
//						value:     float64(s.TaskStats.NrUninterruptible),
//						labels:    []string{"uninterruptible"},
//						timestamp: s.Timestamp,
//					},
//					{
//						value:     float64(s.TaskStats.NrIoWait),
//						labels:    []string{"iowaiting"},
//						timestamp: s.Timestamp,
//					},
//				}
//			},
//		},
//	}...)
//	// container.MemoryUsageMetrics
//	c.containerMetrics = append(c.containerMetrics, []containerMetric{
//		{
//			name:      "container_memory_cache",
//			help:      "Number of bytes of page cache memory.",
//			valueType: prometheus.GaugeValue,
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{{value: float64(s.Memory.Cache), timestamp: s.Timestamp}}
//			},
//		}, {
//			name:      "container_memory_rss",
//			help:      "Size of RSS in bytes.",
//			valueType: prometheus.GaugeValue,
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{{value: float64(s.Memory.RSS), timestamp: s.Timestamp}}
//			},
//		}, {
//			name:      "container_memory_mapped_file",
//			help:      "Size of memory mapped files in bytes.",
//			valueType: prometheus.GaugeValue,
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{{value: float64(s.Memory.MappedFile), timestamp: s.Timestamp}}
//			},
//		}, {
//			name:      "container_memory_swap",
//			help:      "Container swap usage in bytes.",
//			valueType: prometheus.GaugeValue,
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{{value: float64(s.Memory.Swap), timestamp: s.Timestamp}}
//			},
//		}, {
//			name:      "container_memory_failcnt",
//			help:      "Number of memory usage hits limits",
//			valueType: prometheus.CounterValue,
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{{
//					value:     float64(s.Memory.Failcnt),
//					timestamp: s.Timestamp,
//				}}
//			},
//		}, {
//			name:      "container_memory_usage_bytes",
//			help:      "Current memory usage in bytes, including all memory regardless of when it was accessed",
//			valueType: prometheus.GaugeValue,
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{{value: float64(s.Memory.Usage), timestamp: s.Timestamp}}
//			},
//		},
//		{
//			name:      "container_memory_max_usage_bytes",
//			help:      "Maximum memory usage recorded in bytes",
//			valueType: prometheus.GaugeValue,
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{{value: float64(s.Memory.MaxUsage), timestamp: s.Timestamp}}
//			},
//		}, {
//			name:      "container_memory_working_set_bytes",
//			help:      "Current working set in bytes.",
//			valueType: prometheus.GaugeValue,
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{{value: float64(s.Memory.WorkingSet), timestamp: s.Timestamp}}
//			},
//		},
//		{
//			name:        "container_memory_failures_total",
//			help:        "Cumulative count of memory allocation failures.",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"failure_type", "scope"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{
//					{
//						value:     float64(s.Memory.ContainerData.Pgfault),
//						labels:    []string{"pgfault", "container"},
//						timestamp: s.Timestamp,
//					},
//					{
//						value:     float64(s.Memory.ContainerData.Pgmajfault),
//						labels:    []string{"pgmajfault", "container"},
//						timestamp: s.Timestamp,
//					},
//					{
//						value:     float64(s.Memory.HierarchicalData.Pgfault),
//						labels:    []string{"pgfault", "hierarchy"},
//						timestamp: s.Timestamp,
//					},
//					{
//						value:     float64(s.Memory.HierarchicalData.Pgmajfault),
//						labels:    []string{"pgmajfault", "hierarchy"},
//						timestamp: s.Timestamp,
//					},
//				}
//			},
//		},
//	}...)
//	// container.DiskUsageMetrics
//	c.containerMetrics = append(c.containerMetrics, []containerMetric{
//		{
//			name:        "container_fs_inodes_free",
//			help:        "Number of available Inodes",
//			valueType:   prometheus.GaugeValue,
//			extraLabels: []string{"device"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				return fsValues(s.Filesystem, func(fs *info.FsStats) float64 {
//					return float64(fs.InodesFree)
//				}, s.Timestamp)
//			},
//		}, {
//			name:        "container_fs_inodes_total",
//			help:        "Number of Inodes",
//			valueType:   prometheus.GaugeValue,
//			extraLabels: []string{"device"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				return fsValues(s.Filesystem, func(fs *info.FsStats) float64 {
//					return float64(fs.Inodes)
//				}, s.Timestamp)
//			},
//		}, {
//			name:        "container_fs_limit_bytes",
//			help:        "Number of bytes that can be consumed by the container on this filesystem.",
//			valueType:   prometheus.GaugeValue,
//			extraLabels: []string{"device"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				return fsValues(s.Filesystem, func(fs *info.FsStats) float64 {
//					return float64(fs.Limit)
//				}, s.Timestamp)
//			},
//		}, {
//			name:        "container_fs_usage_bytes",
//			help:        "Number of bytes that are consumed by the container on this filesystem.",
//			valueType:   prometheus.GaugeValue,
//			extraLabels: []string{"device"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				return fsValues(s.Filesystem, func(fs *info.FsStats) float64 {
//					return float64(fs.Usage)
//				}, s.Timestamp)
//			},
//		},
//	}...)
//	// container.DiskIOMetrics
//	c.containerMetrics = append(c.containerMetrics, []containerMetric{
//		{
//			name:        "container_fs_reads_bytes_total",
//			help:        "Cumulative count of bytes read",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"device"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				return ioValues(
//					s.DiskIo.IoServiceBytes, "Read", asFloat64,
//					nil, nil,
//					s.Timestamp,
//				)
//			},
//		}, {
//			name:        "container_fs_reads_total",
//			help:        "Cumulative count of reads completed",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"device"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				return ioValues(
//					s.DiskIo.IoServiced, "Read", asFloat64,
//					s.Filesystem, func(fs *info.FsStats) float64 {
//						return float64(fs.ReadsCompleted)
//					},
//					s.Timestamp,
//				)
//			},
//		}, {
//			name:        "container_fs_sector_reads_total",
//			help:        "Cumulative count of sector reads completed",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"device"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				return ioValues(
//					s.DiskIo.Sectors, "Read", asFloat64,
//					s.Filesystem, func(fs *info.FsStats) float64 {
//						return float64(fs.SectorsRead)
//					},
//					s.Timestamp,
//				)
//			},
//		}, {
//			name:        "container_fs_reads_merged_total",
//			help:        "Cumulative count of reads merged",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"device"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				return ioValues(
//					s.DiskIo.IoMerged, "Read", asFloat64,
//					s.Filesystem, func(fs *info.FsStats) float64 {
//						return float64(fs.ReadsMerged)
//					},
//					s.Timestamp,
//				)
//			},
//		}, {
//			name:        "container_fs_read_seconds_total",
//			help:        "Cumulative count of seconds spent reading",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"device"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				return ioValues(
//					s.DiskIo.IoServiceTime, "Read", asNanosecondsToSeconds,
//					s.Filesystem, func(fs *info.FsStats) float64 {
//						return float64(fs.ReadTime) / float64(time.Second)
//					},
//					s.Timestamp,
//				)
//			},
//		}, {
//			name:        "container_fs_writes_bytes_total",
//			help:        "Cumulative count of bytes written",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"device"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				return ioValues(
//					s.DiskIo.IoServiceBytes, "Write", asFloat64,
//					nil, nil,
//					s.Timestamp,
//				)
//			},
//		}, {
//			name:        "container_fs_writes_total",
//			help:        "Cumulative count of writes completed",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"device"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				return ioValues(
//					s.DiskIo.IoServiced, "Write", asFloat64,
//					s.Filesystem, func(fs *info.FsStats) float64 {
//						return float64(fs.WritesCompleted)
//					},
//					s.Timestamp,
//				)
//			},
//		}, {
//			name:        "container_fs_sector_writes_total",
//			help:        "Cumulative count of sector writes completed",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"device"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				return ioValues(
//					s.DiskIo.Sectors, "Write", asFloat64,
//					s.Filesystem, func(fs *info.FsStats) float64 {
//						return float64(fs.SectorsWritten)
//					},
//					s.Timestamp,
//				)
//			},
//		}, {
//			name:        "container_fs_writes_merged_total",
//			help:        "Cumulative count of writes merged",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"device"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				return ioValues(
//					s.DiskIo.IoMerged, "Write", asFloat64,
//					s.Filesystem, func(fs *info.FsStats) float64 {
//						return float64(fs.WritesMerged)
//					},
//					s.Timestamp,
//				)
//			},
//		}, {
//			name:        "container_fs_write_seconds_total",
//			help:        "Cumulative count of seconds spent writing",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"device"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				return ioValues(
//					s.DiskIo.IoServiceTime, "Write", asNanosecondsToSeconds,
//					s.Filesystem, func(fs *info.FsStats) float64 {
//						return float64(fs.WriteTime) / float64(time.Second)
//					},
//					s.Timestamp,
//				)
//			},
//		}, {
//			name:        "container_fs_io_current",
//			help:        "Number of I/Os currently in progress",
//			valueType:   prometheus.GaugeValue,
//			extraLabels: []string{"device"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				return ioValues(
//					s.DiskIo.IoQueued, "Total", asFloat64,
//					s.Filesystem, func(fs *info.FsStats) float64 {
//						return float64(fs.IoInProgress)
//					},
//					s.Timestamp,
//				)
//			},
//		}, {
//			name:        "container_fs_io_time_seconds_total",
//			help:        "Cumulative count of seconds spent doing I/Os",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"device"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				return ioValues(
//					s.DiskIo.IoServiceTime, "Total", asNanosecondsToSeconds,
//					s.Filesystem, func(fs *info.FsStats) float64 {
//						return float64(float64(fs.IoTime) / float64(time.Second))
//					},
//					s.Timestamp,
//				)
//			},
//		}, {
//			name:        "container_fs_io_time_weighted_seconds_total",
//			help:        "Cumulative weighted I/O time in seconds",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"device"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				return fsValues(s.Filesystem, func(fs *info.FsStats) float64 {
//					return float64(fs.WeightedIoTime) / float64(time.Second)
//				}, s.Timestamp)
//			},
//		},
//		{
//			name:        "container_blkio_device_usage_total",
//			help:        "Blkio Device bytes usage",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"device", "major", "minor", "operation"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				var values metricValues
//				for _, diskStat := range s.DiskIo.IoServiceBytes {
//					for operation, value := range diskStat.Stats {
//						values = append(values, metricValue{
//							value: float64(value),
//							labels: []string{diskStat.Device,
//								strconv.Itoa(int(diskStat.Major)),
//								strconv.Itoa(int(diskStat.Minor)),
//								operation},
//							timestamp: s.Timestamp,
//						})
//					}
//				}
//				return values
//			},
//		},
//	}...)
//	// container.NetworkUsageMetrics
//	c.containerMetrics = append(c.containerMetrics, []containerMetric{
//		{
//			name:        "container_network_receive_bytes_total",
//			help:        "Cumulative count of bytes received",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"interface"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				values := make(metricValues, 0, len(s.Network.Interfaces))
//				for _, value := range s.Network.Interfaces {
//					values = append(values, metricValue{
//						value:     float64(value.RxBytes),
//						labels:    []string{value.Name},
//						timestamp: s.Timestamp,
//					})
//				}
//				return values
//			},
//		}, {
//			name:        "container_network_receive_packets_total",
//			help:        "Cumulative count of packets received",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"interface"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				values := make(metricValues, 0, len(s.Network.Interfaces))
//				for _, value := range s.Network.Interfaces {
//					values = append(values, metricValue{
//						value:     float64(value.RxPackets),
//						labels:    []string{value.Name},
//						timestamp: s.Timestamp,
//					})
//				}
//				return values
//			},
//		}, {
//			name:        "container_network_receive_packets_dropped_total",
//			help:        "Cumulative count of packets dropped while receiving",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"interface"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				values := make(metricValues, 0, len(s.Network.Interfaces))
//				for _, value := range s.Network.Interfaces {
//					values = append(values, metricValue{
//						value:     float64(value.RxDropped),
//						labels:    []string{value.Name},
//						timestamp: s.Timestamp,
//					})
//				}
//				return values
//			},
//		}, {
//			name:        "container_network_receive_errors_total",
//			help:        "Cumulative count of errors encountered while receiving",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"interface"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				values := make(metricValues, 0, len(s.Network.Interfaces))
//				for _, value := range s.Network.Interfaces {
//					values = append(values, metricValue{
//						value:     float64(value.RxErrors),
//						labels:    []string{value.Name},
//						timestamp: s.Timestamp,
//					})
//				}
//				return values
//			},
//		}, {
//			name:        "container_network_transmit_bytes_total",
//			help:        "Cumulative count of bytes transmitted",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"interface"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				values := make(metricValues, 0, len(s.Network.Interfaces))
//				for _, value := range s.Network.Interfaces {
//					values = append(values, metricValue{
//						value:     float64(value.TxBytes),
//						labels:    []string{value.Name},
//						timestamp: s.Timestamp,
//					})
//				}
//				return values
//			},
//		}, {
//			name:        "container_network_transmit_packets_total",
//			help:        "Cumulative count of packets transmitted",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"interface"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				values := make(metricValues, 0, len(s.Network.Interfaces))
//				for _, value := range s.Network.Interfaces {
//					values = append(values, metricValue{
//						value:     float64(value.TxPackets),
//						labels:    []string{value.Name},
//						timestamp: s.Timestamp,
//					})
//				}
//				return values
//			},
//		}, {
//			name:        "container_network_transmit_packets_dropped_total",
//			help:        "Cumulative count of packets dropped while transmitting",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"interface"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				values := make(metricValues, 0, len(s.Network.Interfaces))
//				for _, value := range s.Network.Interfaces {
//					values = append(values, metricValue{
//						value:     float64(value.TxDropped),
//						labels:    []string{value.Name},
//						timestamp: s.Timestamp,
//					})
//				}
//				return values
//			},
//		}, {
//			name:        "container_network_transmit_errors_total",
//			help:        "Cumulative count of errors encountered while transmitting",
//			valueType:   prometheus.CounterValue,
//			extraLabels: []string{"interface"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				values := make(metricValues, 0, len(s.Network.Interfaces))
//				for _, value := range s.Network.Interfaces {
//					values = append(values, metricValue{
//						value:     float64(value.TxErrors),
//						labels:    []string{value.Name},
//						timestamp: s.Timestamp,
//					})
//				}
//				return values
//			},
//		},
//	}...)
//	// container.ProcessMetrics
//	c.containerMetrics = append(c.containerMetrics, []containerMetric{
//		{
//			name:      "container_processes",
//			help:      "Number of processes running inside the container.",
//			valueType: prometheus.GaugeValue,
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{{value: float64(s.Processes.ProcessCount), timestamp: s.Timestamp}}
//			},
//		},
//		{
//			name:      "container_file_descriptors",
//			help:      "Number of open file descriptors for the container.",
//			valueType: prometheus.GaugeValue,
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{{value: float64(s.Processes.FdCount), timestamp: s.Timestamp}}
//			},
//		},
//		{
//			name:      "container_sockets",
//			help:      "Number of open sockets for the container.",
//			valueType: prometheus.GaugeValue,
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{{value: float64(s.Processes.SocketCount), timestamp: s.Timestamp}}
//			},
//		},
//		{
//			name:      "container_threads_max",
//			help:      "Maximum number of threads allowed inside the container, infinity if value is zero",
//			valueType: prometheus.GaugeValue,
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{
//					{
//						value:     float64(s.Processes.ThreadsMax),
//						timestamp: s.Timestamp,
//					},
//				}
//			},
//		},
//		{
//			name:      "container_threads",
//			help:      "Number of threads running inside the container",
//			valueType: prometheus.GaugeValue,
//			getValues: func(s *info.ContainerStats) metricValues {
//				return metricValues{
//					{
//						value:     float64(s.Processes.ThreadsCurrent),
//						timestamp: s.Timestamp,
//					},
//				}
//			},
//		},
//		{
//			name:        "container_ulimits_soft",
//			help:        "Soft ulimit values for the container root process. Unlimited if -1, except priority and nice",
//			valueType:   prometheus.GaugeValue,
//			extraLabels: []string{"ulimit"},
//			getValues: func(s *info.ContainerStats) metricValues {
//				values := make(metricValues, 0, len(s.Processes.Ulimits))
//				for _, ulimit := range s.Processes.Ulimits {
//					values = append(values, metricValue{
//						value:     float64(ulimit.SoftLimit),
//						labels:    []string{ulimit.Name},
//						timestamp: s.Timestamp,
//					})
//				}
//				return values
//			},
//		},
//	}...)
//
//	return c
//}
