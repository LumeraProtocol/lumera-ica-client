package client

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/LumeraProtocol/sdk-go/cascade"
	sdkcrypto "github.com/LumeraProtocol/sdk-go/pkg/crypto"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
)

const defaultCascadeTimeout = 30 * time.Second

// Client bundles the cascade client with its backing keyring and owner address.
// The keyring is the controller chain keyring; the Lumera address is derived from it.
type Client struct {
	Cascade      *cascade.Client
	Keyring      keyring.Keyring
	OwnerAddress string
}

// NewCascadeClient initializes the SDK cascade client using controller keyring settings.
// It derives a Lumera bech32 address from the same key name for action registration.
func NewCascadeClient(ctx context.Context, cfg *Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	// Create the controller keyring (used for ICA signing and metadata signing).
	controllerKR, err := newControllerKeyring(cfg.Controller)
	if err != nil {
		return nil, err
	}
	// Validate that keys in the keyring match the configured key types.
	if err := validateKeyType(controllerKR, cfg.Controller.KeyName, cfg.Controller.KeyType); err != nil {
		return nil, fmt.Errorf("controller key type: %w", err)
	}
	if err := validateKeyType(controllerKR, cfg.Lumera.KeyName, cfg.Lumera.KeyType); err != nil {
		return nil, fmt.Errorf("lumera key type: %w", err)
	}
	// Resolve controller owner address using the configured controller account HRP.
	ownerAddr, err := sdkcrypto.AddressFromKey(controllerKR, cfg.Controller.KeyName, cfg.Controller.AccountHRP)
	if err != nil {
		return nil, fmt.Errorf("derive controller address: %w", err)
	}
	// Resolve Lumera address with the Lumera HRP for on-chain action registration.
	lumeraAddr, err := sdkcrypto.AddressFromKey(controllerKR, cfg.Lumera.KeyName, "lumera")
	if err != nil {
		return nil, fmt.Errorf("derive lumera address: %w", err)
	}
	// Initialize cascade SDK client with Lumera connection settings and log level.
	casc, err := cascade.New(ctx, cascade.Config{
		ChainID:         cfg.Lumera.ChainID,
		GRPCAddr:        cfg.Lumera.GRPCEndpoint,
		Address:         lumeraAddr,
		KeyName:         cfg.Lumera.KeyName,
		ICAOwnerKeyName: cfg.Controller.KeyName,
		ICAOwnerHRP:     cfg.Controller.AccountHRP,
		Timeout:         defaultCascadeTimeout,
		LogLevel:        cfg.Lumera.LogLevel,
	}, controllerKR)
	if err != nil {
		return nil, err
	}
	return &Client{Cascade: casc, Keyring: controllerKR, OwnerAddress: ownerAddr}, nil
}

// validateKeyType checks that a key in the keyring uses the algorithm matching
// the configured key_type. This catches misconfigurations early — e.g. when a
// config says key_type = "evm" but the keyring holds a cosmos secp256k1 key.
func validateKeyType(kr keyring.Keyring, keyName, configuredType string) error {
	kt, err := ParseKeyType(configuredType)
	if err != nil {
		return err
	}
	rec, err := kr.Key(keyName)
	if err != nil {
		return fmt.Errorf("key %q not found in keyring: %w", keyName, err)
	}
	pub, err := rec.GetPubKey()
	if err != nil {
		return fmt.Errorf("get pubkey for %q: %w", keyName, err)
	}
	expected := string(kt.SigningAlgo().Name())
	if actual := pub.Type(); actual != expected {
		return fmt.Errorf("key %q type mismatch: config expects %s but keyring has %s", keyName, expected, actual)
	}
	return nil
}

// newControllerKeyring constructs the Cosmos keyring for the controller chain.
func newControllerKeyring(cfg ControllerConfig) (keyring.Keyring, error) {
	passphrase, err := resolvePassphrase(cfg.KeyringPassphrasePlain, cfg.KeyringPassphraseFile)
	if err != nil {
		return nil, err
	}
	// For test backend, fall back to controller.home when keyring_dir is unset.
	dir := strings.TrimSpace(cfg.KeyringDir)
	if dir == "" && strings.EqualFold(cfg.KeyringBackend, "test") {
		dir = strings.TrimSpace(cfg.Home)
	}
	params := sdkcrypto.KeyringParams{
		AppName: keyringAppName(cfg),
		Backend: cfg.KeyringBackend,
		Dir:     dir,
		Input:   passphraseReader(passphrase),
	}
	kr, err := sdkcrypto.NewKeyring(params)
	if err != nil {
		return nil, fmt.Errorf("init keyring: %w", err)
	}
	return kr, nil
}

// resolvePassphrase selects a single passphrase source or returns empty for interactive prompts.
func resolvePassphrase(plain, filePath string) (string, error) {
	plain = strings.TrimSpace(plain)
	filePath = strings.TrimSpace(filePath)
	if plain != "" && filePath != "" {
		return "", fmt.Errorf("only one of keyring passphrase plain/file may be set")
	}
	if plain != "" {
		return plain, nil
	}
	if filePath == "" {
		return "", nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read keyring passphrase file: %w", err)
	}
	pass := strings.TrimSpace(string(data))
	if pass == "" {
		return "", fmt.Errorf("keyring passphrase file is empty")
	}
	return pass, nil
}

func passphraseReader(passphrase string) io.Reader {
	if passphrase == "" {
		return nil
	}
	// Repeat the passphrase to satisfy multiple keyring prompts.
	return &repeatReader{data: []byte(passphrase + "\n")}
}

// keyringAppName selects a stable keyring application name for the controller chain.
func keyringAppName(cfg ControllerConfig) string {
	if cfg.Binary != "" {
		return filepath.Base(cfg.Binary)
	}
	if cfg.ChainID != "" {
		return cfg.ChainID
	}
	return "lumera"
}

type repeatReader struct {
	data []byte
	pos  int
}

// Read loops the underlying data to satisfy repeated keyring reads.
func (r *repeatReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := 0
	for n < len(p) {
		remaining := len(r.data) - r.pos
		if remaining == 0 {
			r.pos = 0
			remaining = len(r.data)
		}
		chunk := remaining
		if chunk > len(p)-n {
			chunk = len(p) - n
		}
		copy(p[n:n+chunk], r.data[r.pos:r.pos+chunk])
		n += chunk
		r.pos += chunk
	}
	return n, nil
}
