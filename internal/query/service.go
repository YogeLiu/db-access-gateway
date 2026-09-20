package query

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yogel/db-access-gateway/internal/authz"
	"github.com/yogel/db-access-gateway/internal/control"
	"github.com/yogel/db-access-gateway/internal/id"
	"github.com/yogel/db-access-gateway/internal/masking"
	"github.com/yogel/db-access-gateway/internal/sqlguard"
	"github.com/yogel/db-access-gateway/internal/target"
)

const maxResultBytes = 1 << 20

type Service struct {
	store    *control.Store
	registry *target.Registry
}

func NewService(store *control.Store, registry *target.Registry) *Service {
	return &Service{store: store, registry: registry}
}

type Result struct {
	RequestID    string   `json:"request_id"`
	ResourceKey  string   `json:"resource_key"`
	Action       string   `json:"action"`
	Columns      []string `json:"columns,omitempty"`
	Rows         [][]any  `json:"rows,omitempty"`
	RowCount     int64    `json:"row_count"`
	AffectedRows int64    `json:"affected_rows,omitempty"`
	Truncated    bool     `json:"truncated"`
	DurationMS   int64    `json:"duration_ms"`
}

type Table struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Column struct {
	Name       string  `json:"name"`
	DataType   string  `json:"data_type"`
	ColumnType string  `json:"column_type"`
	Nullable   bool    `json:"nullable"`
	Key        string  `json:"key,omitempty"`
	Default    *string `json:"default,omitempty"`
	Extra      string  `json:"extra,omitempty"`
}

func (s *Service) VisibleResources(ctx context.Context, principal control.Principal) ([]control.Resource, error) {
	return s.store.VisibleResources(ctx, principal.ID)
}

