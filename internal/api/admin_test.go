package api

import "testing"

func TestValidateResourceRequiresPairedWriteCredentials(t *testing.T) {
	user := "writer"
	_, err := validateResource(resourceRequest{
		ResourceKey: "db", Host: "mysql", DatabaseName: "app",
		ReadUsername: "reader", ReadSecretRef: "read", WriteUsername: &user,
	})
	if err == nil {
		t.Fatal("expected unpaired write credentials to fail")
	}
}

func TestValidateResourceNormalizesEmptyWriteCredentials(t *testing.T) {
	empty := "  "
	r, err := validateResource(resourceRequest{
		ResourceKey: "db", Host: "mysql", DatabaseName: "app",
		ReadUsername: "reader", ReadSecretRef: "read",
		WriteUsername: &empty, WriteSecretRef: &empty,
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.WriteUsername != nil || r.WriteSecretRef != nil {
		t.Fatal("empty optional credentials must normalize to nil")
	}
}
