package config

import (
	"os"
	"strings"
	"testing"
)

func TestResolveEnvVars_Basic(t *testing.T) {
	t.Setenv("TEST_ENVSUBST_BASIC", "hello")

	type simple struct {
		Value string
	}
	s := simple{Value: "${TEST_ENVSUBST_BASIC}"}
	if err := ResolveEnvVars(&s, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Value != "hello" {
		t.Fatalf("got %q, want %q", s.Value, "hello")
	}
}

func TestResolveEnvVars_Multiple(t *testing.T) {
	t.Setenv("TEST_ENVSUBST_A", "alpha")
	t.Setenv("TEST_ENVSUBST_B", "beta")

	type multi struct {
		First  string
		Second string
	}
	m := multi{First: "${TEST_ENVSUBST_A}", Second: "${TEST_ENVSUBST_B}"}
	if err := ResolveEnvVars(&m, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.First != "alpha" {
		t.Fatalf("First: got %q, want %q", m.First, "alpha")
	}
	if m.Second != "beta" {
		t.Fatalf("Second: got %q, want %q", m.Second, "beta")
	}
}

func TestResolveEnvVars_NestedStruct(t *testing.T) {
	t.Setenv("TEST_ENVSUBST_SECRET", "s3cr3t")

	cfg := Config{
		ListenMode: "plain",
		Storage: Storage{
			Backend: "filesystem",
			Path:    "/tmp/test",
			S3: S3{
				SecretKey: "${TEST_ENVSUBST_SECRET}",
			},
		},
	}
	if err := ResolveEnvVars(&cfg, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Storage.S3.SecretKey != "s3cr3t" { // pragma: allowlist secret
		t.Fatalf("S3.SecretKey: got %q, want %q", cfg.Storage.S3.SecretKey, "s3cr3t")
	}
}

func TestResolveEnvVars_PartialString(t *testing.T) {
	t.Setenv("TEST_ENVSUBST_MID", "middle")

	type partial struct {
		Value string
	}
	p := partial{Value: "prefix-${TEST_ENVSUBST_MID}-suffix"}
	if err := ResolveEnvVars(&p, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Value != "prefix-middle-suffix" {
		t.Fatalf("got %q, want %q", p.Value, "prefix-middle-suffix")
	}
}

func TestResolveEnvVars_MissingStrict(t *testing.T) {
	os.Unsetenv("TEST_ENVSUBST_MISSING")

	type simple struct {
		Value string
	}
	s := simple{Value: "${TEST_ENVSUBST_MISSING}"}
	err := ResolveEnvVars(&s, false)
	if err == nil {
		t.Fatal("expected error for missing variable in strict mode")
	}
	if !strings.Contains(err.Error(), "TEST_ENVSUBST_MISSING") {
		t.Fatalf("error should mention variable name, got: %v", err)
	}
}

func TestResolveEnvVars_MissingLenient(t *testing.T) {
	os.Unsetenv("TEST_ENVSUBST_MISSING")

	type simple struct {
		Value string
	}
	s := simple{Value: "${TEST_ENVSUBST_MISSING}"}
	if err := ResolveEnvVars(&s, true); err != nil {
		t.Fatalf("unexpected error in lenient mode: %v", err)
	}
	if s.Value != "${TEST_ENVSUBST_MISSING}" {
		t.Fatalf("placeholder should remain, got %q", s.Value)
	}
}

func TestResolveEnvVars_NoPlaceholders(t *testing.T) {
	type simple struct {
		Value string
	}
	s := simple{Value: "no-placeholders-here"}
	if err := ResolveEnvVars(&s, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Value != "no-placeholders-here" {
		t.Fatalf("value should be unchanged, got %q", s.Value)
	}
}

func TestResolveEnvVars_SliceFields(t *testing.T) {
	t.Setenv("TEST_ENVSUBST_SLICE", "resolved")

	cfg := Config{
		ACL: &ACLConfig{
			DefaultRole:    "deny",
			TrustedProxies: []string{"${TEST_ENVSUBST_SLICE}/8"},
			Rules: []ACLRule{
				{
					Paths:      []string{"/${TEST_ENVSUBST_SLICE}"},
					Identities: []string{"${TEST_ENVSUBST_SLICE}@example.com"},
					Permission: "full-access",
				},
			},
		},
	}
	if err := ResolveEnvVars(&cfg, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ACL.TrustedProxies[0] != "resolved/8" {
		t.Fatalf("TrustedProxies[0]: got %q, want %q", cfg.ACL.TrustedProxies[0], "resolved/8")
	}
	if cfg.ACL.Rules[0].Paths[0] != "/resolved" {
		t.Fatalf("Rules[0].Paths[0]: got %q, want %q", cfg.ACL.Rules[0].Paths[0], "/resolved")
	}
	if cfg.ACL.Rules[0].Identities[0] != "resolved@example.com" {
		t.Fatalf("Rules[0].Identities[0]: got %q, want %q", cfg.ACL.Rules[0].Identities[0], "resolved@example.com")
	}
}

func TestResolveEnvVars_NilPointer(t *testing.T) {
	cfg := Config{
		ACL: nil,
	}
	if err := ResolveEnvVars(&cfg, false); err != nil {
		t.Fatalf("nil pointer should not cause error: %v", err)
	}
}
