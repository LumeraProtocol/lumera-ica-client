// Package client contains the ICA reference client implementation and helpers.
package client

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the root configuration for the ICA reference client.
// It separates Lumera chain settings from the controller chain/keyring settings.
type Config struct {
	Lumera     LumeraConfig     `toml:"lumera"`
	Controller ControllerConfig `toml:"controller"`
}

// LumeraConfig stores the host chain connection settings.
// LogLevel is forwarded into sdk-go to control cascade logging.
type LumeraConfig struct {
	ChainID      string `toml:"chain_id"`
	GRPCEndpoint string `toml:"grpc_endpoint"`
	RPCEndpoint  string `toml:"rpc_endpoint"`
	LogLevel     string `toml:"log_level"`
	KeyName      string `toml:"key_name"`
}

// ControllerConfig stores controller chain and keyring settings.
// The keyring is used for ICA signing and cascade metadata signatures.
type ControllerConfig struct {
	ChainID                  string `toml:"chain_id"`
	GRPCEndpoint             string `toml:"grpc_endpoint"`
	RPCEndpoint              string `toml:"rpc_endpoint"`
	Binary                   string `toml:"binary"`
	Home                     string `toml:"home"`
	KeyName                  string `toml:"key_name"`
	KeyringBackend           string `toml:"keyring_backend"`
	KeyringDir               string `toml:"keyring_dir"`
	KeyringPassphrasePlain   string `toml:"keyring_passphrase_plain"`
	KeyringPassphraseFile    string `toml:"keyring_passphrase_file"`
	GasPrices                string `toml:"gas_prices"`
	AccountHRP               string `toml:"account_hrp"`
	ConnectionID             string `toml:"connection_id"`
	CounterpartyConnectionID string `toml:"counterparty_connection_id"`
}

// LoadConfig reads a TOML config file, expands paths, and validates the result.
func LoadConfig(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if err := cfg.ExpandPaths(); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ExpandPaths expands tilde-prefixed values in place.
// This allows configs to use "~" for home directories.
func (c *Config) ExpandPaths() error {
	var err error
	c.Controller.Home, err = expandHome(c.Controller.Home)
	if err != nil {
		return fmt.Errorf("expand controller.home: %w", err)
	}
	c.Controller.KeyringDir, err = expandHome(c.Controller.KeyringDir)
	if err != nil {
		return fmt.Errorf("expand controller.keyring_dir: %w", err)
	}
	c.Controller.KeyringPassphraseFile, err = expandHome(c.Controller.KeyringPassphraseFile)
	if err != nil {
		return fmt.Errorf("expand controller.keyring_passphrase_file: %w", err)
	}
	return nil
}

// Validate checks the config for required fields and consistency.
// It also normalizes fields like log levels and keyring backend names.
func (c *Config) Validate() error {
	logLevel, err := normalizeLogLevel(c.Lumera.LogLevel)
	if err != nil {
		return err
	}
	c.Lumera.LogLevel = logLevel
	if strings.TrimSpace(c.Lumera.ChainID) == "" {
		return fmt.Errorf("lumera.chain_id is required")
	}
	if strings.TrimSpace(c.Lumera.GRPCEndpoint) == "" {
		return fmt.Errorf("lumera.grpc_endpoint is required")
	}
	if strings.TrimSpace(c.Lumera.RPCEndpoint) == "" {
		return fmt.Errorf("lumera.rpc_endpoint is required")
	}
	if strings.TrimSpace(c.Lumera.KeyName) == "" {
		return fmt.Errorf("lumera.key_name is required")
	}
	if strings.TrimSpace(c.Controller.ChainID) == "" {
		return fmt.Errorf("controller.chain_id is required")
	}
	if strings.TrimSpace(c.Controller.GRPCEndpoint) == "" {
		return fmt.Errorf("controller.grpc_endpoint is required")
	}
	if strings.TrimSpace(c.Controller.RPCEndpoint) == "" {
		return fmt.Errorf("controller.rpc_endpoint is required")
	}
	if strings.TrimSpace(c.Controller.KeyName) == "" {
		return fmt.Errorf("controller.key_name is required")
	}
	if strings.TrimSpace(c.Controller.KeyringBackend) == "" {
		return fmt.Errorf("controller.keyring_backend is required")
	}
	if strings.TrimSpace(c.Controller.AccountHRP) == "" {
		return fmt.Errorf("controller.account_hrp is required")
	}
	if strings.TrimSpace(c.Controller.ConnectionID) == "" {
		return fmt.Errorf("controller.connection_id is required")
	}
	if strings.TrimSpace(c.Controller.KeyringPassphrasePlain) != "" &&
		strings.TrimSpace(c.Controller.KeyringPassphraseFile) != "" {
		return fmt.Errorf("only one of controller.keyring_passphrase_plain or controller.keyring_passphrase_file may be set")
	}
	backend := strings.ToLower(strings.TrimSpace(c.Controller.KeyringBackend))
	switch backend {
	case "os", "file", "test":
		c.Controller.KeyringBackend = backend
	default:
		return fmt.Errorf("controller.keyring_backend must be one of: os, file, test")
	}
	if backend == "file" && strings.TrimSpace(c.Controller.KeyringDir) == "" {
		return fmt.Errorf("controller.keyring_dir is required for file backend")
	}
	if strings.TrimSpace(c.Controller.KeyringPassphraseFile) != "" {
		b, err := os.ReadFile(c.Controller.KeyringPassphraseFile)
		if err != nil {
			return fmt.Errorf("read controller.keyring_passphrase_file: %w", err)
		}
		if strings.TrimSpace(string(b)) == "" {
			return fmt.Errorf("controller.keyring_passphrase_file is empty")
		}
	}
	return nil
}

// normalizeLogLevel maps user input to supported log levels.
func normalizeLogLevel(value string) (string, error) {
	val := strings.ToLower(strings.TrimSpace(value))
	if val == "" {
		return "info", nil
	}
	switch val {
	case "debug", "info", "warn", "error":
		return val, nil
	case "warning":
		return "warn", nil
	default:
		return "", fmt.Errorf("lumera.log_level must be one of: debug, info, warn, error")
	}
}

// expandHome resolves "~" prefixes into the OS user home directory.
func expandHome(value string) (string, error) {
	if value == "" {
		return value, nil
	}
	if value == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}
	if strings.HasPrefix(value, "~/") || strings.HasPrefix(value, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		rest := value[2:]
		return filepath.Join(home, rest), nil
	}
	return value, nil
}
