package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const defaultCloudURL = "https://cloud.skillmux.dev"

type CloudState struct {
	Version    int    `toml:"version"`
	BaseURL    string `toml:"base_url"`
	Token      string `toml:"token"`
	CurrentOrg string `toml:"current_org,omitempty"`
}

type CloudOrg struct {
	Name string `json:"name"`
}

type CloudProfileVersion struct {
	Version     string `json:"version"`
	CreatedAt   string `json:"created_at"`
	Message     string `json:"message,omitempty"`
	Digest      string `json:"digest"`
	Files       int    `json:"files"`
	Recommended bool   `json:"recommended,omitempty"`
}

type cloudClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func (a *App) cloudStatePath() string {
	return filepath.Join(a.SkillmuxHome, "state", "cloud.toml")
}

func (a *App) saveCloudState(state CloudState) error {
	state.Version = 1
	data, err := toml.Marshal(state)
	if err != nil {
		return err
	}
	if err := ensureDir(filepath.Dir(a.cloudStatePath())); err != nil {
		return err
	}
	return os.WriteFile(a.cloudStatePath(), data, 0o600)
}

func (a *App) loadCloudState() (CloudState, error) {
	var state CloudState
	err := readTOML(a.cloudStatePath(), &state)
	return state, err
}

func (a *App) resolvedCloudURL(state CloudState) string {
	if a.CloudURL != "" {
		return strings.TrimRight(a.CloudURL, "/")
	}
	if env := os.Getenv("SKILLMUX_CLOUD_URL"); env != "" {
		return strings.TrimRight(env, "/")
	}
	if state.BaseURL != "" {
		return strings.TrimRight(state.BaseURL, "/")
	}
	return defaultCloudURL
}

func (a *App) newCloudClient() (*cloudClient, CloudState, error) {
	state, err := a.loadCloudState()
	if err != nil {
		return nil, CloudState{}, fmt.Errorf("not logged in; run `skillmux login`")
	}
	if state.Token == "" {
		return nil, CloudState{}, fmt.Errorf("not logged in; run `skillmux login`")
	}
	state.BaseURL = a.resolvedCloudURL(state)
	return &cloudClient{
		baseURL: state.BaseURL,
		token:   state.Token,
		client:  &http.Client{Timeout: 15 * time.Second},
	}, state, nil
}

