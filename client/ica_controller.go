package client

import (
	"context"
	"fmt"
	"strings"

	sdkmath "cosmossdk.io/math"
	actiontypes "github.com/LumeraProtocol/lumera/x/action/v1/types"
	"github.com/LumeraProtocol/sdk-go/blockchain"
	"github.com/LumeraProtocol/sdk-go/ica"
	sdktypes "github.com/LumeraProtocol/sdk-go/types"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const defaultMaxGRPCMsgSize = 10 * 1024 * 1024

// Controller wraps the sdk-go ICA controller for CLI workflows.
type Controller struct {
	inner *ica.Controller
}

// NewICAController builds a gRPC-backed ICA controller using the provided keyring.
func NewICAController(ctx context.Context, cfg *Config, kr keyring.Keyring) (*Controller, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is nil")
	}
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if kr == nil {
		return nil, fmt.Errorf("keyring is nil")
	}

	gasPrice, feeDenom, err := parseGasPrices(cfg.Controller.GasPrices)
	if err != nil {
		return nil, fmt.Errorf("parse controller.gas_prices: %w", err)
	}

	controllerCfg := blockchain.Config{
		ChainID:        cfg.Controller.ChainID,
		GRPCAddr:       cfg.Controller.GRPCEndpoint,
		RPCEndpoint:    cfg.Controller.RPCEndpoint,
		AccountHRP:     cfg.Controller.AccountHRP,
		FeeDenom:       feeDenom,
		GasPrice:       gasPrice,
		Timeout:        defaultGRPCTimeout,
		MaxRecvMsgSize: defaultMaxGRPCMsgSize,
		MaxSendMsgSize: defaultMaxGRPCMsgSize,
	}
	hostCfg := blockchain.Config{
		ChainID:        cfg.Lumera.ChainID,
		GRPCAddr:       cfg.Lumera.GRPCEndpoint,
		RPCEndpoint:    cfg.Lumera.RPCEndpoint,
		Timeout:        defaultGRPCTimeout,
		MaxRecvMsgSize: defaultMaxGRPCMsgSize,
		MaxSendMsgSize: defaultMaxGRPCMsgSize,
	}

	inner, err := ica.NewController(ctx, ica.Config{
		Controller:               controllerCfg,
		Host:                     hostCfg,
		Keyring:                  kr,
		KeyName:                  cfg.Controller.KeyName,
		HostKeyName:              cfg.Lumera.KeyName,
		ConnectionID:             cfg.Controller.ConnectionID,
		CounterpartyConnectionID: cfg.Controller.CounterpartyConnectionID,
	})
	if err != nil {
		return nil, err
	}

	return &Controller{inner: inner}, nil
}

// Close releases gRPC connections held by the controller.
func (c *Controller) Close() error {
	if c == nil || c.inner == nil {
		return nil
	}
	return c.inner.Close()
}

// OwnerAddress returns the controller-chain owner address.
func (c *Controller) OwnerAddress() string {
	if c == nil || c.inner == nil {
		return ""
	}
	return c.inner.OwnerAddress()
}

// AppPubkey returns the controller key public key bytes.
func (c *Controller) AppPubkey() []byte {
	if c == nil || c.inner == nil {
		return nil
	}
	return c.inner.AppPubkey()
}

// EnsureICAAddress resolves or registers an interchain account address.
func (c *Controller) EnsureICAAddress(ctx context.Context) (string, error) {
	if c == nil || c.inner == nil {
		return "", fmt.Errorf("ica controller is not initialized")
	}
	return c.inner.EnsureICAAddress(ctx)
}

// ICAAddress returns the ICA address if already registered.
func (c *Controller) ICAAddress(ctx context.Context) (string, error) {
	if c == nil || c.inner == nil {
		return "", fmt.Errorf("ica controller is not initialized")
	}
	return c.inner.ICAAddress(ctx)
}

// SendRequestAction sends a request action over ICA and returns the action result.
func (c *Controller) SendRequestAction(ctx context.Context, msg *actiontypes.MsgRequestAction) (*sdktypes.ActionResult, error) {
	if c == nil || c.inner == nil {
		return nil, fmt.Errorf("ica controller is not initialized")
	}
	return c.inner.SendRequestAction(ctx, msg)
}

// SendApproveAction sends approve messages over ICA and returns the controller tx hash.
func (c *Controller) SendApproveAction(ctx context.Context, msg *actiontypes.MsgApproveAction) (string, error) {
	if c == nil || c.inner == nil {
		return "", fmt.Errorf("ica controller is not initialized")
	}
	return c.inner.SendApproveAction(ctx, msg)
}

func parseGasPrices(value string) (sdkmath.LegacyDec, string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return sdkmath.LegacyZeroDec(), "", nil
	}
	coins, err := sdk.ParseDecCoins(trimmed)
	if err != nil {
		return sdkmath.LegacyZeroDec(), "", err
	}
	if len(coins) == 0 {
		return sdkmath.LegacyZeroDec(), "", nil
	}
	if len(coins) > 1 {
		return sdkmath.LegacyZeroDec(), "", fmt.Errorf("expected a single gas price, got %d entries", len(coins))
	}
	coin := coins[0]
	return coin.Amount, coin.Denom, nil
}
