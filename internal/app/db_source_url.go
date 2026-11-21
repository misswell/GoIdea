package app

import (
	"net/url"
	"strconv"
	"strings"
)

// dbParseSourceURL reads the connection URLs OpenIDEA accepts in New › Data
// Source from URL and Import from Clipboard. It returns the raw source fields
// and the password separately, as the password never belongs in the source.
func dbParseSourceURL(raw string) (map[string]any, string, bool) {
	text := strings.TrimSpace(raw)
	for len(text) >= 5 && strings.EqualFold(text[:5], "jdbc:") {
		text = text[5:]
	}
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "sqlite:") || strings.HasPrefix(lower, "sqlite3:") {
		if i := strings.IndexByte(text, ':'); i >= 0 {
			file := strings.TrimPrefix(strings.TrimPrefix(text[i+1:], "//"), "///")
			file = strings.TrimPrefix(file, "//")
			if file == "" {
				return nil, "", false
			}
			name := strings.TrimSuffix(filepathBase(file), filepathExtension(file))
			if name == "" {
				name = filepathBase(file)
			}
			return map[string]any{
				"driver": "sqlite", "file": file, "name": name,
			}, "", true
		}
	}
	if strings.HasPrefix(lower, "oracle:thin:@") || strings.HasPrefix(lower, "oracledb:thin:@") {
		if i := strings.Index(text, ":thin:@"); i >= 0 {
			rest := strings.TrimPrefix(text[i+len(":thin:@"):], "//")
			text = "oracle://" + strings.TrimPrefix(rest, "/")
		}
	}

	parsed, err := url.Parse(text)
	if err != nil || parsed.Scheme == "" {
		return nil, "", false
	}
	driver, vendor, ok := dbDriverForURLScheme(parsed.Scheme)
	if !ok || driver == "sqlite" {
		return nil, "", false
	}
	port := ""
	if parsed.Port() != "" {
		value, err := strconv.Atoi(parsed.Port())
		if err != nil || value < 1 || value > 65535 {
			return nil, "", false
		}
		port = strconv.Itoa(value)
	}
	database := strings.TrimPrefix(parsed.EscapedPath(), "/")
	if decoded, err := url.PathUnescape(database); err == nil {
		database = decoded
	}
	database = strings.TrimSuffix(database, "/")
	user := ""
	password := ""
	if parsed.User != nil {
		user = parsed.User.Username()
		password, _ = parsed.User.Password()
	}
	name := database
	if name == "" {
		name = parsed.Hostname()
	}
	if user != "" {
		name = user + "@" + name
	}
	source := map[string]any{
		"name": name, "driver": driver, "host": parsed.Hostname(),
		"port": port, "database": database, "user": user,
	}
	if vendor != "" && vendor != driver {
		source["vendor"] = vendor
	}
	properties := []map[string]any{}
	for key, values := range parsed.Query() {
		value := ""
		if len(values) > 0 {
			value = values[len(values)-1]
		}
		switch strings.ToLower(key) {
		case "user":
			if user == "" {
				user = value
				source["user"] = user
			}
		case "password":
			if password == "" {
				password = value
			}
		case "sslmode", "ssl-mode":
			switch strings.ToLower(value) {
			case "disable":
				source["ssl"] = "disable"
			case "prefer":
				source["ssl"] = "prefer"
			case "require":
				source["ssl"] = "require"
			case "verify-ca", "verifyca":
				source["ssl"] = "verifyCa"
			case "verify-full", "verifyfull":
				source["ssl"] = "verifyFull"
			}
		case "sslrootcert", "ssl-ca":
			source["sslCaFile"] = value
		case "sslcert", "ssl-cert":
			source["sslCertFile"] = value
		case "sslkey", "ssl-key":
			source["sslKeyFile"] = value
		default:
			properties = append(properties, map[string]any{"name": key, "value": value})
		}
	}
	if len(properties) > 0 {
		source["properties"] = properties
	}
	if driver == "clickhouse" {
		switch strings.ToLower(parsed.Scheme) {
		case "https":
			source["ssl"] = "verifyFull"
		case "http":
			source["ssl"] = "disable"
		}
	}
	return source, password, true
}

func dbDriverForURLScheme(scheme string) (driver, vendor string, ok bool) {
	switch strings.ToLower(scheme) {
	case "postgres", "postgresql":
		return "postgres", "", true
	case "redshift":
		return "postgres", "redshift", true
	case "mysql":
		return "mysql", "", true
	case "mariadb":
		return "mysql", "mariadb", true
	case "tidb":
		return "mysql", "tidb", true
	case "sqlserver", "mssql":
		return "sqlserver", "", true
	case "clickhouse", "http", "https":
		return "clickhouse", "", true
	case "oracle", "oracledb":
		return "oracle", "", true
	case "snowflake":
		return "snowflake", "", true
	case "sqlite", "sqlite3":
		return "sqlite", "", true
	}
	return "", "", false
}
