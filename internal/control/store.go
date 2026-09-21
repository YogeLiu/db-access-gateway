package control

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/yogel/db-access-gateway/internal/id"
)

type Store struct {
	db *sql.DB
}

func Open(dsn string) (*Store, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(12)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(10 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) Migrate(ctx context.Context) error {
	for i, statement := range migrations {
		version := int64(i + 1)
		var exists int
		err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&exists)
		if i == 0 && err != nil {
			if _, execErr := s.db.ExecContext(ctx, statement); execErr != nil {
				return fmt.Errorf("create migration table: %w", execErr)
			}
			exists = 0
		} else if err != nil {
			return fmt.Errorf("check migration %d: %w", version, err)
		}
		if exists > 0 {
			continue
		}
		if i > 0 {
			if _, err := s.db.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply migration %d: %w", version, err)
			}
		}
		if _, err := s.db.ExecContext(ctx, "INSERT IGNORE INTO schema_migrations(version) VALUES (?)", version); err != nil {
			return fmt.Errorf("record migration %d: %w", version, err)
		}
	}
	return nil
}

func (s *Store) ListPrincipals(ctx context.Context) ([]Principal, error) {
	rows, err := s.db.QueryContext(ctx, principalSelect+` ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Principal, 0)
	for rows.Next() {
		p, err := scanPrincipal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

const principalSelect = `SELECT id, username, display_name, role, status, password_hash, created_at, updated_at FROM principals`

func scanPrincipal(row scanner) (Principal, error) {
	var p Principal
	var passwordHash sql.NullString
	err := row.Scan(&p.ID, &p.Username, &p.DisplayName, &p.Role, &p.Status, &passwordHash, &p.CreatedAt, &p.UpdatedAt)
	if passwordHash.Valid {
		p.PasswordHash = passwordHash.String
		p.PasswordSet = true
	}
	return p, err
}

func (s *Store) CreatePrincipal(ctx context.Context, username, displayName, passwordHash string) (Principal, error) {
	p := Principal{ID: id.New(), Username: username, DisplayName: displayName, Role: "user", Status: "active", PasswordHash: passwordHash, PasswordSet: passwordHash != ""}
	_, err := s.db.ExecContext(ctx, `INSERT INTO principals(id, username, display_name, role, password_hash, status) VALUES (?,?,?,?,?,?)`, p.ID, p.Username, p.DisplayName, p.Role, p.PasswordHash, p.Status)
	if err != nil {
		return Principal{}, err
	}
	return s.GetPrincipal(ctx, p.ID)
}

func (s *Store) GetPrincipal(ctx context.Context, principalID string) (Principal, error) {
	return scanPrincipal(s.db.QueryRowContext(ctx, principalSelect+` WHERE id=?`, principalID))
}

func (s *Store) GetPrincipalByUsername(ctx context.Context, username string) (Principal, error) {
	return scanPrincipal(s.db.QueryRowContext(ctx, principalSelect+` WHERE username=?`, username))
}

func (s *Store) EnsureDefaultAdmin(ctx context.Context, passwordHash string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO principals
		(id, username, display_name, role, password_hash, status) VALUES (?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE role='admin', password_hash=IF(principals.password_hash IS NULL, VALUES(password_hash), principals.password_hash)`,
		id.New(), "admin", "系统管理员", "admin", passwordHash, "active")
	return err
}

func (s *Store) SetPrincipalStatus(ctx context.Context, principalID, status string) error {
	return s.UpdatePrincipal(ctx, principalID, status, nil)
}

