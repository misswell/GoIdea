package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// dbSourceForm is the native Data Sources and Drivers editor. It keeps the raw
// OpenIDEA source object and updates only the fields the user changed, so
// vendor-specific settings survive an edit.
type dbSourceForm struct {
	open         bool
	raw          map[string]any
	id           string
	name         string
	driver       string
	vendor       string
	host         string
	port         string
	database     string
	user         string
	file         string
	password     string
	comment      string
	color        string
	url          string
	urlOnly      bool
	sslMode      string
	sslCA        string
	sslCert      string
	sslKey       string
	savePassword bool
	readOnly     bool
	tab          int

	defaultSchema  string
	transaction    string
	timeZone       string
	startupScript  string
	keepAlive      string
	autoDisconnect string
	singleDatabase bool

	sshEnabled bool
	sshHost    string
	sshPort    string
	sshUser    string
	sshAuth    string
	sshKey     string
	sshSecret  string
	saveSecret bool

	allDatabases     bool
	databasePatterns string
	allSchemas       bool
	schemaPatterns   string
	showSystem       bool
	showTemplates    bool
	objectFilter     string
	properties       string

	busy bool
	test string
	err  string
}

func (m *Model) OpenDBDataSourceForm(id string) {
	v := m.dbEnsureOpen()
	form := &dbSourceForm{open: true, tab: 0, raw: map[string]any{}}
	if id != "" {
		for _, source := range v.sources {
			if source.ID == id {
				form.raw = map[string]any{
					"id": source.ID, "name": source.Name, "driver": source.Driver,
					"host": source.Host, "database": source.Database, "user": source.User,
					"file": nullableString(source.File),
				}
				break
			}
		}
	}
	dbApplySourceForm(form)
	v.sourceForm = form
	if id != "" {
		m.RefreshDBDataSourceRaw(id, form)
	}
	m.invalidate()
}

func (m *Model) OpenDBDataSourceFormMap(source map[string]any) {
	v := m.dbEnsureOpen()
	form := &dbSourceForm{open: true, raw: dbCloneMap(source)}
	dbApplySourceForm(form)
	v.sourceForm = form
	m.invalidate()
}

func (m *Model) RefreshDBDataSourceRaw(id string, form *dbSourceForm) {
	v := m.db
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
		defer cancel()
		sources, err := m.mgr.DBSourcesRaw(ctx, Window)
		m.post(func() {
			if m.db != v || m.db.sourceForm != form {
				return
			}
			if err == nil {
				for _, source := range sources {
					if fmt.Sprint(source["id"]) == id {
						form.raw = source
						dbApplySourceForm(form)
						break
					}
				}
			}
			m.invalidate()
		})
	}()
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func dbMapString(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func dbMapBool(values map[string]any, key string) bool {
	value, ok := values[key]
	if !ok {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		parsed, _ := strconv.ParseBool(v)
		return parsed
	default:
		return false
	}
}

func dbMapNumbers(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	default:
		return fmt.Sprint(v)
	}
}

func dbMapObject(values map[string]any, key string) map[string]any {
	if value, ok := values[key].(map[string]any); ok {
		return value
	}
	out := map[string]any{}
	if values == nil {
		return out
	}
	values[key] = out
	return out
}

