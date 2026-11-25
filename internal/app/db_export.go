package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const (
	dbExportCSV       = "csv"
	dbExportTSV       = "tsv"
	dbExportJSON      = "json"
	dbExportSQL       = "sql"
	dbExportMarkdown  = "markdown"
	dbExportOneRow    = "onerow"
	dbExportPreviewAt = 100
	dbExportCellAt    = 1000
)

type dbExportFormatInfo struct {
	ID        string
	Label     string
	Extension string
}

var dbExportFormats = []dbExportFormatInfo{
	{ID: dbExportCSV, Label: "CSV", Extension: "csv"},
	{ID: dbExportTSV, Label: "TSV", Extension: "tsv"},
	{ID: dbExportJSON, Label: "JSON", Extension: "json"},
	{ID: dbExportSQL, Label: "SQL Inserts", Extension: "sql"},
	{ID: dbExportMarkdown, Label: "Markdown", Extension: "md"},
	{ID: dbExportOneRow, Label: "One-Row", Extension: "txt"},
}

type dbExportOptions struct {
	ColumnHeader bool
	RowHeader    bool
	Transpose    bool
}

var dbExportDefaultOptions = dbExportOptions{ColumnHeader: true}

type dbExportData struct {
	Columns []openidea.DBColumn
	Rows    [][]*string
	Table   string
	Driver  string
}

type dbExportDialog struct {
	open    bool
	data    dbExportData
	source  string
	format  string
	options dbExportOptions
	file    string
	err     string
}

var dbExportClipboardWrite = mygo.Clipboard.WriteText

func dbExportFormatByID(id string) (dbExportFormatInfo, bool) {
	for _, format := range dbExportFormats {
		if format.ID == id {
			return format, true
		}
	}
	return dbExportFormatInfo{}, false
}

func dbExportFormatByLabel(label string) (dbExportFormatInfo, bool) {
	for _, format := range dbExportFormats {
		if format.Label == label {
			return format, true
		}
	}
	return dbExportFormatInfo{}, false
}

func dbExportFormatLabels() []string {
	labels := make([]string, 0, len(dbExportFormats))
	for _, format := range dbExportFormats {
		labels = append(labels, format.Label)
	}
	return labels
}

func dbExportTabular(format string) bool {
	return format == dbExportCSV || format == dbExportTSV || format == dbExportMarkdown
}

func dbExportEffectiveOptions(format string, options dbExportOptions) dbExportOptions {
	if dbExportTabular(format) {
		return options
	}
	return dbExportDefaultOptions
}

func dbExportNumericColumns(columns []openidea.DBColumn, rows [][]*string) []bool {
	numeric := make([]bool, len(columns))
	for columnIndex, column := range columns {
		typeName := strings.ToLower(strings.TrimSpace(column.TypeName))
		if typeName != "" && typeName != "null" && typeName != "unknown" {
			fixed := typeName == "fixed" ||
				strings.HasPrefix(typeName, "fixed ") ||
				strings.HasPrefix(typeName, "fixed(")
			isNumeric := strings.Contains(typeName, "int") ||
				strings.Contains(typeName, "num") ||
				strings.Contains(typeName, "dec") ||
				strings.Contains(typeName, "real") ||
				strings.Contains(typeName, "float") ||
				strings.Contains(typeName, "double") ||
				strings.Contains(typeName, "money") ||
				strings.Contains(typeName, "serial") ||
				typeName == "bit" ||
				fixed
			numeric[columnIndex] = isNumeric &&
				!strings.Contains(typeName, "point") &&
				!strings.Contains(typeName, "interval")
			continue
		}
		sampled := 0
		for _, row := range rows {
			if sampled == 50 {
				break
			}
			if columnIndex >= len(row) || row[columnIndex] == nil {
				continue
			}
			sampled++
			if !dbExportNumberText(*row[columnIndex]) {
				sampled = -1
				break
			}
		}
		numeric[columnIndex] = sampled > 0
	}
	return numeric
}

