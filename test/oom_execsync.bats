#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

function calculate_highest_cgroup_val() {
	local current_highest=$1
	local to_search=$2
	local cgroup_file=$3
	echo have $current_highest $to_search $cgroup_file >&3
	current=$(cat $cgroup_file | grep $to_search | awk '{
	  if ($2)
	  	print $2;
	  else
	  	print $1;
	}')

	echo $current >&3
	if [ $current -gt $current_highest ]; then
		current_highest=$current
		echo "[$(date +'%T')] highest $to_search $(echo $current_highest | awk '{print $1/1024}') KB" >&3
	fi
	echo $current_highest
}

@test "ctr execsync oom" {
	oomconfig=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["image"]["image"] = "docker.io/library/alpine"; obj["linux"]["resources"]["memory_limit_in_bytes"] = 25165824; obj["command"] = ["top"]; json.dump(obj, sys.stdout)')
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
	#local highest_cache=0
	local highest_memory_usage=0
	sleep 2s
	while [ $attempt -le 100 ]; do
		attempt=$((attempt+1))
		run crictl exec --sync $ctr_id ls
		highest_memory_usage=$(calculate_highest_cgroup_val $highest_memory_usage '-v usage' $slice/memory.usage_in_bytes)
		highest_rss=$(calculate_highest_cgroup_val $highest_rss "'total_rss '" $slice/memory.stat)
		#highest_cache=$(calculate_highest_cgroup_val $highest_cache 'total_cache ' $slice/memory.stat)
	done

	sleep 2s
	echo "final total rss:" $(cat $slice/memory.stat | grep 'total_rss ' | awk '{ print $2/1024}') "KB" >&3

	run crictl rmp -fa
	echo "$output"
	[ "$status" -eq 0 ]
}
