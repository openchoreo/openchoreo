# This makefile contains various helper functions and variables used across other makefiles.

PROJECT_BIN_DIR := $(PROJECT_DIR)/bin


# Store the short git sha of latest commit to be used in the make targets
GIT_REV := $(shell git rev-parse --short HEAD)

# Helper functions for logging
define log-info
echo -e "\033[36m--->$1\033[0m"
endef

define log-error
echo -e "\033[0;31m--->$1\033[0m"
endef

# Helper functions to get the OS and architecture from the platform string
# Format: <os>/<arch>
getPlatformOS = $(word 1, $(subst /, ,$(1)))
getPlatformArch = $(word 2, $(subst /, ,$(1)))
