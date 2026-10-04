package config

import (
	"testing"

	"github.com/spf13/viper"
)

// Regression: ACL defaults (identity_cache_size, verbose_denials) made viper
// allocate an ACL struct even without an acl: block, so Load failed with
// "acl.default_role must be ..." and serve could not start without ACL.
func TestLoad_WithoutACLBlock_ACLDisabled(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	cfg, err := Load(false)
	if err != nil {
		t.Fatalf("Load without acl block: %v", err)
	}
	if cfg.ACL != nil {
		t.Fatalf("ACL should be nil without acl block, got %+v", cfg.ACL)
	}
}

func TestLoad_WithACLBlock_ACLEnabledWithDefaults(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("acl.default_role", "deny")

	cfg, err := Load(false)
	if err != nil {
		t.Fatalf("Load with acl block: %v", err)
	}
	if cfg.ACL == nil {
		t.Fatal("ACL should be enabled when acl.default_role is set")
	}
	if cfg.ACL.IdentityCacheSize != 1000 || !cfg.ACL.VerboseDenials {
		t.Fatalf("ACL defaults not applied: %+v", cfg.ACL)
	}
}
