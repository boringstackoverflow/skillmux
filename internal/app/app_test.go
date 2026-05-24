package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestApp(t *testing.T) (*App, string, *bytes.Buffer) {
	t.Helper()
	home := t.TempDir()
	var out bytes.Buffer
	a, err := New(home, &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	return a, home, &out
}

func writeSkill(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readlink(t *testing.T, path string) string {
	t.Helper()
	target, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("readlink %s: %v", path, err)
	}
	return target
}

func mustGroup(t *testing.T, state RootGroupsState, id string) RootGroup {
	t.Helper()
	for _, group := range state.Groups {
		if group.ID == id {
			return group
		}
	}
	t.Fatalf("missing group %q in %#v", id, state.Groups)
	return RootGroup{}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be missing, got err=%v", path, err)
	}
}

func TestInitDryRunDoesNotMutateFilesystem(t *testing.T) {
	a, home, out := newTestApp(t)
	codexRoot := filepath.Join(home, ".codex", "skills")
	writeSkill(t, codexRoot, "existing", "existing")

	if err := a.Init(InitOptions{Profile: "work", DryRun: true}); err != nil {
		t.Fatal(err)
	}

	assertPathMissing(t, a.SkillmuxHome)
	info, err := os.Lstat(codexRoot)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("dry-run replaced native root with symlink")
	}
	if _, err := os.Stat(filepath.Join(codexRoot, "existing", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Dry run only") {
		t.Fatalf("dry-run output did not explain that no files changed:\n%s", out.String())
	}
}

func TestFreshInitCreatesDefaultClaudeAndCodexViews(t *testing.T) {
	a, home, _ := newTestApp(t)

	if err := a.Init(InitOptions{Profile: "work"}); err != nil {
		t.Fatal(err)
	}

	state, err := a.loadRootGroups()
	if err != nil {
		t.Fatal(err)
	}
	claude := mustGroup(t, state, "claude")
	codex := mustGroup(t, state, "codex")
	if got := readlink(t, filepath.Join(home, ".claude", "skills")); got != a.currentGroupSkillsPath(claude) {
		t.Fatalf("claude root = %q, want %q", got, a.currentGroupSkillsPath(claude))
	}
	if got := readlink(t, filepath.Join(home, ".codex", "skills")); got != a.currentGroupSkillsPath(codex) {
		t.Fatalf("codex root = %q, want %q", got, a.currentGroupSkillsPath(codex))
	}
	active, err := a.loadActive()
	if err != nil {
		t.Fatal(err)
	}
	if got := a.activeProfileForAgent(active, AgentClaude); got != "work" {
		t.Fatalf("claude profile = %q, want work", got)
	}
	if got := a.activeProfileForAgent(active, AgentCodex); got != "work" {
		t.Fatalf("codex profile = %q, want work", got)
	}
	if _, err := a.LatestBackupIDByReason("pre-init"); err != nil {
		t.Fatal(err)
	}
}

func TestUseBeforeInitDoesNotCreateState(t *testing.T) {
	a, _, _ := newTestApp(t)

	err := a.Use("work", "")
	if err == nil {
		t.Fatal("expected use before init to fail")
	}
	assertPathMissing(t, a.SkillmuxHome)
}

func TestUseMissingProfileRequiresExplicitCreate(t *testing.T) {
	a, _, _ := newTestApp(t)
	if err := a.Init(InitOptions{Profile: "work"}); err != nil {
		t.Fatal(err)
	}

	err := a.Use("typo", "")
	if err == nil {
		t.Fatal("expected missing profile to fail")
	}
	if !strings.Contains(err.Error(), "profile \"typo\" does not exist") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertPathMissing(t, a.profileRoot("typo"))
}

