package app

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"mayoi/internal/openidea"
)

// dbTableEditability mirrors OpenIDEA's table-editor gates. A result needs a
// primary key, a writable data source and a driver whose row editor is
// supported.
func dbTableEditability(s *dbTableSheet) (bool, string) {
	if s == nil {
		return false, ""
	}
	if strings.EqualFold(s.driver, "clickhouse") {
		return false, "Use the query console to change ClickHouse data."
	}
	if s.readOnly {
		return false, "Read-only data source."
	}
	for _, column := range s.page.Details.Columns {
		if column.PrimaryKey {
			return true, ""
		}
	}
	return false, "Read-only: the table has no primary key to identify rows."
}

func dbTableHasChanges(s *dbTableSheet) bool {
	return s != nil && (len(s.pending) > 0 || len(s.deletes) > 0 || len(s.inserts) > 0)
}

func dbTablePendingCount(s *dbTableSheet) int {
	if s == nil {
		return 0
	}
	return len(s.pending) + len(s.deletes) + len(s.inserts)
}

func dbTableRowCount(s *dbTableSheet) int {
	if s == nil {
		return 0
	}
	return len(s.page.Result.Rows) + len(s.inserts)
}

func dbTableInserted(s *dbTableSheet, row int) (int, bool) {
	if s == nil || row < len(s.page.Result.Rows) || row >= dbTableRowCount(s) {
		return 0, false
	}
	return row - len(s.page.Result.Rows), true
}

func dbTableCellValue(s *dbTableSheet, row, col int) *string {
	if s == nil || row < 0 || col < 0 || col >= len(s.page.Result.Columns) {
		return nil
	}
	if inserted, ok := dbTableInserted(s, row); ok {
		if key := s.page.Result.Columns[col].Name; s.inserts[inserted] != nil {
			return s.inserts[inserted][key]
		}
		return nil
	}
	if row >= len(s.page.Result.Rows) || col >= len(s.page.Result.Rows[row]) {
		return nil
	}
	return s.page.Result.Rows[row][col]
}

func dbTableDisplayedCell(s *dbTableSheet, row, col int) *string {
	if s == nil || row < 0 || col < 0 || col >= len(s.page.Result.Columns) {
		return nil
	}
	if _, inserted := dbTableInserted(s, row); inserted {
		return dbTableCellValue(s, row, col)
	}
	key := fmt.Sprintf("%d:%d", row, col)
	if value, ok := s.pending[key]; ok {
		return value
	}
	return dbTableCellValue(s, row, col)
}

func dbStringsEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func dbTableSetCellValue(s *dbTableSheet, row, col int, value *string) {
	if s == nil || row < 0 || col < 0 || col >= len(s.page.Result.Columns) {
		return
	}
	name := s.page.Result.Columns[col].Name
	if inserted, ok := dbTableInserted(s, row); ok {
		if s.inserts[inserted] == nil {
			s.inserts[inserted] = map[string]*string{}
		}
		s.inserts[inserted][name] = value
		return
	}
	if row >= len(s.page.Result.Rows) {
		return
	}
	key := fmt.Sprintf("%d:%d", row, col)
	if dbStringsEqual(value, dbTableCellValue(s, row, col)) {
		delete(s.pending, key)
		return
	}
	if s.pending == nil {
		s.pending = map[string]*string{}
	}
	s.pending[key] = value
}

func dbTableResetInsertedCell(s *dbTableSheet, row, col int) {
	if s == nil || row < 0 || col < 0 || col >= len(s.page.Result.Columns) {
		return
	}
	inserted, ok := dbTableInserted(s, row)
	if !ok || s.inserts[inserted] == nil {
		return
	}
	delete(s.inserts[inserted], s.page.Result.Columns[col].Name)
}

