package kalaazu

import (
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"unicode"
)

var (
	ErrUnsupportedDumpSQL = errors.New("unsupported dump sql")
	ErrMalformedDumpSQL   = errors.New("malformed dump sql")
	ErrUnknownDumpColumn  = errors.New("unknown dump column")
	ErrNullDumpValue      = errors.New("null dump value")
)

type DumpRow struct {
	Table   string
	Columns []string
	Values  map[string]DumpValue
}

type DumpValue struct {
	Raw  string
	Null bool
}

func LoadDumpRows(filesystem fs.FS, path string) ([]DumpRow, error) {
	data, err := fs.ReadFile(filesystem, path)
	if err != nil {
		return nil, fmt.Errorf("read dump %q: %w", path, err)
	}
	return ParseDump(path, string(data))
}

func ParseDump(path string, source string) ([]DumpRow, error) {
	parser := dumpParser{path: path, source: source}
	return parser.parse()
}

func (row DumpRow) Value(column string) (DumpValue, bool) {
	value, ok := row.Values[column]
	return value, ok
}

func (row DumpRow) String(column string) (string, error) {
	value, ok := row.Value(column)
	if !ok {
		return "", fmt.Errorf("%s: %w", column, ErrUnknownDumpColumn)
	}
	if value.Null {
		return "", fmt.Errorf("%s: %w", column, ErrNullDumpValue)
	}
	return value.Raw, nil
}

func (row DumpRow) Int(column string) (int, error) {
	raw, err := row.String(column)
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s=%q: %w", column, raw, err)
	}
	return value, nil
}

func (row DumpRow) Bool(column string) (bool, error) {
	raw, err := row.String(column)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(raw) {
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	default:
		return false, fmt.Errorf("%s=%q: %w", column, raw, ErrMalformedDumpSQL)
	}
}

type dumpParser struct {
	path   string
	source string
	index  int
}

func (parser *dumpParser) parse() ([]DumpRow, error) {
	var rows []DumpRow
	for {
		parser.skipSpaceAndComments()
		if parser.done() {
			if len(rows) == 0 {
				return nil, parser.wrap(ErrUnsupportedDumpSQL)
			}
			return rows, nil
		}
		statementRows, err := parser.parseInsertStatement()
		if err != nil {
			return nil, err
		}
		rows = append(rows, statementRows...)
	}
}

func (parser *dumpParser) parseInsertStatement() ([]DumpRow, error) {
	if !parser.consumeKeyword("INSERT") {
		return nil, parser.wrap(ErrUnsupportedDumpSQL)
	}
	if !parser.consumeKeyword("INTO") {
		return nil, parser.wrap(ErrMalformedDumpSQL)
	}
	table, err := parser.parseIdentifier()
	if err != nil {
		return nil, err
	}
	columns, err := parser.parseColumnList()
	if err != nil {
		return nil, err
	}
	if !parser.consumeKeyword("VALUES") {
		return nil, parser.wrap(ErrMalformedDumpSQL)
	}
	rows, err := parser.parseRows(table, columns)
	if err != nil {
		return nil, err
	}
	parser.skipSpaceAndComments()
	if !parser.consumeByte(';') {
		return nil, parser.wrap(ErrMalformedDumpSQL)
	}
	return rows, nil
}

func (parser *dumpParser) parseColumnList() ([]string, error) {
	parser.skipSpaceAndComments()
	if !parser.consumeByte('(') {
		return nil, parser.wrap(ErrMalformedDumpSQL)
	}
	var columns []string
	for {
		column, err := parser.parseIdentifier()
		if err != nil {
			return nil, err
		}
		columns = append(columns, column)
		parser.skipSpaceAndComments()
		if parser.consumeByte(')') {
			break
		}
		if !parser.consumeByte(',') {
			return nil, parser.wrap(ErrMalformedDumpSQL)
		}
	}
	if len(columns) == 0 {
		return nil, parser.wrap(ErrMalformedDumpSQL)
	}
	return columns, nil
}

func (parser *dumpParser) parseRows(table string, columns []string) ([]DumpRow, error) {
	var rows []DumpRow
	for {
		values, err := parser.parseValueTuple()
		if err != nil {
			return nil, err
		}
		if len(values) != len(columns) {
			return nil, parser.wrapf(ErrMalformedDumpSQL, "table %s values=%d columns=%d", table, len(values), len(columns))
		}
		rowValues := make(map[string]DumpValue, len(columns))
		for index, column := range columns {
			rowValues[column] = values[index]
		}
		rows = append(rows, DumpRow{
			Table:   table,
			Columns: append([]string(nil), columns...),
			Values:  rowValues,
		})
		parser.skipSpaceAndComments()
		if parser.consumeByte(',') {
			continue
		}
		break
	}
	if len(rows) == 0 {
		return nil, parser.wrap(ErrMalformedDumpSQL)
	}
	return rows, nil
}