func dbApplySourceForm(f *dbSourceForm) {
	if f.raw == nil {
		f.raw = map[string]any{}
	}
	f.id = dbMapString(f.raw, "id")
	if f.id == "" {
		f.id = dbNewSourceID()
		f.raw["id"] = f.id
	}
	f.name = dbMapString(f.raw, "name")
	f.driver = dbMapString(f.raw, "driver")
	if f.driver == "" {
		f.driver = "postgres"
	}
	f.vendor = dbMapString(f.raw, "vendor")
	f.host = dbMapString(f.raw, "host")
	if f.host == "" {
		f.host = "localhost"
	}
	f.port = dbMapNumbers(f.raw, "port")
	f.database = dbMapString(f.raw, "database")
	f.user = dbMapString(f.raw, "user")
	f.file = dbMapString(f.raw, "file")
	f.comment = dbMapString(f.raw, "comment")
	f.color = dbMapString(f.raw, "color")
	f.url = dbMapString(f.raw, "url")
	f.urlOnly = dbMapBool(f.raw, "urlOnly")
	f.sslMode = dbMapString(f.raw, "ssl")
	if f.sslMode == "" {
		f.sslMode = "prefer"
	}
	f.sslCA = dbMapString(f.raw, "sslCaFile")
	f.sslCert = dbMapString(f.raw, "sslCertFile")
	f.sslKey = dbMapString(f.raw, "sslKeyFile")
	f.savePassword = dbMapBool(f.raw, "savePassword")
	f.readOnly = dbMapBool(f.raw, "readOnly")

	options := dbMapObject(f.raw, "options")
	f.transaction = dbMapString(options, "transaction")
	if f.transaction == "" {
		f.transaction = "auto"
	}
	f.defaultSchema = dbMapString(options, "defaultSchema")
	f.timeZone = dbMapString(options, "timeZone")
	f.startupScript = dbMapString(options, "startupScript")
	f.keepAlive = dbMapNumbers(options, "keepAliveSecs")
	f.autoDisconnect = dbMapNumbers(options, "autoDisconnectSecs")
	f.singleDatabase = dbMapBool(options, "singleDatabase")

	introspection := dbMapObject(f.raw, "introspection")
	f.allDatabases = dbMapBool(introspection, "allDatabases")
	f.databasePatterns = dbJoinPatterns(introspection["databases"])
	f.allSchemas = dbMapBool(introspection, "allSchemas")
	f.schemaPatterns = dbJoinPatterns(introspection["schemas"])
	f.showSystem = dbMapBool(introspection, "showSystem")
	f.showTemplates = dbMapBool(introspection, "showTemplates")
	f.objectFilter = dbMapString(introspection, "objectFilter")

	if ssh, ok := f.raw["ssh"].(map[string]any); ok {
		f.sshEnabled = dbMapBool(ssh, "enabled")
		f.sshHost = dbMapString(ssh, "host")
		f.sshPort = dbMapNumbers(ssh, "port")
		f.sshUser = dbMapString(ssh, "user")
		f.sshAuth = dbMapString(ssh, "auth")
		if f.sshAuth == "" {
			f.sshAuth = "openSsh"
		}
		f.sshKey = dbMapString(ssh, "keyFile")
		f.saveSecret = dbMapBool(ssh, "saveSecret")
	} else {
		f.sshAuth = "openSsh"
	}
	f.properties = dbPropertiesText(f.raw["properties"])
}

func dbJoinPatterns(value any) string {
	items, ok := value.([]any)
	if !ok {
		if stringsList, ok := value.([]string); ok {
			return strings.Join(stringsList, "\n")
		}
		return ""
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, fmt.Sprint(item))
	}
	return strings.Join(out, "\n")
}

func dbSplitPatterns(value string) []string {
	out := []string{}
	for _, line := range strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == ',' || r == ' '
	}) {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func dbPropertiesText(value any) string {
	items, ok := value.([]any)
	if !ok {
		return ""
	}
	lines := []string{}
	for _, item := range items {
		if property, ok := item.(map[string]any); ok {
			lines = append(lines, dbMapString(property, "name")+"="+dbMapString(property, "value"))
		}
	}
	return strings.Join(lines, "\n")
}

func dbParseProperties(text string) []map[string]any {
	out := []map[string]any{}
	for _, line := range strings.Split(text, "\n") {
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) == "" {
			continue
		}
		out = append(out, map[string]any{"name": strings.TrimSpace(name), "value": strings.TrimSpace(value)})
	}
	return out
}

func dbFormNumber(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return nil
	}
	return n
}