func (c *cloudClient) do(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Error != "" {
			if resp.StatusCode == http.StatusUnauthorized {
				return fmt.Errorf("%s; run `skillmux login`", apiErr.Error)
			}
			return fmt.Errorf("%s", apiErr.Error)
		}
		return fmt.Errorf("cloud request failed: %s", resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

func escapePath(value string) string {
	return url.PathEscape(value)
}

func (a *App) CloudLogin(email, token string) error {
	state, _ := a.loadCloudState()
	state.BaseURL = a.resolvedCloudURL(state)
	if token == "" {
		if email == "" {
			return fmt.Errorf("--email or --token is required")
		}
		client := &cloudClient{baseURL: state.BaseURL, client: &http.Client{Timeout: 15 * time.Second}}
		var resp struct {
			Token   string `json:"token"`
			Message string `json:"message"`
		}
		if err := client.do(http.MethodPost, "/api/v1/auth/magic-link", map[string]string{"email": email}, &resp); err != nil {
			return err
		}
		token = resp.Token
		if resp.Message != "" {
			fmt.Fprintln(a.Out, resp.Message)
		}
	}
	state.Token = token
	if err := a.saveCloudState(state); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Logged in to %s\n", state.BaseURL)
	return nil
}

func (a *App) CloudLogout() error {
	if err := removePath(a.cloudStatePath()); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, "Logged out")
	return nil
}

func (a *App) CloudAuthStatus() error {
	state, err := a.loadCloudState()
	if err != nil || state.Token == "" {
		fmt.Fprintln(a.Out, "Not logged in")
		return nil
	}
	fmt.Fprintf(a.Out, "Logged in to %s\n", a.resolvedCloudURL(state))
	if state.CurrentOrg != "" {
		fmt.Fprintf(a.Out, "Current org: %s\n", state.CurrentOrg)
	}
	return nil
}

func (a *App) CloudOrgCreate(name string) error {
	if err := validateOrgName(name); err != nil {
		return err
	}
	client, state, err := a.newCloudClient()
	if err != nil {
		return err
	}
	var resp struct {
		Org CloudOrg `json:"org"`
	}
	if err := client.do(http.MethodPost, "/api/v1/orgs", map[string]string{"name": name}, &resp); err != nil {
		return err
	}
	state.CurrentOrg = resp.Org.Name
	if state.CurrentOrg == "" {
		state.CurrentOrg = name
	}
	if err := a.saveCloudState(state); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Created org %q\n", state.CurrentOrg)
	fmt.Fprintf(a.Out, "Current org: %s\n", state.CurrentOrg)
	return nil
}

func (a *App) CloudOrgInvite(org, email string) error {
	if err := validateOrgName(org); err != nil {
		return err
	}
	client, _, err := a.newCloudClient()
	if err != nil {
		return err
	}
	var resp struct {
		Code  string `json:"code"`
		Email string `json:"email,omitempty"`
	}
	path := fmt.Sprintf("/api/v1/orgs/%s/invites", escapePath(org))
	if err := client.do(http.MethodPost, path, map[string]string{"email": email}, &resp); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Invite code for %s: %s\n", org, resp.Code)
	if resp.Email != "" {
		fmt.Fprintf(a.Out, "Email: %s\n", resp.Email)
	}
	return nil
}

func (a *App) CloudOrgJoin(name, code string) error {
	if err := validateOrgName(name); err != nil {
		return err
	}
	if code == "" {
		return fmt.Errorf("--code is required")
	}
	client, state, err := a.newCloudClient()
	if err != nil {
		return err
	}
	var resp struct {
		Org CloudOrg `json:"org"`
	}
	path := fmt.Sprintf("/api/v1/orgs/%s/join", escapePath(name))
	if err := client.do(http.MethodPost, path, map[string]string{"code": code}, &resp); err != nil {
		return err
	}
	state.CurrentOrg = resp.Org.Name
	if state.CurrentOrg == "" {
		state.CurrentOrg = name
	}
	if err := a.saveCloudState(state); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Joined org %q\n", state.CurrentOrg)
	fmt.Fprintf(a.Out, "Current org: %s\n", state.CurrentOrg)
	return nil
}

func (a *App) CloudOrgList() error {
	client, _, err := a.newCloudClient()
	if err != nil {
		return err
	}
	var resp struct {
		Orgs []CloudOrg `json:"orgs"`
	}
	if err := client.do(http.MethodGet, "/api/v1/orgs", nil, &resp); err != nil {
		return err
	}
	if len(resp.Orgs) == 0 {
		fmt.Fprintln(a.Out, "No orgs")
		return nil
	}
	table := newTable(a.Out)
	defer table.Flush()
	tableRow(table, "ORG")
	for _, org := range resp.Orgs {
		tableRow(table, org.Name)
	}
	return nil
}

func (a *App) CloudOrgCurrent() error {
	state, err := a.loadCloudState()
	if err != nil || state.Token == "" {
		return fmt.Errorf("not logged in; run `skillmux login`")
	}
	if state.CurrentOrg == "" {
		fmt.Fprintln(a.Out, "No current org")
		return nil
	}
	fmt.Fprintf(a.Out, "Current org: %s\n", state.CurrentOrg)
	return nil
}

func (a *App) CloudOrgUse(name string) error {
	if err := validateOrgName(name); err != nil {
		return err
	}
	state, err := a.loadCloudState()
	if err != nil || state.Token == "" {
		return fmt.Errorf("not logged in; run `skillmux login`")
	}
	state.CurrentOrg = name
	if err := a.saveCloudState(state); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Current org: %s\n", name)
	return nil
}

func validateOrgName(name string) error {
	if name == "" {
		return fmt.Errorf("org name is required")
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return fmt.Errorf("org name %q must use lowercase letters, numbers, and dashes", name)
	}
	return nil
}

func (a *App) CloudProfilePush(profile, org, message string) error {
	if org == "" {
		state, err := a.loadCloudState()
		if err == nil {
			org = state.CurrentOrg
		}
	}
	if err := validateOrgName(org); err != nil {
		return err
	}
	snapshot, err := a.BuildProfileSnapshot(profile)
	if err != nil {
		return err
	}
	client, _, err := a.newCloudClient()
	if err != nil {
		return err
	}
	var resp struct {
		Version CloudProfileVersion `json:"version"`
	}
	req := struct {
		Message  string          `json:"message"`
		Snapshot ProfileSnapshot `json:"snapshot"`
	}{Message: message, Snapshot: snapshot}
	path := fmt.Sprintf("/api/v1/orgs/%s/profiles/%s/versions", escapePath(org), escapePath(profile))
	if err := client.do(http.MethodPost, path, req, &resp); err != nil {
		return err
	}
	if err := a.recordCloudProfile(CloudProfileLink{
		Org:           org,
		RemoteProfile: profile,
		LocalProfile:  profile,
		Version:       resp.Version.Version,
	}); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Pushed %s/%s %s\n", org, profile, resp.Version.Version)
	fmt.Fprintf(a.Out, "Digest: %s\n", resp.Version.Digest)
	return nil
}

func (a *App) CloudProfileVersions(ref string) error {
	org, profile, err := parseRemoteProfileRef(ref)
	if err != nil {
		return err
	}
	versions, err := a.cloudProfileVersions(org, profile)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		fmt.Fprintf(a.Out, "No versions for %s/%s\n", org, profile)
		return nil
	}
	table := newTable(a.Out)
	defer table.Flush()
	tableRow(table, "VERSION", "FILES", "DIGEST", "CREATED", "MESSAGE")
	for _, version := range versions {
		label := version.Version
		if version.Recommended {
			label += " *"
		}
		tableRow(table, label, strconv.Itoa(version.Files), shortDigest(version.Digest), version.CreatedAt, version.Message)
	}
	return nil
}

