/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package version

import (
	"runtime"
)

// This package contains the version information used by all the Go binaries in this project.
// These version information is set at build time using the -X linker flag.
// Please make sure to update/verify the linker flags in the Makefile after making any changes to this file.

var (
	// Set by the linker at build time
	componentName = "not-set"
	buildTime     = "not-set"
	gitRevision   = "not-set"
	version       = "not-set"
)

type Info struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	GitRevision string `json:"gitRevision"`
	BuildTime   string `json:"buildTime"`
	GoOS        string `json:"goOS"`
	GoArch      string `json:"goArch"`
	GoVersion   string `json:"goVersion"`
}

func Get() Info {
	return Info{
		Name:        componentName,
		Version:     version,
		GitRevision: gitRevision,
		BuildTime:   buildTime,
		GoOS:        runtime.GOOS,
		GoArch:      runtime.GOARCH,
		GoVersion:   runtime.Version(),
	}
}

func GetLogKeyValues() []any {
	v := Get()
	return []any{
		"name", v.Name,
		"version", v.Version,
		"gitRevision", v.GitRevision,
		"buildTime", v.BuildTime,
		"goOS", v.GoOS,
		"goArch", v.GoArch,
		"goVersion", v.GoVersion,
	}
}
