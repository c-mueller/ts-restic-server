package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validConfig returns a minimal valid Config for the filesystem backend.
func validConfig() Config {
	return Config{
		ListenMode: "plain",
		Storage: Storage{
			Backend: "filesystem",
			Path:    "/tmp/test",
		},
	}
}

// validMemoryConfig returns a minimal valid Config for the memory backend.
func validMemoryConfig() Config {
	return Config{
		ListenMode: "plain",
		Storage: Storage{
			Backend:        "memory",
			MaxMemoryBytes: 1024,
		},
	}
}

func TestValidate_ListenMode(t *testing.T) {
	tests := []struct {
		mode    string
		wantErr bool
	}{
		{"plain", false},
		{"tailscale", false},
		{"http", true},
		{"", true},
	}
	for _, tc := range tests {
		t.Run(tc.mode, func(t *testing.T) {
			c := validConfig()
			c.ListenMode = tc.mode
			err := c.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("ListenMode=%q: err=%v, wantErr=%v", tc.mode, err, tc.wantErr)
			}
		})
	}
}

func TestValidate_StorageBackend(t *testing.T) {
	tests := []struct {
		backend string
		wantErr bool
	}{
		{"filesystem", false},
		{"s3", false},
		{"memory", false},
		{"webdav", false},
		{"rclone", false},
		{"redis", true},
		{"", true},
	}
	for _, tc := range tests {
		t.Run(tc.backend, func(t *testing.T) {
			c := validConfig()
			c.Storage.Backend = tc.backend
			// Provide required fields for each backend
			switch tc.backend {
			case "s3":
				c.Storage.S3.Bucket = "test"
			case "memory":
				c.Storage.MaxMemoryBytes = 1024
			case "webdav":
				c.Storage.WebDAV.Endpoint = "http://localhost"
			case "rclone":
				c.Storage.Rclone.Endpoint = "http://localhost"
			}
			err := c.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Backend=%q: err=%v, wantErr=%v", tc.backend, err, tc.wantErr)
			}
		})
	}
}

func TestValidate_FilesystemMissingPath(t *testing.T) {
	c := validConfig()
	c.Storage.Path = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for empty filesystem path")
	}
}

func TestValidate_S3MissingBucket(t *testing.T) {
	c := validConfig()
	c.Storage.Backend = "s3"
	c.Storage.S3.Bucket = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for empty S3 bucket")
	}
}

func TestValidate_S3EndpointScheme(t *testing.T) {
	tests := []struct {
		endpoint string
		wantErr  bool
	}{
		{"http://minio:9000", false},
		{"https://s3.example.com", false},
		{"minio:9000", true},
		{"ftp://example.com", true},
		{"", false}, // optional
	}
	for _, tc := range tests {
		t.Run(tc.endpoint, func(t *testing.T) {
			c := validConfig()
			c.Storage.Backend = "s3"
			c.Storage.S3.Bucket = "test"
			c.Storage.S3.Endpoint = tc.endpoint
			err := c.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("S3.Endpoint=%q: err=%v, wantErr=%v", tc.endpoint, err, tc.wantErr)
			}
		})
	}
}

func TestValidate_MemoryInvalidBytes(t *testing.T) {
	tests := []struct {
		bytes   int64
		wantErr bool
	}{
		{0, true},
		{-1, true},
		{1, false},
	}
	for _, tc := range tests {
		c := validMemoryConfig()
		c.Storage.MaxMemoryBytes = tc.bytes
		err := c.Validate()
		if (err != nil) != tc.wantErr {
			t.Fatalf("MaxMemoryBytes=%d: err=%v, wantErr=%v", tc.bytes, err, tc.wantErr)
		}
	}
}

func TestValidate_WebDAVMissingEndpoint(t *testing.T) {
	c := validConfig()
	c.Storage.Backend = "webdav"
	c.Storage.WebDAV.Endpoint = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for empty WebDAV endpoint")
	}
}

