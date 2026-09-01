// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowtemplates

import (
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests prove the URL/credential transformation logic in checkout-source
// that static assertions can't: they substitute the Argo {{...}} placeholders,
// rewrite the script's hard-coded absolute paths to temp dirs, put stub `git`
// and `ssh-keygen` on PATH, and run the real script with `sh`. The stub git
// records its arguments so the test can read back the *transformed* repo URL
// that would have been passed to `git clone`.

// gitStub records every git invocation to $GIT_CALLS and creates the clone
// destination so the subsequent `cd` succeeds. Unknown git subcommands fail
// loudly; otherwise a typo such as `git clon` would be reported later as a
// confusing missing-directory failure.
const gitStub = `#!/bin/sh
echo "$*" >> "$GIT_CALLS"
case "$1" in
  clone)
    for dest in "$@"; do :; done
    mkdir -p "$dest" 2>/dev/null || true
    ;;
  rev-parse)
    echo "abcdef1234567890abcdef1234567890abcdef12"
    ;;
  show)
    echo "2026-01-15T10:30:00+00:00"
    ;;
  config|fetch|checkout)
    ;;
  *)
    echo "unexpected git subcommand: $1 (expected clone/config/fetch/checkout/rev-parse/show)" >&2
    exit 127
    ;;
esac
exit 0
`

const sshKeygenStub = "#!/bin/sh\nexit 0\n"

// echoShim normalizes `echo -n` to the busybox/alpine semantics the script
// actually runs under (the checkout container is alpine/git). Without this,
// dash/macOS `sh` print a literal "-n", corrupting the credential-encoding and
// revision-extraction logic. Defining a shell function shadows the builtin in
// all POSIX shells and is inherited by command substitutions.
const echoShim = `echo() { if [ "$1" = "-n" ]; then shift; printf '%s' "$*"; else printf '%s\n' "$*"; fi; }
`

const fakeSSHKey = "-----BEGIN OPENSSH PRIVATE KEY-----\nZmFrZWtleWZha2VrZXlmYWtla2V5\n-----END OPENSSH PRIVATE KEY-----\n"

type checkoutInput struct {
	repo   string
	branch string
	commit string
	// secretFiles are written into the mounted git-secret dir. An empty map
	// means no secret dir (public repo).
	secretFiles map[string]string
}

type checkoutResult struct {
	exitCode         int
	output           string
	gitCalls         []string
	cloneRepo        string // the repo URL passed to `git clone`, after transformation
	root             string
	askpass          string // path to the GIT_ASKPASS helper the script wrote
	secretDir        string // the mounted git-secret dir the helper reads from
	commitSHA        string // contents of the commit-sha output parameter file
	commitAuthoredAt string // contents of the commit-authored-at output parameter file
	sourceBranch     string // contents of the source-branch output parameter file
}

// runCheckout prepares an isolated environment and runs the checkout script.
func runCheckout(t *testing.T, in checkoutInput) checkoutResult {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available; skipping behavioral test")
	}

	root := t.TempDir()
	home := filepath.Join(root, "home")
	source := filepath.Join(root, "source")
	secretDir := filepath.Join(root, "git-secret")
	stubDir := filepath.Join(root, "bin")
	revFile := filepath.Join(root, "git-revision.txt")
	commitSHAFile := filepath.Join(root, "commit-sha.txt")
	commitAuthoredAtFile := filepath.Join(root, "commit-authored-at.txt")
	sourceBranchFile := filepath.Join(root, "source-branch.txt")
	gitCalls := filepath.Join(root, "git-calls.log")
	askpass := filepath.Join(root, "git-askpass.sh")
	require.NoError(t, os.MkdirAll(home, 0o755))
	require.NoError(t, os.MkdirAll(stubDir, 0o755))

	// Stub external binaries.
	writeExec(t, filepath.Join(stubDir, "git"), gitStub)
	writeExec(t, filepath.Join(stubDir, "ssh-keygen"), sshKeygenStub)

	// Materialize the git-secret volume.
	if len(in.secretFiles) > 0 {
		require.NoError(t, os.MkdirAll(secretDir, 0o755))
		for name, content := range in.secretFiles {
			require.NoError(t, os.WriteFile(filepath.Join(secretDir, name), []byte(content), 0o600))
		}
	}

	// Build the runnable script: substitute Argo params and redirect the
	// hard-coded absolute paths to our temp dirs. The auth/URL logic is left
	// byte-for-byte intact.
	script := echoShim + scriptForTemplate(t, "checkout-source.yaml", "checkout")
	replacements := []string{
		"{{workflow.parameters.git-repo}}", in.repo,
		"{{workflow.parameters.branch}}", in.branch,
		"{{workflow.parameters.commit}}", in.commit,
		"/etc/secrets/git-secret", secretDir,
		"/mnt/vol/source", source,
		"/tmp/git-revision.txt", revFile,
		"/tmp/git-askpass.sh", askpass,
		"/tmp/commit-sha.txt", commitSHAFile,
		"/tmp/commit-authored-at.txt", commitAuthoredAtFile,
		"/tmp/source-branch.txt", sourceBranchFile,
	}
	script = strings.NewReplacer(replacements...).Replace(script)

	env := append(os.Environ(),
		"HOME="+home,
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GIT_CALLS="+gitCalls,
	)
	replacer := strings.NewReplacer(replacements...)
	for _, item := range envForTemplate(t, "checkout-source.yaml", "checkout") {
		env = append(env, item.Name+"="+replacer.Replace(item.Value))
	}

	cmd := exec.Command("sh", "-c", script)
	cmd.Env = env
	out, err := cmd.CombinedOutput()

	res := checkoutResult{output: string(out), root: root, askpass: askpass, secretDir: secretDir}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			res.exitCode = exitErr.ExitCode()
		} else {
			res.exitCode = -1
		}
	}

	if data, rerr := os.ReadFile(commitSHAFile); rerr == nil {
		res.commitSHA = string(data)
	}
	if data, rerr := os.ReadFile(commitAuthoredAtFile); rerr == nil {
		res.commitAuthoredAt = string(data)
	}
	if data, rerr := os.ReadFile(sourceBranchFile); rerr == nil {
		res.sourceBranch = string(data)
	}

	if data, rerr := os.ReadFile(gitCalls); rerr == nil {
		for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
			if line == "" {
				continue
			}
			res.gitCalls = append(res.gitCalls, line)
			if fields := strings.Fields(line); fields[0] == "clone" && len(fields) >= 2 {
				// repo URL is the second-to-last positional arg (dest is last).
				res.cloneRepo = fields[len(fields)-2]
			}
		}
	}
	return res
}

