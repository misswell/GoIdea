package openidea

import (
	"context"
	"encoding/json"
)

// DataSource is one entry of the project's database connections, as the
// database pane lists it.
type DataSource struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Driver   string `json:"driver"`
	Host     string `json:"host"`
	Database string `json:"database"`
	User     string `json:"user"`
	File     string `json:"file"`
}

// DBTable is a table or view of a schema; Kind is "table", "view" or
// "materializedView".
type DBTable struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// DBColumn is one column of a query result.
type DBColumn struct {
	Name     string `json:"name"`
	TypeName string `json:"typeName"`
}

// DBQueryResult is what one statement returned; Rows holds NULL as nil.
type DBQueryResult struct {
	Statement   string      `json:"statement"`
	Columns     []DBColumn  `json:"columns"`
	Rows        [][]*string `json:"rows"`
	Affected    *float64    `json:"affected"`
	HasMore     bool        `json:"hasMore"`
	ElapsedMs   float64     `json:"elapsedMs"`
	ExecutionMs float64     `json:"executionMs"`
}

// DBStatement is one statement the backend split out of the console text;
// From and To are UTF-16 offsets.
type DBStatement struct {
	From uint32 `json:"from"`
	To   uint32 `json:"to"`
	Text string `json:"text"`
}

// DBOutcome is the result or the error of one statement of a run.
type DBOutcome struct {
	Statement DBStatement    `json:"statement"`
	Result    *DBQueryResult `json:"result"`
	Error     string         `json:"error"`
}

// DBSources lists the project's data sources.
func (m *Manager) DBSources(ctx context.Context, window string) ([]DataSource, error) {
	raw, err := m.Invoke(ctx, window, "db_sources", nil)
	if err != nil {
		return nil, err
	}
	var out []DataSource
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DBConnected lists the ids of the sources with an open connection.
func (m *Manager) DBConnected(ctx context.Context, window string) ([]string, error) {
	raw, err := m.Invoke(ctx, window, "db_connected", nil)
	if err != nil {
		return nil, err
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DBConnect opens the connection of source id.
func (m *Manager) DBConnect(ctx context.Context, window, id string) error {
	_, err := m.Invoke(ctx, window, "db_connect", map[string]any{"id": id})
	return err
}

// DBDisconnect closes the connection of source id.
func (m *Manager) DBDisconnect(ctx context.Context, window, id string) error {
	_, err := m.Invoke(ctx, window, "db_disconnect", map[string]any{"id": id})
	return err
}

// DBSchemas lists the schemas of a connected source.
func (m *Manager) DBSchemas(ctx context.Context, window, id string) ([]string, error) {
	raw, err := m.Invoke(ctx, window, "db_schemas", map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DBTables lists the tables and views of a schema.
func (m *Manager) DBTables(ctx context.Context, window, id, schema string) ([]DBTable, error) {
	raw, err := m.Invoke(ctx, window, "db_tables", map[string]any{"id": id, "schema": schema})
	if err != nil {
		return nil, err
	}
	var out []DBTable
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DBStatements splits text[from:to] (UTF-16 offsets) into statements.
func (m *Manager) DBStatements(ctx context.Context, window, id, text string, from, to uint32) ([]DBStatement, error) {
	raw, err := m.Invoke(ctx, window, "db_statements", map[string]any{
		"id": id, "text": text, "from": from, "to": to,
	})
	if err != nil {
		return nil, err
	}
	var out []DBStatement
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DBExecute runs statements in order, keeping maxRows of each result. request
// is the backend request id, so a long run can be cancelled.
func (m *Manager) DBExecute(ctx context.Context, window string, request uint32, id string, statements []DBStatement, maxRows int, schema string) ([]DBOutcome, error) {
	args := map[string]any{
		"request":    request,
		"id":         id,
		"statements": statements,
		"maxRows":    maxRows,
	}
	if schema != "" {
		args["schema"] = schema
	}
	raw, err := m.Invoke(ctx, window, "db_execute", args)
	if err != nil {
		return nil, err
	}
	var out []DBOutcome
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DBSourcesRaw lists the project's data sources as the backend stores them,
// so a caller can save the list back with every field intact.
func (m *Manager) DBSourcesRaw(ctx context.Context, window string) ([]map[string]any, error) {
	raw, err := m.Invoke(ctx, window, "db_sources", nil)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DBSaveSourcesRaw replaces the project's data sources.
func (m *Manager) DBSaveSourcesRaw(ctx context.Context, window string, sources []map[string]any) error {
	_, err := m.Invoke(ctx, window, "db_save_sources", map[string]any{"sources": sources})
	return err
}

// DBSaveSQLiteSource adds (or replaces by id) a SQLite data source pointing at
// file, keeping the project's other sources untouched.
func (m *Manager) DBSaveSQLiteSource(ctx context.Context, window, id, name, file string) error {
	sources, err := m.DBSourcesRaw(ctx, window)
	if err != nil {
		return err
	}
	next := make([]map[string]any, 0, len(sources)+1)
	for _, src := range sources {
		if src["id"] != id {
			next = append(next, src)
		}
	}
	next = append(next, map[string]any{
		"id": id, "name": name, "driver": "sqlite", "file": file,
	})
	return m.DBSaveSourcesRaw(ctx, window, next)
}