func (f *dbSourceForm) apply() {
	if f.raw == nil {
		f.raw = map[string]any{}
	}
	f.raw["id"] = f.id
	f.raw["name"] = strings.TrimSpace(f.name)
	f.raw["driver"] = f.driver
	f.raw["vendor"] = f.vendor
	f.raw["host"] = strings.TrimSpace(f.host)
	f.raw["port"] = dbFormNumber(f.port)
	f.raw["database"] = strings.TrimSpace(f.database)
	f.raw["user"] = strings.TrimSpace(f.user)
	f.raw["file"] = nullableString(strings.TrimSpace(f.file))
	f.raw["comment"] = strings.TrimSpace(f.comment)
	if f.color == "" || f.color == "none" {
		f.raw["color"] = nil
	} else {
		f.raw["color"] = f.color
	}
	f.raw["url"] = strings.TrimSpace(f.url)
	f.raw["urlOnly"] = f.urlOnly
	f.raw["ssl"] = f.sslMode
	f.raw["sslCaFile"] = strings.TrimSpace(f.sslCA)
	f.raw["sslCertFile"] = strings.TrimSpace(f.sslCert)
	f.raw["sslKeyFile"] = strings.TrimSpace(f.sslKey)
	f.raw["savePassword"] = f.savePassword
	f.raw["readOnly"] = f.readOnly
	f.raw["global"] = dbMapBool(f.raw, "global")

	options := dbMapObject(f.raw, "options")
	options["transaction"] = f.transaction
	options["defaultSchema"] = strings.TrimSpace(f.defaultSchema)
	options["timeZone"] = strings.TrimSpace(f.timeZone)
	options["startupScript"] = f.startupScript
	options["keepAliveSecs"] = dbFormNumber(f.keepAlive)
	options["autoDisconnectSecs"] = dbFormNumber(f.autoDisconnect)
	options["singleDatabase"] = f.singleDatabase

	introspection := dbMapObject(f.raw, "introspection")
	introspection["allDatabases"] = f.allDatabases
	introspection["databases"] = dbSplitPatterns(f.databasePatterns)
	introspection["allSchemas"] = f.allSchemas
	introspection["schemas"] = dbSplitPatterns(f.schemaPatterns)
	introspection["showSystem"] = f.showSystem
	introspection["showTemplates"] = f.showTemplates
	introspection["objectFilter"] = strings.TrimSpace(f.objectFilter)

	if f.sshEnabled || f.sshHost != "" || f.sshUser != "" || f.sshKey != "" {
		f.raw["ssh"] = map[string]any{
			"enabled": f.sshEnabled, "host": strings.TrimSpace(f.sshHost),
			"port": dbFormNumber(f.sshPort), "user": strings.TrimSpace(f.sshUser),
			"auth": f.sshAuth, "keyFile": strings.TrimSpace(f.sshKey),
			"saveSecret": f.saveSecret,
		}
	}
	f.raw["properties"] = dbParseProperties(f.properties)
}

func (f *dbSourceForm) validate() error {
	if strings.TrimSpace(f.name) == "" {
		return fmt.Errorf("enter a data source name")
	}
	if f.driver == "sqlite" && strings.TrimSpace(f.file) == "" {
		return fmt.Errorf("choose a SQLite database file")
	}
	if f.driver != "sqlite" && !f.urlOnly && strings.TrimSpace(f.host) == "" {
		return fmt.Errorf("enter the server host")
	}
	if f.driver != "sqlite" && f.urlOnly && strings.TrimSpace(f.url) == "" {
		return fmt.Errorf("enter the connection URL")
	}
	return nil
}

func (m *Model) BrowseDBSourceFile() {
	f := m.db.sourceForm
	if f == nil {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title: "Choose a Database File",
			Filters: []mygo.FileFilter{
				{Name: "SQLite Databases", Extensions: []string{"db", "sqlite", "sqlite3"}},
			},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.post(func() {
			f.file = paths[0]
			if strings.TrimSpace(f.name) == "" {
				f.name = strings.TrimSuffix(filepathBase(paths[0]), filepathExtension(paths[0]))
			}
			m.invalidate()
		})
	}()
}

