package app

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ProfileSnapshot struct {
	Version   int                   `json:"version"`
	Profile   string                `json:"profile"`
	CreatedAt string                `json:"created_at"`
	Digest    string                `json:"digest"`
	Files     []ProfileSnapshotFile `json:"files"`
}

type ProfileSnapshotFile struct {
	Path    string `json:"path"`
	Type    string `json:"type"`
	Mode    int64  `json:"mode,omitempty"`
	Content string `json:"content,omitempty"`
	Target  string `json:"target,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
}

type ProfileDiff struct {
	Added    []string
	Modified []string
	Removed  []string
}

type CloudProfilesState struct {
	Version  int                `toml:"version"`
	Profiles []CloudProfileLink `toml:"profiles"`
}

type CloudProfileLink struct {
	Org           string `toml:"org"`
	RemoteProfile string `toml:"remote_profile"`
	LocalProfile  string `toml:"local_profile"`
	Version       string `toml:"version"`
}

func (a *App) cloudProfilesPath() string {
	return filepath.Join(a.SkillmuxHome, "state", "cloud_profiles.toml")
}

func (a *App) loadCloudProfiles() (CloudProfilesState, error) {
	var state CloudProfilesState
	err := readTOML(a.cloudProfilesPath(), &state)
	return state, err
}

func (a *App) saveCloudProfiles(state CloudProfilesState) error {
	state.Version = 1
	sort.Slice(state.Profiles, func(i, j int) bool {
		left := state.Profiles[i].Org + "/" + state.Profiles[i].RemoteProfile + "/" + state.Profiles[i].LocalProfile
		right := state.Profiles[j].Org + "/" + state.Profiles[j].RemoteProfile + "/" + state.Profiles[j].LocalProfile
		return left < right
	})
	return writeTOML(a.cloudProfilesPath(), state)
}

func (a *App) recordCloudProfile(link CloudProfileLink) error {
	state, _ := a.loadCloudProfiles()
	state.Version = 1
	replaced := false
	for i, existing := range state.Profiles {
		if existing.Org == link.Org && existing.RemoteProfile == link.RemoteProfile && existing.LocalProfile == link.LocalProfile {
			state.Profiles[i] = link
			replaced = true
			break
		}
	}
	if !replaced {
		state.Profiles = append(state.Profiles, link)
	}
	return a.saveCloudProfiles(state)
}

func (a *App) BuildProfileSnapshot(profile string) (ProfileSnapshot, error) {
	if err := a.requireProfile(profile); err != nil {
		return ProfileSnapshot{}, err
	}
	root := a.profileRoot(profile)
	snapshot := ProfileSnapshot{
		Version:   1,
		Profile:   profile,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		name := d.Name()
		if name == ".DS_Store" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if shouldSkipSnapshotPath(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		file := ProfileSnapshotFile{
			Path: rel,
			Mode: int64(info.Mode().Perm()),
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			file.Type = "symlink"
			file.Target = target
			sum := sha256.Sum256([]byte(target))
			file.SHA256 = hex.EncodeToString(sum[:])
		} else {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			file.Type = "file"
			file.Content = base64.StdEncoding.EncodeToString(data)
			sum := sha256.Sum256(data)
			file.SHA256 = hex.EncodeToString(sum[:])
		}
		snapshot.Files = append(snapshot.Files, file)
		return nil
	})
	if err != nil {
		return ProfileSnapshot{}, err
	}
	sort.Slice(snapshot.Files, func(i, j int) bool { return snapshot.Files[i].Path < snapshot.Files[j].Path })
	snapshot.Digest = digestSnapshot(snapshot)
	return snapshot, nil
}

func shouldSkipSnapshotPath(path string) bool {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, part := range parts {
		if part == ".system" {
			return true
		}
	}
	return false
}

func digestSnapshot(snapshot ProfileSnapshot) string {
	h := sha256.New()
	for _, file := range snapshot.Files {
		_, _ = h.Write([]byte(file.Path))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(file.Type))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(fmt.Sprint(file.Mode)))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(file.SHA256))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(file.Target))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (a *App) DiffProfileSnapshot(profile string, remote ProfileSnapshot) (ProfileDiff, error) {
	local, err := a.BuildProfileSnapshot(profile)
	if err != nil {
		if !strings.Contains(err.Error(), "does not exist") {
			return ProfileDiff{}, err
		}
		local = ProfileSnapshot{Profile: profile}
	}
	localByPath := snapshotFileMap(local)
	remoteByPath := snapshotFileMap(remote)
	var diff ProfileDiff
	for path, remoteFile := range remoteByPath {
		localFile, ok := localByPath[path]
		if !ok {
			diff.Added = append(diff.Added, path)
			continue
		}
		if localFile.Type != remoteFile.Type || localFile.SHA256 != remoteFile.SHA256 || localFile.Target != remoteFile.Target {
			diff.Modified = append(diff.Modified, path)
		}
	}
	for path := range localByPath {
		if _, ok := remoteByPath[path]; !ok {
			diff.Removed = append(diff.Removed, path)
		}
	}
	sort.Strings(diff.Added)
	sort.Strings(diff.Modified)
	sort.Strings(diff.Removed)
	return diff, nil
}

func snapshotFileMap(snapshot ProfileSnapshot) map[string]ProfileSnapshotFile {
	out := map[string]ProfileSnapshotFile{}
	for _, file := range snapshot.Files {
		out[file.Path] = file
	}
	return out
}

func (d ProfileDiff) Empty() bool {
	return len(d.Added) == 0 && len(d.Modified) == 0 && len(d.Removed) == 0
}

func (a *App) PrintProfileDiff(diff ProfileDiff) {
	if diff.Empty() {
		fmt.Fprintln(a.Out, "No changes.")
		return
	}
	for _, path := range diff.Added {
		fmt.Fprintf(a.Out, "  + %s\n", path)
	}
	for _, path := range diff.Modified {
		fmt.Fprintf(a.Out, "  ~ %s\n", path)
	}
	for _, path := range diff.Removed {
		fmt.Fprintf(a.Out, "  - %s\n", path)
	}
}

func (a *App) ApplyProfileSnapshot(profile string, snapshot ProfileSnapshot) error {
	if _, err := a.requireInitialized(); err != nil {
		return err
	}
	if err := validateProfileName(profile); err != nil {
		return err
	}
	active, _ := a.loadActive()
	for _, entry := range active.Agents {
		if entry.Profile == profile {
			return fmt.Errorf("profile %q is active for %s; switch away or pull into an inactive profile so native agent roots are not changed", profile, entry.Agent)
		}
	}
	if err := validateSnapshot(snapshot); err != nil {
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
	profilesRoot := filepath.Join(a.SkillmuxHome, "profiles")
	if err := ensureDir(profilesRoot); err != nil {
		return err
	}
	tmp := filepath.Join(profilesRoot, ".tmp-"+timestampID("profile-pull"))
	if err := writeSnapshotToDir(tmp, snapshot); err != nil {
		_ = removePath(tmp)
		return err
	}
	if err := a.createProfileDirsInRoot(tmp, state.Groups); err != nil {
		_ = removePath(tmp)
		return err
	}
	assets, _ := a.loadAssets()
	if err := a.createAssetProfilePathsInRoot(tmp, assets.Assets); err != nil {
		_ = removePath(tmp)
		return err
	}
	target := a.profileRoot(profile)
	if _, err := os.Lstat(target); err == nil {
		backup := filepath.Join(a.SkillmuxHome, "profile-backups", timestampID("profile-"+profile))
		if err := copyPath(target, backup); err != nil {
			_ = removePath(tmp)
			return fmt.Errorf("backup existing profile: %w", err)
		}
	} else if !os.IsNotExist(err) {
		_ = removePath(tmp)
		return err
	}
	if err := removePath(target); err != nil {
		_ = removePath(tmp)
		return err
	}
	return os.Rename(tmp, target)
}

func validateSnapshot(snapshot ProfileSnapshot) error {
	if snapshot.Version != 1 {
		return fmt.Errorf("unsupported profile snapshot version %d", snapshot.Version)
	}
	if snapshot.Digest != "" && snapshot.Digest != digestSnapshot(snapshot) {
		return fmt.Errorf("profile snapshot digest mismatch")
	}
	seen := map[string]bool{}
	for _, file := range snapshot.Files {
		if err := validateSnapshotPath(file.Path); err != nil {
			return err
		}
		if seen[file.Path] {
			return fmt.Errorf("duplicate snapshot path %q", file.Path)
		}
		seen[file.Path] = true
		switch file.Type {
		case "file":
			data, err := base64.StdEncoding.DecodeString(file.Content)
			if err != nil {
				return fmt.Errorf("decode %s: %w", file.Path, err)
			}
			sum := sha256.Sum256(data)
			if file.SHA256 != "" && file.SHA256 != hex.EncodeToString(sum[:]) {
				return fmt.Errorf("snapshot file digest mismatch for %s", file.Path)
			}
		case "symlink":
			if file.Target == "" {
				return fmt.Errorf("snapshot symlink %s has empty target", file.Path)
			}
		default:
			return fmt.Errorf("unsupported snapshot file type %q for %s", file.Type, file.Path)
		}
	}
	return nil
}

func validateSnapshotPath(path string) error {
	if path == "" {
		return fmt.Errorf("snapshot path is empty")
	}
	if filepath.IsAbs(path) {
		return fmt.Errorf("snapshot path %q is absolute", path)
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == "." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) || clean == ".." {
		return fmt.Errorf("snapshot path %q escapes profile", path)
	}
	if shouldSkipSnapshotPath(path) {
		return fmt.Errorf("snapshot path %q is reserved", path)
	}
	return nil
}

func writeSnapshotToDir(root string, snapshot ProfileSnapshot) error {
	for _, file := range snapshot.Files {
		rel := filepath.FromSlash(file.Path)
		dst := filepath.Join(root, rel)
		if err := ensureDir(filepath.Dir(dst)); err != nil {
			return err
		}
		switch file.Type {
		case "file":
			data, err := base64.StdEncoding.DecodeString(file.Content)
			if err != nil {
				return err
			}
			mode := fs.FileMode(file.Mode)
			if mode == 0 {
				mode = 0o644
			}
			if err := os.WriteFile(dst, data, mode.Perm()); err != nil {
				return err
			}
		case "symlink":
			if err := os.Symlink(file.Target, dst); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported snapshot file type %q", file.Type)
		}
	}
	return nil
}

func (a *App) createProfileDirsInRoot(root string, groups []RootGroup) error {
	for _, group := range groups {
		if err := ensureDir(filepath.Join(root, filepath.FromSlash(group.ProfilePath), "skills")); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) createAssetProfilePathsInRoot(root string, assets []AssetResource) error {
	for _, asset := range assets {
		path := filepath.Join(root, filepath.FromSlash(asset.ProfilePath))
		if strings.HasSuffix(asset.ProfilePath, "/") {
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

func EncodeSnapshot(snapshot ProfileSnapshot) ([]byte, error) {
	return json.Marshal(snapshot)
}

func DecodeSnapshot(data []byte) (ProfileSnapshot, error) {
	var snapshot ProfileSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return ProfileSnapshot{}, err
	}
	if err := validateSnapshot(snapshot); err != nil {
		return ProfileSnapshot{}, err
	}
	return snapshot, nil
}
