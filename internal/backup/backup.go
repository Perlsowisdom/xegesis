// Package backup exports and imports the user's own data and keeps automatic backup copies of
// library.db. Exports use any's value encodings (see docs/tasks/T29-export-backup.md).
package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// Register adds the export, import and backup commands. dataDir holds library.db; exports go to
// dataDir/exports and backups to dataDir/backups unless a path is given.
func Register(r *command.Registry, dataDir string) {
	r.Add(
		command.Define("export.data", "Write your notes, highlights, lists, collections, reading plans and documents to a folder or .zip in open formats.", false,
			func(ctx context.Context, env *command.Env, p ExportParams) (ExportResult, error) {
				return exportData(ctx, env, dataDir, p)
			}),
		command.Define("import.data", "Merge an export back in: matching IDs are updated, new ones created. Every change is logged.", true, importData),
		command.Define("backup.create", "Make a consistent copy of library.db while the app runs.", false,
			func(ctx context.Context, env *command.Env, p BackupParams) (BackupResult, error) {
				return backupCreate(ctx, env, dataDir, p)
			}),
		command.Define("backup.list", "List the backup copies in the backup folder, newest first.", false,
			func(ctx context.Context, env *command.Env, p BackupListParams) ([]BackupInfo, error) {
				return backupList(ctx, env, dataDir, p)
			}),
		command.Define("backup.settings.get", "Show the automatic backup schedule.", false, settingsGet),
		command.Define("backup.settings.set", "Set the automatic backup schedule: off, daily or weekly, and how many copies to keep.", true, settingsSet),
		command.Define("backup.auto", "Make a scheduled backup if one is due and delete copies beyond the number to keep. The app calls this at start-up and now and then.", false,
			func(ctx context.Context, env *command.Env, _ struct{}) (AutoResult, error) {
				return backupAuto(ctx, env, dataDir)
			}),
	)
}

func needStore(env *command.Env) error {
	if env.Store == nil {
		return command.Errorf("unavailable", "no library is open")
	}
	return nil
}

type BackupParams struct {
	Path string `json:"path,omitempty" desc:"A .db file to write, or a folder to put it in. Default: the backups folder in the data folder"`
}

type BackupResult struct {
	Path          string `json:"path"`
	Bytes         int64  `json:"bytes"`
	SchemaVersion int    `json:"schema_version"`
}

const (
	manualPrefix = "library-"
	autoPrefix   = "library-auto-"
	stampLayout  = "20060102T150405Z"
)

func backupCreate(ctx context.Context, env *command.Env, dataDir string, p BackupParams) (BackupResult, error) {
	if err := needStore(env); err != nil {
		return BackupResult{}, err
	}
	dest := p.Path
	switch {
	case dest == "":
		dest = filepath.Join(dataDir, "backups")
	case strings.HasSuffix(strings.ToLower(dest), ".db"):
		if _, err := os.Stat(dest); err == nil {
			return BackupResult{}, command.Errorf("bad_params", "%s already exists", dest)
		}
	}
	if !strings.HasSuffix(strings.ToLower(dest), ".db") {
		dest = filepath.Join(dest, manualPrefix+now().UTC().Format(stampLayout)+".db")
	}
	return copyLibrary(ctx, env.Store, dest)
}

func copyLibrary(ctx context.Context, s *store.Store, dest string) (BackupResult, error) {
	for i := 2; ; i++ { // two backups in the same second get different names
		if _, err := os.Stat(dest); err != nil {
			break
		}
		ext := filepath.Ext(dest)
		dest = fmt.Sprintf("%s-%d%s", strings.TrimSuffix(dest, ext), i, ext)
		if i > 1000 {
			return BackupResult{}, errors.New("too many backups with the same name")
		}
	}
	if err := vacuumInto(ctx, s, dest); err != nil {
		return BackupResult{}, err
	}
	v, err := verifyCopy(ctx, dest)
	if err != nil {
		os.Remove(dest)
		return BackupResult{}, fmt.Errorf("backup is not usable: %w", err)
	}
	st, err := os.Stat(dest)
	if err != nil {
		return BackupResult{}, err
	}
	return BackupResult{Path: dest, Bytes: st.Size(), SchemaVersion: v}, nil
}

// Schedule ---------------------------------------------------------------------------------------

const scheduleKey = "backup.schedule"

type Schedule struct {
	Every string `json:"every" enum:"off,daily,weekly" desc:"How often to back up"`
	Keep  int    `json:"keep" desc:"How many scheduled copies to keep (1 to 100)"`
	Dir   string `json:"dir,omitempty" desc:"Folder for scheduled copies. Default: the backups folder in the data folder"`
}

var defaultSchedule = Schedule{Every: "off", Keep: 7}

func loadSchedule(ctx context.Context, s *store.Store) (Schedule, error) {
	var raw string
	err := s.QueryRow(ctx, `SELECT value_json FROM settings WHERE key = ?`, scheduleKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultSchedule, nil
	}
	if err != nil {
		return Schedule{}, err
	}
	sc := defaultSchedule
	if err := json.Unmarshal([]byte(raw), &sc); err != nil {
		return Schedule{}, fmt.Errorf("settings %s: %w", scheduleKey, err)
	}
	return sc, nil
}

func settingsGet(ctx context.Context, env *command.Env, _ struct{}) (Schedule, error) {
	if err := needStore(env); err != nil {
		return Schedule{}, err
	}
	return loadSchedule(ctx, env.Store)
}

