// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package celvalidator

import (
	"fmt"
	"strings"
	"testing"
)

func TestTraitPatchPathExpressions(t *testing.T) {
	for _, kind := range []string{"Trait", "ClusterTrait"} {
		for _, op := range []string{"add", "replace", "remove"} {
			for _, tc := range []struct{ expr, code string }{
				{"parameters.containerName", ""},
				{"parameters.contanerName", CodeCELTypeError},
				{"paramenters.containerName", CodeCELUnknownName},
				{"parameters.containerName +", CodeCELParseError},
				{"parameters.containerName + 1", CodeCELUnknownFunction},
				{"metadata.name", ""},
				{"parameters.labels.dynamicKey", ""},
			} {
				t.Run(kind+"/"+op+"/"+tc.expr, func(t *testing.T) {
					source := fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: %s
metadata: {name: storage}
spec:
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        containerName: {type: string}
        labels:
          type: object
          additionalProperties: {type: string}
  patches:
    - target: {group: apps, version: v1, kind: Deployment}
      operations:
        - op: %s
          path: /spec/template/spec/containers/[?(@.name=='${%s}')]/volumeMounts/-
          value: test
`, kind, op, tc.expr)
					findings := validateParses(t, source)
					if tc.code == "" {
						if len(findings) != 0 {
							t.Fatal(findings)
						}
						return
					}
					if len(findings) != 1 || findings[0].Code != tc.code {
						t.Fatalf("%+v", findings)
					}
					if findings[0].Range.Start.Line != 17 {
						t.Fatalf("wrong source line: %+v", findings[0])
					}
					if strings.Contains(tc.expr, "contanerName") && !strings.Contains(findings[0].Message, `Did you mean "containerName"?`) {
						t.Fatal(findings[0])
					}
				})
			}
		}
	}
}

func TestPatchPathLoopAndMultipleExpressions(t *testing.T) {
	source := `apiVersion: openchoreo.dev/v1alpha1
kind: Trait
metadata: {name: storage}
spec:
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        containers:
          type: array
          items: {type: string}
  patches:
    - forEach: ${parameters.containers}
      var: container
      target: {group: apps, version: v1, kind: Deployment}
      operations:
        - op: remove
          path: /spec/${metadata.name}/${container}
`
	if findings := validateParses(t, source); len(findings) != 0 {
		t.Fatal(findings)
	}
	source = strings.Replace(source, "${container}", "${unknownContainer}", 1)
	if findings := validateParses(t, source); len(findings) != 1 || findings[0].Code != CodeCELUnknownName {
		t.Fatal(findings)
	}
}
