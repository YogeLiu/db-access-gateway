package api

import (
	"context"
	"github.com/yogel/db-access-gateway/internal/authz"
	"github.com/yogel/db-access-gateway/internal/control"
	"github.com/yogel/db-access-gateway/internal/masking"
	"net/http"
	"time"
)

func (h *AdminHandler) listMasking(w http.ResponseWriter, r *http.Request) {
	rules, err := h.store.MaskRules(r.Context(), r.PathValue("id"))
	respond(w, rules, err)
}

func (h *AdminHandler) listMaskingTables(w http.ResponseWriter, r *http.Request) {
	resource, err := h.store.GetResource(r.Context(), r.PathValue("id"))
	if err != nil {
		respond(w, nil, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	db, err := h.registry.Get(ctx, resource, authz.SchemaRead)
	if err != nil {
		respond(w, nil, err)
		return
	}
	rows, err := db.QueryContext(ctx, "SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=? AND TABLE_TYPE='BASE TABLE' ORDER BY TABLE_NAME", resource.DatabaseName)
	if err != nil {
		respond(w, nil, err)
		return
	}
	defer rows.Close()
	tables := make([]string, 0)
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			respond(w, nil, err)
			return
		}
		tables = append(tables, table)
	}
	respond(w, tables, rows.Err())
}

func (h *AdminHandler) maskingFields(r *http.Request, table string) ([]masking.Field, error) {
	resource, err := h.store.GetResource(r.Context(), r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	db, err := h.registry.Get(ctx, resource, authz.SchemaRead)
	if err != nil {
		return nil, err
	}
	return masking.Schema(ctx, db, resource.DatabaseName, table)
}

func (h *AdminHandler) listMaskingFields(w http.ResponseWriter, r *http.Request) {
	table := r.URL.Query().Get("table")
	if table == "" {
		writeError(w, 400, "table_required", "select a table first")
		return
	}
	fields, err := h.maskingFields(r, table)
	respond(w, fields, err)
}

func (h *AdminHandler) saveMasking(w http.ResponseWriter, r *http.Request) {
	var rule control.MaskRule
	if !decodeJSON(w, r, &rule) {
		return
	}
	rule.ResourceID = r.PathValue("id")
	if rule.Mode != "plain" && rule.Mode != "partial" && rule.Mode != "full" {
		writeError(w, 400, "invalid_mode", "mode must be plain, partial or full")
		return
	}
	for _, n := range []*int{rule.KeepPrefix, rule.KeepSuffix} {
		if n != nil && (*n < 0 || *n > 64) {
			writeError(w, 400, "invalid_mask_format", "prefix and suffix must be 0..64")
			return
		}
	}
	fields, err := h.maskingFields(r, rule.TableName)
	if err != nil {
		writeError(w, 422, "schema_unavailable", "cannot validate database fields")
		return
	}
	found := false
	for _, f := range fields {
		if f.Table == rule.TableName && f.Column == rule.ColumnName && f.TableType == "BASE TABLE" {
			found = true
			break
		}
	}
	if !found {
		writeError(w, 400, "invalid_field", "choose an existing base-table column")
		return
	}
	if err := h.store.SaveMaskRule(r.Context(), rule); err != nil {
		respond(w, nil, err)
		return
	}
	h.registry.Invalidate(rule.ResourceID)
	h.adminAudit(r.Context(), "update_masking", "resource", rule.ResourceID)
	writeJSON(w, 200, rule)
}