func (s *Store) UpdatePrincipal(ctx context.Context, principalID, status string, passwordHash *string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE principals SET status=?, password_hash=COALESCE(?, password_hash) WHERE id=?`, status, passwordHash, principalID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) UpdatePassword(ctx context.Context, principalID, passwordHash string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE principals SET password_hash=? WHERE id=?`, passwordHash, principalID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) InsertToken(ctx context.Context, tokenID, principalID, name, prefix string, digest [32]byte, expiresAt *time.Time, ciphertext []byte) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO api_tokens(id, principal_id, name, prefix, token_digest, expires_at, token_ciphertext)
		VALUES (?,?,?,?,?,?,?)`, tokenID, principalID, name, prefix, digest[:], expiresAt, ciphertext)
	return err
}

func (s *Store) TokenCiphertext(ctx context.Context, tokenID, principalID string) ([]byte, error) {
	var ciphertext []byte
	err := s.db.QueryRowContext(ctx, `SELECT token_ciphertext FROM api_tokens
	 WHERE id=? AND principal_id=? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>NOW(6))`, tokenID, principalID).Scan(&ciphertext)
	return ciphertext, err
}

func (s *Store) PrincipalByTokenDigest(ctx context.Context, digest [32]byte) (Principal, error) {
	return scanPrincipal(s.db.QueryRowContext(ctx, `SELECT p.id, p.username, p.display_name, p.role, p.status, p.password_hash, p.created_at, p.updated_at
		FROM api_tokens t JOIN principals p ON p.id=t.principal_id
		WHERE t.token_digest=? AND t.revoked_at IS NULL AND (t.expires_at IS NULL OR t.expires_at>NOW(6))
		AND p.status='active'`, digest[:]))
}

func (s *Store) InsertSession(ctx context.Context, sessionID, principalID string, digest [32]byte, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO web_sessions(id, principal_id, token_digest, expires_at) VALUES (?,?,?,?)`, sessionID, principalID, digest[:], expiresAt)
	return err
}

func (s *Store) PrincipalBySessionDigest(ctx context.Context, digest [32]byte) (Principal, error) {
	return scanPrincipal(s.db.QueryRowContext(ctx, `SELECT p.id, p.username, p.display_name, p.role, p.status, p.password_hash, p.created_at, p.updated_at
		FROM web_sessions s JOIN principals p ON p.id=s.principal_id
		WHERE s.token_digest=? AND s.expires_at>NOW(6) AND p.status='active'`, digest[:]))
}

func (s *Store) DeleteSession(ctx context.Context, digest [32]byte) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM web_sessions WHERE token_digest=?`, digest[:])
	return err
}

func (s *Store) DeleteSessionsForPrincipal(ctx context.Context, principalID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM web_sessions WHERE principal_id=?`, principalID)
	return err
}

