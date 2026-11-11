// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"github.com/openchoreo/openchoreo/tools/lsp/hover/internal/catalog"
	"log"
	"os"
)

func main() {
	dir := flag.String("crd-dir", "config/crd/bases", "CRD directory")
	out := flag.String("out", "tools/lsp/hover/field_descriptions.json", "catalog output")
	flag.Parse()
	data, err := catalog.Generate(*dir)
	if err != nil {
		log.Fatal(err)
	}
	if err = os.WriteFile(*out, append(data, '\n'), 0644); err != nil {
		log.Fatal(err)
	}
}
