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
	var currentYear string

	const holder = "The OpenChoreo Authors"

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "license-check-test")
		Expect(err).NotTo(HaveOccurred())

		currentYear = time.Now().Format("2006")
		header = shortHeader(currentYear, holder)
	})

	AfterEach(func() {
		_ = os.RemoveAll(tmpDir)
	})

	writeFile := func(name, content string) string {
		p := filepath.Join(tmpDir, name)
		Expect(os.MkdirAll(filepath.Dir(p), 0o755)).To(Succeed())
		Expect(os.WriteFile(p, []byte(content), 0o644)).To(Succeed())
		return p
	}

	// --- isGoFile ---

	Describe("isGoFile", func() {
		It("returns true for .go extension", func() {
			Expect(isGoFile("main.go")).To(BeTrue())
		})

		It("returns true for .go file with path", func() {
			Expect(isGoFile("path/to/main.go")).To(BeTrue())
		})

		It("returns false for .txt files", func() {
			Expect(isGoFile("file.txt")).To(BeFalse())
		})

		It("returns false for files with no extension", func() {
			Expect(isGoFile("Makefile")).To(BeFalse())
		})

		It("returns false for .go.bak (not final extension)", func() {
			Expect(isGoFile("file.go.bak")).To(BeFalse())
		})

		It("returns false for uppercase .GO", func() {
			Expect(isGoFile("main.GO")).To(BeFalse())
		})

		It("returns false for .py files", func() {
			Expect(isGoFile("script.py")).To(BeFalse())
		})

		It("returns false for empty string", func() {
			Expect(isGoFile("")).To(BeFalse())
		})
	})

	// --- shortHeader ---

	Describe("shortHeader", func() {
		It("generates correct two-line format", func() {
			h := shortHeader("2025", "The OpenChoreo Authors")
			expected := "// Copyright 2025 The OpenChoreo Authors\n// SPDX-License-Identifier: Apache-2.0"
			Expect(h).To(Equal(expected))
		})

		It("uses the provided year and holder", func() {
			h := shortHeader("2030", "Acme Corp")
			Expect(h).To(ContainSubstring("2030"))
			Expect(h).To(ContainSubstring("Acme Corp"))
		})

		It("always uses Apache-2.0 license identifier", func() {
			h := shortHeader("2025", "Anyone")
			Expect(h).To(ContainSubstring("Apache-2.0"))
		})
	})

	// --- hasValidHeader ---

	Describe("hasValidHeader", func() {
		It("detects a valid header with current year", func() {
			content := header + "\n\npackage main\n\nfunc main() {}\n"
			path := writeFile("valid.go", content)

			ok, err := hasValidHeader(path, holder)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
		})

		It("accepts a valid header from a previous year", func() {
			oldHeader := shortHeader("2020", holder)
			content := oldHeader + "\n\npackage main\n"
			path := writeFile("oldyear.go", content)

			ok, err := hasValidHeader(path, holder)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
		})

		It("accepts header with leading blank lines", func() {
			content := "\n\n" + header + "\n\npackage main\n"
			path := writeFile("leadingblanks.go", content)

			ok, err := hasValidHeader(path, holder)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
		})

		It("rejects file with missing header", func() {
			path := writeFile("missing.go", "package main\n\nfunc main() {}\n")

			ok, err := hasValidHeader(path, holder)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})

		It("rejects file with incorrect holder", func() {
			bad := "// Copyright 2025 Someone Else\n// SPDX-License-Identifier: Apache-2.0\n\npackage main\n"
			path := writeFile("badholder.go", bad)

			ok, err := hasValidHeader(path, holder)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})

		It("rejects file with wrong license identifier", func() {
			bad := "// Copyright 2025 The OpenChoreo Authors\n// SPDX-License-Identifier: MIT\n\npackage main\n"
			path := writeFile("wronglicense.go", bad)

			ok, err := hasValidHeader(path, holder)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})

		It("rejects file with only copyright line and no SPDX", func() {
			content := "// Copyright 2025 The OpenChoreo Authors\npackage main\n"
			path := writeFile("nospdx.go", content)

			ok, err := hasValidHeader(path, holder)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})

		It("rejects file with copyright and SPDX but no blank line after", func() {
			content := "// Copyright 2025 The OpenChoreo Authors\n// SPDX-License-Identifier: Apache-2.0\npackage main\n"
			path := writeFile("noblankafter.go", content)

			ok, err := hasValidHeader(path, holder)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})

		It("rejects an empty file", func() {
			path := writeFile("empty.go", "")

			ok, err := hasValidHeader(path, holder)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})

		It("rejects a file with only blank lines", func() {
			path := writeFile("blanks.go", "\n\n\n")

			ok, err := hasValidHeader(path, holder)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})

		It("rejects lowercase copyright keyword", func() {
			content := "// copyright 2025 The OpenChoreo Authors\n// SPDX-License-Identifier: Apache-2.0\n\npackage main\n"
			path := writeFile("lowercase.go", content)

			ok, err := hasValidHeader(path, holder)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})

		It("rejects copyright line without year", func() {
			content := "// Copyright The OpenChoreo Authors\n// SPDX-License-Identifier: Apache-2.0\n\npackage main\n"
			path := writeFile("noyear.go", content)

			ok, err := hasValidHeader(path, holder)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})

		It("rejects header with hash-style comments", func() {
			content := "# Copyright 2025 The OpenChoreo Authors\n# SPDX-License-Identifier: Apache-2.0\n\npackage main\n"
			path := writeFile("hashcomment.go", content)

			ok, err := hasValidHeader(path, holder)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})

		It("rejects SPDX line with wrong format", func() {
			content := "// Copyright 2025 The OpenChoreo Authors\n// SPDX License Identifier: Apache-2.0\n\npackage main\n"
			path := writeFile("badspdx.go", content)

			ok, err := hasValidHeader(path, holder)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})

		It("returns error for non-existent file", func() {
			_, err := hasValidHeader("/nonexistent/path.go", holder)
			Expect(err).To(HaveOccurred())
		})

		It("rejects file that starts with code before copyright", func() {
			content := "package main\n// Copyright 2025 The OpenChoreo Authors\n// SPDX-License-Identifier: Apache-2.0\n"
			path := writeFile("codebeforecopyright.go", content)

			ok, err := hasValidHeader(path, holder)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})
	})

	// --- stripExistingHeader ---

	Describe("stripExistingHeader", func() {
		It("strips a valid header", func() {
			src := []byte("// Copyright 2025 The OpenChoreo Authors\n// SPDX-License-Identifier: Apache-2.0\n\npackage main\n")
			result := stripExistingHeader(src)
			Expect(string(result)).To(Equal("package main\n"))
		})

		It("strips a header with wrong holder", func() {
			src := []byte("// Copyright 2025 Someone Else\n// SPDX-License-Identifier: Apache-2.0\n\npackage main\n")
			result := stripExistingHeader(src)
			Expect(string(result)).To(Equal("package main\n"))
		})

		It("strips a header with a different license identifier", func() {
			src := []byte("// Copyright 2025 The OpenChoreo Authors\n// SPDX-License-Identifier: MIT\n\npackage main\n")
			result := stripExistingHeader(src)
			Expect(string(result)).To(Equal("package main\n"))
		})

		It("strips header preceded by leading blank lines", func() {
			src := []byte("\n\n// Copyright 2025 Foo\n// SPDX-License-Identifier: Apache-2.0\n\npackage main\n")
			result := stripExistingHeader(src)
			Expect(string(result)).To(Equal("package main\n"))
		})

		It("strips header without trailing blank line", func() {
			src := []byte("// Copyright 2025 Foo\n// SPDX-License-Identifier: Apache-2.0\npackage main\n")
			result := stripExistingHeader(src)
			Expect(string(result)).To(Equal("package main\n"))
		})

		It("strips only the first blank line after header", func() {
			src := []byte("// Copyright 2025 Foo\n// SPDX-License-Identifier: Apache-2.0\n\n\npackage main\n")
			result := stripExistingHeader(src)
			Expect(string(result)).To(Equal("\npackage main\n"))
		})

		It("handles file with only a header (no code after)", func() {
			src := []byte("// Copyright 2025 Foo\n// SPDX-License-Identifier: Apache-2.0\n")
			result := stripExistingHeader(src)
			Expect(string(result)).To(Equal(""))
		})

		It("returns unchanged when no header present", func() {
			src := []byte("package main\n\nfunc main() {}\n")
			result := stripExistingHeader(src)
			Expect(string(result)).To(Equal("package main\n\nfunc main() {}\n"))
		})

		It("returns unchanged for empty input", func() {
			result := stripExistingHeader([]byte(""))
			Expect(string(result)).To(Equal(""))
		})

		It("strips copyright-only header (missing SPDX)", func() {
			src := []byte("// Copyright 2025 Foo\npackage main\n")
			result := stripExistingHeader(src)
			Expect(string(result)).To(Equal("package main\n"))
		})

		It("strips copyright-only header with trailing blank line", func() {
			src := []byte("// Copyright 2025 Foo\n\npackage main\n")
			result := stripExistingHeader(src)
			Expect(string(result)).To(Equal("package main\n"))
		})

		It("strips SPDX-only header (missing copyright)", func() {
			src := []byte("// SPDX-License-Identifier: Apache-2.0\npackage main\n")
			result := stripExistingHeader(src)
			Expect(string(result)).To(Equal("package main\n"))
		})

		It("strips SPDX-only header with trailing blank line", func() {
			src := []byte("// SPDX-License-Identifier: Apache-2.0\n\npackage main\n")
			result := stripExistingHeader(src)
			Expect(string(result)).To(Equal("package main\n"))
		})

		It("does not strip non-copyright comment lines", func() {
			src := []byte("// Package main does stuff.\n// More docs.\n\npackage main\n")
			result := stripExistingHeader(src)
			Expect(string(result)).To(Equal("// Package main does stuff.\n// More docs.\n\npackage main\n"))
		})

		It("strips header from a different year", func() {
			src := []byte("// Copyright 2018 Old Author\n// SPDX-License-Identifier: Apache-2.0\n\npackage main\n")
			result := stripExistingHeader(src)
			Expect(string(result)).To(Equal("package main\n"))
		})
	})

	// --- process ---

	Describe("process", func() {
		Context("fix mode", func() {
			It("adds a header when missing", func() {
				path := writeFile("add.go", "package main\n\nfunc main() {}\n")

				updated, err := process(path, header, holder, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated).To(BeTrue())

				ok, err := hasValidHeader(path, holder)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeTrue())
			})

			It("replaces header with wrong holder instead of duplicating", func() {
				bad := "// Copyright 2025 Someone Else\n// SPDX-License-Identifier: Apache-2.0\n\npackage main\n\nfunc main() {}\n"
				path := writeFile("replace.go", bad)

				updated, err := process(path, header, holder, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated).To(BeTrue())

				content, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())

				ok, err := hasValidHeader(path, holder)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeTrue())

				Expect(strings.Count(string(content), "// Copyright")).To(Equal(1))
			})

			It("replaces header with wrong license identifier", func() {
				bad := "// Copyright 2025 The OpenChoreo Authors\n// SPDX-License-Identifier: MIT\n\npackage main\n"
				path := writeFile("wronglicense.go", bad)

				updated, err := process(path, header, holder, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated).To(BeTrue())

				ok, err := hasValidHeader(path, holder)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeTrue())

				content, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())
				Expect(strings.Count(string(content), "// Copyright")).To(Equal(1))
				Expect(strings.Count(string(content), "SPDX-License-Identifier")).To(Equal(1))
			})

			It("replaces copyright-only header (missing SPDX) without duplicating", func() {
				bad := "// Copyright 2026 The OpenChoreo Authors\n\npackage main\n\nfunc main() {}\n"
				path := writeFile("copyrightonly.go", bad)

				updated, err := process(path, header, holder, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated).To(BeTrue())

				content, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())

				ok, err := hasValidHeader(path, holder)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeTrue())

				Expect(strings.Count(string(content), "// Copyright")).To(Equal(1))
			})

			It("replaces SPDX-only header (missing copyright) without duplicating", func() {
				bad := "// SPDX-License-Identifier: Apache-2.0\n\npackage main\n\nfunc main() {}\n"
				path := writeFile("spdxonly.go", bad)

				updated, err := process(path, header, holder, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated).To(BeTrue())

				content, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())

				ok, err := hasValidHeader(path, holder)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeTrue())

				Expect(strings.Count(string(content), "SPDX-License-Identifier")).To(Equal(1))
			})

			It("replaces header with both wrong holder and wrong license", func() {
				bad := "// Copyright 2025 Wrong Corp\n// SPDX-License-Identifier: MIT\n\npackage main\n"
				path := writeFile("bothwrong.go", bad)

				updated, err := process(path, header, holder, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated).To(BeTrue())

				ok, err := hasValidHeader(path, holder)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeTrue())

				content, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())
				Expect(strings.Count(string(content), "// Copyright")).To(Equal(1))
			})

			It("does not modify a compliant file", func() {
				content := header + "\n\npackage main\n\nfunc main() {}\n"
				path := writeFile("compliant.go", content)

				updated, err := process(path, header, holder, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated).To(BeFalse())

				result, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(result)).To(Equal(content))
			})

			It("preserves file content after adding header", func() {
				original := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
				path := writeFile("preserve.go", original)

				_, err := process(path, header, holder, true)
				Expect(err).NotTo(HaveOccurred())

				content, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(HaveSuffix(original))
			})

			It("preserves file content after replacing header", func() {
				code := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
				bad := "// Copyright 2025 Wrong Holder\n// SPDX-License-Identifier: Apache-2.0\n\n" + code
				path := writeFile("preservereplace.go", bad)

				_, err := process(path, header, holder, true)
				Expect(err).NotTo(HaveOccurred())

				content, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(HaveSuffix(code))
			})

			It("is idempotent — running fix twice produces the same result", func() {
				path := writeFile("idempotent.go", "package main\n")

				_, err := process(path, header, holder, true)
				Expect(err).NotTo(HaveOccurred())
				first, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())

				_, err = process(path, header, holder, true)
				Expect(err).NotTo(HaveOccurred())
				second, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())

				Expect(string(second)).To(Equal(string(first)))
			})

			It("returns error for non-existent file", func() {
				_, err := process("/nonexistent/path.go", header, holder, true)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("check-only mode", func() {
			It("reports non-compliance without modifying the file", func() {
				original := "package main\n\nfunc main() {}\n"
				path := writeFile("checkonly.go", original)

				updated, err := process(path, header, holder, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated).To(BeTrue())

				content, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal(original))
			})

			It("does not report a compliant file", func() {
				content := header + "\n\npackage main\n"
				path := writeFile("compliantcheck.go", content)

				updated, err := process(path, header, holder, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated).To(BeFalse())
			})

			It("reports file with wrong holder as non-compliant", func() {
				bad := "// Copyright 2025 Someone Else\n// SPDX-License-Identifier: Apache-2.0\n\npackage main\n"
				path := writeFile("wrongholder.go", bad)

				updated, err := process(path, header, holder, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated).To(BeTrue())
			})

			It("reports empty file as non-compliant", func() {
				path := writeFile("empty.go", "")

				updated, err := process(path, header, holder, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated).To(BeTrue())
			})
		})
	})

	// --- walk ---

	Describe("walk", func() {
		It("finds non-compliant files in check-only mode", func() {
			writeFile("walk1.go", "package main\n\nfunc main() {}\n")

			files, err := walk(tmpDir, header, holder, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(1))
			Expect(files[0]).To(HaveSuffix("walk1.go"))
		})

		It("fixes files and returns their paths", func() {
			writeFile("walk2.go", "package main\n\nfunc main() {}\n")

			files, err := walk(tmpDir, header, holder, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(1))
			Expect(files[0]).To(HaveSuffix("walk2.go"))

			ok, err := hasValidHeader(files[0], holder)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
		})

		It("skips non-Go files", func() {
			writeFile("readme.md", "# README")
			writeFile("config.yaml", "key: value")
			writeFile("script.py", "print('hello')")
			writeFile("main.go", "package main\n")

			files, err := walk(tmpDir, header, holder, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(1))
			Expect(files[0]).To(HaveSuffix("main.go"))
		})

		It("recurses into nested directories", func() {
			writeFile("sub/dir/nested.go", "package nested\n")
			writeFile("top.go", "package main\n")

			files, err := walk(tmpDir, header, holder, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(2))
		})

		It("returns empty for directory with all compliant files", func() {
			writeFile("ok1.go", header+"\n\npackage main\n")
			writeFile("ok2.go", header+"\n\npackage lib\n")

			files, err := walk(tmpDir, header, holder, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(BeEmpty())
		})

		It("returns empty for an empty directory", func() {
			files, err := walk(tmpDir, header, holder, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(BeEmpty())
		})

		It("returns error for non-existent directory", func() {
			_, err := walk("/nonexistent/path", header, holder, false)
			Expect(err).To(HaveOccurred())
		})

		It("reports only non-compliant files in a mixed directory", func() {
			writeFile("good.go", header+"\n\npackage main\n")
			writeFile("bad1.go", "package a\n")
			writeFile("bad2.go", "package b\n")

			files, err := walk(tmpDir, header, holder, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(2))
			for _, f := range files {
				Expect(f).NotTo(HaveSuffix("good.go"))
			}
		})

		It("fixes all non-compliant files in a directory", func() {
			writeFile("fix1.go", "package a\n")
			writeFile("fix2.go", "package b\n")
			writeFile("fix3.go", "package c\n")

			files, err := walk(tmpDir, header, holder, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(3))

			for _, f := range files {
				ok, err := hasValidHeader(f, holder)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeTrue())
			}
		})

		It("skips directories themselves", func() {
			Expect(os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0o755)).To(Succeed())
			writeFile("subdir/a.go", "package sub\n")

			files, err := walk(tmpDir, header, holder, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(1))
			Expect(files[0]).To(HaveSuffix("a.go"))
		})

		It("handles directory with only non-Go files", func() {
			writeFile("notes.txt", "some notes")
			writeFile("data.json", "{}")

			files, err := walk(tmpDir, header, holder, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(BeEmpty())
		})
	})
})
