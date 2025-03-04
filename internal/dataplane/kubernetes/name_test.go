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
