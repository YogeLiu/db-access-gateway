package control

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

func TestTokenSecretOwnershipAndLifecycle(t *testing.T) {
	dsn := os.Getenv("MASKING_TEST_DSN")
	if dsn == "" {
		t.Skip("MASKING_TEST_DSN not set")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ParseTime = true
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// Session-local synthetic table only; never changes business data.
	_, err = db.ExecContext(ctx, `CREATE TEMPORARY TABLE api_tokens (
	id VARCHAR(64), principal_id VARCHAR(64), name VARCHAR(100), prefix VARCHAR(20),
	token_digest BINARY(32), expires_at DATETIME NULL, revoked_at DATETIME NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP, token_ciphertext BLOB NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{db: db}
	if err := s.InsertToken(ctx, "t", "owner", "test", "prefix", [32]byte{}, nil, []byte("synthetic-ciphertext")); err != nil {
		t.Fatal(err)
	}
	got, err := s.TokenCiphertext(ctx, "t", "owner")
	if err != nil || string(got) != "synthetic-ciphertext" {
		t.Fatal("owner cannot retrieve token")
	}
	if _, err := s.TokenCiphertext(ctx, "t", "other"); err != sql.ErrNoRows {
		t.Fatal("cross-user token retrieval allowed")
	}
	items, err := s.ListTokens(ctx, "owner")
	if err != nil || len(items) != 1 || !items[0].ConfigAvailable {
		t.Fatalf("config availability missing: %v", err)
	}
	if items[0].ExpiresAt != nil {
		t.Fatal("default token should never expire")
	}
	if _, err := db.ExecContext(ctx, "UPDATE api_tokens SET expires_at=DATE_SUB(NOW(), INTERVAL 1 DAY)"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TokenCiphertext(ctx, "t", "owner"); err != sql.ErrNoRows {
		t.Fatal("expired token exposed")
	}
	if _, err := db.ExecContext(ctx, "UPDATE api_tokens SET expires_at=NULL, revoked_at=NOW()"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TokenCiphertext(ctx, "t", "owner"); err != sql.ErrNoRows {
		t.Fatal("revoked token exposed")
	}
	if _, err := db.ExecContext(ctx, "UPDATE api_tokens SET revoked_at=NULL, token_ciphertext=NULL"); err != nil {
		t.Fatal(err)
	}
	got, err = s.TokenCiphertext(ctx, "t", "owner")
	if err != nil || len(got) != 0 {
		t.Fatal("legacy token handling failed")
	}
	if err := s.DeleteTokenForPrincipal(ctx, "t", "other"); err != sql.ErrNoRows {
		t.Fatal("cross-user deletion allowed")
	}
	items, err = s.ListTokens(ctx, "owner")
	if err != nil || len(items) != 1 {
		t.Fatal("cross-user deletion changed owner token")
	}
	if _, err := db.ExecContext(ctx, "UPDATE api_tokens SET revoked_at=NOW()"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteTokenForPrincipal(ctx, "t", "owner"); err != nil {
		t.Fatal(err)
	}
	items, err = s.ListTokens(ctx, "owner")
	if err != nil || len(items) != 0 {
		t.Fatal("deleted token still listed")
	}
	if _, err := s.TokenCiphertext(ctx, "t", "owner"); err != sql.ErrNoRows {
		t.Fatal("deleted token still accessible")
	}
	if err := s.DeleteTokenForPrincipal(ctx, "t", "owner"); err != sql.ErrNoRows {
		t.Fatal("missing token deletion should return not found")
	}
}