func dbTableColumnInfo(s *dbTableSheet, col int) (openidea.DBColumnInfo, bool) {
	if s == nil || col < 0 || col >= len(s.page.Result.Columns) {
		return openidea.DBColumnInfo{}, false
	}
	name := s.page.Result.Columns[col].Name
	for _, column := range s.page.Details.Columns {
		if strings.EqualFold(column.Name, name) {
			return column, true
		}
	}
	return openidea.DBColumnInfo{}, false
}

func dbTableColumnType(s *dbTableSheet, col int) string {
	if column, ok := dbTableColumnInfo(s, col); ok && strings.TrimSpace(column.DataType) != "" {
		return column.DataType
	}
	if s != nil && col >= 0 && col < len(s.page.Result.Columns) {
		return s.page.Result.Columns[col].TypeName
	}
	return ""
}

func dbTableColumnNullable(s *dbTableSheet, col int) bool {
	if column, ok := dbTableColumnInfo(s, col); ok {
		return column.Nullable
	}
	return true
}

func dbTableEditableCell(s *dbTableSheet, row, col int) bool {
	editable, _ := dbTableEditability(s)
	return editable && row >= 0 && row < dbTableRowCount(s) && !s.deletes[row] &&
		col >= 0 && col < len(s.page.Result.Columns)
}

func dbTableStartCellEdit(s *dbTableSheet, row, col int, maximized bool) {
	if !dbTableEditableCell(s, row, col) {
		return
	}
	s.selectedRow, s.selectedCol = row, col
	value := dbTableDisplayedCell(s, row, col)
	text := ""
	if value != nil {
		text = *value
	}
	if maximized || len(text) > dbCellWidth || strings.ContainsAny(text, "\r\n") ||
		dbIsJSONColumn(dbTableColumnType(s, col)) || json.Valid([]byte(text)) {
		s.valueOpen = true
		s.valueText = text
		s.valueFind = ""
		s.valueRO = false
		s.editing = false
		return
	}
	s.editing = true
	s.editRow, s.editCol = row, col
	s.editText = text
}

func dbTableCancelCellEdit(s *dbTableSheet) {
	if s == nil {
		return
	}
	s.editing = false
	s.editRow, s.editCol = -1, -1
	s.editText = ""
}

func dbTableCommitCellEdit(s *dbTableSheet) {
	if s == nil || !s.editing || !dbTableEditableCell(s, s.editRow, s.editCol) {
		if s != nil {
			dbTableCancelCellEdit(s)
		}
		return
	}
	value := s.editText
	dbTableSetCellValue(s, s.editRow, s.editCol, &value)
	dbTableCancelCellEdit(s)
}

func dbTableSelectCell(s *dbTableSheet, row, col int) {
	if s == nil || row < 0 || row >= dbTableRowCount(s) ||
		col < 0 || col >= len(s.page.Result.Columns) {
		return
	}
	if s.editing && (s.editRow != row || s.editCol != col) {
		dbTableCommitCellEdit(s)
	}
	s.selectedRow, s.selectedCol = row, col
}

func dbTableMoveSelection(s *dbTableSheet, rowDelta, colDelta int) {
	if s == nil || len(s.page.Result.Columns) == 0 || dbTableRowCount(s) == 0 {
		return
	}
	if s.editing {
		dbTableCommitCellEdit(s)
	}
	row, col := s.selectedRow, s.selectedCol
	if row < 0 {
		row = 0
	}
	if col < 0 {
		col = 0
	}
	row += rowDelta
	col += colDelta
	if row < 0 {
		row = 0
	}
	if row >= dbTableRowCount(s) {
		row = dbTableRowCount(s) - 1
	}
	if col < 0 {
		col = 0
	}
	if col >= len(s.page.Result.Columns) {
		col = len(s.page.Result.Columns) - 1
	}
	s.selectedRow, s.selectedCol = row, col
}

// dbIsBooleanType follows OpenIDEA's cellEditors.ts, including the MySQL and
// SQLite TINYINT(1) alias and SQL Server BIT.
var dbBooleanTypePattern = regexp.MustCompile(`(?i)^(boolean|bool|tinyint\(1\)|bit)$`)

