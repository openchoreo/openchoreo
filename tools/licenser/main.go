// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

/* -------------------------------------------------------------------------- */
/*                               Flag variables                               */
/* -------------------------------------------------------------------------- */

var (
	flagCheckOnly = flag.Bool(
		"check-only",
		false,
		"Only verify headers (exit 1 if non-compliant)",
	)
	flagHolder = flag.String(
		"c",
		"",
		`Copyright holder, e.g. "The OpenChoreo Authors" (required when adding headers)`,
	)
	flagLicense = flag.String(
		"l",
		"apache",
		`License identifier ("apache" or "mit")`,
	)
)

/* -------------------------------------------------------------------------- */
/*                        Header detection / generation                       */
/* -------------------------------------------------------------------------- */

var (
	reCopyright = regexp.MustCompile(`^// Copyright (\d{4}) (.+)$`)
	reSPDX      = regexp.MustCompile(`^// SPDX-License-Identifier: (Apache-2\.0|MIT)$`)
)

func licenseID(l string) string {
	if strings.EqualFold(l, "mit") {
		return "MIT"
	}
	return "Apache-2.0"
}

func shortHeader(year, holder, license string) string {
	return fmt.Sprintf(
		"// Copyright %s %s\n// SPDX-License-Identifier: %s",
		year, holder, licenseID(license),
	)
}

/* -------------------------------------------------------------------------- */
/*                               File helpers                                 */
/* -------------------------------------------------------------------------- */

func isGoFile(path string) bool { return filepath.Ext(path) == ".go" }

func hasValidHeader(file, holder, license string) (bool, error) {
	f, err := os.Open(file)
	if err != nil {
		return false, err
	}
	defer f.Close()

	scan := bufio.NewScanner(f)
	var lines []string
	for scan.Scan() {
		line := scan.Text()
		if strings.TrimSpace(line) == "" && len(lines) == 0 {
			continue // skip leading blank lines
		}
		lines = append(lines, line)
		if len(lines) == 2 {
			break
		}
	}

	if len(lines) < 2 {
		return false, nil
	}

	m1 := reCopyright.FindStringSubmatch(lines[0])
	m2 := reSPDX.FindStringSubmatch(lines[1])
	if m1 == nil || m2 == nil {
		return false, nil
	}

	return m1[2] == holder && m2[1] == licenseID(license), nil
}

func prependHeader(path, header string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(header+"\n\n"), src...), 0o644)
}

/* -------------------------------------------------------------------------- */
/*                            Core processing loop                            */
/* -------------------------------------------------------------------------- */

func process(path, header, holder, license string, fix bool) (changed bool, err error) {
	ok, err := hasValidHeader(path, holder, license)
	if err != nil || ok {
		return false, err
	}
	if !fix {
		return true, nil // non-compliant
	}
	return true, prependHeader(path, header)
}

func walk(root, header, holder, license string, fix bool) ([]string, error) {
	var nonCompliant []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !isGoFile(p) {
			return err
		}
		changed, err := process(p, header, holder, license, fix)
		if err != nil {
			return err
		}
		if changed {
			nonCompliant = append(nonCompliant, p)
		}
		return nil
	})
	return nonCompliant, err
}

/* -------------------------------------------------------------------------- */
/*                                   CLI                                     */
/* -------------------------------------------------------------------------- */

const usageText = `
licenser – validate or add short SPDX license headers to source files

USAGE
  go run ./tools/licenser/main.go [flags] <path/files>

FLAGS
  -check-only           Report problems but do not rewrite files (default: false)
  -c, --copyright <str> Copyright holder (required when adding headers)
  -l, --license   <str> License identifier to write: "apache" (default) or "mit"

EXAMPLES
  # Dry-run: verify headers everywhere under the current directory
  go run ./tools/licenser/main.go -check-only -c "The OpenChoreo Authors" .

  # Insert/fix headers in place
  go run ./tools/licenser/main.go -c "The OpenChoreo Authors" .
`

func main() {
	flag.Usage = func() { fmt.Fprint(os.Stderr, usageText) }
	flag.Parse()

	if flag.NArg() == 0 || (*flagHolder == "" && !*flagCheckOnly) {
		flag.Usage()
		os.Exit(1)
	}

	header := shortHeader(fmt.Sprint(time.Now().Year()), *flagHolder, *flagLicense)
	mode := "CHECK"
	if !*flagCheckOnly {
		mode = "FIX"
	}
	fmt.Printf("Running in %s mode (%s license)\n", mode, licenseID(*flagLicense))

	var offending []string
	for _, dir := range flag.Args() {
		files, err := walk(dir, header, *flagHolder, *flagLicense, !*flagCheckOnly)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error scanning %s: %v\n", dir, err)
			os.Exit(2)
		}
		offending = append(offending, files...)
	}

	if *flagCheckOnly {
		if len(offending) > 0 {
			fmt.Println("❌ Missing or invalid headers:")
			for _, f := range offending {
				fmt.Printf(" • %s\n", f)
			}
			os.Exit(1)
		}
		fmt.Println("✅ All files have valid headers.")
	} else {
		if len(offending) > 0 {
			fmt.Println("🛠 Added headers to:")
			for _, f := range offending {
				fmt.Printf(" • %s\n", f)
			}
		} else {
			fmt.Println("✅ No changes needed – all headers already valid.")
		}
	}
}
