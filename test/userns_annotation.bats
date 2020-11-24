#!/usr/bin/env bats

load helpers

# These values come from https://github.com/containers/storage/blob/be5932a4d81cc01a1cf9cab0fb4cbf9c9892ef5c/store.go#L3463..L3471
# Since the test suite doesn't specify different values
export AUTO_USERNS_USER="containers"
export AUTO_USERNS_MAX_SIZE="65536"
export FIRST_UID=$(grep $AUTO_USERNS_USER /etc/subuid | cut -d : -f 2)
export FIRST_GID=$(grep $AUTO_USERNS_USER /etc/subgid | cut -d : -f 2)

function setup() {
	setup_test
	sboxconfig="$TESTDIR/sandbox_config.json"
	ctrconfig="$TESTDIR/container_config.json"
	create_userns_runtime
	start_crio
}

function teardown() {
	cleanup_test
}

function create_userns_runtime() {
	cat << EOF > "$CRIO_CONFIG_DIR/01-device.conf"
[crio.runtime]
default_runtime = "userns"
[crio.runtime.runtimes.userns]
runtime_path = "$RUNTIME_BINARY_PATH"
runtime_root = "$RUNTIME_ROOT"
runtime_type = "$RUNTIME_TYPE"
allowed_annotations = ["io.kubernetes.cri-o.userns-mode"]
EOF
}

@test "userns annotation auto should succeed" {
	jq '      .annotations."io.kubernetes.cri-o.userns-mode" = "auto"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp $sboxconfig)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$sboxconfig")
	crictl start $ctr_id

	pid=$(crictl inspect "$ctr_id" | jq .info.pid)
	cat /proc/"$pid"/uid_map
	# running auto will allocate the first available uid in the range allocated
	# to the user AUTO_USERNS_USER
	grep $FIRST_UID /proc/"$pid"/uid_map
	grep $FIRST_GID /proc/"$pid"/gid_map
	grep $AUTO_USERNS_MAX_SIZE /proc/"$pid"/uid_map
	grep $AUTO_USERNS_MAX_SIZE /proc/"$pid"/gid_map
}

@test "userns annotation auto with keep-id and map-to-root should fail" {
	jq '      .annotations."io.kubernetes.cri-o.userns-mode" = "auto:keep-id=true;map-to-root=true"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	! crictl runp $sboxconfig
}

@test "userns annotation auto with keep-id should succeed" {
	jq '      .annotations."io.kubernetes.cri-o.userns-mode" = "auto"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp $sboxconfig)

	jq  '      .linux.security_context.run_as_user.value = 0' '      .linux.security_context.run_as_group.value = 0' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl create "$pod_id" "$ctrconfig" "$sboxconfig")
	crictl start $ctr_id

	pid=$(crictl inspect "$ctr_id" | jq .info.pid)
	cat /proc/"$pid"/uid_map
	cat /proc/"$pid"/gid_map

	grep 1234 /proc/"$pid"/uid_map
}

@test "userns annotation auto with size should succeed" {
	jq '      .annotations."io.kubernetes.cri-o.userns-mode" = "auto:size=1234"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp $sboxconfig)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start $ctr_id

	pid=$(crictl inspect "$ctr_id" | jq .info.pid)
	cat /proc/"$pid"/uid_map

	grep 1234 /proc/"$pid"/uid_map
	grep 1234 /proc/"$pid"/gid_map
}
