package control

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT PRIMARY KEY,
		applied_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	`CREATE TABLE IF NOT EXISTS principals (
		id CHAR(36) PRIMARY KEY,
		username VARCHAR(80) NOT NULL UNIQUE,
		display_name VARCHAR(120) NOT NULL DEFAULT '',
		status ENUM('active','disabled') NOT NULL DEFAULT 'active',
		created_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		updated_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	`CREATE TABLE IF NOT EXISTS api_tokens (
		id CHAR(36) PRIMARY KEY,
		principal_id CHAR(36) NOT NULL,
		name VARCHAR(100) NOT NULL,
		prefix VARCHAR(24) NOT NULL,
		token_digest BINARY(32) NOT NULL UNIQUE,
		expires_at TIMESTAMP(6) NULL,
		revoked_at TIMESTAMP(6) NULL,
		created_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		CONSTRAINT fk_tokens_principal FOREIGN KEY (principal_id) REFERENCES principals(id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	`CREATE TABLE IF NOT EXISTS database_resources (
		id CHAR(36) PRIMARY KEY,
		resource_key VARCHAR(100) NOT NULL UNIQUE,
		display_name VARCHAR(160) NOT NULL,
		host VARCHAR(255) NOT NULL,
		port INT UNSIGNED NOT NULL DEFAULT 3306,
		database_name VARCHAR(128) NOT NULL,
		read_username VARCHAR(128) NOT NULL,
		read_secret_ref VARCHAR(128) NOT NULL,
		write_username VARCHAR(128) NULL,
		write_secret_ref VARCHAR(128) NULL,
		tls_mode ENUM('preferred','required','skip_verify','disabled') NOT NULL DEFAULT 'required',
		max_rows INT UNSIGNED NOT NULL DEFAULT 1000,
		max_write_rows INT UNSIGNED NOT NULL DEFAULT 100,
		statement_timeout_ms INT UNSIGNED NOT NULL DEFAULT 10000,
		enabled BOOLEAN NOT NULL DEFAULT FALSE,
		version BIGINT UNSIGNED NOT NULL DEFAULT 1,
		created_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		updated_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	`CREATE TABLE IF NOT EXISTS grants (
		id CHAR(36) PRIMARY KEY,
		principal_id CHAR(36) NOT NULL,
		resource_id CHAR(36) NOT NULL,
		action ENUM('schema_read','query_read','query_write') NOT NULL,
		require_reason BOOLEAN NOT NULL DEFAULT FALSE,
		row_limit INT UNSIGNED NULL,
		statement_timeout_ms INT UNSIGNED NULL,
		revoked_at TIMESTAMP(6) NULL,
		created_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		updated_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
		UNIQUE KEY uk_grant (principal_id, resource_id, action),
		KEY idx_grant_principal (principal_id, revoked_at),
		CONSTRAINT fk_grants_principal FOREIGN KEY (principal_id) REFERENCES principals(id),
		CONSTRAINT fk_grants_resource FOREIGN KEY (resource_id) REFERENCES database_resources(id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	`CREATE TABLE IF NOT EXISTS audit_logs (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		request_id CHAR(36) NOT NULL,
		principal_id CHAR(36) NULL,
		resource_id CHAR(36) NULL,
		tool VARCHAR(64) NOT NULL,
		action VARCHAR(32) NULL,
		sql_fingerprint CHAR(64) NULL,
		reason VARCHAR(255) NULL,
		outcome VARCHAR(32) NOT NULL,
		row_count BIGINT NULL,
		duration_ms BIGINT NULL,
		error_code VARCHAR(64) NULL,
		created_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		KEY idx_audit_created (created_at, id),
		KEY idx_audit_principal (principal_id, created_at),
		KEY idx_audit_resource (resource_id, created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	`CREATE TABLE IF NOT EXISTS admin_audit_logs (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		request_id CHAR(36) NOT NULL,
		action VARCHAR(80) NOT NULL,
		object_type VARCHAR(40) NOT NULL,
		object_id VARCHAR(80) NOT NULL,
		outcome VARCHAR(32) NOT NULL,
		created_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		KEY idx_admin_audit_created (created_at, id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	`ALTER TABLE audit_logs ADD COLUMN sql_text TEXT NULL AFTER sql_fingerprint`,
	`ALTER TABLE principals ADD COLUMN role ENUM('admin','user') NOT NULL DEFAULT 'user' AFTER display_name,
		ADD COLUMN password_hash VARCHAR(255) NULL AFTER role`,
	`CREATE TABLE IF NOT EXISTS web_sessions (
		id CHAR(36) PRIMARY KEY,
		principal_id CHAR(36) NOT NULL,
		token_digest BINARY(32) NOT NULL UNIQUE,
		expires_at TIMESTAMP(6) NOT NULL,
		created_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		KEY idx_sessions_principal (principal_id, expires_at),
		KEY idx_sessions_expires (expires_at),
		CONSTRAINT fk_sessions_principal FOREIGN KEY (principal_id) REFERENCES principals(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	`CREATE TABLE IF NOT EXISTS masking_rules (
 resource_id CHAR(36) NOT NULL,
 table_name VARCHAR(128) COLLATE utf8mb4_bin NOT NULL,
 column_name VARCHAR(128) COLLATE utf8mb4_bin NOT NULL,
 mode ENUM('plain','partial','full') NOT NULL,
 PRIMARY KEY(resource_id,table_name,column_name),
 FOREIGN KEY(resource_id) REFERENCES database_resources(id) ON DELETE CASCADE
 ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	`ALTER TABLE masking_rules ADD COLUMN keep_prefix INT NOT NULL DEFAULT 1,
 ADD COLUMN keep_suffix INT NOT NULL DEFAULT 1`,
	`ALTER TABLE api_tokens ADD COLUMN token_ciphertext BLOB NULL`,
	`ALTER TABLE database_resources
		ADD COLUMN username VARCHAR(128) NULL AFTER database_name,
		ADD COLUMN secret_ref VARCHAR(128) NULL AFTER username`,
	`UPDATE database_resources
		SET username=COALESCE(NULLIF(write_username, ''), read_username),
			secret_ref=COALESCE(NULLIF(write_secret_ref, ''), read_secret_ref)
		WHERE username IS NULL`,
	`ALTER TABLE database_resources
		MODIFY username VARCHAR(128) NOT NULL,
		MODIFY secret_ref VARCHAR(128) NOT NULL`,
	`ALTER TABLE database_resources
		DROP COLUMN read_username,
		DROP COLUMN read_secret_ref,
		DROP COLUMN write_username,
		DROP COLUMN write_secret_ref`,
	`ALTER TABLE database_resources
		ADD COLUMN password_ciphertext BLOB NULL AFTER username,
		MODIFY secret_ref VARCHAR(128) NULL`,
}
