# CLAUDE.md

## Project Overview

`lumera-ica-client` is a Go CLI reference client for executing Lumera Cascade actions (file storage) across Cosmos chains via ICS-27 Interchain Accounts (ICA). It bridges a controller chain (e.g., Osmosis) to the Lumera host chain, handling ICA registration, action submission, supernode file upload/download, and action approval.

## Architecture

- **Controller chain**: where the user key lives; signs `MsgSendTx` wrapping Lumera messages
- **Host chain (Lumera)**: executes `MsgRequestAction` / `MsgApproveAction` via ICA
- **Supernodes**: off-chain mesh for file byte upload/download keyed by `action_id`

Key dependency: `github.com/LumeraProtocol/sdk-go` provides ICA controller, cascade client, and crypto primitives.

## Project Structure

```
main.go                        # Entry point, sets bech32 prefixes
cmd/
  commands.go                  # Root cobra command, shared helpers
  upload.go                    # Upload command (ICA registration + supernode upload)
  download.go                  # Download command
  action.go                    # Action status/approve subcommands
client/
  config.go                    # TOML config loading and validation
  lumera_client.go             # Lumera SDK client wrapper
  cascade_client.go            # Cascade (supernode) client wrapper
  ica_controller.go            # ICA controller wrapper
config.toml                    # Example/default configuration
spec.md                        # Full ICA specification
docs/
  DEVELOPER_GUIDE.md           # Detailed developer documentation
  WORKFLOWS.md                 # Workflow documentation
```

## Build & Run

```bash
make build                     # Outputs binary to build/lumera-ica-client
```

No test suite exists yet. The project uses Go 1.25.5.

## Configuration

Config is TOML-based (`config.toml`). Two sections:
- `[lumera]` — host chain settings (chain_id, grpc/rpc endpoints, key_name, key_type)
- `[controller]` — controller chain settings (chain_id, endpoints, keyring config, connection_id)

Keyring backends: `os`, `file`, `test`. Key types: `cosmos` (secp256k1), `evm` (eth_secp256k1).

## CLI Commands

```bash
lumera-ica-client upload <file> [--public] [--action-id <id>]
lumera-ica-client download <action_id> --out <dir>
lumera-ica-client action status <action_id>
lumera-ica-client action approve <action_id> [--ica-address <addr>]
```

## Conventions

- CLI uses `cobra` with a shared `app` struct for config state
- JSON output to stdout via `writeJSON()`
- 10-minute default command timeout
- Config validation is strict — all required fields must be non-empty
- Bech32 prefix is hardcoded to `lumera` / `lumerapub` in main.go

## Local Development with sdk-go

The `go.mod` has a commented-out replace directive for local sdk-go development:
```
//github.com/LumeraProtocol/sdk-go => ../sdk-go
```
Uncomment to develop against a local clone of sdk-go.
