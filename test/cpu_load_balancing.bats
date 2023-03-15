#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	export activation="cpu-load-balancing.crio.io"
	setup_test
	sboxconfig="$TESTDIR/sbox.json"
	ctrconfig="$TESTDIR/ctr.json"
	shares="200"
	export cpuset="0-1"
	create_workload "$shares" "$cpuset"
}

function teardown() {
	cleanup_test
}

function create_workload() {
	local cpushares="$1"
	local cpuset="$2"
	cat << EOF > "$CRIO_CONFIG_DIR/01-workload.conf"
[crio.runtime.workloads.management]
activation_annotation = "$activation"
annotation_prefix = "$prefix"
[crio.runtime.workloads.management.resources]
cpushares =  $cpushares
cpuset = "$cpuset"
EOF
}

function check_sched_load_balance() {
	local pid="$1"
	local is_enabled="$2"

	if is_cgroup_v2; then
		return
	fi
	cpuset_basepath="/sys/fs/cgroup/cpuset"
	loadbalance_filename="/cpuset.sched_load_balance"

	cgroup="$cpuset_basepath"$(cat /proc/$pid/cgroup | grep cpuset |  tr ":" " " | awk '{ printf $3 }')"$loadbalance_filename"
	cat $cgroup
	[[ "$is_enabled" == "$(cat $cgroup)" ]]
}

@test "test cpu load balancing" {
	start_crio

	# setup container with annotation
	jq --arg act "$activation" --arg set "$cpuset" \
		' .annotations[$act] = "true"
		| .linux.resources.cpuset_cpus= $set' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"


	jq --arg act "$activation" --arg set "$cpuset" \
		' .annotations[$act] = "true"
		| .linux.resources.cpuset_cpus = $set' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	# run container
	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")

	# get pid of the container process
	ctr_pid=$(crictl inspect "$ctr_id" | jq .info.pid)

	# get process affinity (cpu) list
	affinity_list=$(taskset -pc $ctr_pid | cut -d ':' -f 2 | sed -e 's/^[[:space:]]*//' | sed  's/,/-/g')
	[[ "$affinity_list" == *"$cpuset"* ]]

	# check for sched_load_balance
	check_sched_load_balance "$ctr_pid" 0 # enabled
}
