package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type InitOptions struct {
	Profile string
	Enable  []string
	DryRun  bool
}

func (a *App) Init(opts InitOptions) error {
	if opts.Profile == "" {
		opts.Profile = "default"
	}
	for _, agent := range opts.Enable {
		if !IsOptionalAgent(agent) {
			return fmt.Errorf("unsupported optional adapter %q; supported optional adapters: %s", agent, strings.Join(OptionalAgentOrder, ", "))
		}
	}
	if opts.DryRun {
		candidates, state, err := a.Discover()
		if err != nil {
			return err
		}
		state = a.withDefaultGroups(state, opts.Enable)
		a.PrintDiscovery(candidates, state)
		fmt.Fprintln(a.Out)
		fmt.Fprintln(a.Out, "Dry run only; no files changed.")
		return nil
	}
	lock, err := a.Lock()
	if err != nil {
		return err
	}
	defer lock.Close()

	candidates, state, err := a.Discover()
	if err != nil {
		return err
	}
	state = a.withDefaultGroups(state, opts.Enable)
	assets := a.DiscoverAssets()
	a.PrintDiscovery(candidates, state)

	backupPaths := append(a.managedNativePaths(state), assetNativePaths(assets.Assets)...)
	backupID, err := a.CreateBackup("pre-init", backupPaths)
	if err != nil {
		return err
	}
	if err := a.createProfileDirs(opts.Profile, state.Groups); err != nil {
		return err
	}
	if err := a.createAssetProfilePaths(opts.Profile, assets.Assets); err != nil {
		return err
	}
	if err := a.importExistingSkills(opts.Profile, state); err != nil {
		return err
	}
	if err := a.importAssets(opts.Profile, assets.Assets); err != nil {
		return err
	}
	if err := a.saveRootGroups(state); err != nil {
		return err
	}
	if err := a.saveAssets(assets); err != nil {
		return err
	}
	if err := a.switchGroups(opts.Profile, state, state.Groups, true); err != nil {
		return err
	}
	if err := a.switchAssets(opts.Profile, assets.Assets, true); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "\nInitialized Skillmux profile %q\n", opts.Profile)
	fmt.Fprintf(a.Out, "Backup: %s\n", backupID)
	return nil
}

func (a *App) withDefaultGroups(state RootGroupsState, enable []string) RootGroupsState {
	if state.Version == 0 {
		state.Version = 1
	}
	enableAgents := map[string]bool{}
	for _, agent := range enable {
		enableAgents[agent] = true
	}
	for _, group := range state.Groups {
		if contains(group.Agents, AgentAgents) {
			enableAgents[AgentAgents] = true
		}
	}
	for _, agent := range []string{AgentClaude, AgentCodex} {
		if !stateHasAgent(state, agent) {
			state.Groups = append(state.Groups, RootGroup{
				ID:          agent,
				Kind:        "skills",
				ProfilePath: filepath.ToSlash(filepath.Join("roots", agent)),
				Agents:      []string{agent},
				NativePaths: []NativePath{{
					Path:         a.defaultNativePath(agent),
					Agent:        agent,
					Role:         "primary",
					OriginalKind: "absent",
				}},
			})
		}
	}
	for _, agent := range OptionalAgentOrder {
		if enableAgents[agent] && !stateHasAgent(state, agent) {
			state.Groups = append(state.Groups, a.defaultRootGroup(agent, "absent"))
		}
	}
	sort.Slice(state.Groups, func(i, j int) bool { return state.Groups[i].ID < state.Groups[j].ID })
	return state
}

func stateHasAgent(state RootGroupsState, agent string) bool {
	for _, group := range state.Groups {
		if contains(group.Agents, agent) {
			return true
		}
	}
	return false
}

func (a *App) requireInitialized() (RootGroupsState, error) {
	state, err := a.loadRootGroups()
	if err != nil {
		return RootGroupsState{}, fmt.Errorf("skillmux is not initialized: %w", err)
	}
	return state, nil
}

func (a *App) defaultNativePath(agent string) string {
	switch agent {
	case AgentClaude:
		return filepath.Join(a.Home, ".claude", "skills")
	case AgentCodex:
		return filepath.Join(a.Home, ".codex", "skills")
	case AgentCursor:
		return filepath.Join(a.Home, ".cursor", "skills")
	case AgentAgents:
		return filepath.Join(a.Home, ".agents", "skills")
	default:
		return filepath.Join(a.Home, "."+agent, "skills")
	}
}