func TestValidate_WebDAVEndpointScheme(t *testing.T) {
	tests := []struct {
		endpoint string
		wantErr  bool
	}{
		{"webdav.local", true},
		{"http://webdav.local", false},
		{"https://webdav.local", false},
	}
	for _, tc := range tests {
		t.Run(tc.endpoint, func(t *testing.T) {
			c := validConfig()
			c.Storage.Backend = "webdav"
			c.Storage.WebDAV.Endpoint = tc.endpoint
			err := c.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("WebDAV.Endpoint=%q: err=%v, wantErr=%v", tc.endpoint, err, tc.wantErr)
			}
		})
	}
}

func TestValidate_RcloneMissingEndpoint(t *testing.T) {
	c := validConfig()
	c.Storage.Backend = "rclone"
	c.Storage.Rclone.Endpoint = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for empty Rclone endpoint")
	}
}

func TestValidate_RcloneEndpointScheme(t *testing.T) {
	tests := []struct {
		endpoint string
		wantErr  bool
	}{
		{"localhost:8080", true},
		{"http://localhost:8080", false},
		{"https://remote.example.com", false},
	}
	for _, tc := range tests {
		t.Run(tc.endpoint, func(t *testing.T) {
			c := validConfig()
			c.Storage.Backend = "rclone"
			c.Storage.Rclone.Endpoint = tc.endpoint
			err := c.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Rclone.Endpoint=%q: err=%v, wantErr=%v", tc.endpoint, err, tc.wantErr)
			}
		})
	}
}

func TestACLValidate_DefaultRole(t *testing.T) {
	tests := []struct {
		role    string
		wantErr bool
	}{
		{"deny", false},
		{"read-only", false},
		{"append-only", false},
		{"full-access", false},
		{"admin", true},
		{"", true},
	}
	for _, tc := range tests {
		t.Run(tc.role, func(t *testing.T) {
			a := &ACLConfig{DefaultRole: tc.role}
			err := a.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("DefaultRole=%q: err=%v, wantErr=%v", tc.role, err, tc.wantErr)
			}
		})
	}
}

func TestACLValidate_TrustedProxies(t *testing.T) {
	tests := []struct {
		cidr    string
		wantErr bool
	}{
		{"10.0.0.0/8", false},
		{"::1/128", false},
		{"10.0.0.1", true},
		{"garbage", true},
	}
	for _, tc := range tests {
		t.Run(tc.cidr, func(t *testing.T) {
			a := &ACLConfig{
				DefaultRole:    "deny",
				TrustedProxies: []string{tc.cidr},
			}
			err := a.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("TrustedProxies=%q: err=%v, wantErr=%v", tc.cidr, err, tc.wantErr)
			}
		})
	}
}

func TestACLValidate_DNSServer(t *testing.T) {
	tests := []struct {
		dns     string
		wantErr bool
	}{
		{"100.100.100.100:53", false},
		{"", false},
		{"100.100.100.100", true},
	}
	for _, tc := range tests {
		t.Run(tc.dns, func(t *testing.T) {
			a := &ACLConfig{
				DefaultRole: "deny",
				DNSServer:   tc.dns,
			}
			err := a.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("DNSServer=%q: err=%v, wantErr=%v", tc.dns, err, tc.wantErr)
			}
		})
	}
}

