package masking

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/yogel/db-access-gateway/internal/control"
	"os"
	"testing"
	"time"
)

// Uses session-local temporary tables only; never touches business tables.
func TestMySQLMasking(t *testing.T) {
	dsn := os.Getenv("MASKING_TEST_DSN")
	if dsn == "" {
		t.Skip("MASKING_TEST_DSN not set")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, q := range []string{
		"CREATE TEMPORARY TABLE masking_test_people (id INT, phone VARCHAR(40), secret VARCHAR(40))",
		"INSERT INTO masking_test_people VALUES (1,'13812345678','private'),(2,'张三丰',NULL),(3,'张三','x')",
	} {
		if _, err = conn.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	fields := []Field{{Table: "masking_test_people", Column: "id", TableType: "BASE TABLE"}, {Table: "masking_test_people", Column: "phone", TableType: "BASE TABLE"}, {Table: "masking_test_people", Column: "secret", TableType: "BASE TABLE"}}
	for _, tc := range []struct {
		prefix, suffix, id int
		want               string
	}{{3, 4, 1, "138****5678"}, {0, 4, 1, "****5678"}, {3, 0, 1, "138****"}, {0, 0, 1, "****"}, {3, 4, 3, "******"}, {1, 0, 2, "张****"}} {
		rule := control.MaskRule{TableName: "masking_test_people", ColumnName: "phone", Mode: "partial", KeepPrefix: &tc.prefix, KeepSuffix: &tc.suffix}
		q, err := Rewrite(fmt.Sprintf("SELECT phone FROM masking_test_people WHERE id = %d", tc.id), []control.MaskRule{rule}, fields)
		if err != nil {
			t.Fatal(err)
		}
		var got string
		if err := conn.QueryRowContext(ctx, q).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("prefix=%d suffix=%d id=%d: got %q want %q", tc.prefix, tc.suffix, tc.id, got, tc.want)
		}
	}
	rules := []control.MaskRule{{TableName: "masking_test_people", ColumnName: "phone", Mode: "partial"}, {TableName: "masking_test_people", ColumnName: "secret", Mode: "full"}}
	for _, q := range []string{"SELECT * FROM masking_test_people ORDER BY id", "SELECT id,phone AS renamed,secret FROM masking_test_people ORDER BY id", "SELECT id,concat(phone,''),secret FROM masking_test_people ORDER BY id", "SELECT * FROM (SELECT * FROM masking_test_people) x ORDER BY id"} {
		rewritten, err := Rewrite(q, rules, fields)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := conn.QueryContext(ctx, rewritten)
		if err != nil {
			t.Fatal(err)
		}
		index := 0
		for rows.Next() {
			var id int
			var phone string
			var secret sql.NullString
			if err := rows.Scan(&id, &phone, &secret); err != nil {
				t.Fatal(err)
			}
			expected := []string{"1****8", "张****丰", "******"}
			if phone != expected[index] {
				t.Fatalf("unexpected masked value %q", phone)
			}
			if id == 2 {
				if secret.Valid {
					t.Fatal("NULL changed")
				}
			} else if secret.String != "******" {
				t.Fatal("secret leaked")
			}
			index++
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		if index != 3 {
			t.Fatalf("row count %d", index)
		}
	}
}
