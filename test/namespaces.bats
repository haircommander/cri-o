#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	export CONTAINER_NAMESPACES_DIR=$(mktemp -d)
}

function teardown() {
	cleanup_test
	rm -rf "$CONTAINER_NAMESPACES_DIR"
}

@test "pid_namespace_mode_pod_test" {
	start_crio

	pod_config="$TESTDIR"/sandbox_config.json
	jq '	  .linux.security_context.namespace_options = {
			pid: 0,
			host_network: false,
			host_pid: false,
			host_ipc: false
		}' \
		"$TESTDATA"/sandbox_config.json > "$pod_config"
	pod_id=$(crictl runp "$pod_config")

	ctr_config="$TESTDIR"/config.json
	jq '	  del(.linux.security_context.namespace_options)' \
		"$TESTDATA"/container_redis.json > "$ctr_config"
	ctr_id=$(crictl create "$pod_id" "$ctr_config" "$pod_config")
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" cat /proc/1/cmdline)
	[[ "$output" == *"pause"* ]]
}

@test "pin pid namespace" {
	start_crio

	jq '.linux.security_context.namespace_options.pid = 0' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_no_infra.json
	pod_id=$(crictl runp "$TESTDIR"/sandbox_no_infra.json)

	ctr_config="$TESTDIR"/config.json
	jq '	  del(.linux.security_context.namespace_options)' \
		"$TESTDATA"/container_redis.json > "$ctr_config"
	ctr_id=$(crictl create "$pod_id" "$ctr_config" "$pod_config")
	crictl start "$ctr_id"
	pid=$("$CONTAINER_RUNTIME" --root "$RUNTIME_ROOT" state $ctr_id | jq .pid)

	output=$(crictl exec --sync "$ctr_id" cat /proc/1/cmdline)
	output2=$(nsenter --mount --target="$pid" --pid="$CONTAINER_NAMESPACES_DIR/pidns/$pod_id" cat /proc/1/cmdline)
	[[ "$output" == "$output2" ]]
}

@test "pin pid namespace after restart" {
	start_crio

	jq '.linux.security_context.namespace_options.pid = 0' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_no_infra.json
	pod_id=$(crictl runp "$TESTDIR"/sandbox_no_infra.json)

	ctr_config="$TESTDIR"/config.json
	jq '	  del(.linux.security_context.namespace_options)' \
		"$TESTDATA"/container_redis.json > "$ctr_config"
	ctr_id=$(crictl create "$pod_id" "$ctr_config" "$pod_config")
	crictl start "$ctr_id"
	pid=$("$CONTAINER_RUNTIME" --root "$RUNTIME_ROOT" state $ctr_id | jq .pid)

	restart_crio

	output=$(crictl exec --sync "$ctr_id" cat /proc/1/cmdline)
	output2=$(nsenter --mount --target="$pid" --pid="$CONTAINER_NAMESPACES_DIR/pidns/$pod_id" cat /proc/1/cmdline)
	[[ "$output" == "$output2" ]]
}
