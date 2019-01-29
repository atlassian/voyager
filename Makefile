.DEFAULT_GOAL := all
OS = $(shell uname -s | tr A-Z a-z)
BINARY_PREFIX_DIRECTORY=$(OS)_amd64_stripped

KUBECONTEXT ?= kubernetes-admin@kind-1
KUBECONFIG ?= $(shell kind get kubeconfig-path)
#KUBECONFIG ?= $$HOME/.kube/config

#Directories to scan and generate Deepcopy methods
MAIN_PACKAGE_DIR = github.com/atlassian/voyager
APIS_AGGREGATOR_DIR = $(MAIN_PACKAGE_DIR)/pkg/apis/aggregator/v1
APIS_COMPOSITION_DIR = $(MAIN_PACKAGE_DIR)/pkg/apis/composition/v1
APIS_CREATOR_DIR = $(MAIN_PACKAGE_DIR)/pkg/apis/creator/v1
APIS_FORMATION_DIR = $(MAIN_PACKAGE_DIR)/pkg/apis/formation/v1
APIS_OPS_DIR = $(MAIN_PACKAGE_DIR)/pkg/apis/ops/v1
APIS_ORCHESTRATION_DIR = $(MAIN_PACKAGE_DIR)/pkg/apis/orchestration/v1
APIS_REPORTER_DIR = $(MAIN_PACKAGE_DIR)/pkg/apis/reporter/v1
SHAPES_API_DIRS = $(MAIN_PACKAGE_DIR)/pkg/orchestration/wiring/wiringplugin,$(MAIN_PACKAGE_DIR)/pkg/orchestration/wiring/wiringutil/knownshapes
ALL_DIRS=$(APIS_AGGREGATOR_DIR),$(APIS_COMPOSITION_DIR),$(APIS_CREATOR_DIR),$(APIS_FORMATION_DIR),$(APIS_OPS_DIR),$(APIS_ORCHESTRATION_DIR),$(APIS_REPORTER_DIR)

#===============================================================================

define check-git-status
if [[ -n $$(git --no-pager status -s 2> /dev/null) ]] ;\
then \
	echo "Git tree is not clean. Did you forget to commit some files? If you added new dependency please use dep ensure -add before creating PR." ;\
	git --no-pager status ;\
fi
endef

define check-git-status-in-ci
if [[ -n $$(git --no-pager status -s 2> /dev/null) ]] ;\
then \
	echo "Git tree is not clean. Did you forget to commit some files? If you added new dependency please use dep ensure -add before creating PR." ;\
	git --no-pager status ;\
	git --no-pager diff;\
	exit 1 ;\
fi
endef

.PHONY: check-git-status
check-git-status:
	$(check-git-status)

#===============================================================================

define dep-ensure
# Remove BUILD files first so that hash computed by dep is not tainted by them
find vendor -name "BUILD.bazel" -delete
dep ensure -v
# Remove the recursive symlink as Bazel complains with "infinite symlink expansion detected"
rm vendor/github.com/coreos/etcd/cmd/etcd || true
endef

define update-vendor-and-dep
# there is a cyclic dependency between goimports and dep
# dep requires up-to-date imports from go code files in order to determine dependent packages
# goimports requires packages in vendor in order to keep imports to packages with different name than (last fragment of) import path
# we can't break the cycle, but the next best thing to do seems to be
# 1. generating vendor if for some reason it is missing or altered
# 2. updating imports in go files
# 3. updating vendor and Gopkg.lock again, in case some new imports were added or removed in step 2
$(dep-ensure)
bazel run $(BAZEL_OPTIONS) //:goimports
$(dep-ensure)
# generate BUILD files in vendor
bazel run $(BAZEL_OPTIONS) //:gazelle
endef

.PHONY: update-vendor
update-vendor:
	$(update-vendor-and-dep)

.PHONY: verify-vendor
verify-vendor:
	# check git status before to make sure any changes made by this target are related only to vendor/dep
	$(check-git-status)
	$(dep-ensure)
	# generate BUILD files in vendor
	bazel run $(BAZEL_OPTIONS) //:gazelle
	$(check-git-status)

#===============================================================================

.PHONY: update-smith
update-smith:
	dep ensure -v -update github.com/atlassian/smith

.PHONY: bump-dependencies
bump-dependencies: \
	update-smith \
	update-vendor

#===============================================================================

.PHONY: goimports
goimports:
	bazel run $(BAZEL_OPTIONS) //:goimports

.PHONY: goimports-all
goimports-all: update-vendor
	bazel run $(BAZEL_OPTIONS) //:goimports
	$(dep-ensure)

# alias
.PHONY: fmt
fmt: goimports

#===============================================================================

define fmt-build-files
bazel run $(BAZEL_OPTIONS) //:buildozer
bazel run $(BAZEL_OPTIONS) //:buildifier
endef