func sshSecret(extra map[string]string) map[string]string {
	m := map[string]string{"ssh-privatekey": fakeSSHKey}
	maps.Copy(m, extra)
	return m
}

func requireCheckoutSuccess(t *testing.T, res checkoutResult, contract string) {
	t.Helper()
	if res.exitCode == 0 {
		return
	}
	t.Fatalf(`
contract:
  %s

expected:
  exit code: 0

actual:
  exit code: %d

script output:
%s

git calls:
%s`,
		contract,
		res.exitCode,
		formatCheckoutOutput(res),
		formatCheckoutCalls(res),
	)
}

func requireGitCall(t *testing.T, res checkoutResult, prefix string, contract string) {
	t.Helper()
	if hasCall(res.gitCalls, prefix) {
		return
	}
	t.Fatalf(`
contract:
  %s

expected:
  git call prefix: %q

git calls:
%s`,
		contract,
		prefix,
		formatCheckoutCalls(res),
	)
}

func requireCheckoutExitCode(t *testing.T, res checkoutResult, want int, contract string) {
	t.Helper()
	if res.exitCode == want {
		return
	}
	t.Fatalf(`
contract:
  %s

expected:
  exit code: %d

actual:
  exit code: %d

script output:
%s

git calls:
%s`,
		contract,
		want,
		res.exitCode,
		formatCheckoutOutput(res),
		formatCheckoutCalls(res),
	)
}

func requireCloneRepo(t *testing.T, res checkoutResult, want string, contract string) {
	t.Helper()
	if res.cloneRepo == want {
		return
	}
	t.Fatalf(`
contract:
  %s

expected clone repo:
  %s

actual clone repo:
  %s

git calls:
%s`,
		contract,
		want,
		res.cloneRepo,
		formatCheckoutCalls(res),
	)
}

