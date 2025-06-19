// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLicenseCheck(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "License Check Suite")
}

var _ = Describe("License Header Checker", func() {
	var tmpDir string
	var header string

	const (
		holder  = "The OpenChoreo Authors"
		license = "apache"
	)

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "license-check-test")
		Expect(err).NotTo(HaveOccurred())

		header = shortHeader(time.Now().Format("2006"), holder, license)
	})

	AfterEach(func() {
		_ = os.RemoveAll(tmpDir)
	})

	writeFile := func(name, content string) string {
		p := filepath.Join(tmpDir, name)
		Expect(os.WriteFile(p, []byte(content), 0o644)).To(Succeed())
		return p
	}

	It("detects a valid header", func() {
		content := header + "\n\npackage main\n\nfunc main() {}\n"
		path := writeFile("valid.go", content)

		ok, err := hasValidHeader(path, holder, license)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
	})

	It("detects a missing header", func() {
		path := writeFile("missing.go", "package main\n\nfunc main() {}\n")

		ok, err := hasValidHeader(path, holder, license)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())
	})

	It("detects an incorrect holder", func() {
		bad := `// Copyright 2025 Someone Else
// SPDX-License-Identifier: Apache-2.0

package main

func main() {}
`
		path := writeFile("badholder.go", bad)

		ok, err := hasValidHeader(path, holder, license)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())
	})

	It("adds a header when missing", func() {
		path := writeFile("add.go", "package main\n\nfunc main() {}\n")

		updated, err := process(path, header, holder, license, true /* fix */)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated).To(BeTrue())

		ok, err := hasValidHeader(path, holder, license)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
	})

	It("reports non-compliance in check-only mode", func() {
		path := writeFile("checkonly.go", "package main\n\nfunc main() {}\n")

		updated, err := process(path, header, holder, license, false /* check only */)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated).To(BeTrue()) // non-compliant

		ok, err := hasValidHeader(path, holder, license)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())
	})

	It("walks a directory in check-only mode", func() {
		writeFile("walk1.go", "package main\n\nfunc main() {}\n")

		files, err := walk(tmpDir, header, holder, license, false /* check only */)
		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(HaveLen(1))
		Expect(strings.HasSuffix(files[0], "walk1.go")).To(BeTrue())
	})

	It("walks a directory and fixes headers", func() {
		writeFile("walk2.go", "package main\n\nfunc main() {}\n")

		files, err := walk(tmpDir, header, holder, license, true /* fix */)
		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(HaveLen(1))
		Expect(strings.HasSuffix(files[0], "walk2.go")).To(BeTrue())

		ok, err := hasValidHeader(files[0], holder, license)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
	})
})
