package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yogel/db-access-gateway/internal/control"
)

func TestValidateResourceRequiresPasswordOnCreate(t *testing.T) {
	_, err := validateResource(resourceRequest{
		ResourceKey: "db", Host: "mysql", DatabaseName: "app",
	}, true)
	if err == nil {
		t.Fatal("expected missing resource password to fail")
	}
}

func TestValidateResourcePreservesPasswordAndTrimsFields(t *testing.T) {
	r, err := validateResource(resourceRequest{
		ResourceKey: "db", Host: "mysql", DatabaseName: "app",
		Username: " app_user ", Password: " password with spaces ",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if r.Username != "app_user" || r.Password != " password with spaces " {
		t.Fatalf("fields were not normalized correctly: username=%q password=%q", r.Username, r.Password)
	}
}

func TestValidateResourceAllowsBlankPasswordOnUpdate(t *testing.T) {
	if _, err := validateResource(resourceRequest{
		ResourceKey: "db", Host: "mysql", DatabaseName: "app", Username: "app_user",
	}, false); err != nil {
		t.Fatal(err)
	}
}

func TestValidateResourceBatchSharesConnectionFields(t *testing.T) {
	resources, err := validateResourceBatch(resourceBatchRequest{
		Host:     "mysql",
		Username: "app_user",
		Password: "app-password",
		Resources: []resourceBatchItem{
			{ResourceKey: "app", DatabaseName: "app"},
			{ResourceKey: "orders", DatabaseName: "orders"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 2 || resources[1].Host != "mysql" || resources[1].Password != "app-password" {
		t.Fatalf("batch connection fields were not copied: %+v", resources)
	}
}

func TestResourceJSONDoesNotExposePasswordOrLegacyReference(t *testing.T) {
	payload, err := json.Marshal(control.Resource{Password: "database-password", SecretRef: "legacy_ref", PasswordSet: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "database-password") || strings.Contains(string(payload), "legacy_ref") {
		t.Fatalf("resource credentials leaked in JSON: %s", payload)
	}
}