func dbExportNumberText(text string) bool {
	if text == "" {
		return false
	}
	index := 0
	if text[index] == '-' {
		index++
	}
	digitStart := index
	for index < len(text) && text[index] >= '0' && text[index] <= '9' {
		index++
	}
	if index == digitStart {
		return false
	}
	if index < len(text) && text[index] == '.' {
		index++
		fractionStart := index
		for index < len(text) && text[index] >= '0' && text[index] <= '9' {
			index++
		}
		if index == fractionStart {
			return false
		}
	}
	if index < len(text) && (text[index] == 'e' || text[index] == 'E') {
		index++
		if index < len(text) && (text[index] == '+' || text[index] == '-') {
			index++
		}
		exponentStart := index
		for index < len(text) && text[index] >= '0' && text[index] <= '9' {
			index++
		}
		if index == exponentStart {
			return false
		}
	}
	return index == len(text)
}

func dbExportFiniteNumber(text string) (float64, bool) {
	if strings.TrimSpace(text) == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		return 0, false
	}
	return value, true
}

func dbExportJSONNumber(text string) (string, bool) {
	value, ok := dbExportFiniteNumber(text)
	if !ok {
		return "", false
	}
	if value == 0 {
		return "0", true
	}
	formatted := strconv.FormatFloat(value, 'g', -1, 64)
	if exponent := strings.IndexAny(formatted, "eE"); exponent >= 0 {
		mantissa := formatted[:exponent]
		power := formatted[exponent+1:]
		sign := ""
		if strings.HasPrefix(power, "+") || strings.HasPrefix(power, "-") {
			sign, power = power[:1], power[1:]
		}
		power = strings.TrimLeft(power, "0")
		if power == "" {
			power = "0"
		}
		formatted = mantissa + "e" + sign + power
	}
	return formatted, true
}

type dbExportLayout struct {
	header []string
	rows   [][]*string
}

func dbExportStringPointer(value string) *string { return &value }

func dbExportLaidOut(data dbExportData, options dbExportOptions) dbExportLayout {
	header := make([]string, len(data.Columns))
	for columnIndex, column := range data.Columns {
		header[columnIndex] = column.Name
	}
	rows := make([][]*string, len(data.Rows))
	for rowIndex, row := range data.Rows {
		rows[rowIndex] = append([]*string(nil), row...)
	}
	if options.RowHeader {
		header = append([]string{"#"}, header...)
		for rowIndex := range rows {
			rows[rowIndex] = append([]*string{dbExportStringPointer(strconv.Itoa(rowIndex + 1))}, rows[rowIndex]...)
		}
	}
	if !options.Transpose {
		if !options.ColumnHeader {
			header = nil
		}
		return dbExportLayout{header: header, rows: rows}
	}
	transposed := make([][]*string, len(header))
	for columnIndex, name := range header {
		row := make([]*string, 0, len(rows)+1)
		if options.ColumnHeader {
			row = append(row, dbExportStringPointer(name))
		}
		for _, source := range rows {
			if columnIndex < len(source) {
				row = append(row, source[columnIndex])
			} else {
				row = append(row, nil)
			}
		}
		transposed[columnIndex] = row
	}
	return dbExportLayout{rows: transposed}
}

func dbExportCSVText(header []string, rows [][]*string) string {
	csvField := func(cell *string) string {
		if cell == nil {
			return ""
		}
		value := *cell
		if strings.ContainsAny(value, ",\"\r\n") {
			return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
		}
		return value
	}
	lines := make([]string, 0, len(rows)+1)
	if header != nil {
		fields := make([]string, len(header))
		for fieldIndex, field := range header {
			fields[fieldIndex] = csvField(&field)
		}
		lines = append(lines, strings.Join(fields, ","))
	}
	for _, row := range rows {
		fields := make([]string, len(row))
		for fieldIndex, field := range row {
			fields[fieldIndex] = csvField(field)
		}
		lines = append(lines, strings.Join(fields, ","))
	}
	return strings.Join(lines, "\r\n")
}

