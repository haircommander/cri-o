#!/usr/bin/env bats

load helpers

function setup() {
	newconfig=$(mktemp --tmpdir crio-config.XXXXXX.json)
	setup_test
}

function teardown() {
	rm -f "$newconfig"
	cleanup_test
}

@test "ctr execsync" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" echo HELLO
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "HELLO" ]]
	run crictl exec --sync --timeout 1 "$ctr_id" sleep 3
	echo "$output"
	[[ "$output" =~ "command timed out" ]]
	[ "$status" -ne 0 ]
}

@test "ctr execsync conflicting with conmon flags parsing" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" sh -c "echo hello world"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "hello world" ]]
}

@test "ctr execsync terminal" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	# in the past, we've seen a race in conmon that caused some quickly
	# exiting execs to fail. hammer this so we are likely to catch regressions
	for i in {1..5}; do
		run crictl exec --sync --tty --interactive "$ctr_id" echo HELLO
		echo "$output"
		[ "$status" -eq 0 ]
		[[ "$output" == "HELLO" ]]
	done

	run crictl exec --sync --timeout 1 --tty --interactive "$ctr_id" sleep 3
	echo "$output"
	[[ "$output" =~ "command timed out" ]]
	[ "$status" -ne 0 ]

	run crictl exec --sync --timeout 1 --tty --interactive "$ctr_id" sleep 3
	echo "$output"
	[[ "$output" =~ "command timed out" ]]
	[ "$status" -ne 0 ]
}
