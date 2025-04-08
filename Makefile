# All the make targets are implemented in the make/*.mk files.
# To see all the available targets, run `make help`.

PROJECT_DIR := $(realpath $(dir $(abspath $(lastword $(MAKEFILE_LIST)))))

#-----------------------------------------------------------------------------
# Makefile includes
#-----------------------------------------------------------------------------
include make/common.mk
include make/tools.mk
include make/golang.mk
include make/lint.mk
include make/docker.mk
include make/kube.mk
include make/helm.mk