func requireCheckoutOutputContains(t *testing.T, res checkoutResult, needle string, contract string) {
	t.Helper()
	if strings.Contains(res.output, needle) {
		return
	}
	t.Fatalf(`
contract:
  %s

expected output to contain:
  %q

script output:
%s

git calls:
%s`,
		contract,
		needle,
		formatCheckoutOutput(res),
		formatCheckoutCalls(res),
	)
}

// --- SSH provider matrix: https -> ssh rewrite (scenario 2) ---

func TestCheckout_SSH_HTTPSToSSHRewrite(t *testing.T) {
	cases := map[string]struct {
		repo string
		want string
	}{
		"github":    {"https://github.com/org/repo.git", "git@github.com:org/repo.git"},
		"gitlab":    {"https://gitlab.com/org/repo.git", "git@gitlab.com:org/repo.git"},
		"bitbucket": {"https://bitbucket.org/org/repo.git", "git@bitbucket.org:org/repo.git"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			res := runCheckout(t, checkoutInput{
				repo:        tc.repo,
				branch:      "main",
				secretFiles: sshSecret(nil),
			})
			requireCheckoutSuccess(t, res, "SSH auth should clone "+name+" private repo after rewriting https URL to ssh")
			requireCloneRepo(t, res, tc.want, "SSH auth should rewrite "+name+" HTTPS URL to SSH clone URL")
			requireCheckoutOutputContains(t, res, "SSH Key Authentication",
				"checkout output should report SSH key authentication path")
		})
	}
}

// --- AWS CodeCommit SSH: key-id injection (scenario 2) ---

func TestCheckout_SSH_CodeCommitKeyIDInjection(t *testing.T) {
	res := runCheckout(t, checkoutInput{
		repo:   "ssh://git-codecommit.us-east-1.amazonaws.com/v1/repos/myrepo",
		branch: "main",
		secretFiles: sshSecret(map[string]string{
			"ssh-key-id": "APKAEXAMPLEKEYID",
		}),
	})
	requireCheckoutSuccess(t, res, "AWS CodeCommit SSH auth should inject ssh-key-id before clone")
	requireCloneRepo(t, res,
		"ssh://APKAEXAMPLEKEYID@git-codecommit.us-east-1.amazonaws.com/v1/repos/myrepo",
		"AWS CodeCommit SSH URL should include ssh-key-id username when secret provides ssh-key-id")
}

func TestCheckout_SSH_CodeCommitNotRewrittenToScpSyntax(t *testing.T) {
	// A codecommit https URL must NOT be rewritten to git@host:path form.
	res := runCheckout(t, checkoutInput{
		repo:        "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/myrepo",
		branch:      "main",
		secretFiles: sshSecret(nil),
	})
	requireCheckoutSuccess(t, res, "AWS CodeCommit https URL should not be rewritten to scp syntax")
	requireCloneRepo(t, res, "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/myrepo",
		"AWS CodeCommit HTTPS URL must not be rewritten to scp syntax")
}

// --- Basic auth: credential percent-encoding (scenario 1) ---

// runAskpass invokes the GIT_ASKPASS helper the script wrote, the way git
// would: prompt text as the sole argument, answer read from stdout.
func runAskpass(t *testing.T, res checkoutResult, prompt string) string {
	t.Helper()
	cmd := exec.Command("sh", res.askpass, prompt)
	cmd.Env = append(os.Environ(), "GIT_SECRET_PATH="+res.secretDir)
	out, err := cmd.Output()
	require.NoError(t, err, "askpass helper should be executable and succeed")
	return strings.TrimSpace(string(out))
}

func TestCheckout_BasicAuth_KeepsCredentialsOutOfCloneURL(t *testing.T) {
	res := runCheckout(t, checkoutInput{
		repo:   "https://github.com/org/repo.git",
		branch: "main",
		secretFiles: map[string]string{
			"username": "my-user",
			"password": "p@ss:w0rd",
		},
	})
	requireCheckoutSuccess(t, res, "Basic auth should clone without rewriting the repo URL")
	requireCloneRepo(t, res, "https://github.com/org/repo.git",
		"basic auth should leave the clone URL unmodified, since git persists it into .git/config")
	requireCheckoutOutputContains(t, res, "Basic Authentication",
		"checkout output should report basic authentication path")

	require.Equal(t, "my-user", runAskpass(t, res, "Username for 'https://github.com': "),
		"askpass helper should return the username from the mounted secret")
	require.Equal(t, "p@ss:w0rd", runAskpass(t, res, "Password for 'https://my-user@github.com': "),
		"askpass helper should return the password verbatim, with no encoding applied")
}