type SettingsSetParams struct {
	Every string  `json:"every" required:"true" enum:"off,daily,weekly" desc:"How often to back up"`
	Keep  int     `json:"keep,omitempty" desc:"How many scheduled copies to keep (1 to 100). Default: keep the current number"`
	Dir   *string `json:"dir,omitempty" desc:"Folder for scheduled copies; an empty string means the default"`
}

func settingsSet(ctx context.Context, env *command.Env, p SettingsSetParams) (Schedule, error) {
	cur, err := loadSchedule(ctx, env.Store)
	if err != nil {
		return Schedule{}, err
	}
	cur.Every = p.Every
	if p.Keep != 0 {
		if p.Keep < 1 || p.Keep > 100 {
			return Schedule{}, command.Errorf("bad_params", "keep must be between 1 and 100")
		}
		cur.Keep = p.Keep
	}
	if p.Dir != nil {
		cur.Dir = *p.Dir
	}
	raw, _ := json.Marshal(cur)
	err = env.Store.Write(ctx, "backup.settings.set", func(tx *store.Tx) error {
		_, err := tx.Get("settings", store.Row{"key": scheduleKey})
		if errors.Is(err, store.ErrNotFound) {
			return tx.Create("settings", store.Row{"key": scheduleKey, "value_json": string(raw)})
		} else if err != nil {
			return err
		}
		return tx.Update("settings", store.Row{"key": scheduleKey}, store.Row{"value_json": string(raw)})
	})
	return cur, err
}

type AutoResult struct {
	Ran    bool     `json:"ran" desc:"A backup was made"`
	Path   string   `json:"path,omitempty"`
	Pruned []string `json:"pruned,omitempty" desc:"Old copies that were deleted"`
	NextAt string   `json:"next_at,omitempty" desc:"When the next backup is due (RFC 3339)"`
}

func (sc Schedule) interval() time.Duration {
	switch sc.Every {
	case "daily":
		return 24 * time.Hour
	case "weekly":
		return 7 * 24 * time.Hour
	}
	return 0
}

func (sc Schedule) dir(dataDir string) string {
	if sc.Dir != "" {
		return sc.Dir
	}
	return filepath.Join(dataDir, "backups")
}

func backupAuto(ctx context.Context, env *command.Env, dataDir string) (AutoResult, error) {
	if err := needStore(env); err != nil {
		return AutoResult{}, err
	}
	sc, err := loadSchedule(ctx, env.Store)
	if err != nil {
		return AutoResult{}, err
	}
	var res AutoResult
	iv := sc.interval()
	if iv == 0 {
		return res, nil
	}
	dir := sc.dir(dataDir)
	files, err := listBackups(dir, autoPrefix)
	if err != nil {
		return res, err
	}
	if len(files) == 0 || now().Sub(files[0].Time) >= iv {
		r, err := copyLibrary(ctx, env.Store, filepath.Join(dir, autoPrefix+now().UTC().Format(stampLayout)+".db"))
		if err != nil {
			return res, err
		}
		res.Ran, res.Path = true, r.Path
		files, err = listBackups(dir, autoPrefix)
		if err != nil {
			return res, err
		}
	}
	for i := sc.Keep; i < len(files); i++ {
		if err := os.Remove(files[i].Path); err != nil {
			return res, err
		}
		res.Pruned = append(res.Pruned, files[i].Path)
	}
	if len(files) > 0 {
		res.NextAt = files[0].Time.Add(iv).UTC().Format(time.RFC3339)
	}
	return res, nil
}

// Listing ----------------------------------------------------------------------------------------

type BackupInfo struct {
	Path      string    `json:"path"`
	Bytes     int64     `json:"bytes"`
	Time      time.Time `json:"time" desc:"When the copy was made"`
	Scheduled bool      `json:"scheduled" desc:"Made by the automatic schedule"`
}

type BackupListParams struct {
	Dir string `json:"dir,omitempty" desc:"Folder to list. Default: the scheduled-backup folder, or the backups folder"`
}

func backupList(ctx context.Context, env *command.Env, dataDir string, p BackupListParams) ([]BackupInfo, error) {
	dir := p.Dir
	if dir == "" {
		dir = filepath.Join(dataDir, "backups")
		if env.Store != nil {
			sc, err := loadSchedule(ctx, env.Store)
			if err != nil {
				return nil, err
			}
			dir = sc.dir(dataDir)
		}
	}
	return listBackups(dir, manualPrefix)
}

// listBackups returns library-*.db files in dir whose names start with prefix, newest first. The
// time comes from the timestamp in the name, which survives copying the folder.
func listBackups(dir, prefix string) ([]BackupInfo, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []BackupInfo{}, nil
	} else if err != nil {
		return nil, err
	}
	out := []BackupInfo{}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, prefix) || !strings.HasSuffix(n, ".db") {
			continue
		}
		stamp := strings.TrimPrefix(n, prefix)
		if prefix == manualPrefix {
			stamp = strings.TrimPrefix(stamp, "auto-")
		}
		if len(stamp) < len(stampLayout) {
			continue
		}
		t, err := time.Parse(stampLayout, stamp[:len(stampLayout)])
		if err != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		out = append(out, BackupInfo{Path: filepath.Join(dir, n), Bytes: info.Size(), Time: t,
			Scheduled: strings.HasPrefix(n, autoPrefix)})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Time.Equal(out[j].Time) {
			return out[i].Time.After(out[j].Time)
		}
		return out[i].Path > out[j].Path
	})
	return out, nil
}