.PHONY: generate-bazel
generate-bazel:
	bazel run $(BAZEL_OPTIONS) //:gazelle

.PHONY: fmt-bazel
fmt-bazel:
	$(fmt-build-files)

# Generates BUILD.bazel files
.PHONY: generate-bazel-all
generate-bazel-all: goimports update-vendor
	bazel run $(BAZEL_OPTIONS) //:gazelle
	$(fmt-build-files)

#===============================================================================

# This builds (not tests) all the tests with a 'manual' tag to ensure they can be compiled
define bazel-build-manual
bazel build $(BAZEL_OPTIONS) $$(bazel query $(BAZEL_OPTIONS) 'attr(tags, manual, kind(test, //... -//vendor/...))')
endef

define bazel-test-all
bazel test $(BAZEL_OPTIONS) \
	--test_env=KUBE_PATCH_CONVERSION_DETECTOR=true \
	--test_env=KUBE_CACHE_MUTATION_DETECTOR=true \
	-- //... -//vendor/...
$(bazel-build-manual)
endef

.PHONY: test
test: goimports generate-bazel
	bazel test $(BAZEL_OPTIONS) \
		--test_env=KUBE_PATCH_CONVERSION_DETECTOR=true \
		--test_env=KUBE_CACHE_MUTATION_DETECTOR=true \
		-- //... -//vendor/... -//build/...
	$(bazel-build-manual)

.PHONY: test-autowiring
test-autowiring: goimports generate-bazel
	bazel test $(BAZEL_OPTIONS) \
		--test_env=KUBE_PATCH_CONVERSION_DETECTOR=true \
		--test_env=KUBE_CACHE_MUTATION_DETECTOR=true \
		-- //pkg/orchestration/wiring/...

.PHONY: test-all
test-all: goimports generate-bazel-all
	$(bazel-test-all)

# This builds and runs unit tests ONLY. The caveat is that we don't run fmt and generate-bazel here. For speed. Run them yourself when needed.
.PHONY: quick-test
quick-test:
	bazel test $(BAZEL_OPTIONS) \
		--test_env=KUBE_PATCH_CONVERSION_DETECTOR=true \
		--test_env=KUBE_CACHE_MUTATION_DETECTOR=true \
		--build_tests_only \
		-- //... -//vendor/... -//build/...

#===============================================================================

define gometalinter
bazel run $(BAZEL_OPTIONS) //:gometalinter
endef

define buildifier-check
bazel run $(BAZEL_OPTIONS) //:buildifier_check
bazel run $(BAZEL_OPTIONS) //:buildifier_lint
endef

.PHONY: lint
lint:
	$(gometalinter)

.PHONY: lint-fast
lint-fast:
	build/local/script/fast_lint.sh

.PHONY: lint-fast-all
lint-fast-all: goimports
	$(buildifier-check)
	build/local/script/fast_lint.sh

.PHONY: lint-all
lint-all: goimports
	$(buildifier-check)
	$(gometalinter)

#===============================================================================

# Code with updated imports but based on possibly out of sync vendor
.PHONY: code-with-updated-imports
code-with-updated-imports:
	# don't update vendor as it changes very rarely AND should be updated with update-vendor anyway
	bazel run $(BAZEL_OPTIONS) //:goimports

# Code with all Bazel BUILD files but based on possibly out of sync vendor
.PHONY: code-with-all-bazel-build-files
code-with-all-bazel-build-files: code-with-updated-imports
	bazel run $(BAZEL_OPTIONS) //:gazelle
	$(fmt-build-files)

.PHONY: build-and-test-all
build-and-test-all: code-with-updated-imports code-with-all-bazel-build-files
	$(bazel-test-all)

.PHONY: fast-lint-changed-packages
fast-lint-changed-packages: code-with-updated-imports
	build/local/script/fast_lint.sh

#===============================================================================

# Runs everything (well, except generating clients).
.PHONY: all
all:
	$(update-vendor-and-dep)
	# bazel run //:goimports is already part of update-vendor-and-dep
	# bazel run //:gazelle is already part of update-vendor-and-dep
	$(fmt-build-files)
	$(buildifier-check)
	$(bazel-test-all)
	$(gometalinter)
	$(check-git-status)

# Does what CI does. Consider (lunch or coffee) xor make pr, because lint is slooow.
.PHONY: all-ci
all-ci:
	# don't try to fix vendor, just check if it is in sync
	$(dep-ensure)
	bazel run $(BAZEL_OPTIONS) //:goimports
	bazel run $(BAZEL_OPTIONS) //:gazelle
	$(fmt-build-files)
	$(buildifier-check)
	$(bazel-test-all)
	$(gometalinter)
	$(check-git-status-in-ci)

