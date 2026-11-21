package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const goideaUsage = `Usage:
  goidea [path ...]
  goidea --help
  goidea --version

Files open in LightEdit and the first folder opens as the current project.
`

func handleCommandLine(args []string, stdout, stderr io.Writer, version string) (handled bool, code int) {
	args = withoutCLIFlag(args)
	if len(args) == 1 {
		switch args[0] {
		case "--help", "-h", "help":
			_, _ = io.WriteString(stdout, goideaUsage)
			return true, 0
		case "--version", "-V", "version":
			if version == "" {
				version = "Development"
			}
			_, _ = fmt.Fprintf(stdout, "GoIDEA %s\n", version)
			return true, 0
		}
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			_, _ = fmt.Fprintf(stderr, "goidea: unknown option %s\n\n%s", arg, goideaUsage)
			return true, 2
		}
	}
	return false, 0
}

func commandLineTargets(args []string) (dir string, files []string, errs []error) {
	args = withoutCLIFlag(args)
	for _, arg := range args {
		path, err := filepath.Abs(arg)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if info.IsDir() {
			if dir == "" {
				dir = path
			}
			continue
		}
		files = append(files, path)
	}
	return dir, files, errs
}

func withoutCLIFlag(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		if arg != "--cli" {
			out = append(out, arg)
		}
	}
	return out
}
