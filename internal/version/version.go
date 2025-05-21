/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
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