func (a *App) defaultRootGroup(agent, originalKind string) RootGroup {
	return RootGroup{
		ID:          agent,
		Kind:        "skills",
		ProfilePath: filepath.ToSlash(filepath.Join("roots", agent)),
		Agents:      []string{agent},
		NativePaths: []NativePath{{
			Path:         a.defaultNativePath(agent),
			Agent:        agent,
			Role:         "primary",
			OriginalKind: originalKind,
		}},
	}
}

func (a *App) ProfileCreate(name string) error {
	state, err := a.requireInitialized()
	if err != nil {
		return err
	}
	lock, err := a.Lock()
	if err != nil {
		return err
	}
	defer lock.Close()
	state, err = a.requireInitialized()
	if err != nil {
		return err
	}
	if err := validateProfileName(name); err != nil {
		return err
	}
	exists, err := a.profileExists(name)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("profile %q already exists", name)
	}
	if err := a.createProfileDirs(name, state.Groups); err != nil {
		return err
	}
	if assets, err := a.loadAssets(); err == nil {
		if err := a.createAssetProfilePaths(name, assets.Assets); err != nil {
			return err
		}
	}
	fmt.Fprintf(a.Out, "Created profile %q\n", name)
	return nil
}

func (a *App) EnableAgent(agent, profile string) error {
	if agent == "" {
		return errors.New("agent is required")
	}
	if !IsSupportedAgent(agent) {
		return fmt.Errorf("unsupported agent %q; supported agents: %s", agent, strings.Join(AgentOrder, ", "))
	}
	lock, err := a.Lock()
	if err != nil {
		return err
	}
	defer lock.Close()
	state, err := a.requireInitialized()
	if err != nil {
		return err
	}
	if stateHasAgent(state, agent) {
		fmt.Fprintf(a.Out, "Agent %q is already enabled\n", agent)
		return nil
	}
	if profile == "" {
		profile, err = a.singleActiveProfile()
		if err != nil {
			return err
		}
	}
	if err := a.requireProfile(profile); err != nil {
		return err
	}

	candidate := a.inspectDefaultCandidate(agent)
	backupID, err := a.CreateBackup("pre-enable-"+agent, []string{candidate.Path})
	if err != nil {
		return err
	}

	group := a.defaultRootGroup(agent, candidate.Kind)
	if idx := a.matchingManagedGroupIndex(state, candidate); idx >= 0 {
		group = state.Groups[idx]
		group.Agents = orderedAgentList(addUnique(group.Agents, agent))
		group.NativePaths = append(group.NativePaths, NativePath{
			Path:           candidate.Path,
			Agent:          agent,
			Role:           "alias",
			OriginalKind:   candidate.Kind,
			OriginalTarget: candidate.OriginalTarget,
		})
		state.Groups[idx] = group
	} else {
		state.Groups = append(state.Groups, group)
	}
	sort.Slice(state.Groups, func(i, j int) bool { return state.Groups[i].ID < state.Groups[j].ID })

	profiles, err := a.ListProfiles()
	if err != nil {
		return err
	}
	for _, existing := range profiles {
		if err := a.createProfileDirs(existing, []RootGroup{group}); err != nil {
			return err
		}
	}
	if err := a.importGroup(profile, group); err != nil {
		return err
	}
	if err := a.saveRootGroups(state); err != nil {
		return err
	}
	if err := a.switchGroups(profile, state, []RootGroup{group}, true); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Enabled agent %q with profile %q\n", agent, profile)
	fmt.Fprintf(a.Out, "Backup: %s\n", backupID)
	return a.Current()
}

func (a *App) singleActiveProfile() (string, error) {
	active, err := a.loadActive()
	if err != nil {
		return "", fmt.Errorf("--profile is required when no active profile is recorded")
	}
	seen := map[string]bool{}
	var profiles []string
	for _, entry := range active.Agents {
		if entry.Profile == "" || seen[entry.Profile] {
			continue
		}
		seen[entry.Profile] = true
		profiles = append(profiles, entry.Profile)
	}
	if len(profiles) == 1 {
		return profiles[0], nil
	}
	sort.Strings(profiles)
	if len(profiles) == 0 {
		return "", fmt.Errorf("--profile is required when no active profile is recorded")
	}
	return "", fmt.Errorf("--profile is required because active agents use multiple profiles: %s", strings.Join(profiles, ", "))
}

