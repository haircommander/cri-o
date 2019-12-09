#!/usr/bin/env bash

source helpers.bash

function run() {
	output=$("$@")
	status=$?
}

newconfig=$(mktemp --tmpdir crio-config.XXXXXX.json)
setup_test

	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	if [ "$status" -ne 0 ]; then
		exit 1
	fi
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	if [ "$status" -ne 0 ]; then
		exit 1
	fi
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	if [ "$status" -ne 0 ]; then
		exit 1
	fi
	run crictl rm -f "$ctr_id"
	echo "$output"
	if [ "$status" -ne 0 ]; then
		exit 1
	fi
	run crictl stopp "$pod_id"
	echo "$output"
	if [ "$status" -ne 0 ]; then
		exit 1
	fi
	run crictl rmp "$pod_id"
	echo "$output"
	if [ "$status" -ne 0 ]; then
		exit 1
	fi

rm -f "$newconfig"
cleanup_test

