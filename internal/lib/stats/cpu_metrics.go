package statsserver

import (
	"fmt"
	"time"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/oci"
)

func generateContainerCPUMetrics(ctr *oci.Container, cpu *cgmgr.CPUStats) []*types.Metric {
	cpuMetrics := []*containerMetric{
		{
			desc: containerCpuUserSecondsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.UsageInUsermode / uint64(time.Second),
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerCpuSystemSecondsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.UsageInKernelmode / uint64(time.Second),
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerCpuUsageSecondsTotal,
			valueFunc: func() metricValues {
				if len(cpu.PerCPUUsage) == 0 && cpu.TotalUsageNano > 0 {
					return metricValues{{
						value:      cpu.TotalUsageNano / uint64(time.Second),
						labels:     []string{"total"},
						metricType: types.MetricType_COUNTER,
					}}
				}

				metricValues := make(metricValues, 0, len(cpu.PerCPUUsage))
				for i, value := range cpu.PerCPUUsage {
					if value > 0 {
						metricValues = append(metricValues, metricValue{
							value:      value / uint64(time.Second),
							labels:     []string{fmt.Sprintf("cpu%02d", i)},
							metricType: types.MetricType_COUNTER,
						})
					}
				}

				return metricValues
			},
		}, {
			desc: containerCpuCfsPeriodsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.ThrottlingActivePeriods,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerCpuCfsThrottledPeriodsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.ThrottledPeriods,
					metricType: types.MetricType_COUNTER,
				}}
			},
		}, {
			desc: containerCpuCfsThrottledSecondsTotal,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.ThrottledTime / uint64(time.Second),
					metricType: types.MetricType_COUNTER,
				}}
			},
		},
	}

	return computeContainerMetrics(ctr, cpuMetrics, "cpu")
}

func generateContainerCPULoadMetrics(ctr *oci.Container, cpu *cgmgr.CPUStats, load *cgmgr.CPULoadStats) []*types.Metric {
	cpuLoadMetrics := []*containerMetric{
		{
			desc: containerCpuLoadAverage10s,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.LoadAverage,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
		{
			desc: containerCpuLoadDAverage10s,
			valueFunc: func() metricValues {
				return metricValues{{
					value:      cpu.LoadDAverage,
					metricType: types.MetricType_GAUGE,
				}}
			},
		},
		{
			desc: containerTasksState,
			valueFunc: func() metricValues {
				if load == nil {
					return metricValues{}
				}
				return metricValues{{
					value:      load.NrSleeping,
					metricType: types.MetricType_GAUGE,
					labels:     []string{"sleeping"},
				}, {
					value:      load.NrRunning,
					metricType: types.MetricType_GAUGE,
					labels:     []string{"running"},
				}, {
					value:      load.NrStopped,
					metricType: types.MetricType_GAUGE,
					labels:     []string{"stopped"},
				}, {
					value:      load.NrUninterruptible,
					metricType: types.MetricType_GAUGE,
					labels:     []string{"uninterruptible"},
				}, {
					value:      load.NrIoWait,
					metricType: types.MetricType_GAUGE,
					labels:     []string{"iowaiting"},
				}}
			},
		},
	}
	return computeContainerMetrics(ctr, cpuLoadMetrics, "cpuLoad")
}