func (a *App) CloudProfileDiff(ref, version, localProfile string) error {
	org, remoteProfile, err := parseRemoteProfileRef(ref)
	if err != nil {
		return err
	}
	snapshot, resolvedVersion, err := a.fetchCloudProfileSnapshot(org, remoteProfile, version)
	if err != nil {
		return err
	}
	if localProfile == "" {
		localProfile = remoteProfile
	}
	diff, err := a.DiffProfileSnapshot(localProfile, snapshot)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Remote profile: %s/%s\n", org, remoteProfile)
	fmt.Fprintf(a.Out, "Remote version: %s\n", resolvedVersion.Version)
	fmt.Fprintf(a.Out, "Local profile: %s\n\n", localProfile)
	a.PrintProfileDiff(diff)
	return nil
}

func (a *App) CloudProfilePull(ref, version, localProfile string, apply bool) error {
	org, remoteProfile, err := parseRemoteProfileRef(ref)
	if err != nil {
		return err
	}
	snapshot, resolvedVersion, err := a.fetchCloudProfileSnapshot(org, remoteProfile, version)
	if err != nil {
		return err
	}
	if localProfile == "" {
		localProfile = remoteProfile
	}
	diff, err := a.DiffProfileSnapshot(localProfile, snapshot)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Remote profile: %s/%s\n", org, remoteProfile)
	fmt.Fprintf(a.Out, "Remote version: %s\n", resolvedVersion.Version)
	fmt.Fprintf(a.Out, "Local profile: %s\n\n", localProfile)
	a.PrintProfileDiff(diff)
	if !apply {
		fmt.Fprintln(a.Out, "\nDry run only; rerun with --yes to update the inactive local profile.")
		return nil
	}
	if err := a.ApplyProfileSnapshot(localProfile, snapshot); err != nil {
		return err
	}
	if err := a.recordCloudProfile(CloudProfileLink{
		Org:           org,
		RemoteProfile: remoteProfile,
		LocalProfile:  localProfile,
		Version:       resolvedVersion.Version,
	}); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "\nPulled %s/%s %s into local profile %q\n", org, remoteProfile, resolvedVersion.Version, localProfile)
	fmt.Fprintf(a.Out, "Activate it with: skillmux use %s\n", localProfile)
	return nil
}

func (a *App) CloudProfileRollback(ref, to string) error {
	if to == "" {
		return fmt.Errorf("--to is required")
	}
	org, profile, err := parseRemoteProfileRef(ref)
	if err != nil {
		return err
	}
	client, _, err := a.newCloudClient()
	if err != nil {
		return err
	}
	var resp struct {
		Version CloudProfileVersion `json:"version"`
	}
	req := map[string]string{"to": to}
	path := fmt.Sprintf("/api/v1/orgs/%s/profiles/%s/rollback", escapePath(org), escapePath(profile))
	if err := client.do(http.MethodPost, path, req, &resp); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Recommended version for %s/%s is now %s\n", org, profile, resp.Version.Version)
	return nil
}

