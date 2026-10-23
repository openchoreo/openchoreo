// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package specdiff

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiReset  = "\033[0m"
)

// ColorMode decides whether diff output is colored.
type ColorMode string

const (
	ColorAuto   ColorMode = "auto"
	ColorAlways ColorMode = "always"
	ColorNever  ColorMode = "never"
)

const colorFlag = "color"

// AddColorFlag registers the --color flag on a diff command.
func AddColorFlag(cmd *cobra.Command) {
	cmd.Flags().String(colorFlag, string(ColorAuto), "When to color the output: auto, always, or never")
}

// GetColorMode reads and validates the --color flag.
func GetColorMode(cmd *cobra.Command) (ColorMode, error) {
	val, _ := cmd.Flags().GetString(colorFlag)
	return ParseColorMode(val)
}

// ParseColorMode validates a --color value. An empty value means auto.
func ParseColorMode(s string) (ColorMode, error) {
	switch m := ColorMode(s); m {
	case "":
		return ColorAuto, nil
	case ColorAuto, ColorAlways, ColorNever:
		return m, nil
	default:
		return "", fmt.Errorf("invalid --color value %q: must be one of auto, always, never", s)
	}
}

// Enabled reports whether output written to f should be colored. In auto mode
// that is only when f is a terminal that understands escape sequences and
// NO_COLOR is not set.
func (m ColorMode) Enabled(f *os.File) bool {
	switch m {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	default:
		return term.IsTerminal(int(f.Fd())) && os.Getenv("TERM") != "dumb" && os.Getenv("NO_COLOR") == ""
	}
}

// Options describes one comparison for Run.
type Options struct {
	// Kind names the compared resource in the header, for example "componentrelease".
	Kind string
	// From and To are the names of the compared objects.
	From string
	To   string
	// Root is the path prefix of every change, for example "spec".
	Root  string
	Color bool
}

// Run compares from with to and writes the differences to w.
func Run(w io.Writer, opts Options, from, to any) error {
	changes, err := Compare(opts.Root, from, to)
	if err != nil {
		return err
	}
	Render(w, opts, changes)
	return nil
}

// Render writes a header, one line per change and a summary to w.
func Render(w io.Writer, opts Options, changes []Change) {
	if len(changes) == 0 {
		fmt.Fprintf(w, "No differences in %s between %s %q and %q\n", opts.Root, opts.Kind, opts.From, opts.To)
		return
	}
	fmt.Fprintf(w, "Comparing %s %s: %s -> %s\n\n", opts.Kind, opts.Root, opts.From, opts.To)
	var added, removed, modified int
	for _, c := range changes {
		var line, code string
		switch c.Type {
		case Added:
			added++
			line, code = fmt.Sprintf("+ %s: %s", c.Path, formatValue(c.New)), ansiGreen
		case Removed:
			removed++
			line, code = fmt.Sprintf("- %s: %s", c.Path, formatValue(c.Old)), ansiRed
		case Modified:
			modified++
			line, code = fmt.Sprintf("~ %s: %s -> %s", c.Path, formatValue(c.Old), formatValue(c.New)), ansiYellow
		}
		fmt.Fprintln(w, colorize(line, code, opts.Color))
	}
	fmt.Fprintf(w, "\n%s: %d added, %d removed, %d modified\n", plural(len(changes), "change"), added, removed, modified)
}

func colorize(s, code string, enabled bool) string {
	if !enabled {
		return s
	}
	return code + s + ansiReset
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
