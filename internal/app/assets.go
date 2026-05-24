package app

import (
	"os"
	"path/filepath"
)

func (a *App) DiscoverAssets() AssetState {
	candidates := []AssetResource{
		{
			ID:          "claude-commands",
			Agent:       AgentClaude,
			Kind:        "directory",
			NativePath:  filepath.Join(a.Home, ".claude", "commands"),
			ProfilePath: filepath.ToSlash(filepath.Join("assets", AgentClaude, "commands")),
		},
		{
			ID:          "codex-rules",
			Agent:       AgentCodex,
			Kind:        "directory",
			NativePath:  filepath.Join(a.Home, ".codex", "rules"),
			ProfilePath: filepath.ToSlash(filepath.Join("assets", AgentCodex, "rules")),
		},
		{
			ID:          "codex-instructions",
			Agent:       AgentCodex,
			Kind:        "file",
			NativePath:  filepath.Join(a.Home, ".codex", "instructions.md"),
			ProfilePath: filepath.ToSlash(filepath.Join("assets", AgentCodex, "instructions.md")),
		},
	}
	state := AssetState{Version: 1}
	for _, asset := range candidates {
		info, err := os.Lstat(asset.NativePath)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			asset.OriginalKind = "symlink"
		} else if info.IsDir() {
			asset.OriginalKind = "directory"
		} else {
			asset.OriginalKind = "file"
		}
		state.Assets = append(state.Assets, asset)
	}
	return state
}

func (a *App) createAssetProfilePaths(profile string, assets []AssetResource) error {
	for _, asset := range assets {
		path := a.assetProfilePath(profile, asset)
		if asset.Kind == "directory" {
			if err := ensureDir(path); err != nil {
				return err
			}
			continue
		}
		if err := ensureDir(filepath.Dir(path)); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) importAssets(profile string, assets []AssetResource) error {
	for _, asset := range assets {
		if _, err := os.Stat(asset.NativePath); err != nil {
			continue
		}
		dst := a.assetProfilePath(profile, asset)
		if err := removePath(dst); err != nil {
			return err
		}
		if err := copyPath(asset.NativePath, dst); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) switchAssets(profile string, assets []AssetResource, alreadyBackedUp bool) error {
	if len(assets) == 0 {
		return nil
	}
	if err := a.createAssetProfilePaths(profile, assets); err != nil {
		return err
	}
	if !alreadyBackedUp {
		if err := a.backupUnexpectedAssets(assets, "pre-switch-assets"); err != nil {
			return err
		}
	}
	for _, asset := range assets {
		target := a.assetProfilePath(profile, asset)
		if err := replaceSymlink(a.currentAssetPath(asset), target); err != nil {
			return err
		}
		if err := replaceSymlink(asset.NativePath, a.currentAssetPath(asset)); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) backupUnexpectedAssets(assets []AssetResource, reason string) error {
	var paths []string
	for _, asset := range assets {
		info, err := os.Lstat(asset.NativePath)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			paths = append(paths, asset.NativePath)
			continue
		}
		target, err := os.Readlink(asset.NativePath)
		if err != nil || target != a.currentAssetPath(asset) {
			paths = append(paths, asset.NativePath)
		}
	}
	if len(paths) == 0 {
		return nil
	}
	_, err := a.CreateBackup(reason, paths)
	return err
}

func assetsForAgent(assets []AssetResource, agent string) []AssetResource {
	if agent == "" {
		return assets
	}
	var out []AssetResource
	for _, asset := range assets {
		if asset.Agent == agent {
			out = append(out, asset)
		}
	}
	return out
}

func assetNativePaths(assets []AssetResource) []string {
	var paths []string
	for _, asset := range assets {
		paths = append(paths, asset.NativePath)
	}
	return uniqueStrings(paths)
}
