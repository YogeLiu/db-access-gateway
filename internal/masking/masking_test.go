package masking

import (
	"github.com/yogel/db-access-gateway/internal/control"
	"strings"
	"testing"
)

func TestRewrite(t *testing.T) {
	fields := []Field{{Table: "people", Column: "id", TableType: "BASE TABLE"}, {Table: "people", Column: "phone", TableType: "BASE TABLE"}, {Table: "other", Column: "phone", TableType: "BASE TABLE"}}
	rules := []control.MaskRule{{TableName: "people", ColumnName: "phone", Mode: "partial"}}
	for _, sql := range []string{
		"select * from people",
		"select p.phone as contact from people p",
		"select p.phone,o.phone from people p join other o on p.id=1",
		"select concat(phone,phone) from people",
		"select phone from people union all select phone from people",
		"select x.phone from (select phone from people) x",
	} {
		t.Run(sql, func(t *testing.T) {
			rewritten, err := Rewrite(sql, rules, fields)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(rewritten, "'****'") || !strings.Contains(rewritten, "is null") {
				t.Fatalf("missing masking: %s", rewritten)
			}
		})
	}
	for _, sql := range []string{"select * from unknown_view", "select steal(phone) from people", "update people set phone='x' where id=1", "select other.steal(phone) from people"} {
		if _, err := Rewrite(sql, rules, fields); err == nil {
			t.Fatalf("unsafe query accepted: %s", sql)
		}
	}
	full, err := Rewrite("select phone from people", []control.MaskRule{{TableName: "people", ColumnName: "phone", Mode: "full"}}, fields)
	if err != nil || !strings.Contains(full, "'******'") {
		t.Fatal(full, err)
	}
	plain := "select phone from people"
	if got, err := Rewrite(plain, []control.MaskRule{{Mode: "plain"}}, fields); err != nil || got != plain {
		t.Fatal(got, err)
	}
	other, err := Rewrite("select phone from other", rules, fields)
	if err != nil || strings.Contains(other, "****") {
		t.Fatal("unrelated table masked", other, err)
	}
}