func TestUseProfileCreateCreatesAndSwitches(t *testing.T) {
	a, _, _ := newTestApp(t)
	if err := a.Init(InitOptions{Profile: "work"}); err != nil {
		t.Fatal(err)
	}

	if err := a.UseProfile("research", AgentCodex, true); err != nil {
		t.Fatal(err)
	}
	active, err := a.loadActive()
	if err != nil {
		t.Fatal(err)
	}
	if got := a.activeProfileForAgent(active, AgentCodex); got != "research" {
		t.Fatalf("codex active = %q, want research", got)
	}
	if _, err := os.Stat(a.profileRoot("research")); err != nil {
		t.Fatal(err)
	}
}

func TestInitPreservesClaudeSymlinkToAgents(t *testing.T) {
	a, home, _ := newTestApp(t)
	agentsRoot := filepath.Join(home, ".agents", "skills")
	claudeRoot := filepath.Join(home, ".claude", "skills")
	writeSkill(t, agentsRoot, "shared-skill", "shared")
	if err := os.MkdirAll(filepath.Dir(claudeRoot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(agentsRoot, claudeRoot); err != nil {
		t.Fatal(err)
	}

	if err := a.Init(InitOptions{Profile: "frontend"}); err != nil {
		t.Fatal(err)
	}

	state, err := a.loadRootGroups()
	if err != nil {
		t.Fatal(err)
	}
	var shared RootGroup
	for _, group := range state.Groups {
		if group.ID == "shared-claude-agents" {
			shared = group
		}
	}
	if shared.ID == "" {
		t.Fatalf("expected shared-claude-agents group, got %#v", state.Groups)
	}
	if got := readlink(t, agentsRoot); got != a.currentGroupSkillsPath(shared) {
		t.Fatalf("agents root target = %q, want %q", got, a.currentGroupSkillsPath(shared))
	}
	if got := readlink(t, claudeRoot); got != agentsRoot {
		t.Fatalf("claude alias target = %q, want original agents root %q", got, agentsRoot)
	}
	if _, err := os.Stat(filepath.Join(a.groupSkillsPath("frontend", shared), "shared-skill", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestThirdPartyWriteThroughPreservedAliasLandsInActiveProfile(t *testing.T) {
	a, home, _ := newTestApp(t)
	agentsRoot := filepath.Join(home, ".agents", "skills")
	claudeRoot := filepath.Join(home, ".claude", "skills")
	writeSkill(t, agentsRoot, "initial", "initial")
	if err := os.MkdirAll(filepath.Dir(claudeRoot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(agentsRoot, claudeRoot); err != nil {
		t.Fatal(err)
	}
	if err := a.Init(InitOptions{Profile: "frontend"}); err != nil {
		t.Fatal(err)
	}

	writeSkill(t, claudeRoot, "marketplace", "installed through claude")

	state, err := a.loadRootGroups()
	if err != nil {
		t.Fatal(err)
	}
	var shared RootGroup
	for _, group := range state.Groups {
		if group.ID == "shared-claude-agents" {
			shared = group
		}
	}
	if _, err := os.Stat(filepath.Join(a.groupSkillsPath("frontend", shared), "marketplace", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestProfileSwitchingIsolatesMarketplaceWrites(t *testing.T) {
	a, home, _ := newTestApp(t)
	claudeRoot := filepath.Join(home, ".claude", "skills")
	writeSkill(t, claudeRoot, "initial", "initial")
	if err := a.Init(InitOptions{Profile: "work"}); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, claudeRoot, "work-only", "work")

	if err := a.ProfileCreate("personal"); err != nil {
		t.Fatal(err)
	}
	if err := a.Use("personal", ""); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, claudeRoot, "personal-only", "personal")

	state, err := a.loadRootGroups()
	if err != nil {
		t.Fatal(err)
	}
	claude := mustGroup(t, state, "claude")
	if _, err := os.Stat(filepath.Join(a.groupSkillsPath("personal", claude), "work-only", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("work skill leaked into personal profile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a.groupSkillsPath("work", claude), "personal-only", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("personal skill leaked into work profile: %v", err)
	}

	if err := a.Use("work", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(claudeRoot, "work-only", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(claudeRoot, "personal-only", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("personal skill remained visible after switching back to work: %v", err)
	}
}

func TestDirectAgentRootsConflictFailsSafely(t *testing.T) {
	a, home, _ := newTestApp(t)
	writeSkill(t, filepath.Join(home, ".agents", "skills"), "dup", "one")
	writeSkill(t, filepath.Join(home, ".agent", "skills"), "dup", "two")

	err := a.Init(InitOptions{Profile: "default"})
	if err == nil {
		t.Fatal("expected conflict")
	}
	if !strings.Contains(err.Error(), "different contents") {
		t.Fatalf("expected conflict error, got %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".agents", "skills")); statErr != nil {
		t.Fatalf("expected original .agents path to remain: %v", statErr)
	}
}

func TestDirectAgentRootsIdenticalSkillNamesMerge(t *testing.T) {
	a, home, _ := newTestApp(t)
	agentsRoot := filepath.Join(home, ".agents", "skills")
	agentRoot := filepath.Join(home, ".agent", "skills")
	writeSkill(t, agentsRoot, "dup", "same")
	writeSkill(t, agentRoot, "dup", "same")

	if err := a.Init(InitOptions{Profile: "work"}); err != nil {
		t.Fatal(err)
	}

	state, err := a.loadRootGroups()
	if err != nil {
		t.Fatal(err)
	}
	agents := mustGroup(t, state, "agents")
	if len(agents.NativePaths) != 2 {
		t.Fatalf("expected both direct roots in agents group, got %#v", agents.NativePaths)
	}
	if got := readlink(t, agentsRoot); got != a.currentGroupSkillsPath(agents) {
		t.Fatalf(".agents root = %q, want %q", got, a.currentGroupSkillsPath(agents))
	}
	if got := readlink(t, agentRoot); got != agentsRoot {
		t.Fatalf(".agent alias = %q, want %q", got, agentsRoot)
	}
	if _, err := os.Stat(filepath.Join(a.groupSkillsPath("work", agents), "dup", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestUseAgentSwitchesOnlyCodexWhenSeparateRoots(t *testing.T) {
	a, home, _ := newTestApp(t)
	writeSkill(t, filepath.Join(home, ".claude", "skills"), "claude-skill", "claude")
	writeSkill(t, filepath.Join(home, ".codex", "skills"), "codex-skill", "codex")
	if err := a.Init(InitOptions{Profile: "frontend"}); err != nil {
		t.Fatal(err)
	}
	if err := a.ProfileCreate("backend"); err != nil {
		t.Fatal(err)
	}
	if err := a.Use("backend", AgentCodex); err != nil {
		t.Fatal(err)
	}

	active, err := a.loadActive()
	if err != nil {
		t.Fatal(err)
	}
	if got := a.activeProfileForAgent(active, AgentClaude); got != "frontend" {
		t.Fatalf("claude active = %q, want frontend", got)
	}
	if got := a.activeProfileForAgent(active, AgentCodex); got != "backend" {
		t.Fatalf("codex active = %q, want backend", got)
	}
}

func TestProjectEnterSwitchesOnlyConfiguredAgents(t *testing.T) {
	a, home, _ := newTestApp(t)
	writeSkill(t, filepath.Join(home, ".claude", "skills"), "claude-skill", "claude")
	writeSkill(t, filepath.Join(home, ".codex", "skills"), "codex-skill", "codex")
	if err := a.Init(InitOptions{Profile: "default"}); err != nil {
		t.Fatal(err)
	}
	if err := a.ProfileCreate("project"); err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(home, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := []byte("profile = \"project\"\nagents = [\"codex\"]\n")
	if err := os.WriteFile(filepath.Join(projectDir, ".skillmux.toml"), config, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := a.Enter(projectDir); err != nil {
		t.Fatal(err)
	}

	active, err := a.loadActive()
	if err != nil {
		t.Fatal(err)
	}
	if got := a.activeProfileForAgent(active, AgentClaude); got != "default" {
		t.Fatalf("claude active = %q, want default", got)
	}
	if got := a.activeProfileForAgent(active, AgentCodex); got != "project" {
		t.Fatalf("codex active = %q, want project", got)
	}
}

func TestProjectEnterCreateCreatesConfiguredProfile(t *testing.T) {
	a, home, _ := newTestApp(t)
	if err := a.Init(InitOptions{Profile: "default"}); err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(home, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := []byte("profile = \"project\"\nagents = [\"codex\"]\n")
	if err := os.WriteFile(filepath.Join(projectDir, ".skillmux.toml"), config, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := a.Enter(projectDir); err == nil {
		t.Fatal("expected enter to require an existing profile")
	}
	if err := a.EnterProfile(projectDir, true); err != nil {
		t.Fatal(err)
	}
	active, err := a.loadActive()
	if err != nil {
		t.Fatal(err)
	}
	if got := a.activeProfileForAgent(active, AgentCodex); got != "project" {
		t.Fatalf("codex active = %q, want project", got)
	}
}

func TestUninstallRestoresOriginalSymlinkTopology(t *testing.T) {
	a, home, _ := newTestApp(t)
	agentsRoot := filepath.Join(home, ".agents", "skills")
	claudeRoot := filepath.Join(home, ".claude", "skills")
	writeSkill(t, agentsRoot, "shared-skill", "shared")
	if err := os.MkdirAll(filepath.Dir(claudeRoot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(agentsRoot, claudeRoot); err != nil {
		t.Fatal(err)
	}
	if err := a.Init(InitOptions{Profile: "frontend"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Uninstall(""); err != nil {
		t.Fatal(err)
	}
	if got := readlink(t, claudeRoot); got != agentsRoot {
		t.Fatalf("restored claude link = %q, want %q", got, agentsRoot)
	}
	if _, err := os.Stat(filepath.Join(agentsRoot, "shared-skill", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestRepairRestoresOverwrittenNativeRootAndBacksItUp(t *testing.T) {
	a, home, out := newTestApp(t)
	codexRoot := filepath.Join(home, ".codex", "skills")
	writeSkill(t, codexRoot, "codex-skill", "codex")
	if err := a.Init(InitOptions{Profile: "work"}); err != nil {
		t.Fatal(err)
	}
	if err := removePath(codexRoot); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, codexRoot, "rogue", "rogue")

	out.Reset()
	if err := a.Doctor(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "repairable") {
		t.Fatalf("doctor did not report repairable state:\n%s", out.String())
	}
	if err := a.Repair(false); err != nil {
		t.Fatal(err)
	}

	state, err := a.loadRootGroups()
	if err != nil {
		t.Fatal(err)
	}
	codex := mustGroup(t, state, "codex")
	if got := readlink(t, codexRoot); got != a.currentGroupSkillsPath(codex) {
		t.Fatalf("codex root = %q, want %q", got, a.currentGroupSkillsPath(codex))
	}
	if _, err := a.LatestBackupIDByReason("pre-repair"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(codexRoot, "rogue", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("rogue skill remained visible after repair: %v", err)
	}
}

func TestProfileRenameActiveProfileKeepsStateAndLinksHealthy(t *testing.T) {
	a, home, _ := newTestApp(t)
	writeSkill(t, filepath.Join(home, ".codex", "skills"), "codex-skill", "codex")
	if err := a.Init(InitOptions{Profile: "work"}); err != nil {
		t.Fatal(err)
	}
	if err := a.ProfileRename("work", "office"); err != nil {
		t.Fatal(err)
	}

	active, err := a.loadActive()
	if err != nil {
		t.Fatal(err)
	}
	if got := a.activeProfileForAgent(active, AgentCodex); got != "office" {
		t.Fatalf("codex active = %q, want office", got)
	}
	state, err := a.loadRootGroups()
	if err != nil {
		t.Fatal(err)
	}
	codex := mustGroup(t, state, "codex")
	if got := readlink(t, a.currentGroupPath(codex)); got != a.groupProfileRoot("office", codex) {
		t.Fatalf("current codex pointer = %q, want %q", got, a.groupProfileRoot("office", codex))
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "codex-skill", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestProfileDeleteRefusesActiveProfile(t *testing.T) {
	a, _, _ := newTestApp(t)
	if err := a.Init(InitOptions{Profile: "work"}); err != nil {
		t.Fatal(err)
	}
	if err := a.ProfileDelete("work", true); err == nil {
		t.Fatal("expected active profile delete to fail")
	}
	if _, err := os.Stat(a.profileRoot("work")); err != nil {
		t.Fatal(err)
	}
}

func TestReservedSystemSkillsAreLinkedIntoProfile(t *testing.T) {
	a, home, _ := newTestApp(t)
	system := filepath.Join(home, ".codex", "skills", ".system", "builtin")
	if err := os.MkdirAll(system, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(system, "SKILL.md"), []byte("builtin"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.Init(InitOptions{Profile: "frontend"}); err != nil {
		t.Fatal(err)
	}
	state, err := a.loadRootGroups()
	if err != nil {
		t.Fatal(err)
	}
	var codex RootGroup
	for _, group := range state.Groups {
		if group.ID == "codex" {
			codex = group
		}
	}
	link := filepath.Join(a.groupSkillsPath("frontend", codex), ".system")
	if target := readlink(t, link); !strings.Contains(target, filepath.Join(".skillmux", "shared", "roots", "codex", "skills", ".system")) {
		t.Fatalf("unexpected .system target %q", target)
	}
	if _, err := os.Stat(filepath.Join(link, "builtin", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestExistingCustomAssetsAreImportedAndSwitched(t *testing.T) {
	a, home, _ := newTestApp(t)
	commands := filepath.Join(home, ".claude", "commands")
	if err := os.MkdirAll(commands, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commands, "ship.md"), []byte("ship"), 0o644); err != nil {
		t.Fatal(err)
	}
	rules := filepath.Join(home, ".codex", "rules")
	if err := os.MkdirAll(rules, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rules, "security.rules"), []byte("rules"), 0o644); err != nil {
		t.Fatal(err)
	}
	instructions := filepath.Join(home, ".codex", "instructions.md")
	if err := os.WriteFile(instructions, []byte("instructions"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := a.Init(InitOptions{Profile: "frontend"}); err != nil {
		t.Fatal(err)
	}

	if got := readlink(t, commands); !strings.Contains(got, filepath.Join(".skillmux", "current", "assets", "claude-commands")) {
		t.Fatalf("commands link = %q", got)
	}
	if got := readlink(t, rules); !strings.Contains(got, filepath.Join(".skillmux", "current", "assets", "codex-rules")) {
		t.Fatalf("rules link = %q", got)
	}
	if got := readlink(t, instructions); !strings.Contains(got, filepath.Join(".skillmux", "current", "assets", "codex-instructions")) {
		t.Fatalf("instructions link = %q", got)
	}
	if _, err := os.Stat(filepath.Join(a.profileRoot("frontend"), "assets", "claude", "commands", "ship.md")); err != nil {
		t.Fatal(err)
	}
	if err := a.ProfileCreate("backend"); err != nil {
		t.Fatal(err)
	}
	if err := a.Use("backend", AgentCodex); err != nil {
		t.Fatal(err)
	}
	if got := readlink(t, rules); !strings.Contains(got, filepath.Join(".skillmux", "current", "assets", "codex-rules")) {
		t.Fatalf("rules link after switch = %q", got)
	}
	if _, err := os.Stat(filepath.Join(a.profileRoot("backend"), "assets", "codex", "rules")); err != nil {
		t.Fatal(err)
	}
}
