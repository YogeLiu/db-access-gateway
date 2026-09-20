// Package masking rewrites table sources so expressions and aliases only see
// masked values. Rules do not modify stored target data.
package masking

import (
	"context"
	"database/sql"
	"errors"
	"github.com/xwb1989/sqlparser"
	"github.com/yogel/db-access-gateway/internal/control"
	"strconv"
	"strings"
)

type Field struct {
	Table     string `json:"table_name"`
	Column    string `json:"column_name"`
	Type      string `json:"data_type"`
	TableType string `json:"table_type"`
}

func Schema(ctx context.Context, db *sql.DB, database string, table ...string) ([]Field, error) {
	query := `SELECT c.TABLE_NAME,c.COLUMN_NAME,c.DATA_TYPE,t.TABLE_TYPE
 FROM information_schema.COLUMNS c JOIN information_schema.TABLES t
 ON t.TABLE_SCHEMA=c.TABLE_SCHEMA AND t.TABLE_NAME=c.TABLE_NAME
 WHERE c.TABLE_SCHEMA=?`
	args := []any{database}
	if len(table) > 0 {
		query += " AND c.TABLE_NAME=?"
		args = append(args, table[0])
	}
	rows, err := db.QueryContext(ctx, query+" ORDER BY c.TABLE_NAME,c.ORDINAL_POSITION", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Field, 0)
	for rows.Next() {
		var f Field
		if err := rows.Scan(&f.Table, &f.Column, &f.Type, &f.TableType); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func Active(rules []control.MaskRule) bool {
	for _, r := range rules {
		if r.Mode != "plain" {
			return true
		}
	}
	return false
}

func quote(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }

func expression(column string, rule control.MaskRule) string {
	c := quote(column)
	switch rule.Mode {
	case "plain":
		return c
	case "full":
		return "CASE WHEN " + c + " IS NULL THEN NULL ELSE '******' END"
	case "partial":
		prefix, suffix := 1, 1
		if rule.KeepPrefix != nil {
			prefix = *rule.KeepPrefix
		}
		if rule.KeepSuffix != nil {
			suffix = *rule.KeepSuffix
		}
		if prefix < 0 || prefix > 64 || suffix < 0 || suffix > 64 {
			return ""
		}
		return "CASE WHEN " + c + " IS NULL THEN NULL WHEN CHAR_LENGTH(CAST(" + c + " AS CHAR)) <= " + strconv.Itoa(prefix+suffix) + " THEN '******' ELSE CONCAT(LEFT(CAST(" + c + " AS CHAR)," + strconv.Itoa(prefix) + "),'****',RIGHT(CAST(" + c + " AS CHAR)," + strconv.Itoa(suffix) + ")) END"
	default:
		return "" // Invalid policy must never be treated as plaintext.
	}
}

func Rewrite(text string, rules []control.MaskRule, fields []Field) (string, error) {
	if !Active(rules) {
		return text, nil
	}
	stmt, err := sqlparser.Parse(text)
	if err != nil {
		return "", err
	}
	if _, ok := stmt.(sqlparser.SelectStatement); !ok {
		return "", errors.New("masked resources only support read queries")
	}
	tables := map[string][]Field{}
	for _, f := range fields {
		tables[f.Table] = append(tables[f.Table], f)
	}
	modes := map[string]map[string]control.MaskRule{}
	for _, r := range rules {
		if modes[r.TableName] == nil {
			modes[r.TableName] = map[string]control.MaskRule{}
		}
		modes[r.TableName][strings.ToLower(r.ColumnName)] = r
	}
	err = sqlparser.Walk(func(node sqlparser.SQLNode) (bool, error) {
		// Stored functions may read unmasked data internally.
		if f, ok := node.(*sqlparser.FuncExpr); ok {
			allowed := map[string]bool{"count": true, "sum": true, "avg": true, "min": true, "max": true, "concat": true, "concat_ws": true, "left": true, "right": true, "substring": true, "substr": true, "length": true, "char_length": true, "lower": true, "upper": true, "trim": true, "coalesce": true, "ifnull": true, "nullif": true, "if": true, "round": true, "abs": true, "date": true, "year": true, "month": true, "day": true, "now": true, "group_concat": true, "replace": true, "hex": true}
			if !f.Qualifier.IsEmpty() || !allowed[strings.ToLower(f.Name.String())] {
				return false, errors.New("function is unavailable on masked resources")
			}
		}
		a, ok := node.(*sqlparser.AliasedTableExpr)
		if !ok {
			return true, nil
		}
		t, ok := a.Expr.(sqlparser.TableName)
		if !ok {
			return true, nil
		}
		name := t.Name.String()
		columns, ok := tables[name]
		if !ok || len(columns) == 0 || columns[0].TableType != "BASE TABLE" {
			return false, errors.New("masking requires a known base table; views are unavailable")
		}
		if len(modes[name]) == 0 {
			return true, nil
		}
		parts := make([]string, 0, len(columns))
		for _, c := range columns {
			rule := modes[name][strings.ToLower(c.Column)]
			if rule.Mode == "" {
				rule.Mode = "plain"
			}
			expr := expression(c.Column, rule)
			if expr == "" {
				return false, errors.New("invalid masking policy")
			}
			parts = append(parts, expr+" AS "+quote(c.Column))
		}
		source, err := sqlparser.Parse("SELECT " + strings.Join(parts, ",") + " FROM " + sqlparser.String(t))
		if err != nil {
			return false, errors.New("cannot compile masking policy")
		}
		a.Expr = &sqlparser.Subquery{Select: source.(sqlparser.SelectStatement)}
		if a.As.IsEmpty() {
			a.As = sqlparser.NewTableIdent(name)
		}
		a.Hints = nil
		return false, nil
	}, stmt)
	if err != nil {
		return "", err
	}
	return sqlparser.String(stmt), nil
}