func dbIsBooleanType(columnType, driver string) bool {
	kind := strings.TrimSpace(columnType)
	if strings.EqualFold(kind, "bit") &&
		(strings.EqualFold(driver, "postgres") || strings.EqualFold(driver, "mysql")) {
		return false
	}
	return dbBooleanTypePattern.MatchString(kind)
}

func dbBooleanLiterals(driver string) (stringPair, bool) {
	switch strings.ToLower(driver) {
	case "postgres":
		return stringPair{"t", "f"}, true
	case "mysql", "sqlite":
		return stringPair{"1", "0"}, true
	default:
		return stringPair{"true", "false"}, true
	}
}

type stringPair struct {
	trueValue  string
	falseValue string
}

func dbDefaultLiteral(expression *string) (string, bool) {
	if expression == nil {
		return "", false
	}
	text := strings.TrimSpace(*expression)
	for strings.HasPrefix(text, "(") && strings.HasSuffix(text, ")") {
		text = strings.TrimSpace(text[1 : len(text)-1])
	}
	if strings.EqualFold(text, "true") || strings.EqualFold(text, "false") {
		return strings.ToLower(text), true
	}
	if value, ok := dbQuotedDefaultLiteral(text); ok {
		return value, true
	}
	if regexp.MustCompile(`^[-+]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$`).MatchString(text) {
		return text, true
	}
	return "", false
}

func dbQuotedDefaultLiteral(text string) (string, bool) {
	if !strings.HasPrefix(text, "'") {
		return "", false
	}
	var value strings.Builder
	for index := 1; index < len(text); index++ {
		if text[index] != '\'' {
			value.WriteByte(text[index])
			continue
		}
		if index+1 < len(text) && text[index+1] == '\'' {
			value.WriteByte('\'')
			index++
			continue
		}
		rest := strings.TrimSpace(text[index+1:])
		if rest == "" {
			return value.String(), true
		}
		cast := regexp.MustCompile(`(?i)^::\s*(?:[\w]+|"[^"]+")(?:\s*\.\s*(?:[\w]+|"[^"]+"))*(?:\s+(?:varying|precision|with(?:out)?\s+time\s+zone))?(?:\s*\(\d+(?:\s*,\s*\d+)?\))?(?:\[\])?$`)
		if cast.MatchString(rest) {
			return value.String(), true
		}
		return "", false
	}
	return "", false
}

func dbTableDefaultValue(s *dbTableSheet, col int) (string, bool) {
	column, ok := dbTableColumnInfo(s, col)
	if !ok {
		return "", false
	}
	value, usable := dbDefaultLiteral(column.Default)
	if !usable || !dbIsBooleanType(column.DataType, s.driver) {
		return value, usable
	}
	if !regexp.MustCompile(`(?i)^(true|false|t|f|1|0)$`).MatchString(value) {
		return value, usable
	}
	boolValues, _ := dbBooleanLiterals(s.driver)
	if regexp.MustCompile(`(?i)^(true|t|1)$`).MatchString(value) {
		return boolValues.trueValue, true
	}
	return boolValues.falseValue, true
}

func dbTableSetCellNull(s *dbTableSheet) {
	if s == nil || !dbTableEditableCell(s, s.selectedRow, s.selectedCol) ||
		!dbTableColumnNullable(s, s.selectedCol) {
		return
	}
	dbTableSetCellValue(s, s.selectedRow, s.selectedCol, nil)
}

func dbTableSetCellDefault(s *dbTableSheet) {
	if s == nil || !dbTableEditableCell(s, s.selectedRow, s.selectedCol) {
		return
	}
	if _, inserted := dbTableInserted(s, s.selectedRow); inserted {
		dbTableResetInsertedCell(s, s.selectedRow, s.selectedCol)
		return
	}
	value, ok := dbTableDefaultValue(s, s.selectedCol)
	if !ok {
		return
	}
	dbTableSetCellValue(s, s.selectedRow, s.selectedCol, &value)
}

