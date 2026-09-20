package control

import (
	"context"
	"database/sql"
)

type MaskRule struct {
	ResourceID string `json:"resource_id"`
	TableName  string `json:"table_name"`
	ColumnName string `json:"column_name"`
	Mode       string `json:"mode"`
	KeepPrefix *int   `json:"keep_prefix,omitempty"`
	KeepSuffix *int   `json:"keep_suffix,omitempty"`
}

func (s *Store) MaskRules(ctx context.Context, resourceID string) ([]MaskRule, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT resource_id, table_name, column_name, mode, keep_prefix, keep_suffix FROM masking_rules WHERE resource_id=? ORDER BY table_name,column_name", resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MaskRule, 0)
	for rows.Next() {
		var r MaskRule
		if err := rows.Scan(&r.ResourceID, &r.TableName, &r.ColumnName, &r.Mode, &r.KeepPrefix, &r.KeepSuffix); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Updating the resource version invalidates in-flight reads using old policy.
func (s *Store) SaveMaskRule(ctx context.Context, rule MaskRule) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE database_resources SET version=version+1 WHERE id=?", rule.ResourceID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return sql.ErrNoRows
	}
	prefix, suffix := 1, 1
	if rule.KeepPrefix != nil {
		prefix = *rule.KeepPrefix
	}
	if rule.KeepSuffix != nil {
		suffix = *rule.KeepSuffix
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO masking_rules(resource_id,table_name,column_name,mode,keep_prefix,keep_suffix) VALUES (?,?,?,?,?,?)
 ON DUPLICATE KEY UPDATE mode=VALUES(mode),keep_prefix=VALUES(keep_prefix),keep_suffix=VALUES(keep_suffix)`, rule.ResourceID, rule.TableName, rule.ColumnName, rule.Mode, prefix, suffix)
	if err != nil {
		return err
	}
	return tx.Commit()
}
