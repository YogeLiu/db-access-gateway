package authz

import (
	"errors"

	"github.com/yogel/db-access-gateway/internal/control"
)

type Action string

const (
	SchemaRead Action = "schema_read"
	QueryRead  Action = "query_read"
	QueryWrite Action = "query_write"
)

func ParseAction(value string) (Action, error) {
	a := Action(value)
	switch a {
	case SchemaRead, QueryRead, QueryWrite:
		return a, nil
	default:
		return "", errors.New("unknown action")
	}
}

func (granted Action) Includes(requested Action) bool {
	switch granted {
	case QueryWrite:
		return requested == QueryWrite || requested == QueryRead || requested == SchemaRead
	case QueryRead:
		return requested == QueryRead || requested == SchemaRead
	case SchemaRead:
		return requested == SchemaRead
	default:
		return false
	}
}

type Constraints struct {
	RequireReason      bool
	RowLimit           *uint32
	StatementTimeoutMS *uint32
}

type Decision struct {
	Allowed     bool
	Constraints Constraints
}

// Evaluate defaults to deny. All grants whose action includes requested are
// merged most-restrictively, so an additional grant can never loosen a cap.
func Evaluate(grants []control.Grant, requested Action) Decision {
	decision := Decision{}
	for _, grant := range grants {
		granted, err := ParseAction(grant.Action)
		if err != nil || !granted.Includes(requested) {
			continue
		}
		decision.Allowed = true
		decision.Constraints.RequireReason = decision.Constraints.RequireReason || grant.RequireReason
		decision.Constraints.RowLimit = minOptional(decision.Constraints.RowLimit, grant.RowLimit)
		decision.Constraints.StatementTimeoutMS = minOptional(decision.Constraints.StatementTimeoutMS, grant.StatementTimeoutMS)
	}
	return decision
}

func minOptional(a, b *uint32) *uint32 {
	if a == nil {
		return clone(b)
	}
	if b == nil {
		return clone(a)
	}
	v := *a
	if *b < v {
		v = *b
	}
	return &v
}

func clone(value *uint32) *uint32 {
	if value == nil {
		return nil
	}
	v := *value
	return &v
}