func dbTableToggleBoolean(s *dbTableSheet) {
	if s == nil || !dbTableEditableCell(s, s.selectedRow, s.selectedCol) {
		return
	}
	columnType := dbTableColumnType(s, s.selectedCol)
	if !dbIsBooleanType(columnType, s.driver) {
		return
	}
	values, _ := dbBooleanLiterals(s.driver)
	next := values.trueValue
	cell := dbTableDisplayedCell(s, s.selectedRow, s.selectedCol)
	if cell != nil && strings.EqualFold(*cell, values.trueValue) {
		next = values.falseValue
	}
	dbTableSetCellValue(s, s.selectedRow, s.selectedCol, &next)
}

func dbTableAddRow(s *dbTableSheet) {
	editable, _ := dbTableEditability(s)
	if !editable {
		return
	}
	s.inserts = append(s.inserts, map[string]*string{})
	s.selectedRow = len(s.page.Result.Rows) + len(s.inserts) - 1
	s.selectedCol = 0
	if len(s.page.Result.Columns) > 0 {
		dbTableStartCellEdit(s, s.selectedRow, s.selectedCol, false)
	}
}

func dbTableToggleSelectedDelete(s *dbTableSheet) {
	editable, _ := dbTableEditability(s)
	if !editable || s.selectedRow < 0 || s.selectedRow >= dbTableRowCount(s) {
		return
	}
	if inserted, ok := dbTableInserted(s, s.selectedRow); ok {
		s.inserts = append(s.inserts[:inserted], s.inserts[inserted+1:]...)
		if s.selectedRow >= dbTableRowCount(s) {
			s.selectedRow = dbTableRowCount(s) - 1
		}
		return
	}
	if s.deletes == nil {
		s.deletes = map[int]bool{}
	}
	s.deletes[s.selectedRow] = !s.deletes[s.selectedRow]
}

func dbTableDiscardChanges(s *dbTableSheet) {
	if s == nil {
		return
	}
	s.pending = map[string]*string{}
	s.deletes = map[int]bool{}
	s.inserts = nil
	s.err = ""
	dbTableCancelCellEdit(s)
	s.valueOpen = false
	s.valueRO = false
	s.closeAsk = false
	if s.selectedRow >= dbTableRowCount(s) {
		s.selectedRow = dbTableRowCount(s) - 1
	}
}

func dbTableCurrentColumnName(s *dbTableSheet) string {
	if s == nil || s.selectedCol < 0 || s.selectedCol >= len(s.page.Result.Columns) {
		return ""
	}
	return s.page.Result.Columns[s.selectedCol].Name
}

func dbTableSelectedValue(s *dbTableSheet) (string, bool) {
	value := dbTableDisplayedCell(s, s.selectedRow, s.selectedCol)
	if value == nil {
		return "", false
	}
	return *value, true
}

func dbTableParseCellKey(key string) (int, int, bool) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	row, errRow := strconv.Atoi(parts[0])
	col, errCol := strconv.Atoi(parts[1])
	return row, col, errRow == nil && errCol == nil
}

func dbIsJSONColumn(columnType string) bool {
	return strings.Contains(strings.ToLower(columnType), "json")
}

// DBStartCellEdit starts the grid's inline editor, or the native value editor
// for long, multiline and JSON values as OpenIDEA does.
func (m *Model) DBStartCellEdit(row, col int, maximized bool) {
	s := m.dbTable
	if s == nil {
		return
	}
	if !dbTableEditableCell(s, row, col) {
		dbTableSelectCell(s, row, col)
		m.invalidate()
		return
	}
	dbTableStartCellEdit(s, row, col, maximized)
	m.invalidate()
}

