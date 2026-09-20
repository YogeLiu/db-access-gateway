package sqlguard

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/xwb1989/sqlparser"

	"github.com/yogel/db-access-gateway/internal/authz"
)

type Plan struct {
	Action   authz.Action
	SQL      string
	Kind     string
	HasWhere bool
}

var deniedFunctions = map[string]struct{}{
	"benchmark": {}, "get_lock": {}, "load_file": {}, "master_pos_wait": {},
	"release_all_locks": {}, "release_lock": {}, "sleep": {}, "sys_exec": {}, "sys_eval": {},
}

var systemSchemas = map[string]struct{}{
	"information_schema": {}, "mysql": {}, "performance_schema": {}, "sys": {},
}

func Inspect(sqlText, databaseName string) (Plan, error) {
	sqlText = strings.TrimSpace(sqlText)
	if sqlText == "" {
		return Plan{}, errors.New("SQL is required")
	}
	tokenizer := sqlparser.NewStringTokenizer(sqlText)
	statement, err := sqlparser.ParseNext(tokenizer)
	if err != nil {
		return Plan{}, errors.New("SQL parse failed")
	}
	if _, err := sqlparser.ParseNext(tokenizer); err == nil {
		return Plan{}, errors.New("multiple statements are not allowed")
	} else if !errors.Is(err, io.EOF) {
		return Plan{}, errors.New("SQL parse failed")
	}

	plan := Plan{SQL: sqlText}
	switch stmt := statement.(type) {
	case sqlparser.SelectStatement:
		plan.Action, plan.Kind = authz.QueryRead, "select"
	case *sqlparser.Insert:
		if !strings.EqualFold(stmt.Action, sqlparser.InsertStr) {
			return Plan{}, errors.New("REPLACE is not allowed")
		}
		plan.Action, plan.Kind = authz.QueryWrite, "insert"
	case *sqlparser.Update:
		if stmt.Where == nil {
			return Plan{}, errors.New("UPDATE without WHERE is not allowed")
		}
		plan.Action, plan.Kind, plan.HasWhere = authz.QueryWrite, "update", true
	case *sqlparser.Delete:
		if stmt.Where == nil {
			return Plan{}, errors.New("DELETE without WHERE is not allowed")
		}
		plan.Action, plan.Kind, plan.HasWhere = authz.QueryWrite, "delete", true
	default:
		return Plan{}, errors.New("only SELECT, INSERT, UPDATE, and DELETE are allowed")
	}

	if err := sqlparser.Walk(func(node sqlparser.SQLNode) (bool, error) {
		switch value := node.(type) {
		case sqlparser.Comments:
			if len(value) > 0 {
				return false, errors.New("SQL comments and optimizer hints are not allowed")
			}
		case *sqlparser.Select:
			if strings.TrimSpace(value.Lock) != "" {
				return false, errors.New("locking SELECT is not allowed")
			}
			if strings.TrimSpace(value.Hints) != "" {
				return false, errors.New("optimizer hints are not allowed")
			}
		case sqlparser.TableName:
			qualifier := strings.ToLower(strings.TrimSpace(value.Qualifier.String()))
			if qualifier == "" {
				break
			}
			if _, denied := systemSchemas[qualifier]; denied {
				return false, fmt.Errorf("system schema %q is not available", qualifier)
			}
			if !strings.EqualFold(qualifier, strings.TrimSpace(databaseName)) {
				return false, errors.New("cross-database access is not allowed")
			}
		case *sqlparser.FuncExpr:
			name := strings.ToLower(strings.TrimSpace(value.Name.String()))
			if _, denied := deniedFunctions[name]; denied {
				return false, fmt.Errorf("function %q is not allowed", name)
			}
		}
		return true, nil
	}, statement); err != nil {
		return Plan{}, err
	}

	return plan, nil
}
