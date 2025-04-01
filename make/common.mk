# Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
#
# WSO2 Inc. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied. See the License for the
# specific language governing permissions and limitations
# under the License.

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
