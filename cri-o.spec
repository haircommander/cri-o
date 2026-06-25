%global with_debug 1

%if 0%{?with_debug}
%global _find_debuginfo_dwz_opts %{nil}
%global _dwz_low_mem_die_limit 0
%else
%global debug_package %{nil}
%endif

%global import_path github.com/cri-o/cri-o

# https://github.com/cri-o/cri-o/issues/8860
# RHEL 9 go-rpm-macros patches out %%{?__golang_extldflags} from %%gobuild,
# so we must override the macro to inject -Wl,-z,undefs directly.
%if 0%{?rhel} && 0%{?rhel} < 10 && ! 0%{?fedora}
%define gobuild(o:) go build -buildmode pie -compiler gc -tags="rpm_crashtraceback libtrust_openssl ${BUILDTAGS:-}" -ldflags "${LDFLAGS:-} -compressdwarf=false -B 0x$(head -c20 /dev/urandom|od -An -tx1|tr -d ' \\n') -extldflags '%__global_ldflags -Wl,-z,undefs'" -a -v -x %{?**};
%else
%global __golang_extldflags -Wl,-z,undefs
%endif

%if 0%{?rhel} >= 10
%global sequoia 1
%endif

%global service_name crio

%{!?commit:
# DO NOT MODIFY: the value on the line below is sed-like replaced by openshift/doozer
%global commit 5d8e3464d8ff2d88644b396bf3b938dfa05222ac
}
%global shortcommit %(c=%{commit}; echo ${c:0:7})

%if ! 0%{?os_git_vars:1}
# DO NOT MODIFY: the value on the line below is sed-like replaced by openshift/doozer
%global os_git_vars OS_GIT_VERSION='' OS_GIT_COMMIT='' OS_GIT_TREE_STATE=''
%endif

%{!?version: %global version 0.0.1}
%{!?release: %global release 1}

Name:           cri-o
Version:        %{version}
Release:        %{release}%{?dist}
Summary:        Kubernetes Container Runtime Interface for OCI-based containers
License:        ASL 2.0
URL:            https://%{import_path}

Source0:        https://%{import_path}/archive/%{commit}/%{name}-%{version}.tar.gz

# If go_arches not defined fall through to implicit golang archs
%if 0%{?go_arches:1}
ExclusiveArch:  %{go_arches}
%else
ExclusiveArch:  x86_64 aarch64 ppc64le s390x
%endif

BuildRequires:  golang
BuildRequires:  git
BuildRequires:  glib2-devel
BuildRequires:  glibc-static
BuildRequires:  go-md2man
BuildRequires:  gpgme-devel
BuildRequires:  libassuan-devel
BuildRequires:  libseccomp-devel
BuildRequires:  pkgconfig(systemd)
BuildRequires:  systemd-rpm-macros
%if %{undefined rhel} || 0%{?rhel} >= 10
BuildRequires:  go-rpm-macros
%endif

Requires:       shadow-utils
Requires(pre):  container-selinux
Requires:       skopeo-containers >= 1:0.1.40-1
Recommends:     (runc >= 1.0.0-61.rc8 or crun)
Obsoletes:      ocid <= 0.3
Provides:       ocid = %{version}-%{release}
Provides:       %{service_name} = %{version}-%{release}
Requires:       conmon >= 2.0.2-2
%if %{defined sequoia}
Requires:       podman-sequoia
%endif
%{?sysusers_requires_compat}

%description
%{summary}

%prep
%autosetup -Sgit -n %{name}-%{version}
sed -i 's/\.gopathok //' Makefile
sed -i 's/%{version}/%{version}-%{release}/' internal/version/version.go
sed -i 's/\/local//' contrib/systemd/%{service_name}.service

%build
mkdir _output
pushd _output
mkdir -p src/github.com/{cri-o,opencontainers}
ln -s $(dirs +1 -l) src/%{import_path}
popd

ln -s vendor src
export GOPATH=$(pwd)/_output:$(pwd)
export BUILDTAGS="selinux seccomp exclude_graphdriver_devicemapper exclude_graphdriver_btrfs containers_image_ostree_stub"
%if %{defined sequoia}
export BUILDTAGS="$BUILDTAGS containers_image_sequoia"
%endif
export GO111MODULE=off
# https://bugzilla.redhat.com/show_bug.cgi?id=1825623
export VERSION=%{version}

# build crio
%gobuild -o bin/%{service_name} %{import_path}/cmd/%{service_name}

# build pinns and docs
%{__make} bin/pinns
GO_MD2MAN=go-md2man %{__make} docs

%install
./bin/%{service_name} \
      --selinux \
      --cni-plugin-dir "/var/lib/cni/bin" \
      --cgroup-manager "systemd" \
      config > %{service_name}.conf

make PREFIX=%{buildroot}%{_prefix} DESTDIR=%{buildroot} \
            install.bin-nobuild \
            install.completions \
            install.config-nobuild \
            install.man-nobuild \
            install.systemd

# Remove CRI-O wipe service unit
rm %{buildroot}%{_prefix}/lib/systemd/system/%{service_name}-wipe.service

