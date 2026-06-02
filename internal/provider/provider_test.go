package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
)

func TestNewReturnsProvider(t *testing.T) {
	if New() == nil {
		t.Fatal("New() returned nil")
	}
}

func TestMetadata(t *testing.T) {
	resp := &provider.MetadataResponse{}
	New().Metadata(context.Background(), provider.MetadataRequest{}, resp)

	if got, want := resp.TypeName, "awdns"; got != want {
		t.Errorf("TypeName = %q, want %q", got, want)
	}
	if got, want := resp.Version, version; got != want {
		t.Errorf("Version = %q, want %q", got, want)
	}
}

func TestSchemaShape(t *testing.T) {
	resp := &provider.SchemaResponse{}
	New().Schema(context.Background(), provider.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema produced diagnostics: %v", resp.Diagnostics)
	}

	for _, name := range []string{"client_id", "client_secret", "base_url", "request_timeout"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("schema missing attribute %q", name)
		}
	}

	if !resp.Schema.Attributes["client_id"].IsRequired() {
		t.Error("client_id should be required")
	}
	if !resp.Schema.Attributes["client_secret"].IsRequired() {
		t.Error("client_secret should be required")
	}
	if !resp.Schema.Attributes["client_secret"].IsSensitive() {
		t.Error("client_secret should be sensitive")
	}
	if !resp.Schema.Attributes["base_url"].IsOptional() {
		t.Error("base_url should be optional")
	}
}
