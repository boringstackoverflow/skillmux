package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func (a *App) CreateBackup(reason string, paths []string) (string, error) {
	id := timestampID(reason)
	root := filepath.Join(a.backupsPath(), id)
	filesRoot := filepath.Join(root, "files")
	if err := ensureDir(filesRoot); err != nil {
		return "", err
	}
	manifest := BackupManifest{
		Version:   1,
		ID:        id,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Reason:    reason,
	}
	paths = uniqueStrings(paths)
	for i, path := range paths {
		entry := BackupEntry{Path: path}
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				entry.Kind = "absent"
				manifest.Entries = append(manifest.Entries, entry)
				continue
			}
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return "", err
			}
			entry.Kind = "symlink"
			entry.Target = target
			manifest.Entries = append(manifest.Entries, entry)
			continue
		}
		entry.BackupPath = filepath.ToSlash(filepath.Join("files", fmt.Sprintf("%03d", i)))
		if info.IsDir() {
			entry.Kind = "directory"
		} else {
			entry.Kind = "file"
		}
		if err := copyPath(path, filepath.Join(root, filepath.FromSlash(entry.BackupPath))); err != nil {
			return "", err
		}
		manifest.Entries = append(manifest.Entries, entry)
	}
	if err := writeTOML(filepath.Join(root, "manifest.toml"), manifest); err != nil {
		return "", err
	}
	return id, nil
}

func (a *App) RestoreBackup(id string) error {
	root := filepath.Join(a.backupsPath(), id)
	if id == "latest" {
		var err error
		root, err = latestDir(a.backupsPath())
		if err != nil {
			return err
		}
	}
	var manifest BackupManifest
	if err := readTOML(filepath.Join(root, "manifest.toml"), &manifest); err != nil {
		return err
	}
	for _, entry := range manifest.Entries {
		if err := restoreEntry(root, entry); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) LatestBackupIDByReason(reason string) (string, error) {
	entries, err := os.ReadDir(a.backupsPath())
	if err != nil {
		return "", err
	}
	var matches []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var manifest BackupManifest
		if err := readTOML(filepath.Join(a.backupsPath(), entry.Name(), "manifest.toml"), &manifest); err != nil {
			continue
		}
		if manifest.Reason == reason {
			matches = append(matches, entry.Name())
		}
	}
	if len(matches) == 0 {
		return "", os.ErrNotExist
	}
	sort.Strings(matches)
	return matches[len(matches)-1], nil
}

func (a *App) ListBackups() ([]BackupInfo, error) {
	entries, err := os.ReadDir(a.backupsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var backups []BackupInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var manifest BackupManifest
		if err := readTOML(filepath.Join(a.backupsPath(), entry.Name(), "manifest.toml"), &manifest); err != nil {
			continue
		}
		id := manifest.ID
		if id == "" {
			id = entry.Name()
		}
		backups = append(backups, BackupInfo{
			ID:        id,
			CreatedAt: manifest.CreatedAt,
			Reason:    manifest.Reason,
			Entries:   len(manifest.Entries),
		})
	}
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].ID < backups[j].ID
	})
	return backups, nil
}

func (a *App) BackupList() error {
	backups, err := a.ListBackups()
	if err != nil {
		return err
	}
	table := newTable(a.Out)
	defer table.Flush()
	tableRow(table, "BACKUP ID", "REASON", "CREATED", "ENTRIES")
	for _, backup := range backups {
		tableRow(table, backup.ID, backup.Reason, backup.CreatedAt, fmt.Sprintf("%d", backup.Entries))
	}
	return nil
}

func restoreEntry(root string, entry BackupEntry) error {
	if err := ensureDir(filepath.Dir(entry.Path)); err != nil {
		return err
	}
	if err := removePath(entry.Path); err != nil {
		return err
	}
	switch entry.Kind {
	case "absent":
		return nil
	case "symlink":
		return os.Symlink(entry.Target, entry.Path)
	case "directory", "file":
		return copyPath(filepath.Join(root, filepath.FromSlash(entry.BackupPath)), entry.Path)
	default:
		return fmt.Errorf("unknown backup entry kind %q for %s", entry.Kind, entry.Path)
	}
}

func (a *App) managedNativePaths(state RootGroupsState) []string {
	var paths []string
	for _, group := range state.Groups {
		for _, native := range group.NativePaths {
			paths = append(paths, native.Path)
		}
	}
	return uniqueStrings(paths)
}

func (a *App) BackupManaged(reason string) (string, error) {
	state, err := a.loadRootGroups()
	if err != nil {
		paths := []string{}
		for _, c := range a.inspectCandidates() {
			paths = append(paths, c.Path)
		}
		assets := a.DiscoverAssets()
		paths = append(paths, assetNativePaths(assets.Assets)...)
		return a.CreateBackup(reason, paths)
	}
	paths := a.managedNativePaths(state)
	if assets, err := a.loadAssets(); err == nil {
		paths = append(paths, assetNativePaths(assets.Assets)...)
	}
	return a.CreateBackup(reason, paths)
}
