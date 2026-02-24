package config

import (
	"fmt"
	"net"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Listen     string     `mapstructure:"listen"`
	ListenMode string     `mapstructure:"listen_mode"`
	AppendOnly bool       `mapstructure:"append_only"`
	LogLevel   string     `mapstructure:"log_level"`
	Tailscale  Tailscale  `mapstructure:"tailscale"`
	Storage    Storage    `mapstructure:"storage"`
	ACL        *ACLConfig `mapstructure:"acl"`
}

type ACLConfig struct {
	DefaultRole    string    `mapstructure:"default_role"`
	TrustedProxies []string  `mapstructure:"trusted_proxies"`
	Rules          []ACLRule `mapstructure:"rules"`
}

type ACLRule struct {
	Paths      []string `mapstructure:"paths"`
	Identities []string `mapstructure:"identities"`
	Permission string   `mapstructure:"permission"`
}

type Tailscale struct {
	Hostname string `mapstructure:"hostname"`
	StateDir string `mapstructure:"state_dir"`
	AuthKey  string `mapstructure:"auth_key"`
}

type Storage struct {
	Backend        string `mapstructure:"backend"`
	Path           string `mapstructure:"path"`
	MaxMemoryBytes int64  `mapstructure:"max_memory_bytes"`
	DataSharding   bool   `mapstructure:"data_sharding"`
	S3             S3     `mapstructure:"s3"`
	WebDAV         WebDAV `mapstructure:"webdav"`
	Rclone         Rclone `mapstructure:"rclone"`
}

type Rclone struct {
	Endpoint string `mapstructure:"endpoint"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

type WebDAV struct {
	Endpoint string `mapstructure:"endpoint"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Prefix   string `mapstructure:"prefix"`
}

type S3 struct {
	Bucket    string `mapstructure:"bucket"`
	Prefix    string `mapstructure:"prefix"`
	Region    string `mapstructure:"region"`
	Endpoint  string `mapstructure:"endpoint"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
}

func SetDefaults() {
	viper.SetDefault("listen", ":8880")
	viper.SetDefault("listen_mode", "plain")
	viper.SetDefault("append_only", false)
	viper.SetDefault("log_level", "info")
	viper.SetDefault("tailscale.hostname", "restic-server")
	viper.SetDefault("tailscale.state_dir", "./ts-state")
	viper.SetDefault("storage.backend", "filesystem")
	viper.SetDefault("storage.path", "./restic_data")
	viper.SetDefault("storage.max_memory_bytes", 104857600) // 100MB
	viper.SetDefault("storage.data_sharding", true)
}

func Load() (*Config, error) {
	SetDefaults()

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	// Viper may set ACL to a zero-value struct instead of nil when
	// no acl: block is present. Normalize to nil if effectively empty.
	if cfg.ACL != nil && cfg.ACL.DefaultRole == "" && len(cfg.ACL.Rules) == 0 && len(cfg.ACL.TrustedProxies) == 0 {
		cfg.ACL = nil
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

type ValidationError struct {
	Field string
	Value string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid value %q for %s", e.Value, e.Field)
}

func (c *Config) Validate() error {
	switch c.ListenMode {
	case "plain", "tailscale":
	default:
		return fmt.Errorf("invalid listen_mode %q: must be \"plain\" or \"tailscale\"", c.ListenMode)
	}

	switch c.Storage.Backend {
	case "filesystem", "s3", "memory", "webdav", "rclone":
	default:
		return fmt.Errorf("invalid storage.backend %q: must be \"filesystem\", \"s3\", \"memory\", \"webdav\", or \"rclone\"", c.Storage.Backend)
	}

	if c.Storage.Backend == "filesystem" && c.Storage.Path == "" {
		return fmt.Errorf("storage.path is required for filesystem backend")
	}

	if c.Storage.Backend == "s3" && c.Storage.S3.Bucket == "" {
		return fmt.Errorf("storage.s3.bucket is required for s3 backend")
	}

	if c.Storage.Backend == "s3" && c.Storage.S3.Endpoint != "" {
		if !strings.HasPrefix(c.Storage.S3.Endpoint, "http://") && !strings.HasPrefix(c.Storage.S3.Endpoint, "https://") {
			return fmt.Errorf("storage.s3.endpoint %q must include a scheme (http:// or https://)", c.Storage.S3.Endpoint)
		}
	}

	if c.Storage.Backend == "memory" && c.Storage.MaxMemoryBytes <= 0 {
		return fmt.Errorf("storage.max_memory_bytes must be positive for memory backend")
	}

	if c.Storage.Backend == "webdav" && c.Storage.WebDAV.Endpoint == "" {
		return fmt.Errorf("storage.webdav.endpoint is required for webdav backend")
	}

	if c.Storage.Backend == "webdav" && c.Storage.WebDAV.Endpoint != "" {
		if !strings.HasPrefix(c.Storage.WebDAV.Endpoint, "http://") && !strings.HasPrefix(c.Storage.WebDAV.Endpoint, "https://") {
			return fmt.Errorf("storage.webdav.endpoint %q must include a scheme (http:// or https://)", c.Storage.WebDAV.Endpoint)
		}
	}

	if c.Storage.Backend == "rclone" && c.Storage.Rclone.Endpoint == "" {
		return fmt.Errorf("storage.rclone.endpoint is required for rclone backend")
	}

	if c.Storage.Backend == "rclone" && c.Storage.Rclone.Endpoint != "" {
		if !strings.HasPrefix(c.Storage.Rclone.Endpoint, "http://") && !strings.HasPrefix(c.Storage.Rclone.Endpoint, "https://") {
			return fmt.Errorf("storage.rclone.endpoint %q must include a scheme (http:// or https://)", c.Storage.Rclone.Endpoint)
		}
	}

	if c.ACL != nil {
		if err := c.ACL.Validate(); err != nil {
			return err
		}
	}

	return nil
}

var validPermissions = map[string]bool{
	"deny": true, "read-only": true, "append-only": true, "full-access": true,
}

func (a *ACLConfig) Validate() error {
	if !validPermissions[a.DefaultRole] {
		return fmt.Errorf("acl.default_role %q must be deny, read-only, append-only, or full-access", a.DefaultRole)
	}
	for i, cidr := range a.TrustedProxies {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("acl.trusted_proxies[%d] %q is not a valid CIDR: %w", i, cidr, err)
		}
	}
	for i, r := range a.Rules {
		if len(r.Paths) == 0 {
			return fmt.Errorf("acl.rules[%d].paths must not be empty", i)
		}
		if len(r.Identities) == 0 {
			return fmt.Errorf("acl.rules[%d].identities must not be empty", i)
		}
		if !validPermissions[r.Permission] {
			return fmt.Errorf("acl.rules[%d].permission %q must be deny, read-only, append-only, or full-access", i, r.Permission)
		}
	}
	return nil
}
