package client

import (
	"context"
	"time"

	"github.com/LumeraProtocol/sdk-go/blockchain"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
)

const defaultGRPCTimeout = 30 * time.Second

// NewLumeraClient creates a Lumera blockchain client for gRPC queries.
// The keyring is only used for address derivation; no signing occurs here.
func NewLumeraClient(ctx context.Context, cfg *Config, kr keyring.Keyring, keyName string) (*blockchain.Client, error) {
	bcCfg := blockchain.Config{
		ChainID:        cfg.Lumera.ChainID,
		GRPCAddr:       cfg.Lumera.GRPCEndpoint,
		RPCEndpoint:    cfg.Lumera.RPCEndpoint,
		Timeout:        defaultGRPCTimeout,
		MaxRecvMsgSize: 10 * 1024 * 1024,
		MaxSendMsgSize: 10 * 1024 * 1024,
	}
	return blockchain.New(ctx, bcCfg, kr, keyName)
}
