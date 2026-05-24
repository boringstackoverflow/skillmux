package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (a *App) Scan(profile, agent string) error {
	state, err := a.loadRootGroups()
	if err != nil {
		return err
	}
	if !IsSupportedAgent(agent) {
		return fmt.Errorf("unsupported agent %q; supported agents: %s", agent, strings.Join(AgentOrder, ", "))
	}
	if profile == "" {
		active, _ := a.loadActive()
		if agent != "" {
			profile = a.activeProfileForAgent(active, agent)
		}
		if profile == "" && len(active.Agents) > 0 {
			profile = active.Agents[0].Profile
		}
	}
	if profile == "" {
		return fmt.Errorf("profile is required when no active profile is recorded")
	}
	if err := a.requireProfile(profile); err != nil {
		return err
	}
	for _, group := range groupsForAgent(state.Groups, agent) {
		root := a.groupSkillsPath(profile, group)
		fmt.Fprintf(a.Out, "\n[%s] %s\n", group.ID, a.display(root))
		if err := scanSkills(a.Out, root); err != nil {
			return err
		}
	}
	return nil
}

func scanSkills(out io.Writer, root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(out, "missing skills directory")
			return nil
		}
		return err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") && name != ".system" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		fmt.Fprintln(out, "no skills")
		return nil
	}
	table := newTable(out)
	defer table.Flush()
	for _, name := range names {
		skillPath := filepath.Join(root, name)
		status := "ok"
		if _, err := os.Stat(filepath.Join(skillPath, "SKILL.md")); err != nil {
			status = "warning: missing SKILL.md"
		}
		tableRow(table, name, status)
	}
	return nil
}

func (a *App) Doctor() error {
	state, err := a.loadRootGroups()
	if err != nil {
		candidates, discovered, derr := a.Discover()
		if derr != nil {
			return derr
		}
		fmt.Fprintln(a.Out, "Skillmux is not initialized.")
		a.PrintDiscovery(candidates, discovered)
		return nil
	}
	fmt.Fprintln(a.Out, "Skillmux doctor")
	problems := 0
	for _, group := range state.Groups {
		if _, err := os.Lstat(a.currentGroupPath(group)); err != nil {
			fmt.Fprintf(a.Out, "repairable\tmissing current pointer\t%s\n", group.ID)
			problems++
		} else {
			fmt.Fprintf(a.Out, "ok\tcurrent pointer\t%s\n", group.ID)
		}
		for _, native := range group.NativePaths {
			info, err := os.Lstat(native.Path)
			if err != nil {
				fmt.Fprintf(a.Out, "repairable\tmissing native path\t%s\n", a.display(native.Path))
				problems++
				continue
			}
			if info.Mode()&os.ModeSymlink == 0 {
				fmt.Fprintf(a.Out, "repairable\tnative path is not symlink\t%s\n", a.display(native.Path))
				problems++
				continue
			}
			target, err := os.Readlink(native.Path)
			if err != nil {
				fmt.Fprintf(a.Out, "repairable\tcannot read symlink\t%s\n", a.display(native.Path))
				problems++
				continue
			}
			if target != a.expectedTarget(group, native) {
				fmt.Fprintf(a.Out, "repairable\tunexpected symlink target\t%s -> %s\n", a.display(native.Path), target)
				problems++
				continue
			}
			fmt.Fprintf(a.Out, "ok\tnative path\t%s\n", a.display(native.Path))
		}
	}
	if assets, err := a.loadAssets(); err == nil {
		for _, asset := range assets.Assets {
			info, err := os.Lstat(asset.NativePath)
			if err != nil {
				fmt.Fprintf(a.Out, "repairable\tmissing asset path\t%s\n", a.display(asset.NativePath))
				problems++
				continue
			}
			if info.Mode()&os.ModeSymlink == 0 {
				fmt.Fprintf(a.Out, "repairable\tasset path is not symlink\t%s\n", a.display(asset.NativePath))
				problems++
				continue
			}
			target, err := os.Readlink(asset.NativePath)
			if err != nil || target != a.currentAssetPath(asset) {
				fmt.Fprintf(a.Out, "repairable\tunexpected asset target\t%s\n", a.display(asset.NativePath))
				problems++
				continue
			}
			fmt.Fprintf(a.Out, "ok\tasset path\t%s\n", a.display(asset.NativePath))
		}
	}
	if problems > 0 {
		fmt.Fprintf(a.Out, "\nRun `skillmux repair` to relink managed paths.\n")
	}
	return nil
}

func (a *App) Enter(start string) error {
	return a.EnterProfile(start, false)
}

func (a *App) EnterProfile(start string, create bool) error {
	configPath, err := findProjectConfig(start)
	if err != nil {
		return err
	}
	var cfg ProjectConfig
	if err := readTOML(configPath, &cfg); err != nil {
		return err
	}
	if cfg.Profile == "" {
		return fmt.Errorf("%s does not define profile", configPath)
	}
	if len(cfg.Agents) == 0 {
		return a.UseProfile(cfg.Profile, "", create)
	}
	for _, agent := range cfg.Agents {
		if err := a.UseProfile(cfg.Profile, agent, create); err != nil {
			return err
		}
	}
	return nil
}

func findProjectConfig(start string) (string, error) {
	if start == "" {
		var err error
		start, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, ".skillmux.toml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no .skillmux.toml found")
		}
		dir = parent
	}
}
