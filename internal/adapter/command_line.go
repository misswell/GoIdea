package adapter

// The port of src/platform/commandLine.ts's parseArgs / formatArgs: the text
// form of a configuration's arguments and environment, as the run
// configuration editor shows them.

import (
	"strings"
)

// ParseArgs splits a command line into arguments: whitespace, '…' and "…"
// quoting, backslash escapes.
func ParseArgs(input string) []string {
	var out []string
	var cur strings.Builder
	has := false
	quote := byte(0)
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			} else if ch == '\\' && quote == '"' && i+1 < len(input) &&
				strings.ContainsRune(`"\$`+"`", rune(input[i+1])) {
				i++
				cur.WriteByte(input[i])
			} else {
				cur.WriteByte(ch)
			}
			continue
		}
		switch {
		case ch == '\'' || ch == '"':
			quote = ch
			has = true
		case ch == '\\' && i+1 < len(input):
			i++
			cur.WriteByte(input[i])
			has = true
		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\v' || ch == '\f':
			if has || cur.Len() > 0 {
				out = append(out, cur.String())
			}
			cur.Reset()
			has = false
		default:
			cur.WriteByte(ch)
		}
	}
	if has || cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// FormatArgs joins arguments into one line, quoting what whitespace, quotes,
// backslashes or `$`/“ ` “ would otherwise break.
func FormatArgs(args []string) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		switch {
		case a == "":
			parts = append(parts, "''")
		case strings.ContainsAny(a, " \t\n\r\v\f'\"\\$`"):
			parts = append(parts, `'`+strings.ReplaceAll(a, `'`, `'\''`)+`'`)
		default:
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}

// FormatEnv renders environment variables as `KEY=VALUE` lines.
func FormatEnv(env map[string]string) string {
	lines := make([]string, 0, len(env))
	for k, v := range env {
		lines = append(lines, k+"="+v)
	}
	// Deterministic order: the editor shows a stable list.
	for i := 1; i < len(lines); i++ {
		for j := i; j > 0 && lines[j] < lines[j-1]; j-- {
			lines[j], lines[j-1] = lines[j-1], lines[j]
		}
	}
	return strings.Join(lines, "\n")
}

// ParseEnv reads `KEY=VALUE` lines; a line without `=` is skipped, as
// upstream parseEnv's echo of the editor round trip does.
func ParseEnv(text string) map[string]string {
	env := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		if i := strings.Index(line, "="); i > 0 {
			env[line[:i]] = line[i+1:]
		}
	}
	return env
}
