package app

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	AgentClaude = "claude"
	AgentCodex  = "codex"
	AgentAgents = "agents"
)

var AgentOrder = []string{AgentClaude, AgentCodex, AgentAgents}

func SupportedAgents() []string {
	return append([]string(nil), AgentOrder...)
}

func RunnableAgents() []string {
	return []string{AgentClaude, AgentCodex}
}

func IsSupportedAgent(agent string) bool {
	if agent == "" {
		return true
	}
	return contains(AgentOrder, agent)
}

func IsRunnableAgent(agent string) bool {
	return agent == AgentClaude || agent == AgentCodex
}

type App struct {
	Home         string
	SkillmuxHome string
	Out          io.Writer
	Err          io.Writer
}

func New(home string, out, err io.Writer) (*App, error) {
	if home == "" {
		var e error
		home, e = os.UserHomeDir()
		if e != nil {
			return nil, e
		}
	}
	return &App{
		Home:         home,
		SkillmuxHome: filepath.Join(home, ".skillmux"),
		Out:          out,
		Err:          err,
	}, nil
}

type Candidate struct {
	Agent          string
	Path           string
	DisplayPath    string
	Kind           string
	Resolved       string
	OriginalTarget string
	Error          string
}

type RootGroupsState struct {
	Version int         `toml:"version"`
	Groups  []RootGroup `toml:"groups"`
}

type AssetState struct {
	Version int             `toml:"version"`
	Assets  []AssetResource `toml:"assets"`
}

type AssetResource struct {
	ID           string `toml:"id"`
	Agent        string `toml:"agent"`
	Kind         string `toml:"kind"`
	NativePath   string `toml:"native_path"`
	ProfilePath  string `toml:"profile_path"`
	OriginalKind string `toml:"original_kind"`
}

type RootGroup struct {
	ID          string       `toml:"id"`
	Kind        string       `toml:"kind"`
	ProfilePath string       `toml:"profile_path"`
	Agents      []string     `toml:"agents"`
	NativePaths []NativePath `toml:"native_paths"`
}

type NativePath struct {
	Path           string `toml:"path"`
	Agent          string `toml:"agent"`
	Role           string `toml:"role"`
	OriginalKind   string `toml:"original_kind"`
	OriginalTarget string `toml:"original_target,omitempty"`
}

type ActiveState struct {
	Version int           `toml:"version"`
	Agents  []ActiveAgent `toml:"agents"`
}

type ActiveAgent struct {
	Agent   string   `toml:"agent"`
	Profile string   `toml:"profile"`
	Groups  []string `toml:"groups"`
}

type BackupManifest struct {
	Version   int           `toml:"version"`
	ID        string        `toml:"id"`
	CreatedAt string        `toml:"created_at"`
	Reason    string        `toml:"reason"`
	Entries   []BackupEntry `toml:"entries"`
}

type BackupEntry struct {
	Path       string `toml:"path"`
	Kind       string `toml:"kind"`
	Target     string `toml:"target,omitempty"`
	BackupPath string `toml:"backup_path,omitempty"`
}

type BackupInfo struct {
	ID        string
	CreatedAt string
	Reason    string
	Entries   int
}

type ProjectConfig struct {
	Profile string   `toml:"profile"`
	Agents  []string `toml:"agents"`
}

func (a *App) expand(path string) string {
	if path == "~" {
		return a.Home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(a.Home, strings.TrimPrefix(path, "~/"))
	}
	return path
}

func (a *App) display(path string) string {
	clean := filepath.Clean(path)
	home := filepath.Clean(a.Home)
	if clean == home {
		return "~"
	}
	if strings.HasPrefix(clean, home+string(os.PathSeparator)) {
		return "~/" + strings.TrimPrefix(clean, home+string(os.PathSeparator))
	}
	return clean
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
