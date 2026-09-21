package api

import "testing"

func TestValidateResourceRequiresSingleCredential(t *testing.T) {
	_, err := validateResource(resourceRequest{
		ResourceKey: "db", Host: "mysql", DatabaseName: "app",
	})
	if err == nil {
		t.Fatal("expected missing resource credential to fail")
	}
}

func TestValidateResourceTrimsSingleCredential(t *testing.T) {
	r, err := validateResource(resourceRequest{
		ResourceKey: "db", Host: "mysql", DatabaseName: "app",
		Username: " app_user ", SecretRef: " app_prod ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Username != "app_user" || r.SecretRef != "app_prod" {
		t.Fatalf("credentials were not trimmed: username=%q secret_ref=%q", r.Username, r.SecretRef)
	}
}
