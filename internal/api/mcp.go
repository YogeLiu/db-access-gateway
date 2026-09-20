package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/yogel/db-access-gateway/internal/auth"
	"github.com/yogel/db-access-gateway/internal/control"
	queryservice "github.com/yogel/db-access-gateway/internal/query"
)

type principalContextKey struct{}

type MCPHandler struct {
	store       *control.Store
	service     *queryservice.Service
	tokenPepper string
	handler     http.Handler
}

func NewMCPHandler(store *control.Store, service *queryservice.Service, tokenPepper string) *MCPHandler {
	h := &MCPHandler{store: store, service: service, tokenPepper: tokenPepper}
	server := h.newServer()
	h.handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true, SessionTimeout: 30 * time.Minute})
	return h
}

func (h *MCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token, err := auth.Bearer(r.Header.Get("Authorization"))
	if err != nil {
		h.unauthorized(w)
		return
	}
	principal, err := h.store.PrincipalByTokenDigest(r.Context(), auth.Digest(h.tokenPepper, token))
	if err != nil {
		h.unauthorized(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	h.handler.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalContextKey{}, principal)))
}
func (h *MCPHandler) unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="DB Access Gateway MCP"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}
func principalFrom(ctx context.Context) (control.Principal, error) {
	p, ok := ctx.Value(principalContextKey{}).(control.Principal)
	if !ok {
		return control.Principal{}, errors.New("principal missing")
	}
	return p, nil
}

type noArgs struct{}
type resourceArgs struct {
	ResourceKey string `json:"resource_key" jsonschema:"Logical resource key returned by list_databases"`
}
type tableArgs struct {
	ResourceKey string `json:"resource_key"`
	Table       string `json:"table"`
}
type queryParam struct {
	Type  string `json:"type" jsonschema:"string,int,float,bool,null"`
	Value string `json:"value,omitempty"`
}
type queryArgs struct {
	ResourceKey string       `json:"resource_key"`
	SQL         string       `json:"sql"`
	Params      []queryParam `json:"params,omitempty"`
	MaxRows     uint32       `json:"max_rows,omitempty"`
	Reason      string       `json:"reason,omitempty"`
}
type resourceView struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Database string `json:"database"`
}
type listResourcesResult struct {
	Resources []resourceView `json:"resources"`
}
type tablesResult struct {
	ResourceKey string               `json:"resource_key"`
	Tables      []queryservice.Table `json:"tables"`
}
type describeResult struct {
	ResourceKey string                `json:"resource_key"`
	Table       string                `json:"table"`
	Columns     []queryservice.Column `json:"columns"`
}

func (h *MCPHandler) newServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "db-access-gateway", Version: "0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "list_databases", Description: "List only the database resources granted to the authenticated user. Credentials and physical hosts are never returned."}, h.listResources)
	mcp.AddTool(server, &mcp.Tool{Name: "list_tables", Description: "List tables and views for one granted database resource. Requires schema_read or a higher action."}, h.listTables)
	mcp.AddTool(server, &mcp.Tool{Name: "describe_table", Description: "Describe columns for one table on a granted database resource. Requires schema_read or a higher action."}, h.describeTable)
	mcp.AddTool(server, &mcp.Tool{Name: "query_sql", Description: "Execute one MySQL SELECT, INSERT, UPDATE, or DELETE. SELECT requires query_read; DML requires query_write. DDL, multiple statements, cross-database access, locking SELECT, comments, dangerous functions, UPDATE/DELETE without WHERE are rejected."}, h.querySQL)
	return server
}

func (h *MCPHandler) listResources(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, listResourcesResult, error) {
	p, err := principalFrom(ctx)
	if err != nil {
		return toolError("unauthorized"), listResourcesResult{}, nil
	}
	items, err := h.service.VisibleResources(ctx, p)
	if err != nil {
		return toolError("failed to load resources"), listResourcesResult{}, nil
	}
	out := listResourcesResult{Resources: make([]resourceView, 0, len(items))}
	for _, item := range items {
		out.Resources = append(out.Resources, resourceView{Key: item.ResourceKey, Name: item.DisplayName, Database: item.DatabaseName})
	}
	return &mcp.CallToolResult{}, out, nil
}
func (h *MCPHandler) listTables(ctx context.Context, _ *mcp.CallToolRequest, args resourceArgs) (*mcp.CallToolResult, tablesResult, error) {
	p, err := principalFrom(ctx)
	if err != nil {
		return toolError("unauthorized"), tablesResult{}, nil
	}
	items, err := h.service.ListTables(ctx, p, args.ResourceKey)
	if err != nil {
		return toolError(safeMessage(err)), tablesResult{}, nil
	}
	return &mcp.CallToolResult{}, tablesResult{ResourceKey: args.ResourceKey, Tables: items}, nil
}
func (h *MCPHandler) describeTable(ctx context.Context, _ *mcp.CallToolRequest, args tableArgs) (*mcp.CallToolResult, describeResult, error) {
	p, err := principalFrom(ctx)
	if err != nil {
		return toolError("unauthorized"), describeResult{}, nil
	}
	items, err := h.service.DescribeTable(ctx, p, args.ResourceKey, args.Table)
	if err != nil {
		return toolError(safeMessage(err)), describeResult{}, nil
	}
	return &mcp.CallToolResult{}, describeResult{ResourceKey: args.ResourceKey, Table: args.Table, Columns: items}, nil
}
func (h *MCPHandler) querySQL(ctx context.Context, _ *mcp.CallToolRequest, args queryArgs) (*mcp.CallToolResult, queryservice.Result, error) {
	p, err := principalFrom(ctx)
	if err != nil {
		return toolError("unauthorized"), queryservice.Result{}, nil
	}
	params := make([]any, 0, len(args.Params))
	for _, param := range args.Params {
		value, err := decodeParam(param)
		if err != nil {
			return toolError(err.Error()), queryservice.Result{}, nil
		}
		params = append(params, value)
	}
	result, err := h.service.Execute(ctx, p, args.ResourceKey, args.SQL, params, args.MaxRows, args.Reason)
	if err != nil {
		return toolError(safeMessage(err)), queryservice.Result{}, nil
	}
	return &mcp.CallToolResult{}, result, nil
}

func decodeParam(param queryParam) (any, error) {
	switch strings.ToLower(strings.TrimSpace(param.Type)) {
	case "string", "":
		return param.Value, nil
	case "int":
		return strconv.ParseInt(param.Value, 10, 64)
	case "float":
		return strconv.ParseFloat(param.Value, 64)
	case "bool":
		return strconv.ParseBool(param.Value)
	case "null":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported parameter type %q", param.Type)
	}
}
func toolError(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: message}}}
}
func safeMessage(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if json.Valid([]byte(message)) {
		return "request failed"
	}
	return message
}