// DBCommitCellEdit commits the current inline value into the pending-edit
// model. Committing the original text removes the pending edit.
func (m *Model) DBCommitCellEdit() {
	if m.dbTable == nil {
		return
	}
	dbTableCommitCellEdit(m.dbTable)
	m.invalidate()
}

// DBCancelCellEdit discards only the text currently in the inline editor.
func (m *Model) DBCancelCellEdit() {
	if m.dbTable == nil {
		return
	}
	dbTableCancelCellEdit(m.dbTable)
	m.invalidate()
}

func (m *Model) DBSelectTableCell(row, col int) {
	if m.dbTable == nil {
		return
	}
	dbTableSelectCell(m.dbTable, row, col)
	m.invalidate()
}

func (m *Model) DBMoveTableCell(rowDelta, colDelta int) {
	if m.dbTable == nil {
		return
	}
	dbTableMoveSelection(m.dbTable, rowDelta, colDelta)
	m.invalidate()
}

func (m *Model) DBAddTableRow() {
	if m.dbTable == nil {
		return
	}
	dbTableAddRow(m.dbTable)
	m.invalidate()
}

func (m *Model) DBToggleDeletedTableRow() {
	if m.dbTable == nil {
		return
	}
	if m.dbTable.editing {
		dbTableCommitCellEdit(m.dbTable)
	}
	dbTableToggleSelectedDelete(m.dbTable)
	m.invalidate()
}

// DBCloseTable protects pending edits behind OpenIDEA's discard confirmation.
func (m *Model) DBCloseTable() {
	s := m.dbTable
	if s == nil {
		return
	}
	if dbTableHasChanges(s) {
		s.closeAsk = true
		m.invalidate()
		return
	}
	m.closeDBTable()
}

func (m *Model) DBDiscardAndCloseTable() {
	s := m.dbTable
	if s == nil {
		return
	}
	dbTableDiscardChanges(s)
	m.closeDBTable()
}

func (m *Model) dbBlockTableNavigation(action string) bool {
	s := m.dbTable
	if s == nil || !dbTableHasChanges(s) {
		return true
	}
	s.err = action + " would discard unsaved row changes. Submit or revert them first."
	m.invalidate()
	return false
}

func (m *Model) DBRefreshTablePage() {
	if !m.dbBlockTableNavigation("Refresh") {
		return
	}
	m.loadDBTablePage()
}

func (m *Model) DBApplyTableFilter() {
	s := m.dbTable
	if s == nil || !m.dbBlockTableNavigation("Changing the filter") {
		return
	}
	s.offset = 0
	m.loadDBTablePage()
}

func (m *Model) DBClearTableFilter() {
	s := m.dbTable
	if s == nil || !m.dbBlockTableNavigation("Clearing the filter") {
		return
	}
	s.filter = ""
	s.offset = 0
	m.loadDBTablePage()
}

func (m *Model) DBSortTableColumn(col int) {
	s := m.dbTable
	if s == nil || col < 0 || col >= len(s.page.Result.Columns) ||
		!m.dbBlockTableNavigation("Changing the sort order") {
		return
	}
	name := s.page.Result.Columns[col].Name
	s.sort = dbNextTableSort(s.sort, name, false)
	s.offset = 0
	m.loadDBTablePage()
}

// dbNextTableSort implements OpenIDEA's asc -> desc -> none cycle. The UI
// currently sends plain header clicks; add supports additional ORDER BY keys.
func dbNextTableSort(sort []openidea.DBSortKey, column string, add bool) []openidea.DBSortKey {
	rest := []openidea.DBSortKey{}
	var current *openidea.DBSortKey
	for _, key := range sort {
		if key.Column == column {
			copy := key
			current = &copy
			if add {
				continue
			}
		}
		if add {
			rest = append(rest, key)
		}
	}
	if current == nil {
		return append(rest, openidea.DBSortKey{Column: column})
	}
	if current.Descending {
		return rest
	}
	return append(rest, openidea.DBSortKey{Column: column, Descending: true})
}