func (a *App) CloudOrgSync() error {
	links, err := a.loadCloudProfiles()
	if err != nil || len(links.Profiles) == 0 {
		fmt.Fprintln(a.Out, "No linked cloud profiles. Pull or push a profile first.")
		return nil
	}
	table := newTable(a.Out)
	defer table.Flush()
	tableRow(table, "REMOTE", "LOCAL", "LOCAL VERSION", "REMOTE VERSION", "STATUS")
	for _, link := range links.Profiles {
		versions, err := a.cloudProfileVersions(link.Org, link.RemoteProfile)
		if err != nil {
			tableRow(table, link.Org+"/"+link.RemoteProfile, link.LocalProfile, link.Version, "-", "error: "+err.Error())
			continue
		}
		latest, ok := chooseVersion(versions, "")
		if !ok {
			tableRow(table, link.Org+"/"+link.RemoteProfile, link.LocalProfile, link.Version, "-", "no remote versions")
			continue
		}
		status := "current"
		if latest.Version != link.Version {
			status = "update available"
		}
		tableRow(table, link.Org+"/"+link.RemoteProfile, link.LocalProfile, link.Version, latest.Version, status)
	}
	return nil
}

func (a *App) cloudProfileVersions(org, profile string) ([]CloudProfileVersion, error) {
	client, _, err := a.newCloudClient()
	if err != nil {
		return nil, err
	}
	var resp struct {
		Versions []CloudProfileVersion `json:"versions"`
	}
	path := fmt.Sprintf("/api/v1/orgs/%s/profiles/%s/versions", escapePath(org), escapePath(profile))
	if err := client.do(http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	sort.Slice(resp.Versions, func(i, j int) bool {
		return versionNumber(resp.Versions[i].Version) > versionNumber(resp.Versions[j].Version)
	})
	return resp.Versions, nil
}

func (a *App) fetchCloudProfileSnapshot(org, profile, version string) (ProfileSnapshot, CloudProfileVersion, error) {
	versions, err := a.cloudProfileVersions(org, profile)
	if err != nil {
		return ProfileSnapshot{}, CloudProfileVersion{}, err
	}
	chosen, ok := chooseVersion(versions, version)
	if !ok {
		if version == "" {
			return ProfileSnapshot{}, CloudProfileVersion{}, fmt.Errorf("no versions for %s/%s", org, profile)
		}
		return ProfileSnapshot{}, CloudProfileVersion{}, fmt.Errorf("version %q not found for %s/%s", version, org, profile)
	}
	client, _, err := a.newCloudClient()
	if err != nil {
		return ProfileSnapshot{}, CloudProfileVersion{}, err
	}
	var snapshot ProfileSnapshot
	path := fmt.Sprintf("/api/v1/orgs/%s/profiles/%s/versions/%s/archive", escapePath(org), escapePath(profile), escapePath(chosen.Version))
	if err := client.do(http.MethodGet, path, nil, &snapshot); err != nil {
		return ProfileSnapshot{}, CloudProfileVersion{}, err
	}
	if err := validateSnapshot(snapshot); err != nil {
		return ProfileSnapshot{}, CloudProfileVersion{}, err
	}
	if chosen.Digest != "" && snapshot.Digest != chosen.Digest {
		return ProfileSnapshot{}, CloudProfileVersion{}, fmt.Errorf("profile archive digest mismatch")
	}
	return snapshot, chosen, nil
}

func parseRemoteProfileRef(ref string) (string, string, error) {
	parts := strings.Split(ref, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("remote profile must be in org/profile form")
	}
	if err := validateOrgName(parts[0]); err != nil {
		return "", "", err
	}
	if err := validateProfileName(parts[1]); err != nil {
		return "", "", err
	}
	return parts[0], parts[1], nil
}

func chooseVersion(versions []CloudProfileVersion, want string) (CloudProfileVersion, bool) {
	if len(versions) == 0 {
		return CloudProfileVersion{}, false
	}
	if want != "" {
		for _, version := range versions {
			if version.Version == want {
				return version, true
			}
		}
		return CloudProfileVersion{}, false
	}
	for _, version := range versions {
		if version.Recommended {
			return version, true
		}
	}
	sort.Slice(versions, func(i, j int) bool {
		return versionNumber(versions[i].Version) > versionNumber(versions[j].Version)
	})
	return versions[0], true
}

func versionNumber(version string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(version, "v"))
	return n
}

func shortDigest(digest string) string {
	if len(digest) <= 12 {
		return digest
	}
	return digest[:12]
}
