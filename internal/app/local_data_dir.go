package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
)

const localDataDirConfigName = "local-data-dir.json"

type localDataDirConfig struct {
	CustomRoot string `json:"customRoot,omitempty"`
}

// LocalDataDirConfigPath is the stable bootstrap file that has to be readable
// before mygo chooses the application data directory.
func LocalDataDirConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "goidea", localDataDirConfigName), nil
}

func loadCustomLocalDataDir(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var cfg localDataDirConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", err
	}
	root := strings.TrimSpace(cfg.CustomRoot)
	if root == "" {
		return "", nil
	}
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) {
		return "", errors.New("the custom local data directory must be absolute")
	}
	return root, nil
}

// LoadCustomLocalDataDir is the startup-facing form of the bootstrap reader.
func LoadCustomLocalDataDir(path string) (string, error) {
	return loadCustomLocalDataDir(path)
}

func saveCustomLocalDataDir(path, root string) error {
	if path == "" {
		return errors.New("the local data directory configuration path is unavailable")
	}
	root = strings.TrimSpace(root)
	if root != "" {
		root = filepath.Clean(root)
		if !filepath.IsAbs(root) {
			return errors.New("the custom local data directory must be absolute")
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(localDataDirConfig{CustomRoot: root}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".local-data-dir-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// migrateLocalDataDir copies the existing tree into the new location before
// restart. The old tree stays as a backup, which is safer than deleting a live
// backend home while the process is still using it.
func migrateLocalDataDir(from, to string) error {
	from, to = filepath.Clean(from), filepath.Clean(to)
	if from == "" || to == "" || from == to {
		return nil
	}
	within := func(parent, child string) bool {
		rel, err := filepath.Rel(parent, child)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	if within(from, to) || within(to, from) {
		return errors.New("the old and new local data directories cannot contain one another")
	}
	if _, err := os.Stat(from); errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(to, 0o755)
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(from, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case entry.Type().IsRegular():
			return copyLocalDataFile(path, target, info.Mode().Perm())
		case entry.Type()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_ = os.Remove(target)
			return os.Symlink(link, target)
		default:
			return nil
		}
	})
}

func copyLocalDataFile(from, to string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(to, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func (m *Model) goideaDataDir() string {
	if m == nil {
		return ""
	}
	if m.cfg.DataDir != "" {
		return filepath.Clean(m.cfg.DataDir)
	}
	if m.cfg.SettingsPath != "" {
		return filepath.Dir(m.cfg.SettingsPath)
	}
	if m.cfg.Home != "" {
		return filepath.Dir(m.cfg.Home)
	}
	return ""
}

func (m *Model) targetDataDir(custom string) string {
	custom = strings.TrimSpace(custom)
	if custom != "" {
		return filepath.Clean(custom)
	}
	if m.cfg.DefaultDataDir != "" {
		return filepath.Clean(m.cfg.DefaultDataDir)
	}
	return m.goideaDataDir()
}

func (m *Model) customLocalDataDir() string {
	if m == nil || m.cfg.DefaultDataDir == "" {
		return ""
	}
	current := m.goideaDataDir()
	if current != "" && current != filepath.Clean(m.cfg.DefaultDataDir) {
		return current
	}
	return ""
}

func (m *Model) localDataDirLocked() bool {
	return m == nil || os.Getenv("GOIDEA_DATA_DIR") != "" || m.cfg.LocalDataDirConfigPath == ""
}

func (m *Model) chooseLocalDataDir() {
	if m == nil || m.localDataDirLocked() {
		return
	}
	start := strings.TrimSpace(m.settingsDraft.localDataDir)
	if start == "" {
		start = m.goideaDataDir()
	}
	parent := m.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Local data directory", ButtonLabel: "Choose",
			DefaultPath: start, Directory: true, CreateDirectories: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.post(func() {
			m.settingsDraft.localDataDir = filepath.Clean(paths[0])
			m.invalidate()
		})
	}()
}

// applyLocalDataDirectory stores the bootstrap choice and optionally copies
// the current tree. A successful change asks the normal quit pipeline to
// relaunch, so the backend starts with its new owned home.
func (m *Model) applyLocalDataDirectory(custom string, move bool) error {
	if os.Getenv("GOIDEA_DATA_DIR") != "" {
		return errors.New("GOIDEA_DATA_DIR overrides the local data directory")
	}
	old := m.goideaDataDir()
	target := m.targetDataDir(custom)
	if target == "" || !filepath.IsAbs(target) {
		return errors.New("choose an absolute local data directory")
	}
	if move && target != old {
		// The session is part of the data tree; write its latest snapshot before
		// copying so the restarted app opens the same project and tabs.
		m.saveSessionNow()
		if err := migrateLocalDataDir(old, target); err != nil {
			return fmt.Errorf("copy data: %w", err)
		}
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	if err := saveCustomLocalDataDir(m.cfg.LocalDataDirConfigPath, custom); err != nil {
		return err
	}
	if target != old {
		m.restartPending = true
		m.status = "Local data directory changed; restarting GoIDEA…"
	}
	return nil
}
