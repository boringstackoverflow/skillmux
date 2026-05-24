package app

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

func (a *App) rootGroupsPath() string {
	return filepath.Join(a.SkillmuxHome, "state", "root_groups.toml")
}

func (a *App) activePath() string {
	return filepath.Join(a.SkillmuxHome, "state", "active.toml")
}

func (a *App) assetsPath() string {
	return filepath.Join(a.SkillmuxHome, "state", "assets.toml")
}

func (a *App) backupsPath() string {
	return filepath.Join(a.SkillmuxHome, "backups")
}

func writeTOML(path string, value any) error {
	data, err := toml.Marshal(value)
	if err != nil {
		return err
	}
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func readTOML(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return toml.Unmarshal(data, value)
}

func (a *App) saveRootGroups(state RootGroupsState) error {
	return writeTOML(a.rootGroupsPath(), state)
}

func (a *App) loadRootGroups() (RootGroupsState, error) {
	var state RootGroupsState
	err := readTOML(a.rootGroupsPath(), &state)
	return state, err
}

func (a *App) saveAssets(state AssetState) error {
	return writeTOML(a.assetsPath(), state)
}

func (a *App) loadAssets() (AssetState, error) {
	var state AssetState
	err := readTOML(a.assetsPath(), &state)
	return state, err
}

func (a *App) saveActive(state ActiveState) error {
	return writeTOML(a.activePath(), state)
}

func (a *App) loadActive() (ActiveState, error) {
	var state ActiveState
	err := readTOML(a.activePath(), &state)
	return state, err
}

func (a *App) profileRoot(profile string) string {
	return filepath.Join(a.SkillmuxHome, "profiles", profile)
}

func (a *App) groupProfileRoot(profile string, group RootGroup) string {
	return filepath.Join(a.profileRoot(profile), filepath.FromSlash(group.ProfilePath))
}

func (a *App) groupSkillsPath(profile string, group RootGroup) string {
	return filepath.Join(a.groupProfileRoot(profile, group), "skills")
}

func (a *App) currentGroupPath(group RootGroup) string {
	return filepath.Join(a.SkillmuxHome, "current", "roots", group.ID)
}

func (a *App) currentGroupSkillsPath(group RootGroup) string {
	return filepath.Join(a.currentGroupPath(group), "skills")
}

func (a *App) assetProfilePath(profile string, asset AssetResource) string {
	return filepath.Join(a.profileRoot(profile), filepath.FromSlash(asset.ProfilePath))
}

func (a *App) currentAssetPath(asset AssetResource) string {
	return filepath.Join(a.SkillmuxHome, "current", "assets", asset.ID)
}

func (a *App) activeProfileForAgent(active ActiveState, agent string) string {
	for _, entry := range active.Agents {
		if entry.Agent == agent {
			return entry.Profile
		}
	}
	return ""
}
