#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

#function calculate_highest_cgroup_val() {
#	local current_highest=$1
#	local to_search=$2
#	local cgroup_file=$3
#	echo have $current_highest $to_search $cgroup_file >&3
#	current=$(cat $cgroup_file | grep $to_search | awk '{
#	  if ($2)
#	  	print $2;
#	  else
#	  	print $1;
#	}')
#
#	echo $current >&3
#	if [ $current -gt $current_highest ]; then
#		current_highest=$current
#		echo "[$(date +'%T')] highest $to_search $(echo $current_highest | awk '{print $1/1024}') KB" >&3
#	fi
#	echo $current_highest
#}

#function print_current_cgroup_val() {
#	local current_highest=$1
#	local to_search=$2
#	local cgroup_file=$3
#
#	current=$(cat $cgroup_file)
#	if [[ ! -z "$to_search" ]]; then
#		current2=$(echo $current | grep "$to_search")
#		echo $current2
#		current=$current2
#	fi
#	current2=$(echo $current | awk '{print $NF}')
#	current=$current2
#
#
#	if [[ -z "$to_search" ]]; then
#		to_search="usage"
#	fi
#	echo $to_search: $current >&3
#}

function print_rss() {
	local slice=$1
	echo rss: $(cat $slice/memory.stat | grep 'total_rss ' | awk '{ print $2/1024}') "KB" >&3
}

function print_usage() {
	local slice=$1
	echo usage: $(cat $slice/memory.usage_in_bytes  | awk '{ print $1/1024}') "KB" >&3
}

@test "ctr execsync oom" {
	oomconfig=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["image"]["image"] = "docker.io/library/alpine"; obj["linux"]["resources"]["memory_limit_in_bytes"] = 41943040; obj["command"] = ["top"]; json.dump(obj, sys.stdout)')
	echo "$oomconfig" > "$TESTDIR"/container_config_oom.json

	start_crio
	run crictl run "$TESTDIR"/container_config_oom.json "$TESTDATA"/sandbox_config.json 
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	slice=/sys/fs/cgroup/memory$(systemctl status crio-$ctr_id.scope | grep CGroup | awk '{ print $2 }')
	echo $slice >&3
	attempt=0
	local highest_rss=0
	local highest_memory_usage=0
	sleep 2s
	while [ $attempt -le 10000 ]; do
		attempt=$((attempt+1))
		run crictl exec --sync $ctr_id ls&
		print_rss $slice
		print_usage $slice
		sleep .1s
		#print_current_cgroup_val $highest_memory_usage "" $slice/memory.usage_in_bytes
		#print_current_cgroup_val $highest_rss 'total_rss ' $slice/memory.stat
	done

	sleep 2s
	
	echo final:
	print_rss $slice
	print_usage $slice

	run crictl rmp -fa
	echo "$output"
	[ "$status" -eq 0 ]
}
