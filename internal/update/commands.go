//go:build !store

package update

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

const settingsKey = "update.settings"

// autoEvery is how long update.auto waits between checks.
const autoEvery = 20 * time.Hour

// Settings is stored in the settings table. Everything is off until the user says yes (ADR 0009).
type Settings struct {
	Enabled     bool   `json:"enabled" desc:"Check GitHub for new versions once a day. Off until you turn it on"`
	Asked       bool   `json:"asked" desc:"The first-run question has been answered or dismissed"`
	LastChecked string `json:"last_checked,omitempty" desc:"When the last check finished (RFC 3339)"`
	Latest      string `json:"latest,omitempty" desc:"The newest version the last check found"`
}

// Register adds the update commands with the real configuration.
func Register(r *command.Registry) { RegisterWith(r, DefaultConfig()) }

// RegisterWith adds the update commands with cfg (tests give it a local server).
func RegisterWith(r *command.Registry, cfg Config) {
	r.Add(
		command.Define("update.settings.get", "Show the update settings: whether Xegesis checks for new versions (off until you turn it on), when it last checked, and this version.", false,
			func(ctx context.Context, env *command.Env, _ struct{}) (SettingsInfo, error) {
				return settingsInfo(ctx, env, cfg)
			}),
		command.Define("update.settings.set", "Turn checking for updates on or off, or record that the first-run question was dismissed. Nothing is checked or sent by this.", true,
			func(ctx context.Context, env *command.Env, p SetParams) (SettingsInfo, error) {
				return settingsSet(ctx, env, cfg, p)
			}),
		command.Define("update.check", "Look for a newer version now: one HTTPS GET of the release's latest.json. Needs updates turned on.", true,
			func(ctx context.Context, env *command.Env, _ struct{}) (CheckResult, error) {
				return runCheck(ctx, env, cfg)
			}),
		command.Define("update.auto", "Check for a newer version if updates are on and the last check was a day ago. The app calls this at start-up and now and then; it does nothing when updates are off.", true,
			func(ctx context.Context, env *command.Env, _ struct{}) (CheckResult, error) {
				return runAuto(ctx, env, cfg)
			}),
		command.Define("update.install", "Download the newer version, check its SHA-256 and signature, and run its installer; the app then restarts. Needs updates turned on.", false,
			func(ctx context.Context, env *command.Env, _ struct{}) (InstallResult, error) {
				return runInstall(ctx, env, cfg)
			}),
	)
}

type SettingsInfo struct {
	Settings
	Current string `json:"current" desc:"This version"`
	Key     bool   `json:"key_set" desc:"The build has a real update key; false means updates cannot verify yet"`
}

type SetParams struct {
	Enabled *bool `json:"enabled,omitempty" desc:"true to check for updates, false to stop"`
	Asked   *bool `json:"asked,omitempty" desc:"true to record that the first-run question was dismissed without turning updates on; false to ask it again at the next start"`
}

type CheckResult struct {
	Ran     bool   `json:"ran" desc:"A check was made"`
	Current string `json:"current"`
	Latest  string `json:"latest,omitempty" desc:"The newest published version"`
	Newer   bool   `json:"newer" desc:"The newest version is newer than this one"`
	Notes   string `json:"notes,omitempty"`
}

type InstallResult struct {
	Version  string `json:"version"`
	Launched bool   `json:"launched" desc:"The installer was started; the app must quit now"`
}

func needStore(env *command.Env) error {
	if env.Store == nil {
		return command.Errorf("unavailable", "no library is open")
	}
	return nil
}

