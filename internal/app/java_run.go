package app

// Resolving a Java Application/Spring Boot run configuration before run_start:
// OpenIDEA's frontend asks jdtls for the module classpath and the java
// launcher, then substitutes ${classpath}/${testClasspath}. That glue belongs
// to the client, so GoIDEA does the same instead of handing the placeholders
// to the process.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"mayoi/internal/openidea"
)

type jvmRunConfig struct {
	Kind        string   `json:"kind"`
	Command     string   `json:"command"`
	Args        []string `json:"args"`
	Module      *string  `json:"module"`
	IsTest      bool     `json:"isTest"`
	BuildPolicy *string  `json:"buildPolicy"`
}

func isJVMApplication(kind string) bool {
	return kind == "application" || kind == "spring-boot"
}

// resolveJVMConfig returns the configuration with its classpath placeholders
// and java launcher resolved through java_resolve_launch. A configuration of
// another kind comes back unchanged. When the language server cannot resolve
// it (no JDT yet), the raw configuration is kept so the failure stays visible
// in the run console instead of blocking the launch here.
func (m *Model) resolveJVMConfig(ctx context.Context, raw json.RawMessage, root string) (json.RawMessage, error) {
	var cfg jvmRunConfig
	if err := json.Unmarshal(raw, &cfg); err != nil || !isJVMApplication(cfg.Kind) {
		return raw, nil
	}
	mainClass := javaMainClass(cfg.Args)
	module := ""
	if cfg.Module != nil {
		module = *cfg.Module
	} else if mainClass != "" {
		module = javaModuleOf(root, mainClass)
	}
	policy := "automatic"
	if cfg.BuildPolicy != nil && *cfg.BuildPolicy != "" {
		policy = *cfg.BuildPolicy
	}
	request := m.nextRequest()
	rawInfo, err := m.mgr.Invoke(ctx, Window, "java_resolve_launch", map[string]any{
		"module": module, "test": cfg.IsTest, "policy": policy, "request": request,
	})
	if err != nil {
		m.post(func() {
			m.status = "Java 项目尚未导入，改用原始运行配置：" + err.Error()
			m.invalidate()
		})
		return raw, nil
	}
	var info openidea.JavaLaunchInfo
	if err := json.Unmarshal(rawInfo, &info); err != nil {
		return raw, nil
	}
	if info.Build != nil && (*info.Build == "failed" || *info.Build == "withErrors") {
		detail := ""
		if len(info.Errors) > 0 {
			e := info.Errors[0]
			detail = fmt.Sprintf("（%s:%d:%d %s）", e.Path, e.Line, e.Column, e.Message)
		}
		return raw, fmt.Errorf("构建失败，未启动 %s%s", mainClass, detail)
	}
	sep := info.PathListSeparator
	if sep == "" {
		sep = string(os.PathListSeparator)
	}
	classPath := strings.Join(info.ClassPaths, sep)
	args := append([]string(nil), cfg.Args...)
	for i, a := range args {
		if a == "${classpath}" || a == "${testClasspath}" {
			args[i] = classPath
		}
	}
	if len(info.ModulePaths) > 0 && !hasJavaFlag(args, "-p", "--module-path") {
		args = append([]string{"--module-path", strings.Join(info.ModulePaths, sep)}, args...)
	}
	if cfg.Kind == "spring-boot" && !strings.Contains(strings.Join(args, " "), "-Dspring.output.ansi.enabled=") {
		args = append([]string{"-Dspring.output.ansi.enabled=always"}, args...)
	}
	command := cfg.Command
	if command == "java" && info.JavaExec != nil && *info.JavaExec != "" {
		command = *info.JavaExec
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return raw, nil
	}
	obj["args"], obj["command"] = args, command
	return json.Marshal(obj)
}

// javaMainClass reads the main class from a java command line.
func javaMainClass(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-jar" {
			return ""
		}
		if a == "-cp" || a == "-classpath" || a == "--class-path" {
			i++
			continue
		}
		if strings.HasPrefix(a, "-") {
			if javaFlagTakesValue(a) {
				i++
			}
			continue
		}
		return a
	}
	return ""
}

func javaFlagTakesValue(flag string) bool {
	switch flag {
	case "-p", "--module-path", "--add-modules", "--add-opens", "--add-exports", "--add-reads",
		"--patch-module", "--limit-modules", "--upgrade-module-path", "--enable-native-access":
		return true
	}
	return false
}

func hasJavaFlag(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f {
				return true
			}
		}
	}
	return false
}

// javaModuleOf finds the module folder of a main class: the path before
// src/main/java under the project root, "" for a single-module project.
func javaModuleOf(root, mainClass string) string {
	if root == "" || mainClass == "" {
		return ""
	}
	pkg := mainClass
	if i := strings.LastIndex(mainClass, "."); i >= 0 {
		pkg = mainClass[:i]
	}
	simple := mainClass[strings.LastIndex(mainClass, ".")+1:]
	name := simple + ".java"
	var found string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() {
				switch d.Name() {
				case ".git", "target", "build", "node_modules", ".idea", ".openidea":
					return filepath.SkipDir
				}
			}
			return nil
		}
		if d.Name() != name {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		marker := "/src/main/java/"
		i := strings.Index("/"+rel, marker)
		if i < 0 {
			return nil
		}
		mod := ""
		if i > 0 {
			mod = strings.TrimSuffix(rel[:i], "/")
			if mod == rel {
				mod = ""
			}
		}
		if pkg != "" {
			want := "/" + strings.ReplaceAll(pkg, ".", "/") + "/" + name
			if !strings.HasSuffix("/"+rel, want) {
				return nil
			}
		}
		found = mod
		return filepath.SkipAll
	})
	return found
}

var _ = path.Base
