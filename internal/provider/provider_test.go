package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/types"
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

	// client_id/client_secret are Optional in the schema (not Required): a value
	// may arrive via the attribute OR the AWDNS_* env var, with Configure
	// enforcing that one path supplies it. See resolveCredentials.
	if !resp.Schema.Attributes["client_id"].IsOptional() {
		t.Error("client_id should be optional (env-var fallback)")
	}
	if !resp.Schema.Attributes["client_secret"].IsOptional() {
		t.Error("client_secret should be optional (env-var fallback)")
	}
	if !resp.Schema.Attributes["client_secret"].IsSensitive() {
		t.Error("client_secret should be sensitive")
	}
	if !resp.Schema.Attributes["base_url"].IsOptional() {
		t.Error("base_url should be optional")
	}
}

func TestResolveCredentials(t *testing.T) {
	t.Run("config attributes win over env", func(t *testing.T) {
		t.Setenv(envClientID, "env-id")
		t.Setenv(envClientSecret, "env-secret")
		t.Setenv(envBaseURL, "https://env.example/external/v1")

		id, secret, base, diags := resolveCredentials(awdnsProviderModel{
			ClientID:     types.StringValue("cfg-id"),
			ClientSecret: types.StringValue("cfg-secret"),
			BaseURL:      types.StringValue("https://cfg.example/external/v1"),
		})
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if id != "cfg-id" || secret != "cfg-secret" || base != "https://cfg.example/external/v1" {
			t.Errorf("got (%q, %q, %q), want config values", id, secret, base)
		}
	})

	t.Run("env fills in null attributes; base_url defaults", func(t *testing.T) {
		t.Setenv(envClientID, "env-id")
		t.Setenv(envClientSecret, "env-secret")
		t.Setenv(envBaseURL, "")

		id, secret, base, diags := resolveCredentials(awdnsProviderModel{
			ClientID:     types.StringNull(),
			ClientSecret: types.StringNull(),
			BaseURL:      types.StringNull(),
		})
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if id != "env-id" || secret != "env-secret" {
			t.Errorf("got (%q, %q), want env values", id, secret)
		}
		if base != defaultBaseURL {
			t.Errorf("base_url = %q, want default %q", base, defaultBaseURL)
		}
	})

	t.Run("missing client_id and client_secret produce diagnostics", func(t *testing.T) {
		t.Setenv(envClientID, "")
		t.Setenv(envClientSecret, "")

		_, _, base, diags := resolveCredentials(awdnsProviderModel{
			ClientID:     types.StringNull(),
			ClientSecret: types.StringNull(),
			BaseURL:      types.StringNull(),
		})
		if !diags.HasError() {
			t.Fatal("expected diagnostics for missing credentials, got none")
		}
		if got := diags.ErrorsCount(); got != 2 {
			t.Errorf("error count = %d, want 2 (client_id + client_secret)", got)
		}
		if base != defaultBaseURL {
			t.Errorf("base_url = %q, want default %q even when creds missing", base, defaultBaseURL)
		}
	})
}