.PHONY: check-all-automagic-changes-were-commited-before-ci
check-all-automagic-changes-were-commited-before-ci:
	# don't try to fix vendor, just check if it is in sync
	$(dep-ensure)
	bazel run $(BAZEL_OPTIONS) //:goimports
	bazel run $(BAZEL_OPTIONS) //:gazelle
	$(fmt-build-files)
	$(check-git-status-in-ci)

.PHONY: build-and-test-in-ci
build-and-test-in-ci:
	$(bazel-test-all)

.PHONY: lint-all-in-ci
lint-all-in-ci:
	$(buildifier-check)
	$(gometalinter)

#===============================================================================

# Run it after git rebase origin but before creating PR. It auto generates / re-formats files so that your build won't
# fail because of dirty git status, but excludes slow lint so it doesn't take forever to run.
.PHONY: pr
pr: build-and-test-all fast-lint-changed-packages
	$(check-git-status)

#===============================================================================

.PHONY: run-orchestrationadmission
run-orchestrationadmission:
	bazel run \
		//cmd/orchestrationadmission:orchestrationadmission_race \
		-- \
		-config "$(CURDIR)/build/local/orchestrationadmission/config.yaml" \

#===============================================================================

.PHONY: run-orchestration
run-orchestration: validate-cluster
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	bazel run $(BAZEL_OPTIONS) \
		//cmd/orchestration:orchestration_race \
		-- \
		-config "$(CURDIR)/build/local/orchestration/config.yaml" \
		-client-config-from=file \
		-client-config-file-name='$(KUBECONFIG)' \
		-client-config-context='$(KUBECONTEXT)'

#===============================================================================

.PHONY: run-smith
run-smith: validate-cluster
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	bazel run $(BAZEL_OPTIONS) \
		//cmd/smith:smith_race \
		-- \
		-log-encoding=console \
		-leader-elect \
		-client-config-from=file \
		-client-config-file-name='$(KUBECONFIG)' \
		-client-config-context='$(KUBECONTEXT)'

#===============================================================================

.PHONY: generate
generate: \
	generate-clients \
	generate-deepcopy \
	generate-sets \
	generate-bazel \
	goimports

#===============================================================================

.PHONY: generate-clients
generate-clients: \
	generate-deployinator-client \
	generate-composition-client \
	generate-creator-client \
	generate-formation-client \
	generate-ops-client \
	generate-orchestration-client \
	generate-reporter-client

#===============================================================================

.PHONY: update-deployinator-spec
update-deployinator-spec:
	curl -s https://deployinator-trebuchet.prod.atl-paas.net/api/swagger.json | jq '.' > pkg/releases/deployinator-trebuchet.json

#===============================================================================

.PHONY: generate-deployinator-client
generate-deployinator-client:
	bazel build $(BAZEL_OPTIONS) //vendor/github.com/go-swagger/go-swagger/cmd/swagger
	./bazel-bin/vendor/github.com/go-swagger/go-swagger/cmd/swagger/$(BINARY_PREFIX_DIRECTORY)/swagger $(VERIFY_CODE) \
	generate client -f pkg/releases/deployinator-trebuchet.json --skip-validation  \
	-t "pkg/releases/deployinator" \
	-O resolve \
	-O resolveBatch \
	-A deployinator \
	-M ResolutionResponseType \
	-M BatchResolutionResponseType \
	-M PageDetails \
	-M ErrorResponse

#===============================================================================

.PHONY: generate-composition-client
generate-composition-client:
	bazel build $(BAZEL_OPTIONS) //vendor/k8s.io/code-generator/cmd/client-gen
	./bazel-bin/vendor/k8s.io/code-generator/cmd/client-gen/$(BINARY_PREFIX_DIRECTORY)/client-gen $(VERIFY_CODE) \
	--input-base "$(MAIN_PACKAGE_DIR)/pkg/apis" \
	--input "composition/v1" \
	--clientset-path "$(MAIN_PACKAGE_DIR)/pkg/composition" \
	--clientset-name "client" \
	--go-header-file "build/code-generator/boilerplate.go.txt"

#===============================================================================

.PHONY: generate-creator-client
generate-creator-client:
	bazel build $(BAZEL_OPTIONS) //vendor/k8s.io/code-generator/cmd/client-gen
	./bazel-bin/vendor/k8s.io/code-generator/cmd/client-gen/$(BINARY_PREFIX_DIRECTORY)/client-gen $(VERIFY_CODE) \
	--input-base "$(MAIN_PACKAGE_DIR)/pkg/apis" \
	--input "creator/v1" \
	--clientset-path "$(MAIN_PACKAGE_DIR)/pkg/creator" \
	--clientset-name "client" \
	--go-header-file "build/code-generator/boilerplate.go.txt"

#===============================================================================