func load(ctx context.Context, s *store.Store) (Settings, error) {
	var raw string
	err := s.QueryRow(ctx, `SELECT value_json FROM settings WHERE key = ?`, settingsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	var st Settings
	return st, json.Unmarshal([]byte(raw), &st)
}

func save(ctx context.Context, s *store.Store, st Settings) error {
	raw, _ := json.Marshal(st)
	return s.Write(ctx, "update.settings", func(tx *store.Tx) error {
		_, err := tx.Get("settings", store.Row{"key": settingsKey})
		if errors.Is(err, store.ErrNotFound) {
			return tx.Create("settings", store.Row{"key": settingsKey, "value_json": string(raw)})
		} else if err != nil {
			return err
		}
		return tx.Update("settings", store.Row{"key": settingsKey}, store.Row{"value_json": string(raw)})
	})
}

func settingsInfo(ctx context.Context, env *command.Env, cfg Config) (SettingsInfo, error) {
	if err := needStore(env); err != nil {
		return SettingsInfo{}, err
	}
	st, err := load(ctx, env.Store)
	return SettingsInfo{Settings: st, Current: cfg.Current, Key: len(cfg.PubKey) > 0 && !PlaceholderKey()}, err
}

func settingsSet(ctx context.Context, env *command.Env, cfg Config, p SetParams) (SettingsInfo, error) {
	if err := needStore(env); err != nil {
		return SettingsInfo{}, err
	}
	st, err := load(ctx, env.Store)
	if err != nil {
		return SettingsInfo{}, err
	}
	if p.Enabled != nil {
		st.Enabled, st.Asked = *p.Enabled, true
	}
	if p.Asked != nil {
		st.Asked = *p.Asked
	}
	if err := save(ctx, env.Store, st); err != nil {
		return SettingsInfo{}, err
	}
	return settingsInfo(ctx, env, cfg)
}

func requireOn(ctx context.Context, env *command.Env) (Settings, error) {
	if err := needStore(env); err != nil {
		return Settings{}, err
	}
	st, err := load(ctx, env.Store)
	if err != nil {
		return st, err
	}
	if !st.Enabled {
		return st, command.Errorf("unavailable", "updates are off; turn them on in Settings first")
	}
	return st, nil
}

func runCheck(ctx context.Context, env *command.Env, cfg Config) (CheckResult, error) {
	st, err := requireOn(ctx, env)
	if err != nil {
		return CheckResult{}, err
	}
	return check(ctx, env, cfg, st)
}

func check(ctx context.Context, env *command.Env, cfg Config, st Settings) (CheckResult, error) {
	m, err := cfg.Check(ctx)
	if err != nil {
		return CheckResult{Current: cfg.Current}, command.Errorf("failed", "could not check for updates: %v", err)
	}
	st.LastChecked = cfg.now().UTC().Format(time.RFC3339)
	st.Latest = m.Version
	if err := save(ctx, env.Store, st); err != nil {
		return CheckResult{}, err
	}
	return CheckResult{Ran: true, Current: cfg.Current, Latest: m.Version, Newer: Newer(m.Version, cfg.Current), Notes: m.Notes}, nil
}

func runAuto(ctx context.Context, env *command.Env, cfg Config) (CheckResult, error) {
	if err := needStore(env); err != nil {
		return CheckResult{}, err
	}
	st, err := load(ctx, env.Store)
	if err != nil || !st.Enabled {
		return CheckResult{Current: cfg.Current}, err
	}
	if t, err := time.Parse(time.RFC3339, st.LastChecked); err == nil && cfg.now().Sub(t) < autoEvery {
		return CheckResult{Current: cfg.Current, Latest: st.Latest, Newer: Newer(st.Latest, cfg.Current)}, nil
	}
	return check(ctx, env, cfg, st)
}

func runInstall(ctx context.Context, env *command.Env, cfg Config) (InstallResult, error) {
	if _, err := requireOn(ctx, env); err != nil {
		return InstallResult{}, err
	}
	m, err := cfg.Check(ctx)
	if err != nil {
		return InstallResult{}, command.Errorf("failed", "could not check for updates: %v", err)
	}
	if !Newer(m.Version, cfg.Current) {
		return InstallResult{}, command.Errorf("not_found", "Xegesis %s is the newest version", cfg.Current)
	}
	file, err := cfg.Download(ctx, m)
	if err != nil {
		return InstallResult{}, command.Errorf("failed", "%v", err)
	}
	if cfg.Launch == nil {
		return InstallResult{}, command.Errorf("unavailable", "this build cannot start an installer")
	}
	if err := cfg.Launch(file); err != nil {
		return InstallResult{}, command.Errorf("failed", "could not start the installer: %v", err)
	}
	return InstallResult{Version: m.Version, Launched: true}, nil
}