func (s *Service) ListTables(ctx context.Context, principal control.Principal, resourceKey string) ([]Table, error) {
	resource, decision, err := s.authorize(ctx, principal, resourceKey, authz.SchemaRead)
	if err != nil {
		return nil, err
	}
	_ = decision
	db, err := s.registry.Get(ctx, resource, authz.SchemaRead)
	if err != nil {
		return nil, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, effectiveTimeout(resource.StatementTimeoutMS, nil))
	defer cancel()
	rows, err := db.QueryContext(queryCtx, `SELECT TABLE_NAME, TABLE_TYPE FROM information_schema.TABLES
		WHERE TABLE_SCHEMA=? ORDER BY TABLE_NAME`, resource.DatabaseName)
	if err != nil {
		return nil, errors.New("schema query failed")
	}
	defer rows.Close()
	out := make([]Table, 0)
	for rows.Next() {
		var item Table
		if err := rows.Scan(&item.Name, &item.Type); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) DescribeTable(ctx context.Context, principal control.Principal, resourceKey, tableName string) ([]Column, error) {
	if strings.TrimSpace(tableName) == "" {
		return nil, errors.New("table is required")
	}
	resource, _, err := s.authorize(ctx, principal, resourceKey, authz.SchemaRead)
	if err != nil {
		return nil, err
	}
	db, err := s.registry.Get(ctx, resource, authz.SchemaRead)
	if err != nil {
		return nil, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, effectiveTimeout(resource.StatementTimeoutMS, nil))
	defer cancel()
	rules, err := s.store.MaskRules(queryCtx, resource.ID)
	if err != nil {
		return nil, errors.New("masking policy unavailable")
	}
	rows, err := db.QueryContext(queryCtx, `SELECT COLUMN_NAME, DATA_TYPE, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY,
		COLUMN_DEFAULT, EXTRA FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=? AND TABLE_NAME=? ORDER BY ORDINAL_POSITION`,
		resource.DatabaseName, tableName)
	if err != nil {
		return nil, errors.New("schema query failed")
	}
	defer rows.Close()
	out := make([]Column, 0)
	for rows.Next() {
		var item Column
		var nullable string
		var defaultValue sql.NullString
		if err := rows.Scan(&item.Name, &item.DataType, &item.ColumnType, &nullable, &item.Key, &defaultValue, &item.Extra); err != nil {
			return nil, err
		}
		item.Nullable = nullable == "YES"
		if defaultValue.Valid {
			value := defaultValue.String
			for _, rule := range rules {
				if rule.TableName == tableName && strings.EqualFold(rule.ColumnName, item.Name) && rule.Mode != "plain" {
					value = "******"
					break
				}
			}
			item.Default = &value
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) Execute(ctx context.Context, principal control.Principal, resourceKey, sqlText string, params []any, requestedRows uint32, reason string) (Result, error) {
	started := time.Now()
	requestID := id.New()
	resource, err := s.store.GetResourceByKey(ctx, strings.TrimSpace(resourceKey))
	if err != nil || !resource.Enabled {
		return Result{}, errors.New("resource is not available")
	}
	fingerprint := sqlFingerprint(sqlText)
	auditID, err := s.store.InsertAudit(ctx, control.AuditEvent{RequestID: requestID, PrincipalID: &principal.ID,
		ResourceID: &resource.ID, Tool: "query_sql", SQLFingerprint: &fingerprint, SQLText: optionalString(sqlText), Outcome: "RECEIVED"})
	if err != nil {
		return Result{}, errors.New("audit store is unavailable")
	}
	if len([]rune(reason)) > 255 {
		s.auditFailure(auditID, requestID, principal.ID, resource.ID, "query_sql", "REJECTED", "invalid_reason", "", fingerprint, sqlText, "", started)
		return Result{}, errors.New("reason must be at most 255 characters")
	}

	plan, err := sqlguard.Inspect(sqlText, resource.DatabaseName)
	if err != nil {
		s.auditFailure(auditID, requestID, principal.ID, resource.ID, "query_sql", "REJECTED", "sql_rejected", plan.Action, fingerprint, sqlText, reason, started)
		return Result{}, err
	}
	_, decision, err := s.authorizeResource(ctx, principal, resource, plan.Action)
	if err != nil {
		s.auditFailure(auditID, requestID, principal.ID, resource.ID, "query_sql", "DENIED", "forbidden", plan.Action, fingerprint, sqlText, reason, started)
		return Result{}, err
	}
	if decision.Constraints.RequireReason && strings.TrimSpace(reason) == "" {
		s.auditFailure(auditID, requestID, principal.ID, resource.ID, "query_sql", "DENIED", "reason_required", plan.Action, fingerprint, sqlText, reason, started)
		return Result{}, errors.New("this grant requires a reason")
	}

	db, err := s.registry.Get(ctx, resource, plan.Action)
	if err != nil {
		s.auditFailure(auditID, requestID, principal.ID, resource.ID, "query_sql", "FAILED", "target_unavailable", plan.Action, fingerprint, sqlText, reason, started)
		return Result{}, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, effectiveTimeout(resource.StatementTimeoutMS, decision.Constraints.StatementTimeoutMS))
	defer cancel()
	rules, err := s.store.MaskRules(queryCtx, resource.ID)
	if err != nil {
		s.auditFailure(auditID, requestID, principal.ID, resource.ID, "query_sql", "FAILED", "masking_unavailable", plan.Action, fingerprint, sqlText, reason, started)
		return Result{}, errors.New("masking policy unavailable")
	}
	if masking.Active(rules) {
		fields, maskErr := masking.Schema(queryCtx, db, resource.DatabaseName)
		if maskErr == nil {
			plan.SQL, maskErr = masking.Rewrite(plan.SQL, rules, fields)
		}
		if maskErr != nil {
			s.auditFailure(auditID, requestID, principal.ID, resource.ID, "query_sql", "REJECTED", "masking_rejected", plan.Action, fingerprint, sqlText, reason, started)
			return Result{}, errors.New("query cannot be safely executed with masking policy: " + maskErr.Error())
		}
	}
	if plan.Action == authz.QueryWrite {
		return s.executeWrite(queryCtx, principal, resource, plan, params, reason, fingerprint, auditID, requestID, started, db)
	}
	executionSQL := plan.SQL
	plan.SQL = sqlText
	return s.executeRead(queryCtx, principal, resource, plan, executionSQL, params, requestedRows, reason, fingerprint, auditID, requestID, started, db, decision)
}

func (s *Service) executeRead(ctx context.Context, principal control.Principal, resource control.Resource, plan sqlguard.Plan,
	executionSQL string, params []any, requestedRows uint32, reason, fingerprint string, auditID uint64, requestID string, started time.Time, db *sql.DB, decision authz.Decision) (Result, error) {
	limit := effectiveRows(resource.MaxRows, decision.Constraints.RowLimit, requestedRows)
	rows, err := db.QueryContext(ctx, executionSQL, params...)
	if err != nil {
		s.auditFailure(auditID, requestID, principal.ID, resource.ID, "query_sql", "FAILED", "query_failed", plan.Action, fingerprint, plan.SQL, reason, started)
		return Result{}, errors.New("query execution failed")
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		s.auditFailure(auditID, requestID, principal.ID, resource.ID, "query_sql", "FAILED", "query_failed", plan.Action, fingerprint, plan.SQL, reason, started)
		return Result{}, err
	}
	result := Result{RequestID: requestID, ResourceKey: resource.ResourceKey, Action: string(plan.Action), Columns: columns, Rows: make([][]any, 0)}
	var bytesUsed int
	for rows.Next() {
		if uint32(len(result.Rows)) >= limit {
			result.Truncated = true
			break
		}
		values := make([]any, len(columns))
		scan := make([]any, len(columns))
		for i := range values {
			scan[i] = &values[i]
		}
		if err := rows.Scan(scan...); err != nil {
			s.auditFailure(auditID, requestID, principal.ID, resource.ID, "query_sql", "FAILED", "query_failed", plan.Action, fingerprint, plan.SQL, reason, started)
			return Result{}, err
		}
		for i, value := range values {
			values[i] = safeValue(value)
			bytesUsed += len(fmt.Sprint(values[i]))
		}
		if bytesUsed > maxResultBytes {
			result.Truncated = true
			break
		}
		result.Rows = append(result.Rows, values)
	}
	if err := rows.Err(); err != nil {
		s.auditFailure(auditID, requestID, principal.ID, resource.ID, "query_sql", "FAILED", "query_failed", plan.Action, fingerprint, plan.SQL, reason, started)
		return Result{}, errors.New("query execution failed")
	}
	if _, _, err := s.authorizeResource(ctx, principal, resource, plan.Action); err != nil {
		s.auditFailure(auditID, requestID, principal.ID, resource.ID, "query_sql", "DENIED", "policy_changed", plan.Action, fingerprint, plan.SQL, reason, started)
		return Result{}, errors.New("authorization changed while query was running")
	}
	result.RowCount = int64(len(result.Rows))
	result.DurationMS = time.Since(started).Milliseconds()
	action := string(plan.Action)
	count := result.RowCount
	duration := result.DurationMS
	if err := s.store.UpdateAudit(context.WithoutCancel(ctx), auditID, control.AuditEvent{RequestID: requestID, PrincipalID: &principal.ID, ResourceID: &resource.ID,
		Tool: "query_sql", Action: &action, SQLFingerprint: &fingerprint, SQLText: optionalString(plan.SQL), Reason: optionalString(reason), Outcome: "SUCCEEDED", RowCount: &count, DurationMS: &duration}); err != nil {
		return Result{}, errors.New("audit store failed; result withheld")
	}
	return result, nil
}

func (s *Service) executeWrite(ctx context.Context, principal control.Principal, resource control.Resource, plan sqlguard.Plan,
	params []any, reason, fingerprint string, auditID uint64, requestID string, started time.Time, db *sql.DB) (Result, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		s.auditFailure(auditID, requestID, principal.ID, resource.ID, "query_sql", "FAILED", "write_failed", plan.Action, fingerprint, plan.SQL, reason, started)
		return Result{}, errors.New("write transaction failed")
	}
	defer tx.Rollback()
	execResult, err := tx.ExecContext(ctx, plan.SQL, params...)
	if err != nil {
		s.auditFailure(auditID, requestID, principal.ID, resource.ID, "query_sql", "FAILED", "write_failed", plan.Action, fingerprint, plan.SQL, reason, started)
		return Result{}, errors.New("write execution failed")
	}
	affected, err := execResult.RowsAffected()
	if err != nil {
		s.auditFailure(auditID, requestID, principal.ID, resource.ID, "query_sql", "FAILED", "write_failed", plan.Action, fingerprint, plan.SQL, reason, started)
		return Result{}, errors.New("write outcome unavailable")
	}
	if affected > int64(resource.MaxWriteRows) {
		s.auditFailure(auditID, requestID, principal.ID, resource.ID, "query_sql", "REJECTED", "write_row_limit", plan.Action, fingerprint, plan.SQL, reason, started)
		return Result{}, fmt.Errorf("write affected %d rows, exceeding limit %d; rolled back", affected, resource.MaxWriteRows)
	}
	if _, _, err := s.authorizeResource(ctx, principal, resource, authz.QueryWrite); err != nil {
		s.auditFailure(auditID, requestID, principal.ID, resource.ID, "query_sql", "DENIED", "policy_changed", plan.Action, fingerprint, plan.SQL, reason, started)
		return Result{}, errors.New("authorization changed while write was running; rolled back")
	}
	if err := tx.Commit(); err != nil {
		s.auditFailure(auditID, requestID, principal.ID, resource.ID, "query_sql", "FAILED", "commit_failed", plan.Action, fingerprint, plan.SQL, reason, started)
		return Result{}, errors.New("write commit failed")
	}
	result := Result{RequestID: requestID, ResourceKey: resource.ResourceKey, Action: string(plan.Action), AffectedRows: affected, RowCount: affected, DurationMS: time.Since(started).Milliseconds()}
	action := string(plan.Action)
	duration := result.DurationMS
	if err := s.store.UpdateAudit(context.WithoutCancel(ctx), auditID, control.AuditEvent{RequestID: requestID, PrincipalID: &principal.ID, ResourceID: &resource.ID,
		Tool: "query_sql", Action: &action, SQLFingerprint: &fingerprint, SQLText: optionalString(plan.SQL), Reason: optionalString(reason), Outcome: "COMMITTED", RowCount: &affected, DurationMS: &duration}); err != nil {
		return Result{}, errors.New("write committed but final audit failed; do not retry automatically")
	}
	return result, nil
}

func (s *Service) authorize(ctx context.Context, principal control.Principal, resourceKey string, action authz.Action) (control.Resource, authz.Decision, error) {
	resource, err := s.store.GetResourceByKey(ctx, strings.TrimSpace(resourceKey))
	if err != nil {
		return control.Resource{}, authz.Decision{}, errors.New("resource is not available")
	}
	return s.authorizeResource(ctx, principal, resource, action)
}

func (s *Service) authorizeResource(ctx context.Context, principal control.Principal, resource control.Resource, action authz.Action) (control.Resource, authz.Decision, error) {
	current, err := s.store.GetPrincipal(ctx, principal.ID)
	if err != nil || current.Status != "active" {
		return control.Resource{}, authz.Decision{}, errors.New("principal is disabled")
	}
	latest, err := s.store.GetResource(ctx, resource.ID)
	if err != nil || !latest.Enabled || latest.Version != resource.Version {
		return control.Resource{}, authz.Decision{}, errors.New("resource policy changed")
	}
	grants, err := s.store.EffectiveGrants(ctx, principal.ID, resource.ID)
	if err != nil {
		return control.Resource{}, authz.Decision{}, err
	}
	decision := authz.Evaluate(grants, action)
	if !decision.Allowed {
		return control.Resource{}, decision, errors.New("forbidden")
	}
	return latest, decision, nil
}

func effectiveRows(resourceLimit uint32, grantLimit *uint32, requested uint32) uint32 {
	limit := resourceLimit
	if limit == 0 || limit > 1000 {
		limit = 1000
	}
	if grantLimit != nil && *grantLimit < limit {
		limit = *grantLimit
	}
	if requested > 0 && requested < limit {
		limit = requested
	}
	return limit
}

func effectiveTimeout(resourceMS uint32, grantMS *uint32) time.Duration {
	ms := resourceMS
	if ms == 0 || ms > 10000 {
		ms = 10000
	}
	if grantMS != nil && *grantMS < ms {
		ms = *grantMS
	}
	if ms < 100 {
		ms = 100
	}
	return time.Duration(ms) * time.Millisecond
}

func safeValue(value any) any {
	switch v := value.(type) {
	case []byte:
		return string(v)
	case time.Time:
		return v.Format(time.RFC3339Nano)
	default:
		return v
	}
}
func sqlFingerprint(sqlText string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(sqlText)))
	return hex.EncodeToString(sum[:])
}
func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func (s *Service) auditFailure(auditID uint64, requestID, principalID, resourceID, tool, outcome, code string, action authz.Action, fingerprint, sqlText, reason string, started time.Time) {
	duration := time.Since(started).Milliseconds()
	a := string(action)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.store.UpdateAudit(ctx, auditID, control.AuditEvent{RequestID: requestID, PrincipalID: &principalID, ResourceID: &resourceID, Tool: tool,
		Action: &a, SQLFingerprint: &fingerprint, SQLText: optionalString(sqlText), Reason: optionalString(reason), Outcome: outcome, DurationMS: &duration, ErrorCode: &code})
}