.PHONY: generate-formation-client
generate-formation-client:
	bazel build $(BAZEL_OPTIONS) //vendor/k8s.io/code-generator/cmd/client-gen
	./bazel-bin/vendor/k8s.io/code-generator/cmd/client-gen/$(BINARY_PREFIX_DIRECTORY)/client-gen $(VERIFY_CODE) \
	--input-base "$(MAIN_PACKAGE_DIR)/pkg/apis" \
	--input "formation/v1" \
	--clientset-path "$(MAIN_PACKAGE_DIR)/pkg/formation" \
	--clientset-name "client" \
	--go-header-file "build/code-generator/boilerplate.go.txt"

#===============================================================================

.PHONY: generate-ops-client
generate-ops-client:
	bazel build $(BAZEL_OPTIONS) //vendor/k8s.io/code-generator/cmd/client-gen
	./bazel-bin/vendor/k8s.io/code-generator/cmd/client-gen/$(BINARY_PREFIX_DIRECTORY)/client-gen $(VERIFY_CODE) \
	--input-base "$(MAIN_PACKAGE_DIR)/pkg/apis" \
	--input "ops/v1" \
	--clientset-path "$(MAIN_PACKAGE_DIR)/pkg/ops" \
	--clientset-name "client" \
	--go-header-file "build/code-generator/boilerplate.go.txt"

#===============================================================================

.PHONY: generate-orchestration-client
generate-orchestration-client:
	bazel build $(BAZEL_OPTIONS) //vendor/k8s.io/code-generator/cmd/client-gen
	./bazel-bin/vendor/k8s.io/code-generator/cmd/client-gen/$(BINARY_PREFIX_DIRECTORY)/client-gen $(VERIFY_CODE) \
	--input-base "$(MAIN_PACKAGE_DIR)/pkg/apis" \
	--input "orchestration/v1" \
	--clientset-path "$(MAIN_PACKAGE_DIR)/pkg/orchestration" \
	--clientset-name "client" \
	--go-header-file "build/code-generator/boilerplate.go.txt"

#===============================================================================

.PHONY: generate-reporter-client
generate-reporter-client:
	bazel build $(BAZEL_OPTIONS) //vendor/k8s.io/code-generator/cmd/client-gen
	./bazel-bin/vendor/k8s.io/code-generator/cmd/client-gen/$(BINARY_PREFIX_DIRECTORY)/client-gen $(VERIFY_CODE) \
	--input-base "$(MAIN_PACKAGE_DIR)/pkg/apis" \
	--input "reporter/v1" \
	--clientset-path "$(MAIN_PACKAGE_DIR)/pkg/reporter" \
	--clientset-name "client" \
	--go-header-file "build/code-generator/boilerplate.go.txt"

#===============================================================================

.PHONY: generate-deepcopy
generate-deepcopy:
	find pkg -name zz_generated.deepcopy.go -delete
	bazel build $(BAZEL_OPTIONS) //vendor/k8s.io/code-generator/cmd/deepcopy-gen
	./bazel-bin/vendor/k8s.io/code-generator/cmd/deepcopy-gen/$(BINARY_PREFIX_DIRECTORY)/deepcopy-gen $(VERIFY_CODE) \
	--input-dirs "$(ALL_DIRS),$(SHAPES_API_DIRS)" \
	--go-header-file "build/code-generator/boilerplate.go.txt" \
	--output-file-base zz_generated.deepcopy

#===============================================================================

.PHONY: generate-sets
generate-sets:
	rm -rf pkg/util/sets
	bazel build $(BAZEL_OPTIONS) //vendor/k8s.io/gengo/examples/set-gen
	./bazel-bin/vendor/k8s.io/gengo/examples/set-gen/$(BINARY_PREFIX_DIRECTORY)/set-gen $(VERIFY_CODE) \
	--input-dirs "$(MAIN_PACKAGE_DIR),$(ALL_DIRS)" \
	--go-header-file "build/code-generator/boilerplate.go.txt" \
	--output-package '$(MAIN_PACKAGE_DIR)/pkg/util/sets'
	# Hacked removal of bad bool and byte sets
	# For some reason each set is generated a lot of times into the same file. A bug.
	rm pkg/util/sets/bool.go pkg/util/sets/byte.go

#===============================================================================

.PHONY: print-state-crd
print-state-crd:
	bazel run //cmd/crd -- -output-format=yaml -resource=state

.PHONY: print-route-crd
print-route-crd:
	bazel run //cmd/crd -- -output-format=yaml -resource=route

.PHONY: print-sd-crd
print-sd-crd:
	bazel run //cmd/crd -- -output-format=yaml -resource=sd

.PHONY: print-ld-crd
print-ld-crd:
	bazel run //cmd/crd -- -output-format=yaml -resource=ld

#===============================================================================