func (s *Store) RevokeToken(ctx context.Context, tokenID string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE api_tokens SET revoked_at=NOW(6) WHERE id=? AND revoked_at IS NULL`, tokenID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteTokenForPrincipal(ctx context.Context, tokenID, principalID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE id=? AND principal_id=?`, tokenID, principalID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListTokens(ctx context.Context, principalID string) ([]APIToken, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, prefix, expires_at, revoked_at, created_at, token_ciphertext IS NOT NULL
		FROM api_tokens WHERE principal_id=? ORDER BY created_at DESC`, principalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]APIToken, 0)
	for rows.Next() {
		var token APIToken
		var expiresAt, revokedAt sql.NullTime
		if err := rows.Scan(&token.ID, &token.Name, &token.Prefix, &expiresAt, &revokedAt, &token.CreatedAt, &token.ConfigAvailable); err != nil {
			return nil, err
		}
		if expiresAt.Valid {
			token.ExpiresAt = &expiresAt.Time
		}
		if revokedAt.Valid {
			token.RevokedAt = &revokedAt.Time
		}
		out = append(out, token)
	}
	return out, rows.Err()
}

func (s *Store) ListResources(ctx context.Context) ([]Resource, error) {
	rows, err := s.db.QueryContext(ctx, resourceSelect+` ORDER BY resource_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Resource, 0)
	for rows.Next() {
		r, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

const resourceSelect = `SELECT id, resource_key, display_name, host, port, database_name,
	username, secret_ref, tls_mode, max_rows, max_write_rows,
	statement_timeout_ms, enabled, version, created_at, updated_at FROM database_resources`

type scanner interface{ Scan(...any) error }

func scanResource(row scanner) (Resource, error) {
	var r Resource
	err := row.Scan(&r.ID, &r.ResourceKey, &r.DisplayName, &r.Host, &r.Port, &r.DatabaseName,
		&r.Username, &r.SecretRef, &r.TLSMode, &r.MaxRows, &r.MaxWriteRows,
		&r.StatementTimeoutMS, &r.Enabled, &r.Version, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (s *Store) GetResourceByKey(ctx context.Context, key string) (Resource, error) {
	return scanResource(s.db.QueryRowContext(ctx, resourceSelect+` WHERE resource_key=?`, key))
}

func (s *Store) GetResource(ctx context.Context, resourceID string) (Resource, error) {
	return scanResource(s.db.QueryRowContext(ctx, resourceSelect+` WHERE id=?`, resourceID))
}

func (s *Store) CreateResource(ctx context.Context, r Resource) (Resource, error) {
	r.ID = id.New()
	_, err := s.db.ExecContext(ctx, `INSERT INTO database_resources
		(id, resource_key, display_name, host, port, database_name, username, secret_ref,
		 tls_mode, max_rows, max_write_rows, statement_timeout_ms, enabled)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, r.ID, r.ResourceKey, r.DisplayName, r.Host, r.Port,
		r.DatabaseName, r.Username, r.SecretRef,
		r.TLSMode, r.MaxRows, r.MaxWriteRows, r.StatementTimeoutMS, r.Enabled)
	if err != nil {
		return Resource{}, err
	}
	return s.GetResource(ctx, r.ID)
}

func (s *Store) UpdateResource(ctx context.Context, r Resource, expectedVersion uint64) (Resource, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE database_resources SET display_name=?, host=?, port=?, database_name=?,
		username=?, secret_ref=?, tls_mode=?, max_rows=?,
		max_write_rows=?, statement_timeout_ms=?, enabled=?, version=version+1 WHERE id=? AND version=?`, r.DisplayName,
		r.Host, r.Port, r.DatabaseName, r.Username, r.SecretRef,
		r.TLSMode, r.MaxRows, r.MaxWriteRows, r.StatementTimeoutMS, r.Enabled, r.ID, expectedVersion)
	if err != nil {
		return Resource{}, err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return Resource{}, ErrConflict
	}
	return s.GetResource(ctx, r.ID)
}

func (s *Store) ListGrants(ctx context.Context) ([]Grant, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT g.id, g.principal_id, p.username, g.resource_id, r.resource_key,
		g.action, g.require_reason, g.row_limit, g.statement_timeout_ms, g.revoked_at, g.created_at, g.updated_at
		FROM grants g JOIN principals p ON p.id=g.principal_id JOIN database_resources r ON r.id=g.resource_id
		WHERE g.revoked_at IS NULL ORDER BY p.username, r.resource_key, g.action`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Grant, 0)
	for rows.Next() {
		g, err := scanGrant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func scanGrant(row scanner) (Grant, error) {
	var g Grant
	var rowLimit, timeout sql.NullInt64
	var revoked sql.NullTime
	err := row.Scan(&g.ID, &g.PrincipalID, &g.Username, &g.ResourceID, &g.ResourceKey, &g.Action,
		&g.RequireReason, &rowLimit, &timeout, &revoked, &g.CreatedAt, &g.UpdatedAt)
	if rowLimit.Valid {
		v := uint32(rowLimit.Int64)
		g.RowLimit = &v
	}
	if timeout.Valid {
		v := uint32(timeout.Int64)
		g.StatementTimeoutMS = &v
	}
	if revoked.Valid {
		g.RevokedAt = &revoked.Time
	}
	return g, err
}

func (s *Store) UpsertGrant(ctx context.Context, g Grant) (Grant, error) {
	if g.ID == "" {
		g.ID = id.New()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO grants
		(id, principal_id, resource_id, action, require_reason, row_limit, statement_timeout_ms)
		VALUES (?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE require_reason=VALUES(require_reason), row_limit=VALUES(row_limit),
		statement_timeout_ms=VALUES(statement_timeout_ms), revoked_at=NULL`, g.ID, g.PrincipalID, g.ResourceID,
		g.Action, g.RequireReason, g.RowLimit, g.StatementTimeoutMS)
	if err != nil {
		return Grant{}, err
	}
	var out Grant
	var rowLimit, timeout sql.NullInt64
	var revoked sql.NullTime
	err = s.db.QueryRowContext(ctx, `SELECT g.id, g.principal_id, p.username, g.resource_id, r.resource_key,
		g.action, g.require_reason, g.row_limit, g.statement_timeout_ms, g.revoked_at, g.created_at, g.updated_at
		FROM grants g JOIN principals p ON p.id=g.principal_id JOIN database_resources r ON r.id=g.resource_id
		WHERE g.principal_id=? AND g.resource_id=? AND g.action=?`, g.PrincipalID, g.ResourceID, g.Action).
		Scan(&out.ID, &out.PrincipalID, &out.Username, &out.ResourceID, &out.ResourceKey, &out.Action,
			&out.RequireReason, &rowLimit, &timeout, &revoked, &out.CreatedAt, &out.UpdatedAt)
	if rowLimit.Valid {
		v := uint32(rowLimit.Int64)
		out.RowLimit = &v
	}
	if timeout.Valid {
		v := uint32(timeout.Int64)
		out.StatementTimeoutMS = &v
	}
	return out, err
}

func (s *Store) RevokeGrant(ctx context.Context, grantID string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE grants SET revoked_at=NOW(6) WHERE id=? AND revoked_at IS NULL`, grantID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) EffectiveGrants(ctx context.Context, principalID, resourceID string) ([]Grant, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT g.id, g.principal_id, p.username, g.resource_id, r.resource_key,
		g.action, g.require_reason, g.row_limit, g.statement_timeout_ms, g.revoked_at, g.created_at, g.updated_at
		FROM grants g JOIN principals p ON p.id=g.principal_id JOIN database_resources r ON r.id=g.resource_id
		WHERE g.principal_id=? AND g.resource_id=? AND g.revoked_at IS NULL`, principalID, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Grant, 0)
	for rows.Next() {
		g, err := scanGrant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) VisibleResources(ctx context.Context, principalID string) ([]Resource, error) {
	rows, err := s.db.QueryContext(ctx, resourceSelect+` r WHERE r.enabled=TRUE AND EXISTS (
		SELECT 1 FROM grants g WHERE g.resource_id=r.id AND g.principal_id=? AND g.revoked_at IS NULL)
		ORDER BY r.resource_key`, principalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Resource, 0)
	for rows.Next() {
		r, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListPrincipalAccess(ctx context.Context, principalID string) ([]PrincipalAccess, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.id, r.resource_key, r.display_name, r.database_name, r.enabled,
		g.id, g.action, g.require_reason, g.row_limit, g.statement_timeout_ms
		FROM database_resources r JOIN grants g ON g.resource_id=r.id
		WHERE r.enabled=TRUE AND g.principal_id=? AND g.revoked_at IS NULL
		ORDER BY r.resource_key, g.action`, principalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PrincipalAccess, 0)
	index := make(map[string]int)
	for rows.Next() {
		var resourceID, resourceKey, displayName, databaseName string
		var enabled bool
		var grant AccessGrant
		var rowLimit, timeout sql.NullInt64
		if err := rows.Scan(&resourceID, &resourceKey, &displayName, &databaseName, &enabled,
			&grant.ID, &grant.Action, &grant.RequireReason, &rowLimit, &timeout); err != nil {
			return nil, err
		}
		if rowLimit.Valid {
			value := uint32(rowLimit.Int64)
			grant.RowLimit = &value
		}
		if timeout.Valid {
			value := uint32(timeout.Int64)
			grant.StatementTimeoutMS = &value
		}
		position, ok := index[resourceID]
		if !ok {
			position = len(out)
			index[resourceID] = position
			out = append(out, PrincipalAccess{ResourceID: resourceID, ResourceKey: resourceKey, DisplayName: displayName,
				DatabaseName: databaseName, Enabled: enabled, Grants: make([]AccessGrant, 0, 1)})
		}
		out[position].Grants = append(out[position].Grants, grant)
	}
	return out, rows.Err()
}

func (s *Store) InsertAudit(ctx context.Context, e AuditEvent) (uint64, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO audit_logs
		(request_id, principal_id, resource_id, tool, action, sql_fingerprint, sql_text, reason, outcome, row_count, duration_ms, error_code)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, e.RequestID, e.PrincipalID, e.ResourceID, e.Tool, e.Action,
		e.SQLFingerprint, e.SQLText, e.Reason, e.Outcome, e.RowCount, e.DurationMS, e.ErrorCode)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return uint64(id), nil
}

func (s *Store) UpdateAudit(ctx context.Context, auditID uint64, e AuditEvent) error {
	_, err := s.db.ExecContext(ctx, `UPDATE audit_logs SET action=?, sql_fingerprint=?, sql_text=?, reason=?,
		outcome=?, row_count=?, duration_ms=?, error_code=? WHERE id=?`, e.Action, e.SQLFingerprint, e.SQLText,
		e.Reason, e.Outcome, e.RowCount, e.DurationMS, e.ErrorCode, auditID)
	return err
}

func (s *Store) ListAudits(ctx context.Context, page, pageSize int) (AuditPage, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs a
		WHERE a.id=(SELECT MAX(latest.id) FROM audit_logs latest WHERE latest.request_id=a.request_id)`).Scan(&total); err != nil {
		return AuditPage{}, err
	}
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages == 0 {
		page = 1
	} else if page > totalPages {
		page = totalPages
	}
	offset := int64(page-1) * int64(pageSize)
	rows, err := s.db.QueryContext(ctx, `SELECT a.id, a.request_id, a.principal_id, p.username,
		a.resource_id, r.resource_key, a.tool, a.action, a.sql_fingerprint, a.sql_text, a.reason, a.outcome,
		a.row_count, a.duration_ms, a.error_code, a.created_at
		FROM audit_logs a LEFT JOIN principals p ON p.id=a.principal_id
		LEFT JOIN database_resources r ON r.id=a.resource_id
		WHERE a.id=(SELECT MAX(latest.id) FROM audit_logs latest WHERE latest.request_id=a.request_id)
		ORDER BY a.id DESC LIMIT ? OFFSET ?`, pageSize, offset)
	if err != nil {
		return AuditPage{}, err
	}
	defer rows.Close()
	out := make([]AuditLog, 0)
	for rows.Next() {
		var a AuditLog
		var principal, username, resource, key, action, fp, sqlText, reason, errorCode sql.NullString
		var rowCount, duration sql.NullInt64
		if err := rows.Scan(&a.ID, &a.RequestID, &principal, &username, &resource, &key, &a.Tool, &action,
			&fp, &sqlText, &reason, &a.Outcome, &rowCount, &duration, &errorCode, &a.CreatedAt); err != nil {
			return AuditPage{}, err
		}
		if principal.Valid {
			a.PrincipalID = &principal.String
		}
		if username.Valid {
			a.Username = &username.String
		}
		if resource.Valid {
			a.ResourceID = &resource.String
		}
		if key.Valid {
			a.ResourceKey = &key.String
		}
		if action.Valid {
			a.Action = &action.String
		}
		if fp.Valid {
			a.SQLFingerprint = &fp.String
		}
		if sqlText.Valid {
			a.SQLText = &sqlText.String
		}
		if reason.Valid {
			a.Reason = &reason.String
		}
		if rowCount.Valid {
			v := rowCount.Int64
			a.RowCount = &v
		}
		if duration.Valid {
			v := duration.Int64
			a.DurationMS = &v
		}
		if errorCode.Valid {
			a.ErrorCode = &errorCode.String
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return AuditPage{}, err
	}
	return AuditPage{Items: out, Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages}, nil
}

func (s *Store) InsertAdminAudit(ctx context.Context, requestID, action, objectType, objectID, outcome string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO admin_audit_logs(request_id, action, object_type, object_id, outcome) VALUES (?,?,?,?,?)`, requestID, action, objectType, objectID, outcome)
	return err
}

var ErrConflict = errors.New("version conflict")