func filepathBase(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func filepathExtension(path string) string {
	base := filepathBase(path)
	if i := strings.LastIndex(base, "."); i >= 0 {
		return base[i:]
	}
	return ""
}

func (m *Model) TestDBDataSource() {
	f := m.db.sourceForm
	if f == nil || f.busy {
		return
	}
	f.apply()
	if err := f.validate(); err != nil {
		f.err = err.Error()
		m.invalidate()
		return
	}
	f.busy = true
	f.err = ""
	f.test = ""
	m.invalidate()
	source := dbCloneMap(f.raw)
	var password *string
	if f.password != "" {
		password = &f.password
	}
	var sshSecret *string
	if f.sshEnabled && f.sshSecret != "" {
		sshSecret = &f.sshSecret
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 45*time.Second)
		defer cancel()
		version, err := m.mgr.DBTest(ctx, Window, source, password, sshSecret)
		m.post(func() {
			if m.db == nil || m.db.sourceForm != f {
				return
			}
			f.busy = false
			if err != nil {
				f.err = err.Error()
			} else {
				f.test = "Connection successful"
				if version != "" {
					f.test += ": " + version
				}
			}
			m.invalidate()
		})
	}()
}

func (m *Model) SaveDBDataSource() {
	f := m.db.sourceForm
	if f == nil || f.busy {
		return
	}
	f.apply()
	if err := f.validate(); err != nil {
		f.err = err.Error()
		m.invalidate()
		return
	}
	f.busy = true
	f.err = ""
	m.invalidate()
	source := dbCloneMap(f.raw)
	id := f.id
	password := f.password
	sshSecret := f.sshSecret
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		sources, err := m.mgr.DBSourcesRaw(ctx, Window)
		if err == nil {
			replaced := false
			for i := range sources {
				if fmt.Sprint(sources[i]["id"]) == id {
					sources[i] = source
					replaced = true
					break
				}
			}
			if !replaced {
				sources = append(sources, source)
			}
			err = m.mgr.DBSaveSourcesRaw(ctx, Window, sources)
		}
		if err == nil && password != "" {
			err = m.mgr.DBSetPassword(ctx, Window, id, &password, f.savePassword)
		}
		if err == nil && sshSecret != "" && f.sshEnabled {
			err = m.mgr.DBSetSSHSecret(ctx, Window, id, &sshSecret, f.saveSecret)
		}
		m.post(func() {
			if m.db == nil || m.db.sourceForm != f {
				return
			}
			f.busy = false
			if err != nil {
				f.err = "Could not save the data source: " + err.Error()
			} else {
				m.db.sourceForm = nil
				m.db.source = id
				m.RefreshDBSources()
				if m.db.connected[id] {
					m.ConnectDBSource(id)
				}
			}
			m.invalidate()
		})
	}()
}

func (m *Model) dbSourceFormDialog(c *ui.Context, p *ui.Theme) {
	v := m.db
	if v == nil || v.sourceForm == nil {
		return
	}
	f := v.sourceForm
	open := f.open
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(760).Height(600).Gap(10).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Data Sources and Drivers").FontSize(13).Bold().TextColor(p.Text).SingleLine()
				ui.Spacer(c)
				if f.test != "" {
					ui.Text(c, f.test).FontSize(12).TextColor(p.TextMuted).SingleLine()
				}
			})
			tabs := []string{"General", "Options", "SSH/SSL", "Schemas", "Advanced"}
			selected := tabs[0]
			if f.tab >= 0 && f.tab < len(tabs) {
				selected = tabs[f.tab]
			}
			if choice := ui.Select(c, &selected, tabs).Width(520); choice.Changed() {
				for i, label := range tabs {
					if label == selected {
						f.tab = i
						break
					}
				}
			}
			if f.err != "" {
				ui.Text(c, f.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).Padding(4, 4).Gap(8).Children(func() {
					switch f.tab {
					case 0:
						m.dbSourceGeneral(c, p, f)
					case 1:
						m.dbSourceOptions(c, p, f)
					case 2:
						m.dbSourceSSHSSL(c, p, f)
					case 3:
						m.dbSourceSchemas(c, p, f)
					default:
						m.dbSourceAdvanced(c, p, f)
					}
				})
			})
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Test Connection").Disabled(f.busy).OnClick(func() { m.TestDBDataSource() })
				ui.Spacer(c)
				ui.Button(c, "Cancel").OnClick(func() { m.db.sourceForm = nil; m.invalidate() })
				ui.Button(c, "OK").Disabled(f.busy).OnClick(func() { m.SaveDBDataSource() })
			})
		})
	})
	if !open && m.db != nil && m.db.sourceForm == f {
		m.db.sourceForm = nil
		m.invalidate()
	}
}

