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

// DBDatabases is the database selected by a connection and the databases the
// server lets it switch to. Connections use the target "<id>:<database>".
type DBDatabases struct {
	Current string   `json:"current"`
	Names   []string `json:"names"`
}

// DBSessionContext is the warehouse and role of a Snowflake session and the
// values it may switch to.
type DBSessionContext struct {
	Warehouse  string   `json:"warehouse"`
	Role       string   `json:"role"`
	Warehouses []string `json:"warehouses"`
	Roles      []string `json:"roles"`
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

// DBColumnInfo is one column of a table, as the table's details report it.
type DBColumnInfo struct {
	Name       string  `json:"name"`
	DataType   string  `json:"dataType"`
	Nullable   bool    `json:"nullable"`
	Default    *string `json:"default"`
	PrimaryKey bool    `json:"primaryKey"`
}

// DBTableDetails is the schema of a table: its columns, indexes, foreign
// keys and triggers.
type DBTableDetails struct {
	Columns []DBColumnInfo `json:"columns"`
	Indexes []struct {
		Name    string   `json:"name"`
		Unique  bool     `json:"unique"`
		Columns []string `json:"columns"`
	} `json:"indexes"`
	ForeignKeys []struct {
		Schema     string   `json:"schema"`
		Table      string   `json:"table"`
		Columns    []string `json:"columns"`
		RefSchema  string   `json:"refSchema"`
		RefTable   string   `json:"refTable"`
		RefColumns []string `json:"refColumns"`
	} `json:"foreignKeys"`
	Triggers []struct {
		Name   string `json:"name"`
		Timing string `json:"timing"`
		Events string `json:"events"`
	} `json:"triggers"`
}

// DBCatalogSchema is a schema in a saved Snowflake catalog.
type DBCatalogSchema struct {
	Name   string    `json:"name"`
	Tables []DBTable `json:"tables"`
}

// DBCatalogDatabase is one database in a saved introspection catalog.
type DBCatalogDatabase struct {
	Name    string            `json:"name"`
	ReadAt  *float64          `json:"readAt"`
	Schemas []DBCatalogSchema `json:"schemas"`
}

// DBsavedCatalog is the catalog the backend retains for completion and the
// tree when a data source is disconnected.
type DBSavedCatalog struct {
	Databases []DBCatalogDatabase `json:"databases"`
}

// DBRoutine is a function or procedure in a schema.
type DBRoutine struct {
	Name      string  `json:"name"`
	Kind      string  `json:"kind"`
	Arguments string  `json:"arguments"`
	Returns   *string `json:"returns"`
}

// DBSequence is a sequence in a schema.
type DBSequence struct {
	Name     string `json:"name"`
	DataType string `json:"dataType"`
}

// DBPackage is an Oracle package in a schema.
type DBPackage struct {
	Name string `json:"name"`
}

// DBOtherObject is a vendor-specific schema object such as a Snowflake stage.
type DBOtherObject struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

// DBSchemaObjects lists everything below tables and views in the database
// tree.
type DBSchemaObjects struct {
	Routines  []DBRoutine     `json:"routines"`
	Packages  []DBPackage     `json:"packages"`
	Sequences []DBSequence    `json:"sequences"`
	Others    []DBOtherObject `json:"others"`
}

// DBCompletionColumn is one column of the completion model.
type DBCompletionColumn struct {
	Name       string `json:"name"`
	DataType   string `json:"dataType"`
	Nullable   bool   `json:"nullable"`
	PrimaryKey bool   `json:"primaryKey"`
	HasDefault bool   `json:"hasDefault"`
}

// DBCompletionTable is one table of the completion model.
type DBCompletionTable struct {
	Schema  string               `json:"schema"`
	Name    string               `json:"name"`
	Kind    string               `json:"kind"`
	Columns []DBCompletionColumn `json:"columns"`
}

// DBForeignKey is one relationship in the completion model.
type DBForeignKey struct {
	Schema     string   `json:"schema"`
	Table      string   `json:"table"`
	Columns    []string `json:"columns"`
	RefSchema  string   `json:"refSchema"`
	RefTable   string   `json:"refTable"`
	RefColumns []string `json:"refColumns"`
}

// DBCompletion is the model SQL completion and name resolution use.
type DBCompletion struct {
	DefaultSchema string              `json:"defaultSchema"`
	Tables        []DBCompletionTable `json:"tables"`
	ForeignKeys   []DBForeignKey      `json:"foreignKeys"`
}

// DBCellValue is one column value of a pending row edit.
type DBCellValue struct {
	Column string  `json:"column"`
	Value  *string `json:"value"`
}

// DBRowEdit is OpenIDEA's tagged update, insert or delete row edit.
type DBRowEdit struct {
	Kind   string        `json:"kind"`
	Key    []DBCellValue `json:"key,omitempty"`
	Values []DBCellValue `json:"values,omitempty"`
}

// DBSortKey orders a table page.
type DBSortKey struct {
	Column     string `json:"column"`
	Descending bool   `json:"descending"`
}

// DBTablePage is one page of a table's rows with its details.
type DBTablePage struct {
	Result  DBQueryResult  `json:"result"`
	Details DBTableDetails `json:"details"`
	Offset  float64        `json:"offset"`
}

// DBTablePageData reads one page of a table's rows and details; request is
// the backend request id, so a long read can be cancelled.
func (m *Manager) DBTablePageData(ctx context.Context, window string, request uint32, id, schema, table string, offset, limit uint32) (DBTablePage, error) {
	return m.DBTablePageQuery(ctx, window, request, id, schema, table, nil, nil, offset, limit)
}

// DBTablePageQuery reads one page with an optional filter and sort order.
func (m *Manager) DBTablePageQuery(ctx context.Context, window string, request uint32, id, schema, table string, filter *string, sort []DBSortKey, offset, limit uint32) (DBTablePage, error) {
	if sort == nil {
		sort = []DBSortKey{}
	}
	raw, err := m.Invoke(ctx, window, "db_table_page", map[string]any{
		"request": request, "id": id, "schema": schema, "table": table,
		"filter": filter, "offset": offset, "limit": limit, "sort": sort,
	})
	if err != nil {
		return DBTablePage{}, err
	}
	var out DBTablePage
	if err := json.Unmarshal(raw, &out); err != nil {
		return DBTablePage{}, err
	}
	return out, nil
}

// DBTableDDL is the CREATE statement of a table.
func (m *Manager) DBTableDDL(ctx context.Context, window, id, schema, table string) (string, error) {
	raw, err := m.Invoke(ctx, window, "db_ddl", map[string]any{"id": id, "schema": schema, "table": table})
	if err != nil {
		return "", err
	}
	var out string
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return out, nil
}

// DBTableDetailsData reads the columns, keys, indexes and triggers of table.
func (m *Manager) DBTableDetailsData(ctx context.Context, window, id, schema, table string) (DBTableDetails, error) {
	raw, err := m.Invoke(ctx, window, "db_table_details", map[string]any{
		"id": id, "schema": schema, "table": table,
	})
	if err != nil {
		return DBTableDetails{}, err
	}
	var out DBTableDetails
	if err := json.Unmarshal(raw, &out); err != nil {
		return DBTableDetails{}, err
	}
	return out, nil
}

// DBHistoryEntry is one statement the console ran; Time is Unix milliseconds.
type DBHistoryEntry struct {
	Text string  `json:"text"`
	Time float64 `json:"time"`
}

// DBPlanNode is one operation of an explained statement; Parent indexes the
// plan's node list (roots: nil).
type DBPlanNode struct {
	Parent     *uint32 `json:"parent"`
	Operation  string  `json:"operation"`
	Params     string  `json:"params"`
	Properties []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"properties"`
}

// DBHistory lists the statements run against source id, newest first.
func (m *Manager) DBHistory(ctx context.Context, window, id string) ([]DBHistoryEntry, error) {
	raw, err := m.Invoke(ctx, window, "db_history", map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	var out []DBHistoryEntry
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DBExplain is the plan of statement without running it.
func (m *Manager) DBExplain(ctx context.Context, window string, request uint32, id, statement, schema string) ([]DBPlanNode, error) {
	args := map[string]any{"request": request, "id": id, "statement": statement}
	if schema != "" {
		args["schema"] = schema
	}
	raw, err := m.Invoke(ctx, window, "db_explain", args)
	if err != nil {
		return nil, err
	}
	var out []DBPlanNode
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
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

// DBDatabases lists the switchable databases of a server connection.
func (m *Manager) DBDatabases(ctx context.Context, window, id string) (DBDatabases, error) {
	raw, err := m.Invoke(ctx, window, "db_databases", map[string]any{"id": id})
	if err != nil {
		return DBDatabases{}, err
	}
	var out DBDatabases
	if err := json.Unmarshal(raw, &out); err != nil {
		return DBDatabases{}, err
	}
	return out, nil
}

// DBDatabaseCatalog reads the schemas and tables of one database.
func (m *Manager) DBDatabaseCatalog(ctx context.Context, window, id, database string) ([]DBCatalogSchema, error) {
	raw, err := m.Invoke(ctx, window, "db_database_catalog", map[string]any{
		"id": id, "database": database,
	})
	if err != nil {
		return nil, err
	}
	var out []DBCatalogSchema
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DBSavedCatalog reads introspection retained from an earlier connection.
func (m *Manager) DBSavedCatalog(ctx context.Context, window, id string) (*DBSavedCatalog, error) {
	raw, err := m.Invoke(ctx, window, "db_saved_catalog", map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	if string(raw) == "null" {
		return nil, nil
	}
	var out DBSavedCatalog
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DBRetainCatalog drops saved databases that are no longer selected.
func (m *Manager) DBRetainCatalog(ctx context.Context, window, id string, databases []string) error {
	_, err := m.Invoke(ctx, window, "db_retain_catalog", map[string]any{
		"id": id, "databases": databases,
	})
	return err
}

// DBSessionContext reads the active Snowflake warehouse and role.
func (m *Manager) DBSessionContext(ctx context.Context, window, id string) (DBSessionContext, error) {
	raw, err := m.Invoke(ctx, window, "db_session_context", map[string]any{"id": id})
	if err != nil {
		return DBSessionContext{}, err
	}
	var out DBSessionContext
	if err := json.Unmarshal(raw, &out); err != nil {
		return DBSessionContext{}, err
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

// DBSchemaObjects lists routines, sequences, packages and vendor objects.
func (m *Manager) DBSchemaObjects(ctx context.Context, window, id, schema string) (DBSchemaObjects, error) {
	raw, err := m.Invoke(ctx, window, "db_schema_objects", map[string]any{
		"id": id, "schema": schema,
	})
	if err != nil {
		return DBSchemaObjects{}, err
	}
	var out DBSchemaObjects
	if err := json.Unmarshal(raw, &out); err != nil {
		return DBSchemaObjects{}, err
	}
	return out, nil
}

// DBObjectDDL reads the definition of a routine, trigger, package or
// sequence.
func (m *Manager) DBObjectDDL(ctx context.Context, window, id, schema, kind, name string) (string, error) {
	raw, err := m.Invoke(ctx, window, "db_object_ddl", map[string]any{
		"id": id, "schema": schema, "kind": kind, "name": name,
	})
	if err != nil {
		return "", err
	}
	var out string
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return out, nil
}

// DBCompletion reads the schema model used for SQL completion.
func (m *Manager) DBCompletion(ctx context.Context, window, id string) (DBCompletion, error) {
	raw, err := m.Invoke(ctx, window, "db_completion", map[string]any{"id": id})
	if err != nil {
		return DBCompletion{}, err
	}
	var out DBCompletion
	if err := json.Unmarshal(raw, &out); err != nil {
		return DBCompletion{}, err
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
	return m.DBExecutePage(ctx, window, request, id, statements, 0, maxRows, schema)
}

// DBExecutePage runs statements after skipping skip rows in each result.
func (m *Manager) DBExecutePage(ctx context.Context, window string, request uint32, id string, statements []DBStatement, skip int, maxRows int, schema string) ([]DBOutcome, error) {
	args := map[string]any{
		"request":    request,
		"id":         id,
		"statements": statements,
		"skip":       skip,
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

// DBRemoveHistory removes one statement, or all history with texts == nil.
func (m *Manager) DBRemoveHistory(ctx context.Context, window, id string, texts []string) error {
	var value any
	if texts != nil {
		value = texts
	}
	_, err := m.Invoke(ctx, window, "db_remove_history", map[string]any{
		"id": id, "texts": value,
	})
	return err
}

// DBSetManualCommit switches the source's connections between auto-commit and
// manual transactions.
func (m *Manager) DBSetManualCommit(ctx context.Context, window, id string, manual bool) error {
	_, err := m.Invoke(ctx, window, "db_set_manual_commit", map[string]any{
		"id": id, "manual": manual,
	})
	return err
}

// DBFinishTransaction commits or rolls back the open manual transaction.
func (m *Manager) DBFinishTransaction(ctx context.Context, window, id string, commit bool) error {
	_, err := m.Invoke(ctx, window, "db_finish_transaction", map[string]any{
		"id": id, "commit": commit,
	})
	return err
}

// DBApplyEdits submits table-editor changes in one transaction and returns the
// number of changed rows.
func (m *Manager) DBApplyEdits(ctx context.Context, window, id, schema, table string, edits []DBRowEdit) (*float64, error) {
	raw, err := m.Invoke(ctx, window, "db_apply_edits", map[string]any{
		"id": id, "schema": schema, "table": table, "edits": edits,
	})
	if err != nil {
		return nil, err
	}
	if string(raw) == "null" {
		return nil, nil
	}
	var out float64
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DBConsolePath returns the query console file for id, creating it on first
// use.
func (m *Manager) DBConsolePath(ctx context.Context, window, id string) (string, error) {
	raw, err := m.Invoke(ctx, window, "db_console_path", map[string]any{"id": id})
	if err != nil {
		return "", err
	}
	var out string
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return out, nil
}

// DBSetPassword remembers, or with a nil password forgets, the connection
// password. save stores it in the operating system's password store.
func (m *Manager) DBSetPassword(ctx context.Context, window, id string, password *string, save bool) error {
	_, err := m.Invoke(ctx, window, "db_set_password", map[string]any{
		"id": id, "password": password, "save": save,
	})
	return err
}

// DBSetSSHSecret remembers, or with a nil secret forgets, the SSH password or
// key passphrase.
func (m *Manager) DBSetSSHSecret(ctx context.Context, window, id string, secret *string, save bool) error {
	_, err := m.Invoke(ctx, window, "db_set_ssh_secret", map[string]any{
		"id": id, "secret": secret, "save": save,
	})
	return err
}

// TrustDBSSHHost trusts the key of the host that made the last connection
// attempt fail.
func (m *Manager) TrustDBSSHHost(ctx context.Context, window, id string) error {
	_, err := m.Invoke(ctx, window, "db_trust_ssh_host", map[string]any{"id": id})
	return err
}

// DBTest checks unsaved source settings without replacing the configured
// source.
func (m *Manager) DBTest(ctx context.Context, window string, source map[string]any, password, sshSecret *string) (string, error) {
	raw, err := m.Invoke(ctx, window, "db_test", map[string]any{
		"source": source, "password": password, "sshSecret": sshSecret,
	})
	if err != nil {
		return "", err
	}
	var out string
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
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
