package sqlguard

import (
	"testing"

	"github.com/yogel/db-access-gateway/internal/authz"
)

func TestInspectAllowsExpectedStatements(t *testing.T) {
	tests := []struct {
		sql    string
		action authz.Action
	}{
		{"SELECT id FROM users WHERE id = ?", authz.QueryRead},
		{"SELECT id FROM app.users", authz.QueryRead},
		{"INSERT INTO users(name) VALUES (?)", authz.QueryWrite},
		{"UPDATE users SET name=? WHERE id=?", authz.QueryWrite},
		{"DELETE FROM users WHERE id=?", authz.QueryWrite},
	}
	for _, tc := range tests {
		plan, err := Inspect(tc.sql, "app")
		if err != nil {
			t.Fatalf("%s: %v", tc.sql, err)
		}
		if plan.Action != tc.action {
			t.Fatalf("%s action=%s", tc.sql, plan.Action)
		}
	}
}

func TestInspectRejectsDangerousStatements(t *testing.T) {
	queries := []string{
		"SELECT 1; DELETE FROM users WHERE id=1",
		"UPDATE users SET active=0",
		"DELETE FROM users",
		"DROP TABLE users",
		"SELECT * FROM mysql.user",
		"SELECT SLEEP(5)",
		"SELECT /*+ MAX_EXECUTION_TIME(999999) */ 1",
		"SELECT * FROM users FOR UPDATE",
		"REPLACE INTO users(id) VALUES (1)",
	}
	for _, query := range queries {
		if _, err := Inspect(query, "app"); err == nil {
			t.Fatalf("expected rejection: %s", query)
		}
	}
}
