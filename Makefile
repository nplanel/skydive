export GO111MODULE?=on

define VERSION_CMD =
eval ' \
	define=""; \
	version=`git describe --abbrev=0 --tags | tr -d "[a-z]"` ; \
	commit=`git rev-parse --verify HEAD`; \
	tagname=`git show-ref --tags | grep $$commit`; \
	if [ -n "$$tagname" ]; then \
		define=`echo $$tagname | awk -F "/" "{print \\$$NF}" | tr -d "[a-z]"`; \
	else \
		define=`printf "$$version-%.12s" $$commit`; \
	fi; \
	tainted=`git ls-files -m | wc -l` ; \
	if [ "$$tainted" -gt 0 ]; then \
		define="$${define}-tainted"; \
	fi; \
	echo "$$define" \
'
endef

define PROTOC_GEN
go get -u ${PROTOC_GEN_GOFAST_GITHUB}
go get -u ${PROTOC_GEN_GO_GITHUB}
protoc -I. -Iflow/layers -Ivendor/github.com/gogo/protobuf --plugin=$${GOPATH}/bin/protoc-gen-gogofaster --gogofaster_out $$GOPATH/src $1
endef

VERSION?=$(shell $(VERSION_CMD))
GO?=go
BUILD_ID:=$(shell echo 0x$$(head -c20 /dev/urandom|od -An -tx1|tr -d ' \n'))
SKYDIVE_GITHUB:=github.com/skydive-project/skydive
SKYDIVE_PKG:=skydive-${VERSION}
SKYDIVE_PATH:=$(SKYDIVE_PKG)/src/$(SKYDIVE_GITHUB)/
SKYDIVE_GITHUB_VERSION:=$(SKYDIVE_GITHUB)/version.Version=${VERSION}
GO_BINDATA_GITHUB:=github.com/jteeuwen/go-bindata/go-bindata
PROTOC_GEN_GO_GITHUB:=github.com/golang/protobuf/protoc-gen-go
PROTOC_GEN_GOFAST_GITHUB:=github.com/gogo/protobuf/protoc-gen-gogofaster
VPPBINAPI_GITHUB:=git.fd.io/govpp.git/cmd/binapi-generator
VERBOSE_FLAGS?=-v
VERBOSE_TESTS_FLAGS?=-test.v
VERBOSE?=true
ifeq ($(VERBOSE), false)
  VERBOSE_FLAGS:=
  VERBOSE_TESTS_FLAGS:=
endif
ifeq ($(COVERAGE), true)
  TEST_COVERPROFILE?=../functionals.cover
  EXTRA_ARGS+=-test.coverprofile=${TEST_COVERPROFILE}
endif
ifeq ($(WITH_PROF), true)
  EXTRA_ARGS+=-profile
endif
TIMEOUT?=1m
TEST_PATTERN?=
UT_PACKAGES?=$(shell $(GO) list ./... | grep -Ev '/tests|/contrib')
FUNC_TESTS_CMD:="grep -e 'func Test${TEST_PATTERN}' tests/*.go | perl -pe 's|.*func (.*?)\(.*|\1|g' | shuf"
FUNC_TESTS:=$(shell sh -c $(FUNC_TESTS_CMD))
DOCKER_IMAGE?=skydive/skydive
DOCKER_TAG?=devel
DESTDIR?=$(shell pwd)
COVERAGE?=0
COVERAGE_MODE?=atomic
COVERAGE_WD?="."
BOOTSTRAP:=contrib/packaging/rpm/generate-skydive-bootstrap.sh
BOOTSTRAP_ARGS?=
BUILD_TAGS?=$(TAGS)
WITH_LXD?=true
WITH_OPENCONTRAIL?=true
WITH_LIBVIRT_GO?=true
WITH_EBPF_DOCKER_BUILDER?=true
WITH_VPP?=false
STATIC_DIR?=
STATIC_LIBS?=

OS_RHEL := $(shell test -f /etc/redhat-release && echo -n Y)
ifeq ($(OS_RHEL),Y)
	STATIC_DIR := /usr/lib64
	STATIC_LIBS := \
		libz.a \
		liblzma.a \
		libm.a
endif

