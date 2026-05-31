package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boringstackoverflow/skillmux/internal/app"
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

func writeCLISkill(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
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
		"Experimental Cloud Sync",
		"Other Commands",
		"skillmux init --profile work --dry-run",
		"enable",
		"login",
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
	requireContains(t, stdout, "Planned changes for profile \"work\"")
	requireContains(t, stdout, "Backup: would create pre-init backup")
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
	requireContains(t, stdout, "Managed links for profile \"work\"")
	requireContains(t, stdout, "Undo: skillmux restore")
	if _, err := os.Readlink(filepath.Join(home, ".claude", "skills")); err != nil {
		t.Fatalf("claude skills was not linked: %v", err)
	}
	if _, err := os.Readlink(filepath.Join(home, ".codex", "skills")); err != nil {
		t.Fatalf("codex skills was not linked: %v", err)
	}
}

func TestCLICloudLoginStoresTokenOnlyAfterSuccess(t *testing.T) {
	home := t.TempDir()
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
	}))
	defer failing.Close()

	_, _, err := execute(t, "", "--home", home, "--cloud-url", failing.URL, "login", "--email", "dev@example.com")
	if err == nil {
		t.Fatal("expected failed login")
	}
	if _, statErr := os.Stat(filepath.Join(home, ".skillmux", "state", "cloud.toml")); !os.IsNotExist(statErr) {
		t.Fatalf("failed login wrote cloud state: %v", statErr)
	}

	success := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/magic-link" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "dev-token", "message": "issued"})
	}))
	defer success.Close()

	stdout, stderr, err := execute(t, "", "--home", home, "--cloud-url", success.URL, "login", "--email", "dev@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	requireContains(t, stdout, "Logged in")
	info, err := os.Stat(filepath.Join(home, ".skillmux", "state", "cloud.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("cloud token file mode = %v, want 0600", got)
	}
}

func TestCLICloudOrgInviteJoinFlow(t *testing.T) {
	home := t.TempDir()
	var inviteCode string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer dev-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid bearer token"})
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/orgs/acme/invites":
			inviteCode = "skmi_test"
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": inviteCode, "email": "team@example.com"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/orgs/acme/join":
			var req struct {
				Code string `json:"code"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode join: %v", err)
			}
			if req.Code != inviteCode {
				t.Fatalf("join code = %q, want %q", req.Code, inviteCode)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"org": map[string]string{"name": "acme"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if stdout, stderr, err := execute(t, "", "--home", home, "--cloud-url", server.URL, "login", "--token", "dev-token"); err != nil {
		t.Fatalf("login failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	stdout, stderr, err := execute(t, "", "--home", home, "--cloud-url", server.URL, "org", "invite", "acme", "--email", "team@example.com")
	if err != nil {
		t.Fatalf("invite failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	requireContains(t, stdout, "Invite code for acme: skmi_test")

	stdout, stderr, err = execute(t, "", "--home", home, "--cloud-url", server.URL, "org", "join", "acme", "--code", "skmi_test")
	if err != nil {
		t.Fatalf("join failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	requireContains(t, stdout, "Joined org \"acme\"")
	requireContains(t, stdout, "Current org: acme")
}

func TestCLICloudProfilePushPullFlowKeepsNativeRootsStable(t *testing.T) {
	home := t.TempDir()
	initCLIHome(t, home)
	writeCLISkill(t, filepath.Join(home, ".claude", "skills"), "local-review", "local")

	var snapshot app.ProfileSnapshot
	var versionDigest string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer dev-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid bearer token"})
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/orgs/acme/profiles/work/versions":
			var req struct {
				Message  string              `json:"message"`
				Snapshot app.ProfileSnapshot `json:"snapshot"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode push: %v", err)
			}
			snapshot = req.Snapshot
			versionDigest = snapshot.Digest
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"version": map[string]any{
				"version": "v1", "digest": snapshot.Digest, "files": len(snapshot.Files), "created_at": "2026-05-30T00:00:00Z", "recommended": true,
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/orgs/acme/profiles/work/versions":
			_ = json.NewEncoder(w).Encode(map[string]any{"versions": []map[string]any{{
				"version": "v1", "digest": versionDigest, "files": len(snapshot.Files), "created_at": "2026-05-30T00:00:00Z", "recommended": true,
			}}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/orgs/acme/profiles/work/versions/v1/archive":
			_ = json.NewEncoder(w).Encode(snapshot)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, stderr, err := execute(t, "", "--home", home, "--cloud-url", server.URL, "profile", "push", "work", "--org", "acme")
	if err == nil {
		t.Fatal("expected push without login to fail")
	}
	requireContains(t, stderr, "not logged in")

	if stdout, stderr, err := execute(t, "", "--home", home, "--cloud-url", server.URL, "login", "--token", "dev-token"); err != nil {
		t.Fatalf("login failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	stdout, stderr, err := execute(t, "", "--home", home, "--cloud-url", server.URL, "profile", "push", "work", "--org", "acme", "--message", "Initial")
	if err != nil {
		t.Fatalf("push failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	requireContains(t, stdout, "Pushed acme/work v1")

	addSnapshotFile(&snapshot, "roots/claude/skills/remote-only/SKILL.md", "remote")
	versionDigest = ""

	stdout, stderr, err = execute(t, "", "--home", home, "--cloud-url", server.URL, "profile", "pull", "acme/work", "--profile", "incoming")
	if err != nil {
		t.Fatalf("pull preview failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	requireContains(t, stdout, "Dry run only")
	if _, err := os.Stat(filepath.Join(home, ".skillmux", "profiles", "incoming")); !os.IsNotExist(err) {
		t.Fatalf("preview created incoming profile: %v", err)
	}

	_, stderr, err = execute(t, "", "--home", home, "--cloud-url", server.URL, "profile", "pull", "acme/work", "--profile", "work", "--yes")
	if err == nil {
		t.Fatal("expected pull into active profile to fail")
	}
	requireContains(t, stderr, "native agent roots are not changed")

	stdout, stderr, err = execute(t, "", "--home", home, "--cloud-url", server.URL, "profile", "pull", "acme/work", "--profile", "incoming", "--yes")
	if err != nil {
		t.Fatalf("pull apply failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	requireContains(t, stdout, "Activate it with: skillmux use incoming")
	if _, err := os.Stat(filepath.Join(home, ".skillmux", "profiles", "incoming", "roots", "claude", "skills", "remote-only", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "remote-only", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("pull changed active native root: %v", err)
	}

	versionDigest = "bad"
	_, stderr, err = execute(t, "", "--home", home, "--cloud-url", server.URL, "profile", "diff", "acme/work", "--profile", "incoming")
	if err == nil {
		t.Fatal("expected digest mismatch")
	}
	requireContains(t, stderr, "archive digest mismatch")
}

func addSnapshotFile(snapshot *app.ProfileSnapshot, path, body string) {
	sum := sha256.Sum256([]byte(body))
	snapshot.Files = append(snapshot.Files, app.ProfileSnapshotFile{
		Path:    path,
		Type:    "file",
		Mode:    0o644,
		Content: base64.StdEncoding.EncodeToString([]byte(body)),
		SHA256:  hex.EncodeToString(sum[:]),
	})
	snapshot.Digest = ""
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

func TestCLIEnableCursorAddsOptionalAgent(t *testing.T) {
	home := t.TempDir()
	initCLIHome(t, home)
	cursorSkill := filepath.Join(home, ".cursor", "skills", "cursor-only")
	if err := os.MkdirAll(cursorSkill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cursorSkill, "SKILL.md"), []byte("cursor"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := execute(t, "", "--home", home, "enable", "cursor", "--profile", "work", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	requireContains(t, stdout, "Enabled agent \"cursor\"")
	requireContains(t, stdout, "cursor")
	if _, err := os.Readlink(filepath.Join(home, ".cursor", "skills")); err != nil {
		t.Fatalf("cursor skills was not linked: %v", err)
	}
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
	requireContains(t, stdout, "cursor")
	requireContains(t, stdout, "agents")

	stdout, _, err = execute(t, "", "--home", home, "__complete", "init", "--enable", "")
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, stdout, "cursor")
	requireContains(t, stdout, "agents")
	if strings.Contains(stdout, "claude\t") || strings.Contains(stdout, "codex\t") {
		t.Fatalf("init --enable should only complete optional adapters, got:\n%s", stdout)
	}

	stdout, _, err = execute(t, "", "--home", home, "__complete", "enable", "")
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, stdout, "cursor")
	requireContains(t, stdout, "agents")

	stdout, _, err = execute(t, "", "--home", home, "__complete", "run", "")
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, stdout, "claude")
	requireContains(t, stdout, "codex")
	if strings.Contains(stdout, "agents\t") || strings.Contains(stdout, "cursor\t") {
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

	_, stderr, err = execute(t, "", "--home", home, "run", "cursor", "--profile", "work")
	if err == nil {
		t.Fatal("expected non-runnable cursor to fail")
	}
	requireContains(t, stderr, "unsupported runnable agent \"cursor\"")

	_, stderr, err = execute(t, "", "--home", home, "enable", "nope", "--yes")
	if err == nil {
		t.Fatal("expected invalid enable agent to fail")
	}
	requireContains(t, stderr, "unsupported agent \"nope\"")
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