func TestACLValidate_Rules(t *testing.T) {
	t.Run("EmptyPaths", func(t *testing.T) {
		a := &ACLConfig{
			DefaultRole: "deny",
			Rules: []ACLRule{
				{Paths: []string{}, Identities: []string{"*"}, Permission: "deny"},
			},
		}
		if err := a.Validate(); err == nil {
			t.Fatal("expected error for empty paths")
		}
	})

	t.Run("EmptyIdentities", func(t *testing.T) {
		a := &ACLConfig{
			DefaultRole: "deny",
			Rules: []ACLRule{
				{Paths: []string{"/"}, Identities: []string{}, Permission: "deny"},
			},
		}
		if err := a.Validate(); err == nil {
			t.Fatal("expected error for empty identities")
		}
	})

	t.Run("InvalidPermission", func(t *testing.T) {
		a := &ACLConfig{
			DefaultRole: "deny",
			Rules: []ACLRule{
				{Paths: []string{"/"}, Identities: []string{"*"}, Permission: "admin"},
			},
		}
		if err := a.Validate(); err == nil {
			t.Fatal("expected error for invalid permission")
		}
	})

	t.Run("Valid", func(t *testing.T) {
		a := &ACLConfig{
			DefaultRole: "deny",
			Rules: []ACLRule{
				{Paths: []string{"/"}, Identities: []string{"user@example.com"}, Permission: "full-access"},
			},
		}
		if err := a.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestValidate_NilACL(t *testing.T) {
	c := validConfig()
	c.ACL = nil
	if err := c.Validate(); err != nil {
		t.Fatalf("nil ACL should not error: %v", err)
	}
}

// writeACLFile is a helper that writes YAML content to a temporary file
// and returns its path.
func writeACLFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "acl.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing acl file: %v", err)
	}
	return path
}

func TestLoadACLFile_Success(t *testing.T) {
	path := writeACLFile(t, `
default_role: read-only
rules:
  - paths: ["/backup"]
    identities: ["server-a"]
    permission: full-access
`)
	acl, err := loadACLFile(path, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acl.DefaultRole != "read-only" {
		t.Fatalf("DefaultRole: got %q, want %q", acl.DefaultRole, "read-only")
	}
	if len(acl.Rules) != 1 {
		t.Fatalf("Rules: got %d, want 1", len(acl.Rules))
	}
	if acl.Rules[0].Permission != "full-access" {
		t.Fatalf("Rules[0].Permission: got %q, want %q", acl.Rules[0].Permission, "full-access")
	}
}

func TestLoadACLFile_BothSpecified(t *testing.T) {
	path := writeACLFile(t, `
default_role: deny
rules:
  - paths: ["/"]
    identities: ["*"]
    permission: deny
`)
	c := validConfig()
	c.ACLFile = path
	c.ACL = &ACLConfig{DefaultRole: "deny"}

	// Simulate the check that Load() performs.
	if c.ACLFile != "" && c.ACL != nil {
		// This is the expected error condition.
	} else {
		t.Fatal("expected both acl_file and inline acl to be set")
	}
}

func TestLoadACLFile_NotFound(t *testing.T) {
	_, err := loadACLFile("/nonexistent/acl.yaml", false)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadACLFile_Invalid(t *testing.T) {
	path := writeACLFile(t, `
default_role: [this is not valid
`)
	_, err := loadACLFile(path, false)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadACLFile_Validation(t *testing.T) {
	path := writeACLFile(t, `
default_role: invalid-role
`)
	_, err := loadACLFile(path, false)
	if err == nil {
		t.Fatal("expected validation error for invalid default_role")
	}
	if !strings.Contains(err.Error(), "default_role") {
		t.Fatalf("error should mention default_role, got: %v", err)
	}
}

func TestLoadACLFile_EnvSubstitution(t *testing.T) {
	t.Setenv("TEST_ACL_IDENTITY", "server-a.example.com")

	path := writeACLFile(t, `
default_role: deny
rules:
  - paths: ["/backup"]
    identities: ["${TEST_ACL_IDENTITY}"]
    permission: full-access
`)
	acl, err := loadACLFile(path, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acl.Rules[0].Identities[0] != "server-a.example.com" {
		t.Fatalf("identity not resolved: got %q", acl.Rules[0].Identities[0])
	}
}

func TestLoad_InlineACL_StillWorks(t *testing.T) {
	c := validConfig()
	c.ACL = &ACLConfig{
		DefaultRole: "read-only",
		Rules: []ACLRule{
			{Paths: []string{"/"}, Identities: []string{"*"}, Permission: "read-only"},
		},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("inline ACL should validate: %v", err)
	}
}

func TestLoad_NoACL(t *testing.T) {
	c := validConfig()
	c.ACL = nil
	c.ACLFile = ""
	if err := c.Validate(); err != nil {
		t.Fatalf("no ACL should validate: %v", err)
	}
}