func (parser *dumpParser) parseValueTuple() ([]DumpValue, error) {
	parser.skipSpaceAndComments()
	if !parser.consumeByte('(') {
		return nil, parser.wrap(ErrMalformedDumpSQL)
	}
	var values []DumpValue
	for {
		value, err := parser.parseValue()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		parser.skipSpaceAndComments()
		if parser.consumeByte(')') {
			break
		}
		if !parser.consumeByte(',') {
			return nil, parser.wrap(ErrMalformedDumpSQL)
		}
	}
	return values, nil
}

func (parser *dumpParser) parseValue() (DumpValue, error) {
	parser.skipSpaceAndComments()
	if parser.done() {
		return DumpValue{}, parser.wrap(ErrMalformedDumpSQL)
	}
	if parser.peek() == '\'' {
		raw, err := parser.parseQuotedString()
		return DumpValue{Raw: raw}, err
	}
	start := parser.index
	for !parser.done() {
		next := parser.peek()
		if next == ',' || next == ')' {
			break
		}
		parser.index++
	}
	raw := strings.TrimSpace(parser.source[start:parser.index])
	if raw == "" {
		return DumpValue{}, parser.wrap(ErrMalformedDumpSQL)
	}
	if strings.EqualFold(raw, "NULL") {
		return DumpValue{Null: true}, nil
	}
	return DumpValue{Raw: raw}, nil
}

func (parser *dumpParser) parseQuotedString() (string, error) {
	if !parser.consumeByte('\'') {
		return "", parser.wrap(ErrMalformedDumpSQL)
	}
	var builder strings.Builder
	for !parser.done() {
		next := parser.peek()
		parser.index++
		if next == '\'' {
			return builder.String(), nil
		}
		if next == '\\' {
			if parser.done() {
				return "", parser.wrap(ErrMalformedDumpSQL)
			}
			escaped := parser.peek()
			parser.index++
			builder.WriteByte(unescapeSQLByte(escaped))
			continue
		}
		builder.WriteByte(next)
	}
	return "", parser.wrap(ErrMalformedDumpSQL)
}

func unescapeSQLByte(value byte) byte {
	switch value {
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	default:
		return value
	}
}

func (parser *dumpParser) parseIdentifier() (string, error) {
	parser.skipSpaceAndComments()
	if parser.done() {
		return "", parser.wrap(ErrMalformedDumpSQL)
	}
	if parser.peek() == '`' {
		parser.index++
		start := parser.index
		for !parser.done() && parser.peek() != '`' {
			parser.index++
		}
		if parser.done() {
			return "", parser.wrap(ErrMalformedDumpSQL)
		}
		identifier := parser.source[start:parser.index]
		parser.index++
		if strings.TrimSpace(identifier) == "" {
			return "", parser.wrap(ErrMalformedDumpSQL)
		}
		return identifier, nil
	}
	start := parser.index
	for !parser.done() {
		next := parser.peek()
		if !(unicode.IsLetter(rune(next)) || unicode.IsDigit(rune(next)) || next == '_') {
			break
		}
		parser.index++
	}
	if start == parser.index {
		return "", parser.wrap(ErrMalformedDumpSQL)
	}
	return parser.source[start:parser.index], nil
}

func (parser *dumpParser) consumeKeyword(keyword string) bool {
	parser.skipSpaceAndComments()
	end := parser.index + len(keyword)
	if end > len(parser.source) || !strings.EqualFold(parser.source[parser.index:end], keyword) {
		return false
	}
	if end < len(parser.source) {
		next := parser.source[end]
		if unicode.IsLetter(rune(next)) || unicode.IsDigit(rune(next)) || next == '_' {
			return false
		}
	}
	parser.index = end
	return true
}

func (parser *dumpParser) consumeByte(value byte) bool {
	parser.skipSpaceAndComments()
	if parser.done() || parser.peek() != value {
		return false
	}
	parser.index++
	return true
}

func (parser *dumpParser) skipSpaceAndComments() {
	for {
		for !parser.done() && unicode.IsSpace(rune(parser.peek())) {
			parser.index++
		}
		if parser.index+1 >= len(parser.source) || parser.source[parser.index] != '-' || parser.source[parser.index+1] != '-' {
			return
		}
		parser.index += 2
		for !parser.done() && parser.peek() != '\n' {
			parser.index++
		}
	}
}

func (parser *dumpParser) done() bool {
	return parser.index >= len(parser.source)
}

func (parser *dumpParser) peek() byte {
	return parser.source[parser.index]
}

func (parser *dumpParser) wrap(err error) error {
	return fmt.Errorf("%s:%d: %w", parser.path, parser.index, err)
}

func (parser *dumpParser) wrapf(err error, format string, args ...any) error {
	return fmt.Errorf("%s:%d: %s: %w", parser.path, parser.index, fmt.Sprintf(format, args...), err)
}