func TestCheckout_BasicAuth_DefaultsUsernameToGit(t *testing.T) {
	res := runCheckout(t, checkoutInput{
		repo:        "https://github.com/org/repo.git",
		branch:      "main",
		secretFiles: map[string]string{"password": "token123"},
	})
	requireCheckoutSuccess(t, res, "Basic auth should default username to git")
	requireCloneRepo(t, res, "https://github.com/org/repo.git",
		"basic auth should leave the clone URL unmodified")
	require.Equal(t, "git", runAskpass(t, res, "Username for 'https://github.com': "),
		"askpass helper should default the username to git when the secret has no username key")
	require.Equal(t, "token123", runAskpass(t, res, "Password for 'https://git@github.com': "),
		"askpass helper should still return the password when the username is defaulted")
}

// --- Auth-type precedence and edge cases (scenario 5) ---

func TestCheckout_SSHTakesPrecedenceOverBasic(t *testing.T) {
	// Both an ssh key and a password are present: ssh path wins.
	res := runCheckout(t, checkoutInput{
		repo:   "https://github.com/org/repo.git",
		branch: "main",
		secretFiles: sshSecret(map[string]string{
			"password": "ignored",
		}),
	})
	requireCheckoutSuccess(t, res, "SSH credentials should take precedence over basic auth credentials")
	requireCloneRepo(t, res, "git@github.com:org/repo.git",
		"SSH credentials should win when both ssh-privatekey and password are present")
	requireCheckoutOutputContains(t, res, "SSH Key Authentication",
		"checkout output should report SSH key authentication when both auth types exist")
}

func TestCheckout_PublicRepo_NoSecret(t *testing.T) {
	res := runCheckout(t, checkoutInput{
		repo:   "https://github.com/org/repo.git",
		branch: "main",
		// no secret files -> no secret dir -> public path
	})
	requireCheckoutSuccess(t, res, "Public repo checkout should clone without auth transformation")
	requireCloneRepo(t, res, "https://github.com/org/repo.git",
		"public repository checkout should leave clone URL unchanged")
	requireCheckoutOutputContains(t, res, "public repository",
		"checkout output should report public repository path when no secret is mounted")
}

func TestCheckout_SecretPresentButUnrecognized_Fails(t *testing.T) {
	res := runCheckout(t, checkoutInput{
		repo:        "https://github.com/org/repo.git",
		branch:      "main",
		secretFiles: map[string]string{"unexpected": "data"},
	})
	requireCheckoutExitCode(t, res, 1,
		"unrecognized git-secret contents should fail before clone")
	requireCheckoutOutputContains(t, res, "does not contain recognized authentication data",
		"unrecognized git-secret failure should explain the secret schema problem")
}

// --- Validation guards (scenario 6) ---

func TestCheckout_MissingRepo_Fails(t *testing.T) {
	res := runCheckout(t, checkoutInput{repo: "", branch: "main"})
	requireCheckoutExitCode(t, res, 1,
		"missing git repository URL should fail before clone")
	requireCheckoutOutputContains(t, res, "Git repository URL is required",
		"missing repo failure should explain that git-repo is required")
}

func TestCheckout_MissingBranchAndCommit_Fails(t *testing.T) {
	res := runCheckout(t, checkoutInput{repo: "https://github.com/org/repo.git"})
	requireCheckoutExitCode(t, res, 1,
		"missing branch and commit should fail before clone")
	requireCheckoutOutputContains(t, res, "Either a branch or commit must be specified",
		"missing revision failure should explain that branch or commit is required")
}

// --- Checkout mode: branch vs commit (scenario 3) ---

func TestCheckout_ByBranch_UsesSingleBranchClone(t *testing.T) {
	res := runCheckout(t, checkoutInput{
		repo:   "https://github.com/org/repo.git",
		branch: "feature-x",
	})
	requireCheckoutSuccess(t, res, "Branch checkout should use git clone --single-branch")
	requireGitCall(t, res, "clone --single-branch --branch feature-x",
		"branch checkout must clone exactly the requested branch")
}

