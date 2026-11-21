package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	goideaCLICommand = "goidea"
	goideaCLIMarker  = "GoIDEA command launcher v1"
)

type cliInstallResult struct {
	Path          string
	PathDirectory string
}

// InstallCLI installs the goidea launcher used by terminals and other apps.
func (m *Model) InstallCLI() {
	exe, err := os.Executable()
	if err != nil {
		m.notifyUser("app.cli", "error", "Cannot install the goidea command", err.Error())
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		m.notifyUser("app.cli", "error", "Cannot install the goidea command", err.Error())
		return
	}
	go func() {
		result, err := installCLI(exe, home, os.Getenv("PATH"), runtime.GOOS)
		m.post(func() {
			if err != nil {
				m.notifyUser("app.cli", "error", "Cannot install the goidea command", err.Error())
				return
			}
			body := result.Path
			if result.PathDirectory != "" {
				body += "\nAdd " + result.PathDirectory + " to PATH, then open a new terminal."
			} else {
				body += "\nOpen a new terminal to use the goidea command."
			}
			m.notifyUser("app.cli", "info", "The goidea command is installed", body)
		})
	}()
}

// UninstallCLI removes only launchers carrying GoIDEA's marker.
func (m *Model) UninstallCLI() {
	home, err := os.UserHomeDir()
	if err != nil {
		m.notifyUser("app.cli", "error", "Cannot uninstall the goidea command", err.Error())
		return
	}
	go func() {
		links, err := uninstallCLI(home, runtime.GOOS)
		m.post(func() {
			if err != nil {
				m.notifyUser("app.cli", "error", "Cannot uninstall the goidea command", err.Error())
				return
			}
			if len(links) == 0 {
				m.notifyUser("app.cli", "info", "The goidea command is not installed", "")
				return
			}
			m.notifyUser("app.cli", "info", "The goidea command is uninstalled", strings.Join(links, "\n"))
		})
	}()
}

func installCLI(exe, home, pathEnv, goos string) (cliInstallResult, error) {
	if strings.Contains(filepath.ToSlash(exe), "/AppTranslocation/") {
		return cliInstallResult{}, errors.New("move GoIDEA to the Applications folder first")
	}
	exe, err := filepath.Abs(exe)
	if err != nil {
		return cliInstallResult{}, err
	}
	info, err := os.Stat(exe)
	if err != nil {
		return cliInstallResult{}, fmt.Errorf("GoIDEA executable: %w", err)
	}
	if info.IsDir() {
		return cliInstallResult{}, errors.New("the GoIDEA executable path is a directory")
	}
	dir, err := goideaCLIBinDir(home, goos)
	if err != nil {
		return cliInstallResult{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return cliInstallResult{}, err
	}
	path := goideaCLIWrapperPath(dir, goos)
	if _, err := os.Lstat(path); err == nil {
		own, err := isGoideaCLIWrapper(path, goos)
		if err != nil {
			return cliInstallResult{}, err
		}
		if !own {
			return cliInstallResult{}, fmt.Errorf("%s already exists and is not a GoIDEA command", path)
		}
	} else if !os.IsNotExist(err) {
		return cliInstallResult{}, err
	}
	if err := writeGoideaCLIWrapper(path, exe, goos); err != nil {
		return cliInstallResult{}, err
	}
	result := cliInstallResult{Path: path}
	if !pathListContains(pathEnv, dir, goos) {
		result.PathDirectory = dir
	}
	return result, nil
}

func uninstallCLI(home, goos string) ([]string, error) {
	dirs := goideaCLIUninstallDirs(home, goos)
	var removed []string
	for _, dir := range dirs {
		path := goideaCLIWrapperPath(dir, goos)
		own, err := isGoideaCLIWrapper(path, goos)
		if err != nil {
			return removed, err
		}
		if !own {
			continue
		}
		if err := os.Remove(path); err != nil {
			return removed, err
		}
		removed = append(removed, path)
	}
	return removed, nil
}

func goideaCLIBinDir(home, goos string) (string, error) {
	if home == "" {
		return "", errors.New("no user directory for the goidea command")
	}
	if goos == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, "GoIDEA", "bin"), nil
		}
	}
	return filepath.Join(home, ".local", "bin"), nil
}

func goideaCLIUninstallDirs(home, goos string) []string {
	dir, err := goideaCLIBinDir(home, goos)
	if err != nil {
		return nil
	}
	if goos == "windows" || dir == filepath.Clean("/usr/local/bin") {
		return []string{dir}
	}
	return []string{dir, "/usr/local/bin"}
}

func goideaCLIWrapperPath(dir, goos string) string {
	name := goideaCLICommand
	if goos == "windows" {
		name += ".cmd"
	}
	return filepath.Join(dir, name)
}

func writeGoideaCLIWrapper(path, exe, goos string) error {
	text := goideaCLIWrapper(exe, goos)
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(text); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if goos == "windows" {
		_ = os.Remove(path)
	}
	return os.Rename(name, path)
}

func goideaCLIWrapper(exe, goos string) string {
	if goos == "windows" {
		return "@echo off\r\nrem " + goideaCLIMarker + "\r\n\"" + strings.ReplaceAll(exe, "%", "%%") + "\" %*\r\n"
	}
	return "#!/bin/sh\n# " + goideaCLIMarker + "\nexec " + shellQuote(exe) + " \"$@\"\n"
}

func isGoideaCLIWrapper(path, goos string) (bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) < 2 {
		return false, nil
	}
	if goos == "windows" {
		return lines[1] == "rem "+goideaCLIMarker, nil
	}
	return lines[1] == "# "+goideaCLIMarker, nil
}

func pathListContains(pathEnv, dir, goos string) bool {
	if goos == "windows" {
		dir = strings.TrimRight(strings.ToLower(dir), `\/`)
		for _, item := range strings.Split(pathEnv, ";") {
			item = strings.TrimRight(strings.ToLower(strings.Trim(item, `"`)), `\/`)
			if item == dir {
				return true
			}
		}
		return false
	}
	for _, item := range filepath.SplitList(pathEnv) {
		if filepath.Clean(item) == filepath.Clean(dir) {
			return true
		}
	}
	return false
}
