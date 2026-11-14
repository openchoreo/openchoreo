// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"log"
	"os"

	"github.com/openchoreo/openchoreo/tools/lint/cel-validator/internal/contextdocs"
)

func main() {
	root := flag.String("pipeline-dir", "internal/pipeline", "pipeline source")
	out := flag.String("out", "tools/lint/cel-validator/context_descriptions.json", "output catalog")
	flag.Parse()
	raw, err := contextdocs.Generate(*root)
	if err != nil {
		log.Fatal(err)
	}
	if err = os.WriteFile(*out, append(raw, '\n'), 0600); err != nil {
		log.Fatal(err)
	}
}
