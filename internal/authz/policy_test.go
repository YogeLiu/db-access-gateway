package authz

import (
	"testing"

	"github.com/yogel/db-access-gateway/internal/control"
)

func u32(v uint32) *uint32 { return &v }

func TestActionHierarchy(t *testing.T) {
	if !QueryWrite.Includes(QueryRead) || !QueryWrite.Includes(SchemaRead) {
		t.Fatal("write must include read and schema")
	}
	if !QueryRead.Includes(SchemaRead) {
		t.Fatal("read must include schema")
	}
	if QueryRead.Includes(QueryWrite) || SchemaRead.Includes(QueryRead) {
		t.Fatal("lower action upgraded access")
	}
}

func TestEvaluateDefaultDenyAndRestrictiveMerge(t *testing.T) {
	if Evaluate(nil, QueryRead).Allowed {
		t.Fatal("empty grants must deny")
	}
	decision := Evaluate([]control.Grant{
		{Action: "query_read", RowLimit: u32(1000), StatementTimeoutMS: u32(5000)},
		{Action: "query_write", RequireReason: true, RowLimit: u32(100), StatementTimeoutMS: u32(8000)},
	}, QueryRead)
	if !decision.Allowed || !decision.Constraints.RequireReason {
		t.Fatal("expected allowed with reason")
	}
	if got := *decision.Constraints.RowLimit; got != 100 {
		t.Fatalf("row limit=%d", got)
	}
	if got := *decision.Constraints.StatementTimeoutMS; got != 5000 {
		t.Fatalf("timeout=%d", got)
	}
}

func TestRequestedAuthorizationMatrix(t *testing.T) {
	userAOnA := []control.Grant{{Action: "query_read"}}
	userAOnC := []control.Grant(nil)
	userCOnC := []control.Grant{{Action: "query_write"}}

	if !Evaluate(userAOnA, QueryRead).Allowed {
		t.Fatal("user_a querying database_a must be allowed")
	}
	if Evaluate(userAOnC, QueryRead).Allowed {
		t.Fatal("user_a querying database_c must be denied")
	}
	if !Evaluate(userCOnC, QueryWrite).Allowed {
		t.Fatal("user_c writing database_c must be allowed")
	}
	if Evaluate(userAOnA, QueryWrite).Allowed {
		t.Fatal("query_read must not authorize INSERT")
	}
}