func TestCheckout_ByCommit_UsesNoCheckoutCloneAndFetch(t *testing.T) {
	res := runCheckout(t, checkoutInput{
		repo:   "https://github.com/org/repo.git",
		commit: "1234567890abcdef",
	})
	requireCheckoutSuccess(t, res, "Commit checkout should use git clone --no-checkout then git checkout")
	requireGitCall(t, res, "clone --no-checkout --depth 1",
		"commit checkout must clone without checkout before fetching the commit")
	requireGitCall(t, res, "checkout 1234567890abcdef",
		"commit checkout must checkout the requested commit SHA")
	requireCheckoutOutputContains(t, res, "Checked out commit: 1234567890abcdef",
		"commit checkout should report the checked out commit")
}

// --- WorkloadSource provenance outputs (scenario 23) ---

func TestCheckout_ByBranch_EmitsCommitProvenance(t *testing.T) {
	res := runCheckout(t, checkoutInput{
		repo:   "https://github.com/org/repo.git",
		branch: "feature-x",
	})
	requireCheckoutSuccess(t, res, "Branch checkout should succeed so commit provenance outputs are written")
	requireEqualContract(t, res.commitSHA, "abcdef1234567890abcdef1234567890abcdef12",
		"branch checkout must write the full resolved commit SHA to commit-sha.txt")
	requireEqualContract(t, res.commitAuthoredAt, "2026-01-15T10:30:00+00:00",
		"branch checkout must write the commit author timestamp to commit-authored-at.txt")
}

func TestCheckout_ByCommit_EmitsCommitProvenance(t *testing.T) {
	res := runCheckout(t, checkoutInput{
		repo:   "https://github.com/org/repo.git",
		commit: "1234567890abcdef",
	})
	requireCheckoutSuccess(t, res, "Commit checkout should succeed so commit provenance outputs are written")
	requireEqualContract(t, res.commitSHA, "abcdef1234567890abcdef1234567890abcdef12",
		"commit checkout must canonicalize the requested commit via rev-parse before writing commit-sha.txt")
	requireEqualContract(t, res.commitAuthoredAt, "2026-01-15T10:30:00+00:00",
		"commit checkout must write the commit author timestamp to commit-authored-at.txt")
}

func hasCall(calls []string, prefix string) bool {
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func formatCheckoutOutput(res checkoutResult) string {
	out := strings.TrimSpace(normalizeCheckoutPath(res, res.output))
	if out == "" {
		return "  (empty)"
	}

	var b strings.Builder
	for _, line := range strings.Split(out, "\n") {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatCheckoutCalls(res checkoutResult) string {
	if len(res.gitCalls) == 0 {
		return "  (none)"
	}

	var b strings.Builder
	for _, c := range res.gitCalls {
		b.WriteString("  - ")
		b.WriteString(normalizeCheckoutPath(res, c))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func normalizeCheckoutPath(res checkoutResult, s string) string {
	if res.root == "" {
		return s
	}
	return strings.ReplaceAll(s, res.root, "$TEST_ROOT")
}

// TestCheckoutSourceBranchProvenance pins which branch a build is attributed to.
// A commit-pinned run checks out a detached commit and never consults the branch
// parameter, so reporting the requested branch would claim the build came from a
// branch that was never checked out -- and a commit can belong to any number of
// branches, so there is no correct value to report.
func TestCheckoutSourceBranchProvenance(t *testing.T) {
	t.Run("branch checkout records the branch it cloned", func(t *testing.T) {
		res := runCheckout(t, checkoutInput{
			repo:   "https://github.com/acme/widgets.git",
			branch: "feature-x",
		})
		require.Equal(t, 0, res.exitCode, "checkout should succeed: %s", res.output)
		require.Equal(t, "feature-x", res.sourceBranch,
			"a branch checkout must report the branch it actually cloned")
	})

	t.Run("commit checkout records no branch", func(t *testing.T) {
		res := runCheckout(t, checkoutInput{
			repo:   "https://github.com/acme/widgets.git",
			branch: "feature-x",
			commit: "1234567890abcdef",
		})
		require.Equal(t, 0, res.exitCode, "checkout should succeed: %s", res.output)
		require.Empty(t, res.sourceBranch,
			"a commit-pinned checkout never uses the branch parameter, so it must not "+
				"report feature-x as this build's branch")
		require.NotEmpty(t, res.commitSHA,
			"commit provenance must still be recorded on the commit path")
	})
}
