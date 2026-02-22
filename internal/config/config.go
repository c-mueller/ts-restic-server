package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	Listen     string     `mapstructure:"listen"`
	ListenMode string     `mapstructure:"listen_mode"`
	AppendOnly bool       `mapstructure:"append_only"`
	LogLevel   string     `mapstructure:"log_level"`
	Tailscale  Tailscale  `mapstructure:"tailscale"`
	Storage    Storage    `mapstructure:"storage"`
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
	S3             S3     `mapstructure:"s3"`
}

type S3 struct {
	Bucket   string `mapstructure:"bucket"`
	Prefix   string `mapstructure:"prefix"`
	Region   string `mapstructure:"region"`
	Endpoint string `mapstructure:"endpoint"`
}

func SetDefaults() {
	viper.SetDefault("listen", ":8880")
	viper.SetDefault("listen_mode", "plain")
	viper.SetDefault("append_only", false)
	viper.SetDefault("log_level", "info")
	viper.SetDefault("tailscale.hostname", "restic-server")
	viper.SetDefault("tailscale.state_dir", "/var/lib/ts-restic")
	viper.SetDefault("storage.backend", "filesystem")
	viper.SetDefault("storage.path", "/var/lib/restic")
	viper.SetDefault("storage.max_memory_bytes", 104857600) // 100MB
}

func Load() (*Config, error) {
	SetDefaults()

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
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
	case "filesystem", "s3", "memory":
	default:
		return fmt.Errorf("invalid storage.backend %q: must be \"filesystem\", \"s3\", or \"memory\"", c.Storage.Backend)
	}

	if c.Storage.Backend == "filesystem" && c.Storage.Path == "" {
		return fmt.Errorf("storage.path is required for filesystem backend")
	}

	if c.Storage.Backend == "s3" && c.Storage.S3.Bucket == "" {
		return fmt.Errorf("storage.s3.bucket is required for s3 backend")
	}

	if c.Storage.Backend == "memory" && c.Storage.MaxMemoryBytes <= 0 {
		return fmt.Errorf("storage.max_memory_bytes must be positive for memory backend")
	}

	return nil
}
