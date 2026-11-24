package app

import (
	"fmt"
	"path"
	"regexp"
	"runtime"
	"strings"
	"unicode"

	"mayoi/internal/openidea"
)

var dockerShellSafe = regexp.MustCompile(`^[\w@%+=:,./-]+$`)

func dockerQuote(s string, windows bool) string {
	if s != "" && dockerShellSafe.MatchString(s) {
		return s
	}
	if windows {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
func dockerLine(words []string, windows bool) string {
	q := make([]string, len(words))
	for i, w := range words {
		q[i] = dockerQuote(w, windows)
	}
	return strings.Join(q, " ")
}
func dockerList(s string, whitespace bool) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == '\n' || (whitespace && (unicode.IsSpace(r) || r == ',')) })
}
func dockerWords(s string) ([]string, error) {
	var out []string
	var b strings.Builder
	var quote rune
	escaped, active := false, false
	for _, r := range s {
		if escaped {
			b.WriteRune(r)
			escaped = false
			active = true
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			active = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
			active = true
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			active = true
			continue
		}
		if unicode.IsSpace(r) {
			if active {
				out = append(out, b.String())
				b.Reset()
				active = false
			}
			continue
		}
		b.WriteRune(r)
		active = true
	}
	if quote != 0 || escaped {
		return nil, fmt.Errorf("unfinished quote or escape in Docker options")
	}
	if active {
		out = append(out, b.String())
	}
	return out, nil
}
func dockerImageTag(file string) string {
	dir := path.Base(path.Dir(strings.ReplaceAll(file, `\`, "/")))
	dir = strings.ToLower(dir)
	dir = regexp.MustCompile(`[^a-z0-9._-]+`).ReplaceAllString(dir, "-")
	dir = strings.Trim(dir, "-._")
	if dir == "" {
		dir = "image"
	}
	return dir + ":latest"
}
func dockerContainerName(given, image string) string {
	if strings.TrimSpace(given) != "" {
		return strings.TrimSpace(given)
	}
	image = strings.SplitN(image, "@", 2)[0]
	image = path.Base(image)
	if i := strings.LastIndex(image, ":"); i >= 0 {
		image = image[:i]
	}
	image = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`).ReplaceAllString(image, "-")
	image = strings.TrimLeft(image, "-_.")
	if image == "" {
		return "container"
	}
	return image
}

// dockerRunCommand builds the shell command and stop command used by run_start.
func dockerRunCommand(kind string, params map[string]any, windows bool) (string, *string, error) {
	p := func(k string) string { s, _ := params[k].(string); return s }
	on := func(k string) bool { return p(k) == "true" }
	trim := func(k string) string { return strings.TrimSpace(p(k)) }
	var stop *string
	container := func(image string) (string, error) {
		name := dockerContainerName(p("containerName"), image)
		args := []string{"docker", "run", "--name", name}
		if on("pull") {
			args = append(args, "--pull", "always")
		}
		for _, field := range []struct {
			k, flag string
			ws      bool
		}{{"ports", "-p", true}, {"volumes", "-v", false}, {"containerEnv", "-e", false}} {
			for _, v := range dockerList(p(field.k), field.ws) {
				args = append(args, field.flag, strings.TrimSpace(v))
			}
		}
		options, err := dockerWords(p("runOptions"))
		if err != nil {
			return "", err
		}
		args = append(args, options...)
		args = append(args, image)
		rm := dockerLine([]string{"docker", "rm", "-f", name}, windows)
		if windows {
			rm += " >nul 2>&1 &"
		} else {
			rm += " >/dev/null 2>&1;"
		}
		command := rm + " " + dockerLine(args, windows)
		if trim("command") != "" {
			command += " " + trim("command")
		}
		s := dockerLine([]string{"docker", "stop", name}, windows)
		stop = &s
		return command, nil
	}
	switch kind {
	case "docker-compose":
		base := []string{"docker", "compose"}
		for _, f := range dockerList(p("files"), false) {
			base = append(base, "-f", strings.TrimSpace(f))
		}
		if trim("project") != "" {
			base = append(base, "-p", trim("project"))
		}
		up := append(append([]string{}, base...), "up")
		if on("detach") {
			up = append(up, "-d")
		}
		if on("build") {
			up = append(up, "--build")
		}
		opts, err := dockerWords(p("upOptions"))
		if err != nil {
			return "", nil, err
		}
		up = append(up, opts...)
		up = append(up, dockerList(p("services"), true)...)
		if !on("detach") {
			s := dockerLine(append(base, "down"), windows)
			stop = &s
		}
		return dockerLine(up, windows), stop, nil
	case "docker-image":
		command, err := container(trim("image"))
		return "(" + command + ")", stop, err
	case "docker-dockerfile":
		file := trim("dockerfile")
		if file == "" {
			file = "Dockerfile"
		}
		context := trim("context")
		if context == "" {
			context = path.Dir(strings.ReplaceAll(file, `\`, "/"))
		}
		tag := trim("tag")
		if tag == "" {
			tag = dockerImageTag(context + "/Dockerfile")
		}
		args := []string{"docker", "build", "-f", file, "-t", tag}
		if trim("target") != "" {
			args = append(args, "--target", trim("target"))
		}
		for _, arg := range dockerList(p("buildArgs"), false) {
			args = append(args, "--build-arg", strings.TrimSpace(arg))
		}
		opts, err := dockerWords(p("buildOptions"))
		if err != nil {
			return "", nil, err
		}
		args = append(args, opts...)
		args = append(args, context)
		cmd := dockerLine(args, windows)
		if on("runImage") {
			run, err := container(tag)
			if err != nil {
				return "", nil, err
			}
			cmd += " && (" + run + ")"
		}
		return cmd, stop, nil
	}
	return "", nil, fmt.Errorf("unknown Docker run type %q", kind)
}
func dockerPrepareRunConfig(cfg *openidea.RunConfig) (*openidea.RunConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("missing Docker configuration")
	}
	command, stop, err := dockerRunCommand(cfg.Kind, cfg.Params, runtime.GOOS == "windows")
	if err != nil {
		return nil, err
	}
	out := *cfg
	out.Command = command
	out.Args = []string{}
	out.StopCommand = stop
	return &out, nil
}
