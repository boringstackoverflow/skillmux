package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (a *App) candidatePaths() []Candidate {
	return []Candidate{
		{Agent: AgentClaude, Path: filepath.Join(a.Home, ".claude", "skills")},
		{Agent: AgentCodex, Path: filepath.Join(a.Home, ".codex", "skills")},
		{Agent: AgentCursor, Path: filepath.Join(a.Home, ".cursor", "skills")},
		{Agent: AgentAgents, Path: filepath.Join(a.Home, ".agents", "skills")},
		{Agent: AgentAgents, Path: filepath.Join(a.Home, ".agent", "skills")},
	}
}

func (a *App) Discover() ([]Candidate, RootGroupsState, error) {
	candidates := a.inspectCandidates()
	groups, err := a.groupCandidates(candidates)
	if err != nil {
		return candidates, RootGroupsState{}, err
	}
	return candidates, RootGroupsState{Version: 1, Groups: groups}, nil
}

func (a *App) inspectCandidates() []Candidate {
	out := a.candidatePaths()
	for i := range out {
		c := &out[i]
		c.DisplayPath = a.display(c.Path)
		info, err := os.Lstat(c.Path)
		if err != nil {
			if os.IsNotExist(err) {
				c.Kind = "absent"
			} else {
				c.Kind = "error"
				c.Error = err.Error()
			}
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			c.Kind = "symlink"
			target, err := os.Readlink(c.Path)
			if err != nil {
				c.Kind = "error"
				c.Error = err.Error()
				continue
			}
			c.OriginalTarget = target
			resolved, err := filepath.EvalSymlinks(c.Path)
			if err != nil {
				c.Kind = "broken_symlink"
				c.Error = err.Error()
				continue
			}
			c.Resolved = filepath.Clean(resolved)
			continue
		}
		if info.IsDir() {
			c.Kind = "directory"
			resolved, err := filepath.EvalSymlinks(c.Path)
			if err != nil {
				c.Resolved = filepath.Clean(c.Path)
			} else {
				c.Resolved = filepath.Clean(resolved)
			}
			continue
		}
		c.Kind = "unexpected_file"
	}
	return out
}

func (a *App) groupCandidates(candidates []Candidate) ([]RootGroup, error) {
	directTargets := map[string]bool{}
	for _, c := range candidates {
		if c.Agent == AgentAgents && (c.Kind == "directory" || c.Kind == "symlink") && c.Resolved != "" {
			directTargets[c.Resolved] = true
		}
	}
	buckets := map[string][]Candidate{}
	for _, c := range candidates {
		if c.Kind != "directory" && c.Kind != "symlink" {
			continue
		}
		key := "resolved:" + c.Resolved
		if c.Agent == AgentAgents || directTargets[c.Resolved] {
			key = "direct"
		}
		buckets[key] = append(buckets[key], c)
	}
	var groups []RootGroup
	for _, bucket := range buckets {
		sort.Slice(bucket, func(i, j int) bool { return pathPriority(bucket[i].Path) < pathPriority(bucket[j].Path) })
		agents := orderedAgents(bucket)
		id := groupID(agents)
		group := RootGroup{
			ID:          id,
			Kind:        "skills",
			ProfilePath: filepath.ToSlash(filepath.Join("roots", id)),
			Agents:      agents,
		}
		primaryIndex := 0
		for i, c := range bucket {
			if c.Kind == "directory" && pathPriority(c.Path) < pathPriority(bucket[primaryIndex].Path) {
				primaryIndex = i
			}
		}
		for i, c := range bucket {
			role := "alias"
			if i == primaryIndex {
				role = "primary"
			}
			group.NativePaths = append(group.NativePaths, NativePath{
				Path:           c.Path,
				Agent:          c.Agent,
				Role:           role,
				OriginalKind:   c.Kind,
				OriginalTarget: c.OriginalTarget,
			})
		}
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	return groups, nil
}

func orderedAgents(candidates []Candidate) []string {
	seen := map[string]bool{}
	var out []string
	for _, agent := range AgentOrder {
		for _, c := range candidates {
			if c.Agent == agent && !seen[agent] {
				seen[agent] = true
				out = append(out, agent)
			}
		}
	}
	return out
}

func groupID(agents []string) string {
	if len(agents) == 1 {
		return agents[0]
	}
	return "shared-" + strings.Join(agents, "-")
}

func pathPriority(path string) int {
	switch {
	case strings.Contains(path, string(filepath.Separator)+".agents"+string(filepath.Separator)+"skills"):
		return 10
	case strings.Contains(path, string(filepath.Separator)+".agent"+string(filepath.Separator)+"skills"):
		return 20
	case strings.Contains(path, string(filepath.Separator)+".claude"+string(filepath.Separator)+"skills"):
		return 30
	case strings.Contains(path, string(filepath.Separator)+".codex"+string(filepath.Separator)+"skills"):
		return 40
	case strings.Contains(path, string(filepath.Separator)+".cursor"+string(filepath.Separator)+"skills"):
		return 50
	default:
		return 100
	}
}

func (a *App) PrintDiscovery(candidates []Candidate, state RootGroupsState) {
	fmt.Fprintln(a.Out, "Detected skill roots:")
	fmt.Fprintln(a.Out)
	table := newTable(a.Out)
	defer table.Flush()
	tableRow(table, "GROUP", "NATIVE PATH", "KIND", "TARGET")
	groupByPath := map[string]string{}
	for _, group := range state.Groups {
		for _, native := range group.NativePaths {
			groupByPath[native.Path] = group.ID
		}
	}
	for _, c := range candidates {
		if c.Kind == "absent" {
			continue
		}
		group := groupByPath[c.Path]
		if group == "" {
			group = "-"
		}
		target := "-"
		if c.OriginalTarget != "" {
			target = c.OriginalTarget
		}
		tableRow(table, group, c.DisplayPath, c.Kind, target)
	}
}

func (a *App) PrintInitPlan(profile string, state RootGroupsState, assets AssetState, dryRun bool) {
	if dryRun {
		fmt.Fprintf(a.Out, "Planned changes for profile %q:\n", profile)
	} else {
		fmt.Fprintf(a.Out, "Managed links for profile %q:\n", profile)
	}
	fmt.Fprintln(a.Out)

	table := newTable(a.Out)
	tableRow(table, "GROUP", "NATIVE PATH", "ROLE", "AFTER")
	for _, group := range state.Groups {
		for _, native := range group.NativePaths {
			tableRow(table, group.ID, a.display(native.Path), native.Role, a.displayLinkTarget(a.expectedTarget(group, native)))
		}
	}
	_ = table.Flush()

	if len(assets.Assets) == 0 {
		return
	}
	fmt.Fprintln(a.Out)
	assetTable := newTable(a.Out)
	tableRow(assetTable, "ASSET", "NATIVE PATH", "AFTER")
	for _, asset := range assets.Assets {
		tableRow(assetTable, asset.ID, a.display(asset.NativePath), a.display(a.currentAssetPath(asset)))
	}
	_ = assetTable.Flush()
}

func (a *App) displayLinkTarget(target string) string {
	if filepath.IsAbs(target) {
		return a.display(target)
	}
	return target
}