func dbExportFlattenTSV(text string) string {
	var out strings.Builder
	out.Grow(len(text))
	inBreak := false
	for _, r := range text {
		if r == '\t' || r == '\r' || r == '\n' {
			if !inBreak {
				out.WriteByte(' ')
				inBreak = true
			}
			continue
		}
		inBreak = false
		out.WriteRune(r)
	}
	return out.String()
}

func dbExportTSVText(header []string, rows [][]*string) string {
	field := func(cell *string) string {
		if cell == nil {
			return ""
		}
		return dbExportFlattenTSV(*cell)
	}
	lines := make([]string, 0, len(rows)+1)
	if header != nil {
		fields := make([]string, len(header))
		for fieldIndex, value := range header {
			fields[fieldIndex] = dbExportFlattenTSV(value)
		}
		lines = append(lines, strings.Join(fields, "\t"))
	}
	for _, row := range rows {
		fields := make([]string, len(row))
		for fieldIndex, value := range row {
			fields[fieldIndex] = field(value)
		}
		lines = append(lines, strings.Join(fields, "\t"))
	}
	return strings.Join(lines, "\n")
}

func dbExportJSONString(text string) string {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(text); err != nil {
		return `""`
	}
	return strings.TrimSuffix(encoded.String(), "\n")
}

func dbExportJSONValue(cell *string, numeric bool) string {
	if cell == nil {
		return "null"
	}
	if numeric {
		if number, ok := dbExportJSONNumber(*cell); ok {
			return number
		}
	}
	return dbExportJSONString(*cell)
}

func dbExportJSONText(data dbExportData, numeric []bool) string {
	if len(data.Rows) == 0 {
		return "[]"
	}
	var out strings.Builder
	out.WriteString("[\n")
	for rowIndex, row := range data.Rows {
		if rowIndex > 0 {
			out.WriteString(",\n")
		}
		out.WriteString("  {\n")
		for columnIndex, column := range data.Columns {
			if columnIndex > 0 {
				out.WriteString(",\n")
			}
			var cell *string
			if columnIndex < len(row) {
				cell = row[columnIndex]
			}
			out.WriteString("    ")
			out.WriteString(dbExportJSONString(column.Name))
			out.WriteString(": ")
			out.WriteString(dbExportJSONValue(cell, numeric[columnIndex]))
		}
		if len(data.Columns) > 0 {
			out.WriteByte('\n')
		}
		out.WriteString("  }")
	}
	out.WriteString("\n]")
	return out.String()
}

