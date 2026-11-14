// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var changelogVersionRE = regexp.MustCompile(`^##\s+v(\d+)\.(\d+)\.(\d+)`)
var explicitVersionRE = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

const defaultRoot = "../openchoreo"
const versionFileName = "template-version.json"

type job struct {
	root          string
	apiDir        string
	version       string
	fromChangelog bool
}

func main() {
	flag.Parse()
	cfgFile := defaultConfigPath()

	jobs, templatesDir, err := resolveJobs(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to prepare jobs: %v\n", err)
		os.Exit(1)
	}

	generated := make([]string, 0, len(jobs))
	for _, j := range jobs {
		if err := resolveJobVersion(&j); err != nil {
			fmt.Fprintf(os.Stderr, "failed to resolve version: %v\n", err)
			os.Exit(1)
		}
		targetDir := filepath.Join(templatesDir, "cel-envs", versionDir(j.version))
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", targetDir, err)
			os.Exit(1)
		}
		if err := generateForJob(targetDir); err != nil {
			fmt.Fprintf(os.Stderr, "generate: %v\n", err)
			os.Exit(1)
		}
		generated = append(generated, j.version)
	}

	if _, err := updateVersionFile(templatesDir, generated); err != nil {
		fmt.Fprintf(os.Stderr, "warning: update version file: %v\n", err)
		os.Exit(1)
	}
}

func resolveJobs(configPath string) ([]job, string, error) {
	mkJob := func(root, apiOverride, versionOverride string) (job, error) {
		apiDir := filepath.Join(root, "api", "v1alpha1")
		if apiOverride != "" {
			apiDir = apiOverride
		}
		j := job{root: root, apiDir: apiDir}
		if versionOverride != "" {
			if !explicitVersionRE.MatchString(versionOverride) {
				return job{}, fmt.Errorf("invalid version %q", versionOverride)
			}
			j.version = versionOverride
			j.fromChangelog = false
		}
		return j, nil
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		return nil, "", err
	}
	templatesDir := cfg.TemplatesDirOrDefault()

	if len(cfg.Sources) == 0 {
		j, err := mkJob(defaultRoot, "", "")
		if err != nil {
			return nil, "", err
		}
		return []job{j}, templatesDir, nil
	}

	var jobs []job
	for _, s := range cfg.Sources {
		j, err := mkJob(s.Root, s.API, s.Version)
		if err != nil {
			return nil, "", err
		}
		jobs = append(jobs, j)
	}
	return jobs, templatesDir, nil
}

func resolveJobVersion(j *job) error {
	if j.version != "" {
		return nil
	}
	cl := filepath.Join(j.root, "CHANGELOG.md")
	f, err := os.Open(cl)
	if err != nil {
		return fmt.Errorf("open %s: %w", cl, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		m := changelogVersionRE.FindStringSubmatch(line)
		if len(m) == 4 {
			j.version = fmt.Sprintf("v%s.%s.%s", m[1], m[2], m[3])
			j.fromChangelog = true
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("no version found in %s", cl)
}

func versionDir(v string) string {
	return strings.TrimPrefix(v, "v")
}

func updateVersionFile(templatesDir string, generated []string) (string, error) {
	if len(generated) == 0 {
		return "", nil
	}
	sort.Strings(generated)
	manifest := map[string]any{
		"latest":   generated[len(generated)-1],
		"versions": generated,
	}
	path := filepath.Join(templatesDir, versionFileName)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(manifest); err != nil {
		f.Close()
		return "", err
	}
	return manifest["latest"].(string), f.Close()
}
