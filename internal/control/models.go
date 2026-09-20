package control

import "time"

type Principal struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	DisplayName  string    `json:"display_name"`
	Role         string    `json:"role"`
	Status       string    `json:"status"`
	PasswordSet  bool      `json:"password_set"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Resource struct {
	ID                 string    `json:"id"`
	ResourceKey        string    `json:"resource_key"`
	DisplayName        string    `json:"display_name"`
	Host               string    `json:"host"`
	Port               uint16    `json:"port"`
	DatabaseName       string    `json:"database_name"`
	ReadUsername       string    `json:"read_username"`
	ReadSecretRef      string    `json:"read_secret_ref"`
	WriteUsername      *string   `json:"write_username,omitempty"`
	WriteSecretRef     *string   `json:"write_secret_ref,omitempty"`
	TLSMode            string    `json:"tls_mode"`
	MaxRows            uint32    `json:"max_rows"`
	MaxWriteRows       uint32    `json:"max_write_rows"`
	StatementTimeoutMS uint32    `json:"statement_timeout_ms"`
	Enabled            bool      `json:"enabled"`
	Version            uint64    `json:"version"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type Grant struct {
	ID                 string     `json:"id"`
	PrincipalID        string     `json:"principal_id"`
	Username           string     `json:"username,omitempty"`
	ResourceID         string     `json:"resource_id"`
	ResourceKey        string     `json:"resource_key,omitempty"`
	Action             string     `json:"action"`
	RequireReason      bool       `json:"require_reason"`
	RowLimit           *uint32    `json:"row_limit,omitempty"`
	StatementTimeoutMS *uint32    `json:"statement_timeout_ms,omitempty"`
	RevokedAt          *time.Time `json:"revoked_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type AccessGrant struct {
	ID                 string  `json:"id"`
	Action             string  `json:"action"`
	RequireReason      bool    `json:"require_reason"`
	RowLimit           *uint32 `json:"row_limit,omitempty"`
	StatementTimeoutMS *uint32 `json:"statement_timeout_ms,omitempty"`
}

type PrincipalAccess struct {
	ResourceID   string        `json:"resource_id"`
	ResourceKey  string        `json:"resource_key"`
	DisplayName  string        `json:"display_name"`
	DatabaseName string        `json:"database_name"`
	Enabled      bool          `json:"enabled"`
	Grants       []AccessGrant `json:"grants"`
}

type APIToken struct {
	ConfigAvailable bool       `json:"config_available"`
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Prefix          string     `json:"prefix"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type AuditLog struct {
	ID             uint64    `json:"id"`
	RequestID      string    `json:"request_id"`
	PrincipalID    *string   `json:"principal_id,omitempty"`
	Username       *string   `json:"username,omitempty"`
	ResourceID     *string   `json:"resource_id,omitempty"`
	ResourceKey    *string   `json:"resource_key,omitempty"`
	Tool           string    `json:"tool"`
	Action         *string   `json:"action,omitempty"`
	SQLFingerprint *string   `json:"sql_fingerprint,omitempty"`
	SQLText        *string   `json:"sql_text,omitempty"`
	Reason         *string   `json:"reason,omitempty"`
	Outcome        string    `json:"outcome"`
	RowCount       *int64    `json:"row_count,omitempty"`
	DurationMS     *int64    `json:"duration_ms,omitempty"`
	ErrorCode      *string   `json:"error_code,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type AuditEvent struct {
	RequestID      string
	PrincipalID    *string
	ResourceID     *string
	Tool           string
	Action         *string
	SQLFingerprint *string
	SQLText        *string
	Reason         *string
	Outcome        string
	RowCount       *int64
	DurationMS     *int64
	ErrorCode      *string
}

type AuditPage struct {
	Items      []AuditLog `json:"items"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
	Total      int64      `json:"total"`
	TotalPages int        `json:"total_pages"`
}