func dbExportSQLString(text, driver string) string {
	if driver == "clickhouse" || driver == "snowflake" {
		text = strings.ReplaceAll(text, `\`, `\\`)
	}
	return `'` + strings.ReplaceAll(text, `'`, `''`) + `'`
}

func dbExportSQLLiteral(cell *string, numeric bool, driver string) string {
	if cell == nil {
		return "NULL"
	}
	if numeric {
		if _, ok := dbExportFiniteNumber(*cell); ok {
			return *cell
		}
	}
	return dbExportSQLString(*cell, driver)
}

func dbExportSQLTable(table, driver string) string {
	table = strings.TrimSpace(table)
	if table == "" {
		table = "MY_TABLE"
	}
	parts := strings.Split(table, ".")
	for partIndex, part := range parts {
		parts[partIndex] = dbQuoteIdent(driver, strings.TrimSpace(part))
	}
	return strings.Join(parts, ".")
}

func dbExportSQLText(data dbExportData, numeric []bool) string {
	names := make([]string, len(data.Columns))
	for columnIndex, column := range data.Columns {
		names[columnIndex] = dbQuoteIdent(data.Driver, column.Name)
	}
	head := "INSERT INTO " + dbExportSQLTable(data.Table, data.Driver) +
		" (" + strings.Join(names, ", ") + ") VALUES"
	lines := make([]string, len(data.Rows))
	for rowIndex, row := range data.Rows {
		values := make([]string, len(row))
		for columnIndex, cell := range row {
			numbers := columnIndex < len(numeric) && numeric[columnIndex]
			values[columnIndex] = dbExportSQLLiteral(cell, numbers, data.Driver)
		}
		lines[rowIndex] = head + " (" + strings.Join(values, ", ") + ");"
	}
	return strings.Join(lines, "\n")
}

func dbExportMarkdownCell(cell *string) string {
	if cell == nil {
		return ""
	}
	text := strings.ReplaceAll(*cell, "|", `\|`)
	text = strings.ReplaceAll(text, "\r\n", " ")
	return strings.ReplaceAll(text, "\n", " ")
}

func dbExportMarkdownText(data dbExportData, options dbExportOptions, numeric []bool) string {
	layout := dbExportLaidOut(data, options)
	width := 0
	switch {
	case layout.header != nil:
		width = len(layout.header)
	case len(layout.rows) > 0:
		width = len(layout.rows[0])
	}
	header := layout.header
	if header == nil {
		newHeader := make([]string, width)
		header = newHeader
	}
	align := make([]string, width)
	for alignIndex := range align {
		align[alignIndex] = "---"
	}
	if !options.Transpose && layout.header != nil {
		offset := 0
		if options.RowHeader {
			align[0] = "--:"
			offset = 1
		}
		for columnIndex, isNumeric := range numeric {
			if offset+columnIndex < len(align) && isNumeric {
				align[offset+columnIndex] = "--:"
			}
		}
	}
	lines := make([]string, 0, len(layout.rows)+2)
	lines = append(lines, dbExportMarkdownRow(header))
	lines = append(lines, dbExportMarkdownRow(align))
	for _, row := range layout.rows {
		cells := make([]string, len(row))
		for cellIndex, cell := range row {
			cells[cellIndex] = dbExportMarkdownCell(cell)
		}
		lines = append(lines, dbExportMarkdownRow(cells))
	}
	return strings.Join(lines, "\n")
}

func dbExportMarkdownRow(cells []string) string {
	return "| " + strings.Join(cells, " | ") + " |"
}

func dbExportOneRowText(data dbExportData, numeric []bool) string {
	values := make([]string, 0)
	for _, row := range data.Rows {
		for columnIndex, cell := range row {
			if cell == nil {
				values = append(values, "NULL")
				continue
			}
			isNumeric := columnIndex < len(numeric) && numeric[columnIndex]
			if isNumeric {
				if _, ok := dbExportFiniteNumber(*cell); ok {
					values = append(values, *cell)
					continue
				}
			}
			values = append(values, dbExportSQLString(*cell, ""))
		}
	}
	return strings.Join(values, ", ")
}

func dbExportText(format string, data dbExportData, options dbExportOptions) string {
	effective := dbExportEffectiveOptions(format, options)
	numeric := dbExportNumericColumns(data.Columns, data.Rows)
	switch format {
	case dbExportCSV:
		layout := dbExportLaidOut(data, effective)
		return dbExportCSVText(layout.header, layout.rows)
	case dbExportTSV:
		layout := dbExportLaidOut(data, effective)
		return dbExportTSVText(layout.header, layout.rows)
	case dbExportJSON:
		return dbExportJSONText(data, numeric)
	case dbExportSQL:
		return dbExportSQLText(data, numeric)
	case dbExportMarkdown:
		return dbExportMarkdownText(data, effective, numeric)
	case dbExportOneRow:
		return dbExportOneRowText(data, numeric)
	default:
		return ""
	}
}

func dbExportCellPreview(value string) string {
	units := utf16.Encode([]rune(value))
	if len(units) <= dbExportCellAt {
		return value
	}
	end := dbExportCellAt
	if units[end-1] >= 0xD800 && units[end-1] <= 0xDBFF {
		end--
	}
	return string(utf16.Decode(units[:end])) + "…"
}

func dbExportPreview(format string, data dbExportData, options dbExportOptions) string {
	rowCount := len(data.Rows)
	if rowCount > dbExportPreviewAt {
		rowCount = dbExportPreviewAt
	}
	preview := dbExportData{
		Columns: data.Columns,
		Table:   data.Table,
		Driver:  data.Driver,
		Rows:    make([][]*string, rowCount),
	}
	for rowIndex := 0; rowIndex < rowCount; rowIndex++ {
		row := make([]*string, len(data.Rows[rowIndex]))
		for columnIndex, cell := range data.Rows[rowIndex] {
			if cell != nil {
				row[columnIndex] = dbExportStringPointer(dbExportCellPreview(*cell))
			}
		}
		preview.Rows[rowIndex] = row
	}
	text := dbExportText(format, preview, options)
	if len(data.Rows) > rowCount {
		text += fmt.Sprintf("\n… %d more rows", len(data.Rows)-rowCount)
	}
	return text
}

func dbExportQueryTable(statement string) string {
	depth := 0
	for index := 0; index < len(statement); {
		switch {
		case strings.HasPrefix(statement[index:], "--"):
			end := strings.IndexByte(statement[index:], '\n')
			if end < 0 {
				return ""
			}
			index += end + 1
		case strings.HasPrefix(statement[index:], "/*"):
			end := strings.Index(statement[index+2:], "*/")
			if end < 0 {
				return ""
			}
			index += end + 4
		case statement[index] == '\'':
			next, ok := dbExportSkipQuoted(statement, index, '\'')
			if !ok {
				return ""
			}
			index = next
		case statement[index] == '"' || statement[index] == '`' || statement[index] == '[':
			next, ok := dbExportSkipQuoted(statement, index, statement[index])
			if !ok {
				return ""
			}
			index = next
		case statement[index] == '(':
			depth++
			index++
		case statement[index] == ')':
			if depth > 0 {
				depth--
			}
			index++
		case dbExportIdentifierByte(statement[index]):
			start := index
			for index < len(statement) && dbExportIdentifierByte(statement[index]) {
				index++
			}
			if depth != 0 || !strings.EqualFold(statement[start:index], "from") {
				continue
			}
			table, next, ok := dbExportReadTable(statement, index)
			if ok {
				return table
			}
			index = next
		default:
			index++
		}
	}
	return ""
}

func dbExportIdentifierByte(ch byte) bool {
	return ch == '_' || ch == '$' ||
		ch >= 'a' && ch <= 'z' ||
		ch >= 'A' && ch <= 'Z' ||
		ch >= '0' && ch <= '9'
}

func dbExportSkipQuoted(text string, start int, quote byte) (int, bool) {
	closeQuote := quote
	if quote == '[' {
		closeQuote = ']'
	}
	for index := start + 1; index < len(text); index++ {
		if text[index] != closeQuote {
			continue
		}
		if index+1 < len(text) && text[index+1] == closeQuote {
			index++
			continue
		}
		return index + 1, true
	}
	return len(text), false
}

func dbExportReadTable(text string, start int) (string, int, bool) {
	index := start
	parts := make([]string, 0, 3)
	for len(parts) < 3 {
		for index < len(text) && (text[index] == ' ' || text[index] == '\t' || text[index] == '\r' || text[index] == '\n') {
			index++
		}
		if index >= len(text) || text[index] == '(' {
			return "", index, false
		}
		var part string
		switch text[index] {
		case '"', '`', '[':
			quote := text[index]
			closeQuote := quote
			if quote == '[' {
				closeQuote = ']'
			}
			contentStart := index + 1
			var content strings.Builder
			index = contentStart
			closed := false
			for index < len(text) {
				if text[index] == closeQuote {
					if index+1 < len(text) && text[index+1] == closeQuote {
						content.WriteString(text[contentStart:index])
						content.WriteByte(closeQuote)
						index += 2
						contentStart = index
						continue
					}
					content.WriteString(text[contentStart:index])
					index++
					closed = true
					break
				}
				index++
			}
			if !closed {
				return "", index, false
			}
			part = content.String()
		default:
			partStart := index
			for index < len(text) && dbExportIdentifierByte(text[index]) {
				index++
			}
			if partStart == index {
				return "", index, false
			}
			part = text[partStart:index]
		}
		parts = append(parts, part)
		for index < len(text) && (text[index] == ' ' || text[index] == '\t' || text[index] == '\r' || text[index] == '\n') {
			index++
		}
		if index >= len(text) || text[index] != '.' {
			break
		}
		index++
	}
	if len(parts) == 0 {
		return "", index, false
	}
	return strings.Join(parts, "."), index, true
}

func dbExportSourceLabel(v *dbView, source string) string {
	if v != nil {
		for _, candidate := range v.sources {
			if source == candidate.ID || strings.HasPrefix(source, candidate.ID+":") {
				if candidate.Name != "" {
					return candidate.Name
				}
				return candidate.ID
			}
		}
	}
	return source
}

func dbExportResultSource(v *dbView, index int) string {
	if v != nil && index >= 0 && index < len(v.results) && v.results[index].source != "" {
		return v.results[index].source
	}
	if v != nil {
		return v.source
	}
	return ""
}

func (m *Model) dbExportDataForResult(index int) (dbExportData, bool) {
	if m == nil || m.db == nil || index < 0 || index >= len(m.db.results) {
		return dbExportData{}, false
	}
	result := m.db.results[index]
	for _, outcome := range result.outcomes {
		if outcome.Result == nil || len(outcome.Result.Columns) == 0 {
			continue
		}
		rows := make([][]*string, len(outcome.Result.Rows))
		for rowIndex, row := range outcome.Result.Rows {
			rows[rowIndex] = append([]*string(nil), row...)
		}
		table := dbExportQueryTable(outcome.Statement.Text)
		if table == "" {
			table = "MY_TABLE"
		}
		return dbExportData{
			Columns: append([]openidea.DBColumn(nil), outcome.Result.Columns...),
			Rows:    rows,
			Table:   table,
			Driver:  m.dbDriver(dbExportResultSource(m.db, index)),
		}, true
	}
	return dbExportData{}, false
}

func dbExportResultEnabled(m *Model) bool {
	if m == nil || m.db == nil || m.db.active < 0 {
		return false
	}
	_, ok := m.dbExportDataForResult(m.db.active)
	return ok
}

func dbExportRequestedFormat(arg any) string {
	switch value := arg.(type) {
	case string:
		if _, ok := dbExportFormatByID(value); ok {
			return value
		}
	case map[string]any:
		if format, ok := value["format"].(string); ok {
			if _, valid := dbExportFormatByID(format); valid {
				return format
			}
		}
	}
	return dbExportCSV
}

func dbExportDefaultFileName(table, extension string) string {
	table = strings.TrimSpace(table)
	if table == "" {
		table = "results"
	}
	table = strings.ReplaceAll(strings.ReplaceAll(table, "/", "-"), `\`, "-")
	if table == "" {
		table = "results"
	}
	return table + "." + extension
}

func dbExportReplaceExtension(path, extension string) string {
	if strings.TrimSpace(path) == "" {
		return path
	}
	separator := strings.LastIndexAny(path, `/\`)
	dot := strings.LastIndex(path, ".")
	if dot > separator {
		return path[:dot] + "." + extension
	}
	return path + "." + extension
}

func (m *Model) OpenDBExport(arg any) {
	v := m.db
	if v == nil {
		return
	}
	index := dbResultIndex(arg, v.active)
	data, ok := m.dbExportDataForResult(index)
	if !ok {
		return
	}
	formatID := dbExportRequestedFormat(arg)
	format, _ := dbExportFormatByID(formatID)
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	v.exportDialog = &dbExportDialog{
		open:    true,
		data:    data,
		source:  dbExportSourceLabel(v, dbExportResultSource(v, index)),
		format:  formatID,
		options: dbExportDefaultOptions,
		file:    filepath.Join(home, dbExportDefaultFileName(data.Table, format.Extension)),
	}
	m.invalidate()
}

func (m *Model) DBExportAction(arg any) {
	m.OpenDBExport(arg)
}

func (m *Model) DBExportCopy(arg any) {
	v := m.db
	if v == nil {
		return
	}
	index := dbResultIndex(arg, v.active)
	data, ok := m.dbExportDataForResult(index)
	if !ok {
		return
	}
	formatID := dbExportRequestedFormat(arg)
	format, _ := dbExportFormatByID(formatID)
	dbExportClipboardWrite(dbExportText(formatID, data, dbExportDefaultOptions))
	v.outLog = append(v.outLog, fmt.Sprintf("%s · %d rows copied", format.Label, len(data.Rows)))
	if len(v.outLog) > 200 {
		v.outLog = v.outLog[len(v.outLog)-200:]
	}
	m.notifyInfo("db.export", "Data copied", fmt.Sprintf("%d rows as %s", len(data.Rows), format.Label))
	m.invalidate()
}

func (m *Model) dbExportSetFormat(dialog *dbExportDialog, formatID string) {
	format, ok := dbExportFormatByID(formatID)
	if !ok || formatID == dialog.format {
		return
	}
	dialog.format = formatID
	dialog.file = dbExportReplaceExtension(dialog.file, format.Extension)
	dialog.err = ""
	m.invalidate()
}

func (m *Model) DBExportBrowse() {
	v := m.db
	if v == nil || v.exportDialog == nil {
		return
	}
	dialog := v.exportDialog
	format, _ := dbExportFormatByID(dialog.format)
	parent := m.win
	start := dialog.file
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:            parent,
			Title:             "Export Data",
			ButtonLabel:       "Export",
			DefaultPath:       start,
			CreateDirectories: true,
			Filters: []mygo.FileFilter{{
				Name:       format.Label,
				Extensions: []string{format.Extension},
			}},
		})
		if err != nil || path == "" {
			return
		}
		m.post(func() {
			if m.db != v || m.db.exportDialog != dialog {
				return
			}
			dialog.file = path
			dialog.err = ""
			m.invalidate()
		})
	}()
}

func dbExportWriteFile(path, text string) error {
	return os.WriteFile(path, []byte(text), 0o644)
}

func (m *Model) DBExportToFile() {
	v := m.db
	if v == nil || v.exportDialog == nil {
		return
	}
	dialog := v.exportDialog
	path := strings.TrimSpace(dialog.file)
	if path == "" {
		dialog.err = "Choose an output file"
		m.invalidate()
		return
	}
	options := dbExportEffectiveOptions(dialog.format, dialog.options)
	text := dbExportText(dialog.format, dialog.data, options)
	if err := dbExportWriteFile(path, text); err != nil {
		dialog.err = "Cannot export data: " + err.Error()
		m.invalidate()
		return
	}
	v.exportDialog = nil
	v.outLog = append(v.outLog, fmt.Sprintf("%s · %d rows exported to %s",
		dbExportFormatLabel(dialog.format), len(dialog.data.Rows), path))
	if len(v.outLog) > 200 {
		v.outLog = v.outLog[len(v.outLog)-200:]
	}
	m.notifyInfo("db.export", fmt.Sprintf("%d rows exported", len(dialog.data.Rows)), path)
	m.invalidate()
}

func (m *Model) DBExportToClipboard() {
	v := m.db
	if v == nil || v.exportDialog == nil {
		return
	}
	dialog := v.exportDialog
	options := dbExportEffectiveOptions(dialog.format, dialog.options)
	dbExportClipboardWrite(dbExportText(dialog.format, dialog.data, options))
	v.exportDialog = nil
	v.outLog = append(v.outLog, fmt.Sprintf("%s · %d rows copied",
		dbExportFormatLabel(dialog.format), len(dialog.data.Rows)))
	if len(v.outLog) > 200 {
		v.outLog = v.outLog[len(v.outLog)-200:]
	}
	m.notifyInfo("db.export", "Data copied",
		fmt.Sprintf("%d rows as %s", len(dialog.data.Rows), dbExportFormatLabel(dialog.format)))
	m.invalidate()
}

func dbExportFormatLabel(formatID string) string {
	if format, ok := dbExportFormatByID(formatID); ok {
		return format.Label
	}
	return formatID
}

func (m *Model) dbExportDialog(c *ui.Context, p *ui.Theme) {
	if m.db == nil || m.db.exportDialog == nil {
		return
	}
	dialog := m.db.exportDialog
	open := dialog.open
	nativeModal(c, &open, func() {
		ui.Column(c).Width(860).Height(620).Gap(12).Children(func() {
			ui.Text(c, "Export Data").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			ui.Row(c).Grow(1).MinHeight(0).Gap(16).AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).Width(280).Gap(8).Children(func() {
					ui.Text(c, "Source:").FontSize(12).TextColor(p.TextMuted).SingleLine()
					source := dialog.source
					ui.TextInput(c, &source).Height(28).ReadOnly(true)
					ui.Text(c, "Extractor:").FontSize(12).TextColor(p.TextMuted).SingleLine()
					selectedLabel := dbExportFormatLabel(dialog.format)
					if choice := ui.Select(c, &selectedLabel, dbExportFormatLabels()); choice.Changed() {
						if format, ok := dbExportFormatByLabel(selectedLabel); ok {
							m.dbExportSetFormat(dialog, format.ID)
						}
					}
					effective := dbExportEffectiveOptions(dialog.format, dialog.options)
					transpose := effective.Transpose
					if ui.Checkbox(c, &transpose, "Transpose").
						Disabled(!dbExportTabular(dialog.format)).Changed() {
						dialog.options.Transpose = transpose
						m.invalidate()
					}
					columnHeader := effective.ColumnHeader
					if ui.Checkbox(c, &columnHeader, "Add column header").
						Disabled(!dbExportTabular(dialog.format)).Changed() {
						dialog.options.ColumnHeader = columnHeader
						m.invalidate()
					}
					rowHeader := effective.RowHeader
					if ui.Checkbox(c, &rowHeader, "Add row header").
						Disabled(!dbExportTabular(dialog.format)).Changed() {
						dialog.options.RowHeader = rowHeader
						m.invalidate()
					}
					ui.Text(c, "Output file:").FontSize(12).TextColor(p.TextMuted).SingleLine()
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						file := ui.TextInput(c, &dialog.file).Height(28).Font("monospace").Grow(1)
						if file.Changed() {
							dialog.err = ""
						}
						ui.Button(c, "Browse…").OnClick(m.DBExportBrowse)
					})
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
					ui.Text(c, "Export preview:").FontSize(12).TextColor(p.TextMuted).SingleLine()
					preview := dbExportPreview(dialog.format, dialog.data, dialog.options)
					ui.TextArea(c, &preview).ReadOnly(true).Grow(1).MinHeight(0).
						Font("monospace").FontSize(11)
				})
			})
			if dialog.err != "" {
				ui.Text(c, dialog.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, fmt.Sprintf("%d rows · %s", len(dialog.data.Rows),
					dbExportFormatLabel(dialog.format))).FontSize(12).TextColor(p.TextMuted).SingleLine()
				ui.Spacer(c)
				ui.Button(c, "Cancel").OnClick(func() {
					if m.db != nil && m.db.exportDialog == dialog {
						m.db.exportDialog = nil
					}
					m.invalidate()
				})
				ui.Button(c, "Copy to Clipboard").OnClick(m.DBExportToClipboard)
				ui.Button(c, "Export to File").Disabled(strings.TrimSpace(dialog.file) == "").
					OnClick(m.DBExportToFile)
			})
		})
	})
	if !open && m.db != nil && m.db.exportDialog == dialog {
		m.db.exportDialog = nil
		m.invalidate()
	}
}
