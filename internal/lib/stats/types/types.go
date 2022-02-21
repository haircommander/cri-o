package statstypes

import types "k8s.io/cri-api/pkg/apis/runtime/v1"

type ContainerStats struct {
	CRIStats      *types.ContainerStats
	Spec          *SpecUsage
	CpuLoad       *CpuLoadUsage
	Cpu           *CpuUsage
	Memory        *MemoryUsage
	WritableLayer *FilesystemUsage
	// TODO FIXME: do we need ProcessUsage?
}

type PodSandboxStats struct {
	CRIStats *types.PodSandboxStats
	Spec     *SpecUsage
	CpuLoad  *CpuLoadUsage
	Cpu      *CpuUsage
	Memory   *MemoryUsage
	Network  *NetworkUsage
	Process  *ProcessUsage
}

// CpuUsageMetrics
type CpuUsage struct {
	// Corresponds to no cAdvisor metrics
	CRIUsage *types.CpuUsage
	// Corresponds to cAdvisor container_cpu_cfs_periods_total
	CfsPeriodsTotal uint64
	// Corresponds to cAdvisor container_cpu_cfs_throttled_seconds_total
	CfsThrottledPeriodsTotal uint64
	// Corresponds to cAdvisor container_cpu_cfs_periods_total
	CfsThrottledSecondsTotal uint64
	// Corresponds to cAdvisor container_cpu_system_seconds_total
	SystemSecondsTotal uint64
	// Corresponds to cAdvisor container_cpu_usage_seconds_total
	UsageSecondsTotal uint64
	// Corresponds to cAdvisor container_cpu_user_seconds_total
	UserSecondsTotal uint64
}

// cadvisormetrics.CpuLoadMetrics
type CpuLoadUsage struct {
	// Corresponds to cAdvisor container_cpu_load_average_10s
	LoadAverage10s uint64
	// Corresponds to cAdvisor container_tasks_state
	TasksState uint64
}

// cadvisormetrics.MemoryUsageMetrics
type MemoryUsage struct {
	// Contains cAdvisor container_memory_rss, container_memory_usage_bytes, container_memory_working_set_bytes
	CRIUsage *types.MemoryUsage
	// Corresponds to cAdvisor container_memory_cache
	Cache uint64
	// Corresponds to cAdvisor container_memory_failcnt
	Failcnt uint64
	// Corresponds to cAdvisor container_memory_failures_total
	FailuresTotal uint64
	// Corresponds to cAdvisor container_memory_mapped_file
	MappedFile uint64
	// Corresponds to cAdvisor container_memory_max_usage_bytes
	MaxUsageBytes uint64
	// Corresponds to cAdvisor container_memory_swap
	Swap uint64
}

// FilesystemUsage provides the filesystem usage information.
// container.DiskUsageMetrics
type FilesystemUsage struct {
	// Contains cAdvsior container_fs_usage_bytes
	CRIUsage *types.FilesystemUsage
	// Corresponds to cAdvisor container_fs_limit_bytes
	LimitBytes uint64
	// Corresponds to cAdvisor container_fs_inodes_free
	InodesFree uint64
	// Corresponds to cAdvisor container_fs_inodes_total
	InodesTotal uint64
}

// container.DiskIOMetrics
type DiskIOUsage struct {
	// Corresponds to cAdvisor container_fs_io_current
	IoCurrent uint64
	// Corresponds to cAdvisor container_fs_io_time_seconds_total
	IoTimeSecondsTotal uint64
	// Corresponds to cAdvisor container_fs_io_time_weighted_seconds_total
	IoTimeWeightedSecondsTotal uint64
	// Corresponds to cAdvisor container_fs_read_seconds_total
	ReadSecondsTotal uint64
	// Corresponds to cAdvisor container_fs_reads_bytes_total
	ReadsBytesTotal uint64
	// Corresponds to cAdvisor container_fs_reads_merged_total
	ReadsMergedTotal uint64
	// Corresponds to cAdvisor container_fs_reads_total
	ReadsTotal uint64
	// Corresponds to cAdvisor container_fs_sector_reads_total
	SectorReadsTotal uint64
	// Corresponds to cAdvisor container_fs_sector_writes_total
	SectorWritesTotal uint64
	// Corresponds to cAdvisor container_fs_write_seconds_total
	WriteSecondsTotal uint64
	// Corresponds to cAdvisor container_fs_writes_bytes_total
	WritesBytesTotal uint64
	// Corresponds to cAdvisor container_fs_writes_merged_total
	WritesMergedTotal uint64
	// Corresponds to cAdvisor container_fs_writes_total
	WritesTotal uint64
	// Corresponds to cAdvisor container_blkio_device_usage_total
	BlkioDeviceUsageTotal uint64
}

type NetworkUsage struct {
	// Contains cAdvsior container_network_receive_bytes_total, container_network_receive_errors_total,
	// container_network_transmit_bytes_total, container_network_transmit_errors_total.
	CRIUsage *types.NetworkUsage
	// Corresponds to cAdvisor container_network_receive_packets_dropped_total
	RxPacketsDroppedTotal uint64
	// Corresponds to cAdvisor container_network_receive_packets_total
	RxPacketsTotal uint64
	// Corresponds to cAdvisor container_network_transmit_packets_dropped_total
	TxPacketsDroppedTotal uint64
	// Corresponds to cAdvisor container_network_transmit_packets_total
	TxPacketsTotal uint64
}

type ProcessUsage struct {
	// Contains cAdvsior container_processes
	CRIUsage *types.ProcessUsage
	// Corresponds to cAdvisor container_file_descriptors
	FileDescriptors uint64
	// Corresponds to cAdvisor container_sockets
	Sockets uint64
	// Corresponds to cAdvisor container_threads_max
	ThreadsMax uint64
	// Corresponds to cAdvisor container_threads
	Threads uint64
	// Corresponds to cAdvisor container_ulimits_soft
	UlimitsSoft uint64
}

type SpecUsage struct {
	// Corresponds to cAdvisor container_start_time_seconds
	StartTimeSeconds uint64
	// Corresponds to cAdvisor container_spec_cpu_period
	CpuPeriod uint64
	// Corresponds to cAdvisor container_spec_cpu_quota
	CpuQuota uint64
	// Corresponds to cAdvisor container_spec_cpu_shares
	CpuShares uint64
	// Corresponds to cAdvisor container_spec_memory_limit_bytes
	MemoryLimitBytes uint64
	// Corresponds to cAdvisor container_spec_memory_reservation_limit_bytes
	MemoryReservationLimitBytes uint64
	// Corresponds to cAdvisor container_spec_memory_swap_limit_bytes
	MemorySwapLimitBytes uint64
}
