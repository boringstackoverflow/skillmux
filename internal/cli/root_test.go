package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func execute(t *testing.T, input string, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewRootCommand()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func requireContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected output to contain %q, got:\n%s", want, got)
	}
}

func initCLIHome(t *testing.T, home string) {
	t.Helper()
	stdout, stderr, err := execute(t, "", "--home", home, "init", "--profile", "work", "--yes")
	if err != nil {
		t.Fatalf("init failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("unexpected init stderr: %s", stderr)
	}
}

func TestCLIRootHelpIsGroupedAndActionable(t *testing.T) {
	stdout, stderr, err := execute(t, "", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	for _, want := range []string{
		"Getting Started",
		"Profiles",
		"Maintenance",
		"Agent",
		"Other Commands",
		"skillmux init --profile work --dry-run",
		"completion",
	} {
		requireContains(t, stdout, want)
	}
}

func TestCLIUninstallHelpDoesNotExposeUnusedRestoreFlag(t *testing.T) {
	stdout, stderr, err := execute(t, "", "uninstall", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	requireContains(t, stdout, "--backup-id")
	requireContains(t, stdout, "latest pre-init backup")
	if strings.Contains(stdout, "--restore") {
		t.Fatalf("uninstall help still exposes --restore:\n%s", stdout)
	}
}

func TestCLIErrorOutputIsVisibleAndConcise(t *testing.T) {
	_, stderr, err := execute(t, "", "profle")
	if err == nil {
		t.Fatal("expected unknown command to fail")
	}
	requireContains(t, stderr, "unknown command \"profle\"")
	requireContains(t, stderr, "profile")
	if strings.Contains(stderr, "Usage:") {
		t.Fatalf("unexpected full usage dump for unknown command:\n%s", stderr)
	}

	_, stderr, err = execute(t, "", "use")
	if err == nil {
		t.Fatal("expected missing arg to fail")
	}
	requireContains(t, stderr, "accepts 1 arg(s), received 0")
	if strings.Contains(stderr, "Usage:") {
		t.Fatalf("unexpected full usage dump for missing arg:\n%s", stderr)
	}

	home := t.TempDir()
	_, stderr, err = execute(t, "", "--home", home, "current")
	if err == nil {
		t.Fatal("expected current before init to fail")
	}
	requireContains(t, stderr, "skillmux is not initialized")
}

func TestCLIInitDryRunDoesNotPromptOrMutate(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := execute(t, "", "--home", home, "init", "--profile", "work", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Dry run only") {
		t.Fatalf("expected dry-run output, got:\n%s", stdout)
	}
	if _, err := os.Lstat(filepath.Join(home, ".skillmux")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created .skillmux: %v", err)
	}
}

func TestCLIInitRequiresConfirmationWithoutYes(t *testing.T) {
	home := t.TempDir()

	_, stderr, err := execute(t, "n\n", "--home", home, "init", "--profile", "work")
	if err == nil {
		t.Fatal("expected init to abort")
	}
	if !strings.Contains(err.Error(), "aborted") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "Continue?") {
		t.Fatalf("expected confirmation prompt, got stderr:\n%s", stderr)
	}
	if _, err := os.Lstat(filepath.Join(home, ".skillmux")); !os.IsNotExist(err) {
		t.Fatalf("aborted init created .skillmux: %v", err)
	}
}

func TestCLIInitWithYesCreatesManagedRoots(t *testing.T) {
	home := t.TempDir()

	stdout, stderr, err := execute(t, "", "--home", home, "init", "--profile", "work", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Initialized Skillmux profile \"work\"") {
		t.Fatalf("expected init success output, got:\n%s", stdout)
	}
	if _, err := os.Readlink(filepath.Join(home, ".claude", "skills")); err != nil {
		t.Fatalf("claude skills was not linked: %v", err)
	}
	if _, err := os.Readlink(filepath.Join(home, ".codex", "skills")); err != nil {
		t.Fatalf("codex skills was not linked: %v", err)
	}
}

func TestCLIAliasesRouteToCanonicalCommands(t *testing.T) {
	home := t.TempDir()
	initCLIHome(t, home)

	if stdout, stderr, err := execute(t, "", "--home", home, "profile", "create", "personal"); err != nil {
		t.Fatalf("profile create failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	stdout, stderr, err := execute(t, "", "--home", home, "profile", "ls")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	requireContains(t, stdout, "personal")

	stdout, stderr, err = execute(t, "", "--home", home, "switch", "personal")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	requireContains(t, stdout, "Active profile: personal")

	stdout, stderr, err = execute(t, "", "--home", home, "status")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	requireContains(t, stdout, "personal")

	stdout, _, err = execute(t, "", "--home", home, "check")
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, stdout, "Skillmux doctor")

	if stdout, stderr, err := execute(t, "", "--home", home, "profile", "create", "old"); err != nil {
		t.Fatalf("profile create failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if stdout, stderr, err := execute(t, "", "--home", home, "profile", "rm", "old", "--force"); err != nil {
		t.Fatalf("profile rm failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
}

func TestCLIUseMissingProfileRequiresCreateFlag(t *testing.T) {
	home := t.TempDir()
	initCLIHome(t, home)

	_, stderr, err := execute(t, "", "--home", home, "use", "typo")
	if err == nil {
		t.Fatal("expected missing profile to fail")
	}
	requireContains(t, stderr, "profile \"typo\" does not exist")
	if _, err := os.Stat(filepath.Join(home, ".skillmux", "profiles", "typo")); !os.IsNotExist(err) {
		t.Fatalf("missing profile was created without --create: %v", err)
	}

	stdout, stderr, err := execute(t, "", "--home", home, "use", "typo", "--create")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	requireContains(t, stdout, "Created profile \"typo\"")
	requireContains(t, stdout, "Active profile: typo")
}

func TestCLIEnterCreateControlsProjectProfileCreation(t *testing.T) {
	home := t.TempDir()
	initCLIHome(t, home)
	projectDir := filepath.Join(home, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := []byte("profile = \"project\"\nagents = [\"codex\"]\n")
	if err := os.WriteFile(filepath.Join(projectDir, ".skillmux.toml"), config, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(projectDir)

	_, stderr, err := execute(t, "", "--home", home, "enter")
	if err == nil {
		t.Fatal("expected enter to require an existing profile")
	}
	requireContains(t, stderr, "profile \"project\" does not exist")

	stdout, stderr, err := execute(t, "", "--home", home, "enter", "--create")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	requireContains(t, stdout, "Created profile \"project\"")
	requireContains(t, stdout, "codex")
	requireContains(t, stdout, "project")
}

func TestCLIDynamicCompletionReturnsDomainValues(t *testing.T) {
	home := t.TempDir()
	initCLIHome(t, home)
	if stdout, stderr, err := execute(t, "", "--home", home, "profile", "create", "frontend"); err != nil {
		t.Fatalf("profile create failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	stdout, _, err := execute(t, "", "--home", home, "__complete", "use", "")
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, stdout, "work")
	requireContains(t, stdout, "frontend")

	stdout, _, err = execute(t, "", "--home", home, "__complete", "scan", "--profile", "")
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, stdout, "frontend")

	stdout, _, err = execute(t, "", "--home", home, "__complete", "use", "--agent", "")
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, stdout, "claude")
	requireContains(t, stdout, "codex")
	requireContains(t, stdout, "agents")

	stdout, _, err = execute(t, "", "--home", home, "__complete", "init", "--enable", "")
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, stdout, "agents")
	if strings.Contains(stdout, "claude\t") || strings.Contains(stdout, "codex\t") {
		t.Fatalf("init --enable should only complete optional adapters, got:\n%s", stdout)
	}

	stdout, _, err = execute(t, "", "--home", home, "__complete", "run", "")
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, stdout, "claude")
	requireContains(t, stdout, "codex")
	if strings.Contains(stdout, "agents\t") {
		t.Fatalf("run should not complete non-runnable agents, got:\n%s", stdout)
	}

	stdout, _, err = execute(t, "", "--home", home, "__complete", "restore", "")
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, stdout, "-pre-init")
}

func TestCLIRejectsInvalidAgents(t *testing.T) {
	home := t.TempDir()
	initCLIHome(t, home)

	_, stderr, err := execute(t, "", "--home", home, "use", "work", "--agent", "nope")
	if err == nil {
		t.Fatal("expected invalid switch agent to fail")
	}
	requireContains(t, stderr, "unsupported agent \"nope\"")

	_, stderr, err = execute(t, "", "--home", home, "run", "agents", "--profile", "work")
	if err == nil {
		t.Fatal("expected non-runnable agent to fail")
	}
	requireContains(t, stderr, "unsupported runnable agent \"agents\"")
}

func TestCLIBackupListShowsAvailableBackups(t *testing.T) {
	home := t.TempDir()
	initCLIHome(t, home)

	stdout, stderr, err := execute(t, "", "--home", home, "backup", "list")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	requireContains(t, stdout, "BACKUP ID")
	requireContains(t, stdout, "pre-init")
}