func (a *App) inspectDefaultCandidate(agent string) Candidate {
	path := a.defaultNativePath(agent)
	for _, candidate := range a.inspectCandidates() {
		if candidate.Agent == agent && candidate.Path == path {
			return candidate
		}
	}
	return Candidate{
		Agent:       agent,
		Path:        path,
		DisplayPath: a.display(path),
		Kind:        "absent",
	}
}

func (a *App) matchingManagedGroupIndex(state RootGroupsState, candidate Candidate) int {
	if candidate.Resolved == "" {
		return -1
	}
	for i, group := range state.Groups {
		for _, native := range group.NativePaths {
			resolved, err := filepath.EvalSymlinks(native.Path)
			if err != nil {
				continue
			}
			if filepath.Clean(resolved) == filepath.Clean(candidate.Resolved) {
				return i
			}
		}
	}
	return -1
}

func orderedAgentList(values []string) []string {
	var out []string
	for _, agent := range AgentOrder {
		if contains(values, agent) {
			out = append(out, agent)
		}
	}
	return out
}

func (a *App) ListProfiles() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(a.SkillmuxHome, "profiles"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var profiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			profiles = append(profiles, entry.Name())
		}
	}
	sort.Strings(profiles)
	return profiles, nil
}

func (a *App) profileExists(name string) (bool, error) {
	if err := validateProfileName(name); err != nil {
		return false, err
	}
	info, err := os.Stat(a.profileRoot(name))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("profile path for %q is not a directory", name)
	}
	return true, nil
}

func (a *App) requireProfile(name string) error {
	exists, err := a.profileExists(name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("profile %q does not exist", name)
	}
	return nil
}

func validateProfileName(name string) error {
	if name == "" {
		return errors.New("profile name is required")
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return fmt.Errorf("profile name %q is not filesystem-safe", name)
	}
	return nil
}