OS_DEB := $(shell test -f /etc/debian_version && echo -n Y)
ifeq ($(OS_DEB),Y)
	STATIC_DIR := $(shell dpkg-architecture -c 'sh' -c 'echo /usr/lib/$$DEB_HOST_MULTIARCH')
	STATIC_LIBS := \
		libz.a \
		liblzma.a \
		libc.a \
		libdl.a \
		libpthread.a \
		libc++.a \
		libm.a
endif

ifeq ($(WITH_DPDK), true)
  BUILD_TAGS+=dpdk
endif

ifeq ($(WITH_EBPF), true)
  BUILD_TAGS+=ebpf
  EXTRABINDATA+=probe/ebpf/*.o
endif

ifeq ($(WITH_PROF), true)
  BUILD_TAGS+=prof
endif

ifeq ($(WITH_SCALE), true)
  BUILD_TAGS+=scale
endif

ifeq ($(WITH_NEUTRON), true)
  BUILD_TAGS+=neutron
endif

ifeq ($(WITH_CDD), true)
  BUILD_TAGS+=cdd
endif

ifeq ($(WITH_MUTEX_DEBUG), true)
  BUILD_TAGS+=mutexdebug
endif

ifeq ($(WITH_K8S), true)
  BUILD_TAGS+=k8s
  ANALYZER_TEST_PROBES+=k8s
endif

ifeq ($(WITH_ISTIO), true)
  BUILD_TAGS+=k8s istio
  ANALYZER_TEST_PROBES+=istio
endif

ifeq ($(WITH_OVN), true)
  ANALYZER_TEST_PROBES+=ovn
endif

ifeq ($(WITH_HELM), true)
  BUILD_TAGS+=helm
endif

ifeq ($(WITH_OPENCONTRAIL), true)
  BUILD_TAGS+=opencontrail
  AGENT_TEST_EXTRA_PROBES+=opencontrail
ifeq ($(OS_RHEL),Y)
  STATIC_LIBS+=libxml2.a
endif
ifeq ($(OS_DEB),Y)
  STATIC_LIBS+=libicuuc.a \
               libicudata.a \
               libxml2.a
endif
endif

ifeq ($(WITH_LXD), true)
  BUILD_TAGS+=lxd
endif

ifeq ($(WITH_LIBVIRT_GO), true)
  BUILD_TAGS+=libvirt
endif

ifeq ($(WITH_VPP), true)
  BUILD_TAGS+=vpp
  AGENT_TEST_EXTRA_PROBES+=vpp
endif

ifeq (${DEBUG}, true)
  GOFLAGS=-gcflags='-N -l'
  GO_BINDATA_FLAGS+=-debug
  export DEBUG
endif

comma:= ,
empty:=
space:= $(empty) $(empty)
EXTRA_ARGS+=-analyzer.topology.probes=$(subst $(space),$(comma),$(ANALYZER_TEST_PROBES)) -agent.topology.probes=$(subst $(space),$(comma),$(AGENT_TEST_EXTRA_PROBES))
STATIC_LIBS_ABS := $(addprefix $(STATIC_DIR)/,$(STATIC_LIBS))
STATIC_BUILD_TAGS := $(filter-out libvirt,$(BUILD_TAGS))

.PHONY: all install
all install: skydive

.PHONY: version
version:
	@echo -n ${VERSION}

skydive.yml: etc/skydive.yml.default
	[ -e $@ ] || cp $< $@

DLV_FLAGS=--check-go-version=false

ifeq (${DEBUG}, true)
define skydive_run
sudo -E $$(which dlv) $(DLV_FLAGS) exec $$(which skydive) -- $1 -c skydive.yml
endef
else
define skydive_run
sudo -E $$(which skydive) $1 -c skydive.yml
endef
endif

.PHONY: debug.agent
run.agent:
	$(call skydive_run,agent)

.PHONY: debug.analyzer
run.analyzer:
	$(call skydive_run,analyzer)

GEN_PROTO_FILES = $(patsubst %.proto,%.pb.go,$(shell find . -name *.proto | grep -v ^./vendor))
GEN_EASYJSON_FILES = $(patsubst %.go,%_easyjson.go,$(shell git grep //go:generate | grep "easyjson" | grep -v Makefile | cut -d ":" -f 1))
GEN_DECODER_FILES = $(patsubst %.go,%_gendecoder.go,$(shell git grep //go:generate | grep "gendecoder" | grep -v Makefile | cut -d ":" -f 1))
# to remove when generated files will be added to git
GEN_DECODER_FILES += flow/flow.pb_gendecoder.go
GEN_EASYJSON_FILES += flow/flow.pb_easyjson.go

%.pb.go: %.proto
	$(call PROTOC_GEN,$<)

flow/flow.pb.go: flow/flow.proto filters/filters.proto
	$(call PROTOC_GEN,flow/flow.proto)

	# always export flow.ParentUUID as we need to store this information to know
	# if it's a Outer or Inner packet.
	sed -e 's/ParentUUID\(.*\),omitempty\(.*\)/ParentUUID\1\2/' \
		-e 's/Protocol\(.*\),omitempty\(.*\)/Protocol\1\2/' \
		-e 's/ICMPType\(.*\),omitempty\(.*\)/ICMPType\1\2/' \
		-e 's/int64\(.*\),omitempty\(.*\)/int64\1\2/' \
		-i $@
	# add omitempty to RTT as it is not always filled
	sed -e 's/json:"RTT"/json:"RTT,omitempty"/' -i $@
	# do not export LastRawPackets used internally
	sed -e 's/json:"LastRawPackets,omitempty"/json:"-"/g' -i $@
	# add flowState to flow generated struct
	sed -e 's/type Flow struct {/type Flow struct { XXX_state flowState `json:"-"`/' -i $@
	# to fix generated layers import
	sed -e 's/layers "flow\/layers"/layers "github.com\/skydive-project\/skydive\/flow\/layers"/' -i $@
	sed -e 's/type FlowMetric struct {/\/\/ gendecoder\ntype FlowMetric struct {/' -i $@
	sed -e 's/type FlowLayer struct {/\/\/ gendecoder\ntype FlowLayer struct {/' -i $@
	sed -e 's/type TransportLayer struct {/\/\/ gendecoder\ntype TransportLayer struct {/' -i $@
	sed -e 's/type ICMPLayer struct {/\/\/ gendecoder\ntype ICMPLayer struct {/' -i $@
	sed -e 's/type IPMetric struct {/\/\/ gendecoder\ntype IPMetric struct {/' -i $@
	sed -e 's/type TCPMetric struct {/\/\/ gendecoder\ntype TCPMetric struct {/' -i $@
	# This is to allow calling go generate on flow/flow.pb.go
	sed -e 's/DO NOT EDIT./DO NOT MODIFY/' -i $@
	sed '1 i //go:generate go run github.com/skydive-project/skydive/scripts/gendecoder' -i $@
	gofmt -s -w $@

flow/flow.pb_easyjson.go: flow/flow.pb.go
	go run github.com/safchain/easyjson/easyjson -all $<

websocket/structmessage.pb.go: websocket/structmessage.proto
	$(call PROTOC_GEN,$<)

	sed -e 's/type StructMessage struct {/type StructMessage struct { XXX_state structMessageState `json:"-"`/' -i websocket/structmessage.pb.go
	gofmt -s -w $@

.proto: vendor $(GEN_PROTO_FILES)

.PHONY: .proto.clean
.proto.clean:
	find . \( -name *.pb.go ! -path './vendor/*' \) -exec rm {} \;
	rm -rf flow/layers/generated.proto

%_easyjson.go: %.go
	go generate -run easyjson $<

.PHONY: .easyjson
.easyjson: flow/flow.pb_easyjson.go $(GEN_EASYJSON_FILES)

.PHONY: .easyjson.clean
.easyjson.clean:
	find . \( -name *_easyjson.go ! -path './vendor/*' \) -exec rm {} \;

%_gendecoder.go: %.go
	go generate -run gendecoder $<

.PHONY: .gendecoder
.gendecoder: $(GEN_DECODER_FILES)

.PHONY: .vppbinapi
.vppbinapi:
	go generate -tags "${BUILD_TAGS}" ${SKYDIVE_GITHUB}/topology/probes/vpp

.PHONY: .vppbinapi.clean
.vppbinapi.clean:
ifeq ($(WITH_VPP), true)
	rm -rf topology/probes/vpp/bin_api
endif

BINDATA_DIRS := \
	js/*.js \
	rbac/policy.csv \
	statics/index.html \
	statics/css/* \
	statics/css/themes/*/* \
	statics/fonts/* \
	statics/img/* \
	statics/js/* \
	statics/schemas/* \
	statics/workflows/*.yaml \
	${EXTRABINDATA}

.PHONY: .typescript
.typescript:
	make -C js

.PHONY: .typescript.clean
.typescript.clean:
	make -C js clean

.PHONY: .bindata
.bindata: statics/bindata.go

statics/bindata.go: .typescript ebpf.build $(shell find statics -type f \( ! -iname "bindata.go" \))
	go run ${GO_BINDATA_GITHUB} ${GO_BINDATA_FLAGS} -nometadata -o statics/bindata.go -pkg=statics -ignore=bindata.go $(BINDATA_DIRS)
	gofmt -w -s statics/bindata.go

.PHONY: .go-generate.clean
.go-generate.clean:
	find . \( -name *_gendecoder.go ! -path './vendor/*' \) -exec rm {} \;

.PHONY: swagger
swagger: .go-generate
	go run github.com/go-swagger/go-swagger/cmd/swagger generate spec -m -o /tmp/swagger.json
	for def in `ls api/server/*_swagger.json`; do \
		jq -s  '.[0] * .[1] * {tags: (.[0].tags + .[1].tags)}' /tmp/swagger.json $$def > swagger.json; \
		cp swagger.json /tmp; \
	done
	jq -s  '.[0] * .[1]' /tmp/swagger.json api/server/swagger_base.json > swagger.json
	sed -i 's/easyjson:json//g' swagger.json

.PHONY: compile
compile:
	CGO_CFLAGS_ALLOW='.*' CGO_LDFLAGS_ALLOW='.*' $(GO) install \
		-ldflags="${LDFLAGS} -B $(BUILD_ID) -X $(SKYDIVE_GITHUB_VERSION)" \
		${GOFLAGS} -tags="${BUILD_TAGS}" ${VERBOSE_FLAGS} \
		${SKYDIVE_GITHUB}

.PHONY: compile.static
compile.static:
	CGO_CFLAGS_ALLOW='.*' CGO_LDFLAGS_ALLOW='.*' $(GO) install \
		-ldflags="${LDFLAGS} -B $(BUILD_ID) -X $(SKYDIVE_GITHUB_VERSION) '-extldflags=-static $(STATIC_LIBS_ABS)'" \
		${GOFLAGS} \
		${VERBOSE_FLAGS} -tags "netgo ${STATIC_BUILD_TAGS}" \
		-installsuffix netgo || true

.PHONY: skydive
skydive: genlocalfiles compile

.PHONY: skydive.clean
skydive.clean:
	go clean -i $(SKYDIVE_GITHUB)

.PHONY: bench
bench:	skydive bench.flow

flow/pcaptraces/201801011400.pcap.gz:
	aria2c -s 16 -x 16 -o $@ "http://mawi.nezu.wide.ad.jp/mawi/samplepoint-F/2018/201801011400.pcap.gz"

flow/pcaptraces/%.pcap: flow/pcaptraces/%.pcap.gz
	gunzip -fk $<

flow/pcaptraces/201801011400.small.pcap: flow/pcaptraces/201801011400.pcap
	tcpdump -r $< -w $@ "ip host 203.82.244.188"

bench.flow.traces: flow/pcaptraces/201801011400.small.pcap

bench.flow: bench.flow.traces
	$(GO) test -bench=. ${SKYDIVE_GITHUB}/flow

.PHONY: static
static: skydive.clean genlocalfiles
	$(MAKE) compile.static WITH_LIBVIRT_GO=false
	$(MAKE) contribs.static

.PHONY: contribs.clean
contribs.clean: contrib.exporters.clean contrib.snort.clean contrib.collectd.clean

.PHONY: contribs.test
contribs.test: contrib.exporters.test

.PHONY: contribs
contribs: contrib.exporters contrib.snort contrib.collectd

.PHONY: contribs.static
contribs.static:
	$(MAKE) -C contrib/exporters static

.PHONY: contrib.exporters.clean
contrib.exporters.clean:
	$(MAKE) -C contrib/exporters clean

.PHONY: contrib.exporters
contrib.exporters: genlocalfiles
	$(MAKE) -C contrib/exporters

.PHONY: contrib.exporters.test
contrib.exporters.test: genlocalfiles
	$(MAKE) -C contrib/exporters test

.PHONY: contribs.snort.clean
contrib.snort.clean:
	$(MAKE) -C contrib/snort clean

.PHONY: contrib.snort
contrib.snort:genlocalfiles
	$(MAKE) -C contrib/snort

.PHONY: contrib.collectd.clean
contrib.collectd.clean:
	$(MAKE) -C contrib/collectd clean

.PHONY: contrib.collectd
contrib.collectd: genlocalfiles
	$(MAKE) -C contrib/collectd

.PHONY: ebpf.build
ebpf.build: vendor # vendor is required because the eBPF Makefile uses the 'vendor' directory
ifeq ($(WITH_EBPF), true)
ifeq ($(WITH_EBPF_DOCKER_BUILDER), true)
	$(MAKE) -C probe/ebpf docker-ebpf-build
else
	$(MAKE) -C probe/ebpf ebpf-build
endif
endif

.PHONY: ebpf.clean
ebpf.clean:
	$(MAKE) -C probe/ebpf clean

.PHONY: test.functionals.clean
test.functionals.clean:
	rm -f tests/functionals

.PHONY: test.functionals.compile
test.functionals.compile: genlocalfiles
	$(GO) test -tags "${BUILD_TAGS} test" -race ${GOFLAGS} ${VERBOSE_FLAGS} -timeout ${TIMEOUT} -c -o tests/functionals ./tests/

.PHONY: test.functionals.static
test.functionals.static: genlocalfiles
	$(GO) test -tags "netgo ${STATIC_BUILD_TAGS} test" \
		-ldflags "${LDFLAGS} -X $(SKYDIVE_GITHUB_VERSION) -extldflags \"-static $(STATIC_LIBS_ABS)\"" \
		-installsuffix netgo \
		-race ${GOFLAGS} ${VERBOSE_FLAGS} -timeout ${TIMEOUT} \
		-c -o tests/functionals ./tests/

ifeq (${DEBUG}, true)
define functionals_run
cd tests && sudo -E $$(which dlv) $(DLV_FLAGS) exec ./functionals -- $1
endef
else
define functionals_run
cd tests && sudo -E ./functionals $1
endef
endif

.PHONY: test.functionals.run
test.functionals.run:
	cd tests && sudo -E ./functionals ${VERBOSE_TESTS_FLAGS} -test.run "${TEST_PATTERN}" -test.timeout ${TIMEOUT} ${ARGS} ${EXTRA_ARGS}

.PHONY: tests.functionals.all
test.functionals.all: test.functionals.compile
	$(MAKE) TIMEOUT="8m" ARGS="${ARGS}" test.functionals.run EXTRA_ARGS="${EXTRA_ARGS}"

.PHONY: test.functionals.batch
test.functionals.batch: test.functionals.compile
	set -e ; $(MAKE) ARGS="${ARGS} " test.functionals.run EXTRA_ARGS="${EXTRA_ARGS}" TEST_PATTERN="${TEST_PATTERN}"

.PHONY: test.functionals
test.functionals: test.functionals.compile
	for functest in ${FUNC_TESTS} ; do \
		$(MAKE) ARGS="-test.run $$functest$$\$$ ${ARGS}" test.functionals.run EXTRA_ARGS="${EXTRA_ARGS}"; \
	done

.PHONY: functional
functional:
	$(MAKE) test.functionals VERBOSE=true TIMEOUT=10m ARGS='-standalone -analyzer.topology.backend elasticsearch -analyzer.flow.backend elasticsearch' TEST_PATTERN="${TEST_PATTERN}"

.PHONY: test
test: genlocalfiles
ifeq ($(COVERAGE), true)
	set -v ; \
	for pkg in ${UT_PACKAGES}; do \
		if [ -n "$$pkg" ]; then \
			coverfile="${COVERAGE_WD}/$$(echo $$pkg | tr / -).cover"; \
			$(GO) test -tags "${BUILD_TAGS} test" -covermode=${COVERAGE_MODE} -coverprofile="$$coverfile" ${VERBOSE_FLAGS} -timeout ${TIMEOUT} $$pkg; \
		fi; \
	done
else
ifneq ($(TEST_PATTERN),)
	set -v ; \
	$(GO) test -tags "${BUILD_TAGS} test" -ldflags="${LDFLAGS}" -race ${GOFLAGS} ${VERBOSE_FLAGS} -timeout ${TIMEOUT} -test.run ${TEST_PATTERN} ${UT_PACKAGES}
else
	set -v ; \
	$(GO) test -tags "${BUILD_TAGS} test" -ldflags="${LDFLAGS}" -race ${GOFLAGS} ${VERBOSE_FLAGS} -timeout ${TIMEOUT} ${UT_PACKAGES}
endif
endif

.PHONY: fmt
fmt: genlocalfiles
	@echo "+ $@"
	@test -z "$$($(GO) fmt +local)" || \
		(echo "+ please format Go code with 'gofmt -s'" && /bin/false)

.PHONY: vet
vet:
	@echo "+ $@"
	test -z "$$($(GO) tool vet $$( \
			$(GO) list ./... \
			| perl -pe 's|$(SKYDIVE_GITHUB)/?||g' \
			| grep -v '^tests') 2>&1 \
		| tee /dev/stderr \
		| grep -v '^flow/probes/afpacket/' \
		| grep -v 'exit status 1' \
		)"

.PHONY: check
check: lint
	# check if Go modules are in sync
	# $(GO) mod tidy
	# @test -z "$$(git diff go.mod go.sum)" || \
	#	(echo -e "Go modules of sync:\n$$(git diff go.mod go.sum)" && /bin/false)
	nbnotcomment=$$(grep '"linter":"golint"' lint.json | wc -l); \
	if [ $$nbnotcomment -gt 0 ]; then \
		cat lint.json; \
		echo "===> You should comment you code <==="; \
		exit 1; \
	fi

.PHONY: golangci-lint
golangci-lint:
	@echo "+ $@"
	go run github.com/golangci/golangci-lint/cmd/golangci-lint run ${GOMETALINTER_FLAGS} -e '.*\.pb.go' -e '.*\._easyjson.go' -e '.*\._gendecoder.go' -e 'statics/bindata.go' --skip-dirs=statics --deadline 10m --out-format=json ./... | tee lint.json || true

.PHONY: lint
lint:
	make golangci-lint GOMETALINTER_FLAGS="--disable-all --enable=golint"

.PHONY: genlocalfiles
genlocalfiles: .proto .bindata .gendecoder .easyjson .vppbinapi

.PHONY: clean
clean: skydive.clean test.functionals.clean contribs.clean ebpf.clean .easyjson.clean .proto.clean .go-generate.clean .typescript.clean .vppbinapi.clean
	go clean -i >/dev/null 2>&1 || true

.PHONY: srpm
srpm:
	$(BOOTSTRAP) -s ${BOOTSTRAP_ARGS}

.PHONY: rpm
rpm:
	$(BOOTSTRAP) -b ${BOOTSTRAP_ARGS}

.PHONY: docker-image
docker-image: static
	cp $$GOPATH/bin/skydive contrib/docker/skydive.$$(uname -m)
	docker build -t ${DOCKER_IMAGE}:${DOCKER_TAG} --build-arg ARCH=$$(uname -m) -f contrib/docker/Dockerfile contrib/docker/

SKYDIVE_TAR_INPUT:= \
	vendor \
	statics/bindata.go \
	$(GEN_PROTO_FILES) \
	$(GEN_DECODER_FILES) \
	$(GEN_EASYJSON_FILES)

SKYDIVE_TAR:=${DESTDIR}/$(SKYDIVE_PKG).tar

define TAR_CMD
tar $1 -f $(SKYDIVE_TAR) --transform="s||$(SKYDIVE_PATH)|" $2
endef

define TAR_APPEND
$(call TAR_CMD,--append,$(SKYDIVE_TAR_INPUT))
endef

.PHONY: vendor
vendor:
ifeq ($(WITH_VPP), true)
	$(MAKE) .vppbinapi
endif
ifeq (${GO111MODULE}, on)
	go mod vendor
endif

.PHONY: localdist
localdist: genlocalfiles vendor
	git ls-files | $(call TAR_CMD,--create,--files-from -)
	$(call TAR_APPEND,)
	gzip -f $(SKYDIVE_TAR)

.PHONY: dist
dist: genlocalfiles vendor
	git archive -o $(SKYDIVE_TAR) --prefix $(SKYDIVE_PATH) HEAD
	$(call TAR_APPEND,)
	gzip -f $(SKYDIVE_TAR)