# Install seccomp.json
install -D -p -m 0644 openshift/rpm/seccomp.json %{buildroot}%{_sysconfdir}/%{service_name}/seccomp.json
install -dp %{buildroot}%{_sharedstatedir}/containers
install -dp %{buildroot}%{_libexecdir}/%{service_name}
install -dp %{buildroot}%{_sharedstatedir}/cni/bin
install -dp %{buildroot}%{_sysconfdir}/kubernetes/cni/net.d
install -dp %{buildroot}%{_datadir}/containers/oci/hooks.d
install -dp %{buildroot}/opt/cni/bin
install -D -p -m 0644 openshift/rpm/unshare.json %{buildroot}%{_libexecdir}/%{service_name}/unshare.json

# Install cri-o.tmpfiles
install -D -p -m 0644 openshift/rpm/cri-o.tmpfiles %{buildroot}%{_tmpfilesdir}/%{service_name}.conf

# Install cri-o.sysusers
install -D -p -m 0644 openshift/rpm/cri-o.sysusers %{buildroot}%{_sysusersdir}/%{service_name}.conf

install -dp %{buildroot}/%{_unitdir}/irqbalance.service.d
install -m 644 openshift/rpm/restart-limits.conf %{buildroot}/%{_unitdir}/irqbalance.service.d/restart-limits.conf

# Install cri-o rootless containers user update service
cat > %{service_name}-subid.service <<'EOF'

[Unit]
Description=Update user for rootless containers
Before=crio.service

[Service]
Type=oneshot
Environment="ROOTLESS_USER=containers"
Environment="ROOTLESS_SUBUIDS=200000-16199999"
Environment="ROOTLESS_SUBGIDS=200000-16199999"
ExecCondition=/bin/bash -c 'if ! /usr/bin/getent -i passwd $ROOTLESS_USER >/dev/null 2>&1 ; then exit 1 ; else exit 0 ; fi'
ExecCondition=/bin/bash -c 'if /usr/bin/grep -q $ROOTLESS_USER /etc/subuid ; then exit 1 ; else exit 0 ; fi'
ExecStart=/usr/sbin/usermod --add-subuids $ROOTLESS_SUBUIDS --add-subgids $ROOTLESS_SUBGIDS $ROOTLESS_USER

[Install]
WantedBy=multi-user.target crio.service
EOF

install -D -p -m 0644 %{service_name}-subid.service %{buildroot}%{_unitdir}/%{service_name}-subid.service

cat > %{service_name}-subid.preset <<'EOF'
enable crio-subid.service
EOF

install -D -p -m 0644 %{service_name}-subid.preset %{buildroot}%{_presetdir}/50-%{service_name}-subid.preset

%check
%if 0%{?with_check}
export GOPATH=%{buildroot}%{gopath}:$(pwd)/Godeps/_workspace:%{gopath}
%endif

%pre
%sysusers_create_compat openshift/rpm/cri-o.sysusers

%post
%systemd_post %{service_name}
%systemd_post %{service_name}-subid
%tmpfiles_create %{_tmpfilesdir}/%{service_name}.conf

%preun
%systemd_preun %{service_name}
%systemd_preun %{service_name}-subid

%postun
%systemd_postun_with_restart %{service_name}
%systemd_postun %{service_name}-subid

%{!?_licensedir:%global license %doc}

%files
%license LICENSE
%doc README.md
%{_bindir}/%{service_name}
%{_bindir}/pinns
%{_mandir}/man5/%{service_name}.conf*5*
%{_mandir}/man8/%{service_name}*.8*
%dir %{_sysconfdir}/%{service_name}
%config(noreplace) %{_sysconfdir}/%{service_name}/%{service_name}.conf
%config(noreplace) %{_sysconfdir}/%{service_name}/seccomp.json
%config(noreplace) %{_sysconfdir}/crictl.yaml
%{_unitdir}/%{service_name}.service
%{_unitdir}/%{service_name}-subid.service
%{_presetdir}/50-%{service_name}-subid.preset
%dir %{_sharedstatedir}/containers
%dir %{_sharedstatedir}/cni
%dir %{_sharedstatedir}/cni/bin
%dir %{_sysconfdir}/kubernetes
%dir %{_sysconfdir}/kubernetes/cni
%dir %{_sysconfdir}/kubernetes/cni/net.d
%dir %{_datadir}/containers
%dir %{_datadir}/containers/oci
%dir %{_datadir}/containers/oci/hooks.d
%dir /opt/cni
%dir /opt/cni/bin
%dir %{_datadir}/oci-umount
%dir %{_datadir}/oci-umount/oci-umount.d
%dir %{_libexecdir}/%{service_name}
%{_libexecdir}/%{service_name}/unshare.json
%{_datadir}/oci-umount/oci-umount.d/%{service_name}-umount.conf
%{_datadir}/bash-completion/completions/%{service_name}*
%{_datadir}/fish/completions/%{service_name}*.fish
%{_datadir}/zsh/site-functions/_%{service_name}*
%{_tmpfilesdir}/%{service_name}.conf
%{_sysusersdir}/%{service_name}.conf
%dir %{_unitdir}/irqbalance.service.d
%{_unitdir}/irqbalance.service.d/restart-limits.conf

%changelog