func (a *App) createProfileDirs(profile string, groups []RootGroup) error {
	if err := validateProfileName(profile); err != nil {
		return err
	}
	for _, group := range groups {
		if err := ensureDir(a.groupSkillsPath(profile, group)); err != nil {
			return err
		}
		if err := a.ensureReservedLinks(profile, group); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) importExistingSkills(profile string, state RootGroupsState) error {
	for _, group := range state.Groups {
		if err := a.importGroup(profile, group); err != nil {
			return err
		}
		if err := a.ensureReservedLinks(profile, group); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) importGroup(profile string, group RootGroup) error {
	targetRoot := a.groupSkillsPath(profile, group)
	for _, native := range group.NativePaths {
		info, err := os.Stat(native.Path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if !info.IsDir() {
			continue
		}
		entries, err := os.ReadDir(native.Path)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			name := entry.Name()
			if name == ".DS_Store" {
				continue
			}
			src := filepath.Join(native.Path, name)
			if name == ".system" {
				if err := a.importReserved(group, src); err != nil {
					return err
				}
				continue
			}
			dst := filepath.Join(targetRoot, name)
			if err := copyWithConflictCheck(src, dst); err != nil {
				return fmt.Errorf("conflict importing %s into group %s: %w", name, group.ID, err)
			}
		}
	}
	return nil
}

func copyWithConflictCheck(src, dst string) error {
	if _, err := os.Lstat(dst); err == nil {
		srcDigest, err := digestPath(src)
		if err != nil {
			return err
		}
		dstDigest, err := digestPath(dst)
		if err != nil {
			return err
		}
		if srcDigest == dstDigest {
			return nil
		}
		srcInfo, err := os.Lstat(src)
		if err != nil {
			return err
		}
		dstInfo, err := os.Lstat(dst)
		if err != nil {
			return err
		}
		if srcInfo.IsDir() && dstInfo.IsDir() {
			return mergeDirWithConflictCheck(src, dst)
		}
		return fmt.Errorf("destination %s already exists with different contents", dst)
	} else if !os.IsNotExist(err) {
		return err
	}
	return copyPath(src, dst)
}

func mergeDirWithConflictCheck(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyWithConflictCheck(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) importReserved(group RootGroup, src string) error {
	dst := filepath.Join(a.SkillmuxHome, "shared", "roots", group.ID, "skills", ".system")
	return copyWithConflictCheck(src, dst)
}

func (a *App) ensureReservedLinks(profile string, group RootGroup) error {
	shared := filepath.Join(a.SkillmuxHome, "shared", "roots", group.ID, "skills", ".system")
	if _, err := os.Lstat(shared); err != nil {
		return nil
	}
	link := filepath.Join(a.groupSkillsPath(profile, group), ".system")
	if _, err := os.Lstat(link); err == nil {
		return nil
	}
	return replaceSymlink(link, shared)
}

func (a *App) Use(profile, agent string) error {
	return a.UseProfile(profile, agent, false)
}

func (a *App) UseProfile(profile, agent string, create bool) error {
	if _, err := a.requireInitialized(); err != nil {
		return err
	}
	if !IsSupportedAgent(agent) {
		return fmt.Errorf("unsupported agent %q; supported agents: %s", agent, strings.Join(AgentOrder, ", "))
	}
	lock, err := a.Lock()
	if err != nil {
		return err
	}
	defer lock.Close()
	state, err := a.requireInitialized()
	if err != nil {
		return err
	}
	if err := validateProfileName(profile); err != nil {
		return err
	}
	exists, err := a.profileExists(profile)
	if err != nil {
		return err
	}
	if !exists && !create {
		return fmt.Errorf("profile %q does not exist; create it with `skillmux profile create %s` or rerun with --create", profile, profile)
	}
	groups := groupsForAgent(state.Groups, agent)
	if len(groups) == 0 {
		return fmt.Errorf("no enabled groups match agent %q", agent)
	}
	assets, _ := a.loadAssets()
	if !exists {
		if err := a.createProfileDirs(profile, state.Groups); err != nil {
			return err
		}
		if err := a.createAssetProfilePaths(profile, assets.Assets); err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "Created profile %q\n", profile)
	}
	filteredAssets := assetsForAgent(assets.Assets, agent)
	if err := a.createProfileDirs(profile, groups); err != nil {
		return err
	}
	if err := a.createAssetProfilePaths(profile, filteredAssets); err != nil {
		return err
	}
	if err := a.switchGroups(profile, state, groups, false); err != nil {
		return err
	}
	if err := a.switchAssets(profile, filteredAssets, false); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Active profile: %s\n", profile)
	return a.Current()
}

func groupsForAgent(groups []RootGroup, agent string) []RootGroup {
	if agent == "" {
		return groups
	}
	var out []RootGroup
	for _, group := range groups {
		if contains(group.Agents, agent) {
			out = append(out, group)
		}
	}
	return out
}

func (a *App) switchGroups(profile string, state RootGroupsState, groups []RootGroup, alreadyBackedUp bool) error {
	if !alreadyBackedUp {
		if err := a.backupUnexpectedLinks(groups, "pre-switch"); err != nil {
			return err
		}
	}
	for _, group := range groups {
		if err := a.createProfileDirs(profile, []RootGroup{group}); err != nil {
			return err
		}
		if err := replaceSymlink(a.currentGroupPath(group), a.groupProfileRoot(profile, group)); err != nil {
			return err
		}
		if err := a.linkNativeGroup(group); err != nil {
			return err
		}
	}
	return a.updateActive(profile, state, groups)
}

func (a *App) linkNativeGroup(group RootGroup) error {
	primary := primaryNative(group)
	for _, native := range group.NativePaths {
		target := a.currentGroupSkillsPath(group)
		if native.Role != "primary" {
			target = primary.Path
			if native.OriginalKind == "symlink" && native.OriginalTarget != "" {
				target = native.OriginalTarget
			}
		}
		if err := replaceSymlink(native.Path, target); err != nil {
			return err
		}
	}
	return nil
}

func primaryNative(group RootGroup) NativePath {
	if len(group.NativePaths) == 0 {
		return NativePath{}
	}
	for _, native := range group.NativePaths {
		if native.Role == "primary" {
			return native
		}
	}
	return group.NativePaths[0]
}

func (a *App) expectedTarget(group RootGroup, native NativePath) string {
	if native.Role == "primary" {
		return a.currentGroupSkillsPath(group)
	}
	if native.OriginalKind == "symlink" && native.OriginalTarget != "" {
		return native.OriginalTarget
	}
	return primaryNative(group).Path
}

func (a *App) backupUnexpectedLinks(groups []RootGroup, reason string) error {
	var paths []string
	for _, group := range groups {
		for _, native := range group.NativePaths {
			info, err := os.Lstat(native.Path)
			if err != nil {
				continue
			}
			if info.Mode()&os.ModeSymlink == 0 {
				paths = append(paths, native.Path)
				continue
			}
			target, err := os.Readlink(native.Path)
			if err != nil || target != a.expectedTarget(group, native) {
				paths = append(paths, native.Path)
			}
		}
	}
	if len(paths) == 0 {
		return nil
	}
	_, err := a.CreateBackup(reason, paths)
	return err
}

func (a *App) updateActive(profile string, state RootGroupsState, groups []RootGroup) error {
	active, _ := a.loadActive()
	active.Version = 1
	byAgent := map[string]ActiveAgent{}
	for _, entry := range active.Agents {
		byAgent[entry.Agent] = entry
	}
	for _, group := range groups {
		for _, agent := range group.Agents {
			entry := byAgent[agent]
			entry.Agent = agent
			entry.Profile = profile
			entry.Groups = addUnique(entry.Groups, group.ID)
			byAgent[agent] = entry
		}
	}
	active.Agents = active.Agents[:0]
	for _, agent := range AgentOrder {
		if entry, ok := byAgent[agent]; ok {
			active.Agents = append(active.Agents, entry)
		}
	}
	return a.saveActive(active)
}

func addUnique(values []string, value string) []string {
	for _, v := range values {
		if v == value {
			return values
		}
	}
	return append(values, value)
}

func (a *App) Current() error {
	state, err := a.loadRootGroups()
	if err != nil {
		return fmt.Errorf("skillmux is not initialized: %w", err)
	}
	active, err := a.loadActive()
	if err != nil {
		return fmt.Errorf("no active profile is recorded: %w", err)
	}
	table := newTable(a.Out)
	defer table.Flush()
	tableRow(table, "AGENT", "PROFILE", "ROOT GROUP", "NATIVE PATHS")
	for _, agent := range AgentOrder {
		profile := a.activeProfileForAgent(active, agent)
		if profile == "" {
			continue
		}
		for _, group := range state.Groups {
			if !contains(group.Agents, agent) {
				continue
			}
			var paths []string
			for _, native := range group.NativePaths {
				paths = append(paths, a.display(native.Path))
			}
			tableRow(table, agent, profile, group.ID, strings.Join(paths, ", "))
		}
	}
	return nil
}

func (a *App) ProfileList() error {
	if _, err := a.requireInitialized(); err != nil {
		return err
	}
	profiles, err := a.ListProfiles()
	if err != nil {
		return err
	}
	active, _ := a.loadActive()
	table := newTable(a.Out)
	defer table.Flush()
	tableRow(table, "PROFILE", "ACTIVE AGENTS")
	for _, profile := range profiles {
		var agents []string
		for _, activeEntry := range active.Agents {
			if activeEntry.Profile == profile {
				agents = append(agents, activeEntry.Agent)
			}
		}
		tableRow(table, profile, strings.Join(agents, ","))
	}
	return nil
}

func (a *App) ProfileShow(profile, agent string) error {
	state, err := a.loadRootGroups()
	if err != nil {
		return err
	}
	if !IsSupportedAgent(agent) {
		return fmt.Errorf("unsupported agent %q; supported agents: %s", agent, strings.Join(AgentOrder, ", "))
	}
	if err := a.requireProfile(profile); err != nil {
		return err
	}
	groups := groupsForAgent(state.Groups, agent)
	if len(groups) == 0 {
		return fmt.Errorf("no groups found for agent %q", agent)
	}
	for _, group := range groups {
		root := a.groupSkillsPath(profile, group)
		fmt.Fprintf(a.Out, "\n[%s] %s\n", group.ID, a.display(root))
		if err := scanSkills(a.Out, root); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) ProfileRename(oldName, newName string) error {
	if _, err := a.requireInitialized(); err != nil {
		return err
	}
	lock, err := a.Lock()
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := validateProfileName(newName); err != nil {
		return err
	}
	if err := a.requireProfile(oldName); err != nil {
		return err
	}
	exists, err := a.profileExists(newName)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("profile %q already exists", newName)
	}
	if err := os.Rename(a.profileRoot(oldName), a.profileRoot(newName)); err != nil {
		return err
	}

	active, activeErr := a.loadActive()
	if activeErr != nil {
		return nil
	}
	changed := false
	for i := range active.Agents {
		if active.Agents[i].Profile == oldName {
			active.Agents[i].Profile = newName
			changed = true
		}
	}
	if !changed {
		return nil
	}

	if state, err := a.loadRootGroups(); err == nil {
		for _, group := range state.Groups {
			if profileForGroup(active, group) == newName {
				if err := replaceSymlink(a.currentGroupPath(group), a.groupProfileRoot(newName, group)); err != nil {
					return err
				}
			}
		}
	}
	if assets, err := a.loadAssets(); err == nil {
		for _, asset := range assets.Assets {
			if a.activeProfileForAgent(active, asset.Agent) == newName {
				if err := replaceSymlink(a.currentAssetPath(asset), a.assetProfilePath(newName, asset)); err != nil {
					return err
				}
			}
		}
	}
	return a.saveActive(active)
}

func (a *App) ProfileDelete(name string, force bool) error {
	if _, err := a.requireInitialized(); err != nil {
		return err
	}
	lock, err := a.Lock()
	if err != nil {
		return err
	}
	defer lock.Close()
	if !force {
		return errors.New("profile delete requires --force")
	}
	if err := a.requireProfile(name); err != nil {
		return err
	}
	if active, err := a.loadActive(); err == nil {
		for _, entry := range active.Agents {
			if entry.Profile == name {
				return fmt.Errorf("cannot delete active profile %q; switch affected agents first", name)
			}
		}
	}
	return removePath(a.profileRoot(name))
}

func (a *App) Repair(dryRun bool) error {
	if _, err := a.requireInitialized(); err != nil {
		return err
	}
	lock, err := a.Lock()
	if err != nil {
		return err
	}
	defer lock.Close()
	state, err := a.requireInitialized()
	if err != nil {
		return err
	}
	active, err := a.loadActive()
	if err != nil {
		return err
	}
	if dryRun {
		fmt.Fprintln(a.Out, "Repair would relink managed native paths:")
		for _, group := range state.Groups {
			for _, native := range group.NativePaths {
				fmt.Fprintf(a.Out, "- %s -> %s\n", a.display(native.Path), a.expectedTarget(group, native))
			}
		}
		return nil
	}
	if err := a.backupUnexpectedLinks(state.Groups, "pre-repair"); err != nil {
		return err
	}
	assets, _ := a.loadAssets()
	if err := a.backupUnexpectedAssets(assets.Assets, "pre-repair-assets"); err != nil {
		return err
	}
	for _, group := range state.Groups {
		profile := profileForGroup(active, group)
		if profile == "" {
			continue
		}
		if err := replaceSymlink(a.currentGroupPath(group), a.groupProfileRoot(profile, group)); err != nil {
			return err
		}
		if err := a.linkNativeGroup(group); err != nil {
			return err
		}
	}
	for _, asset := range assets.Assets {
		profile := a.activeProfileForAgent(active, asset.Agent)
		if profile == "" {
			continue
		}
		if err := a.switchAssets(profile, []AssetResource{asset}, true); err != nil {
			return err
		}
	}
	fmt.Fprintln(a.Out, "Repair complete")
	return nil
}

func profileForGroup(active ActiveState, group RootGroup) string {
	for _, agent := range group.Agents {
		for _, entry := range active.Agents {
			if entry.Agent == agent && entry.Profile != "" {
				return entry.Profile
			}
		}
	}
	return ""
}

func (a *App) Uninstall(backupID string) error {
	if _, err := a.requireInitialized(); err != nil {
		return err
	}
	if backupID == "" {
		var err error
		backupID, err = a.LatestBackupIDByReason("pre-init")
		if err != nil {
			return fmt.Errorf("no pre-init backup found: %w", err)
		}
	}
	lock, err := a.Lock()
	if err != nil {
		return err
	}
	defer lock.Close()
	if _, err := a.BackupManaged("pre-uninstall"); err != nil {
		return err
	}
	if err := a.RestoreBackup(backupID); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, "Uninstalled Skillmux-managed native links and restored backup", backupID)
	return nil
}
