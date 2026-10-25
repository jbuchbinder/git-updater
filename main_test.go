package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTruncateErr(t *testing.T) {
	// Short messages are returned untouched.
	for _, in := range []string{
		"fatal: not a git repository",
		strings.Repeat("a", 65),
		"",
	} {
		if got := truncateErr(in); got != in {
			t.Errorf("truncateErr(%q) = %q, want it unchanged", in, got)
		}
	}

	// Long messages must stay within the column but keep the reason that git
	// puts at the end, and must never split a multi-byte rune.
	long := []string{
		"fatal: unable to access 'https://github.com/boykopovar/AnyPS5.git/': Could not resolve host: github.com",
		"From https://github.com/boykopovar/AnyPS5 ! [rejected] v0.1.2 -> v0.1.2 (would clobber existing tag)",
		"error: Your local changes to the following files would be overwritten by merge: CMakeLists.txt",
		strings.Repeat("é", 100),
	}
	for _, in := range long {
		got := truncateErr(in)
		rs, gs := []rune(in), []rune(got)

		if len(gs) > 65 {
			t.Errorf("truncateErr(%q) = %d runes, want <= 65", in, len(gs))
		}
		if !strings.Contains(got, "...") {
			t.Errorf("truncateErr(%q) = %q, want an elision marker", in, got)
		}
		if !strings.HasPrefix(got, string(rs[:20])) {
			t.Errorf("truncateErr(%q) = %q, want it to keep the leading context", in, got)
		}
		if !strings.HasSuffix(got, string(rs[len(rs)-25:])) {
			t.Errorf("truncateErr(%q) = %q, want it to keep the trailing reason", in, got)
		}
	}
}

// TestPullRepoSurvivesMovedTag is the regression test for a repo that a plain
// `git pull --all --tags` can never update: when a tag moved upstream, fetch
// exits non-zero, so the merge never runs and the branch stays behind forever.
func TestPullRepoSurvivesMovedTag(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	seed := filepath.Join(root, "seed")
	work := filepath.Join(root, "work")

	mustGit(t, root, "init", "-q", "--bare", remote)
	mustGit(t, root, "clone", "-q", remote, seed)
	mustGit(t, seed, "config", "user.email", "t@example.com")
	mustGit(t, seed, "config", "user.name", "t")
	writeFile(t, filepath.Join(seed, "f"), "one\n")
	mustGit(t, seed, "add", "f")
	mustGit(t, seed, "commit", "-qm", "c1")
	mustGit(t, seed, "tag", "v1.0")
	mustGit(t, seed, "push", "-q", "origin", "main", "--tags")

	mustGit(t, root, "clone", "-q", remote, work)
	before := head(t, work)

	// Upstream rewrites the tag and adds commits on top.
	writeFile(t, filepath.Join(seed, "f"), "one\ntwo\n")
	mustGit(t, seed, "commit", "-qam", "c2")
	writeFile(t, filepath.Join(seed, "f"), "one\ntwo\nthree\n")
	mustGit(t, seed, "commit", "-qam", "c3")
	mustGit(t, seed, "tag", "-f", "v1.0")
	mustGit(t, seed, "push", "-q", "origin", "main", "--tags", "--force")
	want := head(t, seed)

	res, ok := pullRepoCmd(work)().(pullResultMsg)
	if !ok {
		t.Fatalf("pullRepoCmd returned an unexpected message type")
	}
	if res.err != nil {
		t.Fatalf("pull failed: %v", res.err)
	}

	if got := head(t, work); got != want {
		t.Fatalf("HEAD = %s, want %s (repo was left behind)", got, want)
	}
	if !res.changes {
		t.Errorf("changes = false, want true after pulling 2 commits")
	}
	if res.commits != 2 {
		t.Errorf("commits = %d, want 2", res.commits)
	}
	if got := head(t, work); got == before {
		t.Errorf("HEAD did not advance: %s", got)
	}
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		"GIT_CONFIG_NOSYSTEM=1",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func head(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD in %s: %v", dir, err)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