func (m *Model) dbSourceGeneral(c *ui.Context, p *ui.Theme, f *dbSourceForm) {
	drivers := []string{"postgres", "mysql", "sqlite", "sqlServer", "clickhouse", "oracle", "snowflake"}
	driverIndex := 0
	for i, driver := range drivers {
		if driver == f.driver {
			driverIndex = i
			break
		}
	}
	ui.Text(c, "Name").FontSize(12).TextColor(p.TextMuted)
	ui.TextArea(c, &f.name).Height(28).SingleLine()
	ui.Text(c, "Type").FontSize(12).TextColor(p.TextMuted)
	if choice := ui.Select(c, &drivers[driverIndex], drivers).Width(260); choice.Changed() {
		f.driver = drivers[driverIndex]
		if f.driver == "sqlite" {
			f.sslMode = "disable"
		}
	}
	ui.Checkbox(c, &f.urlOnly, "Use connection URL")
	if f.urlOnly {
		ui.Text(c, "URL").FontSize(12).TextColor(p.TextMuted)
		ui.TextArea(c, &f.url).Height(28).SingleLine()
	} else if f.driver == "sqlite" {
		ui.Text(c, "File").FontSize(12).TextColor(p.TextMuted)
		ui.Row(c).Gap(8).Children(func() {
			ui.TextArea(c, &f.file).Height(28).SingleLine().Grow(1)
			ui.Button(c, "Browse…").OnClick(func() { m.BrowseDBSourceFile() })
		})
	} else {
		ui.Text(c, "Host").FontSize(12).TextColor(p.TextMuted)
		ui.Row(c).Gap(8).Children(func() {
			ui.TextArea(c, &f.host).Height(28).SingleLine().Grow(1)
			ui.Text(c, "Port").FontSize(12).TextColor(p.TextMuted).Width(40)
			ui.TextArea(c, &f.port).Height(28).SingleLine().Width(100)
		})
		ui.Text(c, "Database").FontSize(12).TextColor(p.TextMuted)
		ui.TextArea(c, &f.database).Height(28).SingleLine()
		ui.Text(c, "User").FontSize(12).TextColor(p.TextMuted)
		ui.TextArea(c, &f.user).Height(28).SingleLine()
		ui.Text(c, "Password").FontSize(12).TextColor(p.TextMuted)
		ui.TextInput(c, &f.password).Height(28).Password()
		ui.Checkbox(c, &f.savePassword, "Save password")
		ui.Checkbox(c, &f.readOnly, "Read only")
	}
	ui.Text(c, "Color").FontSize(12).TextColor(p.TextMuted)
	if choice := ui.Select(c, &f.color, []string{"", "red", "orange", "yellow", "green", "blue", "violet"}).Width(180); choice.Changed() {
		f.apply()
	}
	ui.Text(c, "Comment").FontSize(12).TextColor(p.TextMuted)
	ui.TextArea(c, &f.comment).Height(56)
}

func (m *Model) dbSourceOptions(c *ui.Context, p *ui.Theme, f *dbSourceForm) {
	ui.Text(c, "Transaction control").FontSize(12).TextColor(p.TextMuted)
	ui.Select(c, &f.transaction, []string{"auto", "manual"}).Width(180)
	ui.Text(c, "Default schema").FontSize(12).TextColor(p.TextMuted)
	ui.TextArea(c, &f.defaultSchema).Height(28).SingleLine()
	ui.Text(c, "Session time zone").FontSize(12).TextColor(p.TextMuted)
	ui.TextArea(c, &f.timeZone).Height(28).SingleLine()
	ui.Text(c, "Startup script").FontSize(12).TextColor(p.TextMuted)
	ui.TextArea(c, &f.startupScript).Height(90).Font("monospace").FontSize(12)
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, "Keep-alive (seconds)").FontSize(12).TextColor(p.TextMuted).Grow(1)
		ui.TextArea(c, &f.keepAlive).Height(28).SingleLine().Width(100)
	})
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, "Auto-disconnect (seconds)").FontSize(12).TextColor(p.TextMuted).Grow(1)
		ui.TextArea(c, &f.autoDisconnect).Height(28).SingleLine().Width(100)
	})
	ui.Checkbox(c, &f.singleDatabase, "Show only the configured database")
}

