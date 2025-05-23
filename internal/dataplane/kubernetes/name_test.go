/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package kubernetes

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GenerateK8sName", func() {
	DescribeTable("should generate a valid K8s name",
		func(input []string, expectedName string) {
			generatedName := GenerateK8sName(input...)

			Expect(generatedName).To(Equal(expectedName))
			Expect(len(generatedName)).To(BeNumerically("<=", 253))
		},
		Entry("for normal names",
			[]string{"project", "component"},
			"project-component-932d1646",
		),
		Entry("for names with special characters",
			[]string{"My_Project$", "Component@Name"},
			"my-project-component-name-40314333",
		),
		Entry("for very long names",
			[]string{strings.Repeat("a", 300), "component"},
			fmt.Sprintf("%s-component-a250aee3", strings.Repeat("a", 122)),
		),
		Entry("for names starting and ending with invalid characters",
			[]string{"-invalid-start", "invalid-end-"},
			"invalid-start-invalid-end-f124257d",
		),
		Entry("for empty names",
			[]string{"", ""},
			"3973e022",
		),
		Entry("for names with only invalid characters",
			[]string{"!!!", "###"},
			"606b424e",
		),
		Entry("for names exceeding max length after sanitization",
			[]string{strings.Repeat("a", 260), strings.Repeat("b", 260)},
			fmt.Sprintf("%s-%s-654ba7cd", strings.Repeat("a", 122), strings.Repeat("b", 121)),
		),
		Entry("for names with uppercase letters",
			[]string{"ProjectName", "ComponentName"},
			"projectname-componentname-755d4ced",
		),
		Entry("for names with dots",
			[]string{"project.name", "component.name"},
			"project.name-component.name-851db5b2",
		),
		Entry("for names with underscores and spaces",
			[]string{"project_name with spaces", "component_name"},
			"project-name-with-spaces-component-name-101f1326",
		),
	)
})
