all: centos-7.tar rockylinux-8.tar rockylinux-8.6.tar rockylinux-8.7.tar rockylinux-9.tar openhpc-2.tar
.PHONY: all

rockylinux-8.%.tar: rockylinux-8/Containerfile.tmpl
	podman save $$(env TAG=8.$* envsubst <rockylinux-8/Containerfile.tmpl | podman build --quiet --tag docker.io/warewulf/rockylinux:8.$* --file - rockylinux-8) --output $@

rockylinux-%.tar: rockylinux-%/Containerfile.tmpl
	podman save $$(env TAG=$* envsubst <rockylinux-$*/Containerfile.tmpl | podman build --quiet --tag docker.io/warewulf/rockylinux:$* --file - rockylinux-$*) --output $@

centos-%.tar: centos-%/Containerfile
	podman save $$(podman build --quiet --tag docker.io/warewulf/centos:$* --file centos-$*/Containerfile centos-$*) --output $@

%.tar: %/Containerfile
	podman save $$(podman build --quiet $*) --output $@

clean:
	rm -rf *.tar
.PHONY: clean