func (m *Model) dbSourceSSHSSL(c *ui.Context, p *ui.Theme, f *dbSourceForm) {
	ui.Checkbox(c, &f.sshEnabled, "Use SSH tunnel")
	if f.sshEnabled {
		ui.Text(c, "SSH host").FontSize(12).TextColor(p.TextMuted)
		ui.Row(c).Gap(8).Children(func() {
			ui.TextArea(c, &f.sshHost).Height(28).SingleLine().Grow(1)
			ui.Text(c, "Port").FontSize(12).TextColor(p.TextMuted).Width(40)
			ui.TextArea(c, &f.sshPort).Height(28).SingleLine().Width(100)
		})
		ui.Text(c, "SSH user").FontSize(12).TextColor(p.TextMuted)
		ui.TextArea(c, &f.sshUser).Height(28).SingleLine()
		ui.Text(c, "Authentication").FontSize(12).TextColor(p.TextMuted)
		ui.Select(c, &f.sshAuth, []string{"openSsh", "password", "keyPair"}).Width(180)
		if f.sshAuth == "keyPair" {
			ui.Text(c, "Private key").FontSize(12).TextColor(p.TextMuted)
			ui.TextArea(c, &f.sshKey).Height(28).SingleLine()
		}
		if f.sshAuth != "openSsh" {
			label := "SSH secret"
			if f.sshAuth == "password" {
				label = "SSH password"
			} else {
				label = "Key passphrase"
			}
			ui.Text(c, label).FontSize(12).TextColor(p.TextMuted)
			ui.TextInput(c, &f.sshSecret).Height(28).Password()
		}
		ui.Checkbox(c, &f.saveSecret, "Save SSH secret")
	}
	ui.Text(c, "SSL mode").FontSize(12).TextColor(p.TextMuted)
	ui.Select(c, &f.sslMode, []string{"disable", "prefer", "require", "verifyCa", "verifyFull"}).Width(180)
	if f.sslMode != "disable" {
		ui.Text(c, "CA file").FontSize(12).TextColor(p.TextMuted)
		ui.TextArea(c, &f.sslCA).Height(28).SingleLine()
		ui.Text(c, "Client certificate").FontSize(12).TextColor(p.TextMuted)
		ui.TextArea(c, &f.sslCert).Height(28).SingleLine()
		ui.Text(c, "Client key").FontSize(12).TextColor(p.TextMuted)
		ui.TextArea(c, &f.sslKey).Height(28).SingleLine()
	}
}

func (m *Model) dbSourceSchemas(c *ui.Context, p *ui.Theme, f *dbSourceForm) {
	ui.Checkbox(c, &f.allDatabases, "All databases")
	if !f.allDatabases {
		ui.Text(c, "Database patterns").FontSize(12).TextColor(p.TextMuted)
		ui.TextArea(c, &f.databasePatterns).Height(70)
	}
	ui.Checkbox(c, &f.allSchemas, "All schemas")
	if !f.allSchemas {
		ui.Text(c, "Schema patterns").FontSize(12).TextColor(p.TextMuted)
		ui.TextArea(c, &f.schemaPatterns).Height(70)
	}
	ui.Checkbox(c, &f.showSystem, "Show system schemas")
	ui.Checkbox(c, &f.showTemplates, "Show template databases")
	ui.Text(c, "Object filter").FontSize(12).TextColor(p.TextMuted)
	ui.TextArea(c, &f.objectFilter).Height(28).SingleLine()
}

func (m *Model) dbSourceAdvanced(c *ui.Context, p *ui.Theme, f *dbSourceForm) {
	ui.Text(c, "Driver properties (name=value)").FontSize(12).TextColor(p.TextMuted)
	ui.TextArea(c, &f.properties).Height(240).Font("monospace").FontSize(12)
}
