package config

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen             string        `mapstructure:"listen"`
	ListenMode         string        `mapstructure:"listen_mode"`
	AppendOnly         bool          `mapstructure:"append_only"`
	LogLevel           string        `mapstructure:"log_level"`
	ShutdownTimeout    int           `mapstructure:"shutdown_timeout"`
	Tailscale          Tailscale     `mapstructure:"tailscale"`
	Storage            Storage       `mapstructure:"storage"`
	ACLFile            string        `mapstructure:"acl_file"`
	ACL                *ACLConfig    `mapstructure:"acl"`
	MaxRequestBodySize string        `mapstructure:"max_request_body_size"`
	Metrics            MetricsConfig `mapstructure:"metrics"`
	Stats              StatsConfig   `mapstructure:"stats"`
	UI                 UIConfig      `mapstructure:"ui"`
}

type StatsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	DBPath  string `mapstructure:"db_path"`
}

type MetricsConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	Password       string `mapstructure:"password"`
	PerHostEnabled bool   `mapstructure:"per_host_enabled"`
	ACLEnabled     bool   `mapstructure:"acl_enabled"`
}

type UIConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Auth    UIAuth `mapstructure:"auth"`
}

type UIAuth struct {
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

type ACLConfig struct {
	DefaultRole       string    `mapstructure:"default_role" yaml:"default_role"`
	TrustedProxies    []string  `mapstructure:"trusted_proxies" yaml:"trusted_proxies"`
	DNSServer         string    `mapstructure:"dns_server" yaml:"dns_server"`
	RDNSCacheTTL      int       `mapstructure:"rdns_cache_ttl" yaml:"rdns_cache_ttl"`
	IdentityCacheSize int       `mapstructure:"identity_cache_size" yaml:"identity_cache_size"`
	VerboseDenials    bool      `mapstructure:"verbose_denials" yaml:"verbose_denials"`
	Rules             []ACLRule `mapstructure:"rules" yaml:"rules"`
}

type ACLRule struct {
	Paths      []string `mapstructure:"paths" yaml:"paths"`
	Identities []string `mapstructure:"identities" yaml:"identities"`
	Permission string   `mapstructure:"permission" yaml:"permission"`
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
	SMB            SMB    `mapstructure:"smb"`
	NFS            NFS    `mapstructure:"nfs"`
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

type SMB struct {
	Server   string `mapstructure:"server"`
	Share    string `mapstructure:"share"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Domain   string `mapstructure:"domain"`
	Port     int    `mapstructure:"port"`
	BasePath string `mapstructure:"base_path"`
}

type NFS struct {
	Server   string `mapstructure:"server"`
	Export   string `mapstructure:"export"`
	BasePath string `mapstructure:"base_path"`
	UID      uint32 `mapstructure:"uid"`
	GID      uint32 `mapstructure:"gid"`
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
	viper.SetDefault("shutdown_timeout", 30)
	viper.SetDefault("max_request_body_size", "256MB")
	viper.SetDefault("storage.smb.port", 445)
	viper.SetDefault("storage.smb.domain", "WORKGROUP")
	viper.SetDefault("storage.nfs.uid", 65534)
	viper.SetDefault("storage.nfs.gid", 65534)
	viper.SetDefault("acl.identity_cache_size", 1000)
	viper.SetDefault("acl.verbose_denials", true)
	viper.SetDefault("metrics.enabled", true)
	viper.SetDefault("metrics.password", "")
	viper.SetDefault("metrics.per_host_enabled", true)
	viper.SetDefault("metrics.acl_enabled", false)
	viper.SetDefault("stats.enabled", false)
	viper.SetDefault("stats.db_path", "./stats.db")
	viper.SetDefault("ui.enabled", false)
}

func Load(envLenient bool) (*Config, error) {
	SetDefaults()

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	// Viper may set ACL to a zero-value struct instead of nil when
	// no acl: block is present. Normalize to nil if effectively empty.
	if cfg.ACL != nil && cfg.ACL.DefaultRole == "" && len(cfg.ACL.Rules) == 0 && len(cfg.ACL.TrustedProxies) == 0 && cfg.ACL.DNSServer == "" && cfg.ACL.RDNSCacheTTL == 0 && cfg.ACL.IdentityCacheSize == 0 {
		cfg.ACL = nil
	}

	// Resolve ${VAR_NAME} placeholders in all string fields.
	if err := ResolveEnvVars(&cfg, envLenient); err != nil {
		return nil, fmt.Errorf("resolving env vars: %w", err)
	}

	// Load ACL from separate file if configured.
	if cfg.ACLFile != "" {
		if cfg.ACL != nil {
			return nil, fmt.Errorf("both acl_file and inline acl specified; use one or the other")
		}
		aclCfg, err := loadACLFile(cfg.ACLFile, envLenient)
		if err != nil {
			return nil, fmt.Errorf("loading acl file %q: %w", cfg.ACLFile, err)
		}
		cfg.ACL = aclCfg
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

func loadACLFile(path string, envLenient bool) (*ACLConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var aclCfg ACLConfig
	if err := yaml.Unmarshal(data, &aclCfg); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	if err := ResolveEnvVars(&aclCfg, envLenient); err != nil {
		return nil, err
	}

	if err := aclCfg.Validate(); err != nil {
		return nil, err
	}

	return &aclCfg, nil
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

	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("shutdown_timeout must be a positive integer (got %d)", c.ShutdownTimeout)
	}

	switch c.Storage.Backend {
	case "filesystem", "s3", "memory", "webdav", "rclone", "smb", "nfs":
	default:
		return fmt.Errorf("invalid storage.backend %q: must be \"filesystem\", \"s3\", \"memory\", \"webdav\", \"rclone\", \"smb\", or \"nfs\"", c.Storage.Backend)
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

	if c.Storage.Backend == "smb" {
		if c.Storage.SMB.Server == "" {
			return fmt.Errorf("storage.smb.server is required for smb backend")
		}
		if c.Storage.SMB.Share == "" {
			return fmt.Errorf("storage.smb.share is required for smb backend")
		}
		if c.Storage.SMB.Port <= 0 || c.Storage.SMB.Port > 65535 {
			return fmt.Errorf("storage.smb.port must be between 1 and 65535 (got %d)", c.Storage.SMB.Port)
		}
	}

	if c.Storage.Backend == "nfs" {
		if c.Storage.NFS.Server == "" {
			return fmt.Errorf("storage.nfs.server is required for nfs backend")
		}
		if c.Storage.NFS.Export == "" {
			return fmt.Errorf("storage.nfs.export is required for nfs backend")
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
	if a.DNSServer != "" {
		if _, _, err := net.SplitHostPort(a.DNSServer); err != nil {
			return fmt.Errorf("acl.dns_server %q must be in host:port format: %w", a.DNSServer, err)
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
