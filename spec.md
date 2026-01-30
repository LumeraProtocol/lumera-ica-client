# Actions over IBC via ICS-27

> The best approach it to use ICS-27 Interchain Accounts as the sole transport for executing MsgRequestAction on Lumera. The “creator” that pays the escrowed registration fee is the ICA address on Lumera, so all existing escrow logic in the Action module continues to work unchanged (escrow until finalize/expire, then distribute/refund).

---

## Part 1: Core Framework

### 1.1 Scope & Goals

1. The **core framework** for executing Lumera Actions (Cascade, Sense, Inference) from other Cosmos chains using **ICS-27 Interchain Accounts (ICA)**.
2. The **signature and account model** for `MsgRequestAction` / `MsgApproveAction` under ICA.
3. The **minimal controller-side “Lumera Client”** role needed for any integration.
4. The **required changes** in:
    - Lumera chain (Action module + ICA Host wiring), and
    - `sdk-go` client library.
5. A set of **Phase-2 / optional extensions** (ICQ proofs, read-only IBC app, fee abstraction, Archival/Orchestrator service, tooling) that build on top of the core pattern but are not required for the first version.

---

### 1.2 Core High-Level Summary

We implement cross-chain Lumera Actions via **ICA**:

- **Host chain**: Lumera runs ICA Host; it executes `MsgRequestAction` / `MsgApproveAction` where `creator` is the ICA address.
- **Controller chain**: A partner chain (e.g., Injective) runs ICA Controller and signs `MsgSendTx` with its *local* user/contract keys.
- **Off-chain “Lumera Client”**: A controller-side CLI/service that:
    1. Builds the Action metadata (Cascade/Sense/Inference) using `sdk-go` (or language equivalent).
    2. Wraps `MsgRequestAction` into a controller-chain `MsgSendTx`, signed on the controller chain.
    3. Waits for the IBC **ack** returning `action_id`.
    4. Uploads file bytes to the **SuperNode network** keyed by that `action_id`.
    Reads/proofs come in **Phase-2** via ICQ or a minimal read-only IBC app.

---

### 1.3 Signatures & Accounts (Core)

### 1.3.1 Existing `MsgRequestAction` semantics

Today, `MsgRequestAction` uses `creator` as both:

- The Cosmos signer enforced by ante, and
- The logical “requester” whose key signs/attests the metadata (e.g., Cascade layout signatures).

```protobuf
message MsgRequestAction {
  option (cosmos.msg.v1.signer) = "creator";
  string creator        = 1;  // MUST equal the host-side tx signer
  string actionType     = 2;
  string metadata       = 3;  // app-defined bytes/JSON/CBOR/etc.
  string price          = 4;
  string expirationTime = 5;
}

```

- Cosmos ante enforces: `GetSigners(msg)` → `{creator}` must match the tx signer.
- On a normal (non-ICA) account:
    - The account has a **user-held private key** and a **stored public key** in `x/auth`.
    - The module can verify metadata-level signatures using that public key.

### 1.3.2 Problem under ICS-27

Under **ICS-27**:

- The host-side signer is an **ICA account**.
- `creator` **must be** the ICA address on Lumera.
- ICA accounts:
    - Have **no user-held private key**.
    - Typically have **no pubkey stored in `x/auth`** in the normal way.
    - Are controlled by the ICA Host module, which executes msgs on behalf of the controller chain.
    Result: you **cannot** interpret `creator` as “the key that signed metadata”, because that private key doesn’t exist off-chain and there’s no regular pubkey to verify against.

### 1.3.3 Solution: application-level key for ICA

For ICA, we introduce an **application-level keypair**:

- The controller-side Lumera Client holds `app_privkey` / `app_pubkey`.
- For ICA:
    - `creator` remains the **ICA account address** (for escrow, fees, and auth).
    - The Action metadata includes a **cryptographic signature** produced by `app_privkey` over a canonical representation of the metadata.
    - The Action message includes `app_pubkey` at top-level so the Lumera keeper can verify that signature even though `creator` has no stored pubkey.
    We keep the **semantics for non-ICA** unchanged:
- If `creator` is a regular account:
    - We still rely on the existing account pubkey.
    - Application-level signatures are optional (can be added later for parity).

### 1.3.4 Top-level app key fields (core shape)

At minimum, we add a top-level `app_pubkey` used when `creator` is an ICA account:

```protobuf
message MsgRequestAction {
  option (cosmos.msg.v1.signer) = "creator";

  string creator        = 1;
  string actionType     = 2;
  string metadata       = 3;
  string price          = 4;
  string expirationTime = 5;

  // Only required/used when creator is an ICA account.
  bytes  app_pubkey     = 6;
}

```

The metadata itself contains an app-level signatures (e.g., inside the Cascade payload). The keeper uses `app_pubkey` to verify that signatures whenever `creator` is an ICA account.

---

## Part 2: Core Controller-Side Client (“Lumera Client”)

### 2.1. Role

The **Lumera Client** is the reference implementation library/CLI for coordinating cross-chain Actions — the **generic controller-side client** that knows how to:

- Talk to the **controller chain** (build and sign `MsgSendTx`).
- Talk to **Lumera** (decode acks, inspect actions).
- Talk to the **SuperNode mesh** (upload bytes based on `action_id`).
**Core Features:**
- ICA management (creation, funding)
- Message construction with app signatures
- IBC packet handling and ack parsing
- SuperNode upload coordination
- Retry logic and error handling
**Beyond the Reference Implementation:**
The CLI serves as the reference implementation, demonstrating the framework's capabilities. However, developers can use the same underlying framework to build:
- **Interactive tools** (CLIs, dashboards)

- **Automated services** (workers, daemons)
- **User-facing applications** (web, mobile)
- **Protocol integrations** (smart contracts, other chains)
- **Enterprise solutions** (APIs, microservices)
The framework abstracts the complexity of cross-chain Actions, allowing developers to focus on their specific application logic rather than IBC mechanics.

---

### 2.2 Requirements

- **File access (for Cascade / any storage action)**:
    - Local FS, S3/GCS, or pipeline outputs.
- **Controller-chain keyring**:
    - Private key to sign `MsgRegisterInterchainAccount` (one-time) and `MsgSendTx`.
    - Application private key (`app_privkey`) to produce the **metadata-level signature** used when `creator` is an ICA.
    - These can be the same key or distinct keys.

---

### 2.3. Minimal ICA flow (example: controller = Injective)

1. Ensure ICA exists and record the ICA address on Lumera.
2. Prefund ICA on Lumera (ICS-20 `ulume`; or accepted `ibc/INJ` if enabled).
3. Build `MsgRequestAction` with `sdk-go`:
    - `creator = <ICA address on Lumera>`
    - `metadata` contains Action payload signed by `app_privkey`
    - `app_pubkey` set when `creator` is ICA.
4. Wrap in controller-chain `MsgSendTx`, sign/broadcast on controller.
5. Wait for IBC ack; parse `action_id`.
6. Call `UploadToSupernode(action_id, file)` to send bytes to the SuperNode mesh.
7. (Optional) Send `MsgApproveAction` via ICA; parse ack.
Operational concerns stay purely **off-chain** in the Lumera Client:
- Retries and backoff.
- Idempotency.
- Metrics (send/ack/upload latency).

---

### 2.4. Architecture Summary (Core v1)

### 2.4.1 Control vs Data Plane

| Component | Decision / Role |
| --- | --- |
| Control-plane (actions) | **ICS-27**: controller → Lumera `MsgRequestAction` / `MsgApproveAction` |
| Data-plane (file bytes) | Off-chain via `UploadToSupernode(action_id, file)` using `sdk-go` |
| Who sends ICS-27? | Off-chain **Lumera Client**, not on-chain contracts |
| Automation cadence | Off-chain (inside Lumera Client) |
| Batching (core) | Multiple `MsgRequestAction` per `MsgSendTx` allowed |
| Fees / escrow | `creator = ICA` pays escrow as if local; **no on-chain economics change** |
| Partner integration | Run Lumera Client + relayer; maintain an ICA. No controller-chain code required for basic flows |

---

### 2.5. Core Lifecycle & Changes

### 2.5.1 High-level core sequence

```
Lumera Client (controller side)
  ├─ CreateRequestActionMessage(...)               // sdk-go: layout/metadata
  ├─ Add app_pubkey                                // for ICA
  ├─ Ensure ICA + fund ICA (ICS-20)                // one-time + top-ups
  ├─ Send MsgSendTx(Any{MsgRequestAction})         // controller SDK
  ├─ Wait for IBC ack → extract action_id
  └─ UploadToSupernode(action_id, file)            // sdk-go, off-chain
  ... (optional, after successful upload / policy checks)
     └─ Send MsgSendTx(Any{MsgApproveAction})
        ↳ Ack → confirm approval (action_id in MsgApproveActionResponse)

```

### 2.5.2 One-time required actions (ops / relayer)

- Create IBC client/connection/channel for ICA (Hermes or equivalent).
- Register/open ICA on Lumera; persist the ICA address in config.
- Fund the ICA address on Lumera (ICS-20 transfer).
- Run relayer continuously for controller ↔ Lumera packets/acks.
- (Optional) Allowlist accepted controller chain IDs; set per-channel rate limits.

### 2.5.3 Lumera chain changes (core)

1. **ICA Host stack**
    - Ensure Router-V2/ICS-4 stack is wired with ICA Host and (optionally) IBC callbacks middleware.
    - Confirm `MsgSendTx` → `MsgRequestAction` / `MsgApproveAction` path is supported.
2. **Action responses**
    - `MsgRequestActionResponse` includes `action_id` in `TxMsgData`.
    - `MsgApproveActionResponse` includes `action_id` in `TxMsgData`.
3. **Signature verification branch (ICA vs non-ICA)**
    - For non-ICA `creator`: keep existing behavior (account pubkey).
    - For ICA `creator`: use `app_pubkey` + metadata signature (see keeper note in Appendix).
4. **Events**
    - On request/finalize/approve emit structured events:
        - `action_id`
        - `creator` (ICA address)
        - escrow denom/amount
        - payload sizes.
5. **Keeper / params**
    - Escrow logic unchanged: ICA account behaves like any other payer.
    - Document min price and fee parameters for external users.
6. **Allowlist / safety**
    - Optionally restrict ICA Host to only accept:
        - `MsgRequestAction`
        - `MsgApproveAction` (if in use)
    - Add configuration for:
        - gas cap per ICA tx
        - per-channel throughput limits.

### 2.5.4 SDK-Go changes (core)

```go
type UploadOptions struct {
    Public           bool
    FileName         string
    ICACreatorAddress string // NEW: override creator with ICA when set
}

// Build niched messages, compute layouts, sign content.
func CreateRequestActionMessage(ctx, file, opts...) (*action.MsgRequestAction, []byte, error)
func CreateApproveActionMessage(ctx, actionID, opts...) (*action.MsgApproveAction, error)

// Local broadcast path (unchanged)
func SendRequestActionMessage(ctx, msg *action.MsgRequestAction) (actionID, txHash string, err error)
func SendApproveActionMessage(ctx, msg *action.MsgApproveAction) (txHash string, err error)

// Data plane (unchanged)
func UploadToSupernode(ctx, actionID, file string, opts...) (taskID string, err error)

// ICA helpers
func PackRequestForICA(msg *action.MsgRequestAction) ([]byte, error)
func PackApproveForICA(msg *action.MsgApproveAction) ([]byte, error)
func SendRequestActionViaICA(ctx, ctrlCfg, payload any) (actionID string, err error)

```

Rules in `sdk-go`:

- If `ICACreatorAddress` is not empty:
    - Use it in `msg.creator`.
    - Attach `app_pubkey` and embed app-level signature in metadata.
- Otherwise:
    - Use the local account as `creator`, relying on standard account pubkey.

---

## Part 3: Specialized Services and Tools

### 3.1 Archival Service (extensions)

### 3.1.1 Overview

A specialized autonomous service that monitors blockchain events and triggers automatic Cascade storage.
***Archival Service*** is long-running backend (of single or multiple instances) that:

- Maintains a Lumera Client internally.
- Watches controller-chain or external event streams.
- Applies policies:
    - When to register new Actions.
    - How to batch them.
    - How to parallelize uploads.

### 3.1.2 **Features**

- Event-based triggers (trades, mints, governance votes)
- Periodic archival (every N blocks)
- Contract state change detection
- Configurable archival policies

### 3.1.3 **Architecture:**

- Runs as independent service (not just a client)
- Maintains its own ICA and fee management
- Can serve multiple chains simultaneously
- Provides archival-as-a-service to other protocols

### 3.1.4 **Possible future improvements:**

1. **Batch `MsgSendTx`**
    - Pack 5–100+ `MsgRequestAction`s per ICS-27 transaction.
    - Requires:
        - Configurable batch limits.
        - Parallel `UploadToSupernode` workers.
    - Helps:
        - “Helix-style” high-frequency data ingestion.
2. **Streaming Uploads**
    - Pipeline mode to:
        - Split large files.
        - Stream chunks to multiple SNs in parallel.
    - Reduces end-to-end upload latency for large assets.
    All of this remains **off-chain** and does not change the on-chain core design.

### 3.1.5 Advanced Partner Tooling & Sandbox (Phase 2)

Additional tooling on top of Archival Service:

- **GUI dashboard** for partners:
    - ICA balances.
    - Action send/ack status.
    - Uploaded proof lists.
    - Pending actions and latency metrics.
- **Testing sandbox**:
    - Localnet: Lumera + sample controller chain + Hermes + orchestrator.
    - CI-ready Docker images and scripts.

---

### 3.2 Enterprise Gateway Service (Phase 2)

REST API wrapper for organizations not running blockchain infrastructure.
**Features:**

- HTTP endpoints for action submission
- Managed ICA pool for high throughput
- Fee abstraction and billing integration
- SLA guarantees and monitoring

---

### 3.3 Multi-language SDKs & Tooling (Phase 2)

To support different ecosystems:

- **Python SDK** for data-science / ML workflows.
- **JS/TS SDK** for dApps and frontends.
- **Rust SDK** for validator / infra integrations.
- **Helm charts** for deploying the Archival/Orchestrator in Kubernetes.

---

## Part 4: Advanced Features (Phase 2)

The following are **extensions** that build on the core ICS-27 pattern. They are **not required** for the initial integration but recommended long-term.

### 4.1 Read/Verification Layer

### Option A: ICQ (Interchain Queries)

- Query action state and proofs via standardized ICQ
- Add an ICQ profile to query `Action{action_id}` and, optionally, proof bundles.
- No Lumera code changes required:
    - Just protobuf schemas, query examples, and ICQ config.
- Suitable for periodic verification needs

### **Option B: Custom Read-Only IBC App**

- Introduce a **read-only Cascade IBC port** (`portID = "cascade"`) with no write permissions.
- Packet types:
    - `VerifyRequest{ action_id, fields[] }` → `VerifyAck{ value, proof, height }`
    - `ReceiptRequest{ action_id }` → `ReceiptAck{ receipt, height }`
- Lumera:
    - Small IBC module implementing `OnRecvPacket`.
    - Queries the keeper, builds ICS-23 proofs, returns structured acks.
- Consumer/controller chains:
    - Decode acks and verify using their Lumera light client state.
- Targets:
    - Wallets, DeFi, indexers, storage verifiers that need on-chain proofs.

---

### 4.2 Fee Abstraction

### Option A: Feegrant (core V2-lite)

- Use `feegrant` to grant ICA addresses a limited fee budget.
- Requires **no protocol changes**; simply operational config.
- Good for partners that don’t want to manage `ulume` actively.

### Option B: Payment Vouchers (core V2-major)

- Add a micro-app or middleware that processes packets containing:
    - A **payment voucher** signed by a sponsor, and
    - An inner `MsgRequestAction` or `MsgApproveAction`.
- Flow:
    1. Controller sends voucher + inner msg in one ICS-27 payload.
    2. Lumera verifies voucher signature, redeems it to `ulume` (mint/escrow) and then executes the inner msg.
- Enables:
    - “Sponsor pays fees” model.
    - SaaS-style integrations where users never touch `ulume`.

> This is where the enterprise UX lives, but it is explicitly Phase-2.
> 

---

### 4.3 Automation Framework

We distinguish two levels: off-chain and on-chain.
**Off-chain Automation (recommended baseline)**

- Block watchers and event monitors
    - Trigger uploads every X blocks.
    - Trigger on specific events (trades, mints, governance votes).
- State watchers
    - Trigger when certain contract/storage conditions are met.
- Scheduled triggers
- Policy-based execution
**On-chain instructions (optional advanced)**
- A CosmWasm contract on the controller chain emits an event:
    - “Upload file F for action type T”.
    **Native module integrations (future)**
- Controller chains that want deep integration can:
    - Implement a native module that periodically instructs the Archival Service to perform Cascade uploads.
- Still offloads file bytes to the SuperNode mesh.
Again: these are **architectural patterns** on top of the core, not required for the base ICS-27 flow.

---

## Part 5: Deliverables & Milestone Checklists

### 5.1 Lumera (core)

- [x]  ICA Host stack wired and tested end-to-end.
- [x]  `MsgRequestActionResponse` returns `action_id`.
- [x]  `MsgApproveActionResponse` returns `action_id`.
- [x]  Signature verification branch (ICA vs non-ICA)
- [ ]  Enriched events and docs for price/fee params.
- [ ]  ICA allowlist + rate limiting configuration.

---

### 5.2 `sdk-go` (core)

- [x]  Support for `ICACreatorAddress` and `app_pubkey`.
- [x]  Helpers to build ICA-ready messages.
- [x]  Ack parser for `action_id`.
- [ ]  Data-plane uploader exposed as CLI/API.
- [ ]  Basic documentation

---

### 5.3 Lumera Client (core)

- [ ]  CLI + YAML config.
- [ ]  Hermes scripts (clients/connection/channel).
- [ ]  Create / sign / broadcast `MsgSendTx`.
- [ ]  Wait / parse ack → `action_id`.
- [ ]  Upload file to SuperNodes using `action_id`.
- [ ]  ICS-20 funder / top-up routines.
- [ ]  E2E dockerized localnet example.

---

### 5.4 Archival Service Prototype (extensions / Phase 2)

**Goal:** Demonstrate a policy-based, event-driven Archival Service built on top of the Lumera Client.

- [ ]  Design & skeleton (stack, config model, modules).
- [ ]  Event sources & triggers (“every N blocks”, “on event type X”).
- [ ]  Integration with Lumera Client (build/send/ack/upload).
- [ ]  Batching & throughput (multi-actions per tx, parallel uploads).
- [ ]  Localnet demo (Lumera + controller + Hermes + service).

---

### 5.5 Production Deployment (core + services)

**Goal:** Make Lumera Client and Archival Service deployable in production.

### 5.5.1 Core Lumera Client Deployment (required)

- [ ]  Docker image + config (env / YAML).
- [ ]  ICA lifecycle scripts (create, fund, open ICS-27 channel).
- [ ]  Example flows (single action, batch, ack → `action_id` → upload).
- [ ]  Ops docs (key rotation, monitoring, gas/fee tuning).

### 5.5.2 Archival Service Deployment

- [ ]  Docker / Helm / docker-compose packaging.
- [ ]  Multi-chain support (per-chain ICAs, rate limits, policies).
- [ ]  Runbook (add/remove chains, pause/resume, outage handling).
- [ ]  Security guidelines (key/secrets storage, API/dashboard access).

### 5.5.3 Partner-Facing Documentation

- [ ]  Core integration guide (self-hosted Lumera Client + relayer).
- [ ]  Managed Archival / Enterprise options (if offered).
- [ ]  Architecture diagrams (core vs extended with services).

---

### 5.6 Enhanced Capabilities (Phase 2 / Q2 2025)

**Goal:** Track delivery of Phase-2 features from Parts 3–4.

### 5.6.1 Read / Verification Layer

- [ ]  ICQ option: profiles/configs, proto docs, sample verifier.
- [ ]  Read-only IBC app option: `cascade` port, Verify/Receipt packets, consumer integration.

### 5.6.2 Fee Abstraction

- [ ]  Feegrant patterns and examples for ICAs.
- [ ]  Voucher-based fee proxy (format, redeem flow, “sponsor pays fees” example).

### 5.6.3 Automation Framework

- [ ]  Off-chain triggers in Archival Service (block/event/state/scheduled, policies).
- [ ]  On-chain intent patterns (CosmWasm example emitting upload intents).
- [ ]  Native module integration sketch (optional periodic archival).

### 5.6.4 Developer Tools & Dashboards

- [ ]  Multi-language SDKs (Python, JS/TS, Rust).
- [ ]  Monitoring dashboard (ICA balances, send/ack, errors, upload latency).
- [ ]  End-to-end examples & CI-ready localnet/test harnesses.

---

## Part 6: Partner Integration Guide

### 6.1 Minimal Integration Requirements

1. **Run or access a relayer** (e.g., Hermes)
2. **Maintain an ICA** on Lumera
3. **Use Lumera Client SDK** or implement the protocol

### 6.2 Integration Options

**Option A: SDK Integration**

- Use official Lumera Client SDK
- Minimal code required
- Best for most use cases
**Option B: Direct Protocol Implementation**
- Implement ICS-27 messaging directly
- Full control over the flow
- Best for deep integrations
**Option C: Service Integration**
- Use Archival Service or Enterprise Gateway
- No blockchain infrastructure needed
- Best for web2 organizations

---

## Appendix

### Risks & Mitigations (Core + Extensions)

- **Relayer stalls**
    - Mitigation: Idempotent retries; treat upload as pending until `action_id` is confirmed in an ack.
- **Underfunded ICA**
    - Mitigation: Preflight balance checks; optional auto ICS-20 top-ups; future feegrant/voucher mechanisms.
- **Spam/DoS**
    - Mitigation:
        - Minimum price enforcement.
        - Gas caps per ICA tx.
        - Per-channel throughput limits.
        - Allowlists for trusted controller chains.

---

### Observability

- Log fields:
    - Controller tx hash.
    - Lumera ack height.
    - `action_id`.
    - SuperNode upload quorum / SN list.
- Metrics:
    - ICA send → ack latency.
    - Upload throughput and duration.
    - Retry counts.
    - Time-to-DONE / APPROVED.

---

### Keeper Note: ICA Detection Branch (Sketch)

This sketch shows the keeper logic for distinguishing ICA vs non-ICA creators and using `app_pubkey` appropriately. Exact field names/placement for app-level signatures can be adjusted during implementation.

```go
acct := k.accountKeeper.GetAccount(ctx, creatorAddr)

if isICAAccount(acct) {
    // Canonical bytes for metadata + relevant fields
    rq := CanonicalizeForSignature(msg)

    // Verify application-level signature using app_pubkey
    if !VerifyAppSig(msg.AppSigAlg, msg.AppPubkey, rq, msg.AppSignature) {
        return "", ErrInvalidSignature
    }

    // Optionally bind app_pubkey to ICA once and enforce consistency
    if bound, ok := k.GetBoundAppKey(ctx, creatorAddr); !ok {
        k.BindAppKey(ctx, creatorAddr, msg.AppPubkey)
    } else if !bytes.Equal(bound, msg.AppPubkey) {
        return "", ErrAppKeyMismatch
    }
} else {
    // Non-ICA:
    // - Ante ensures tx signer == creator
    // - Existing account pubkey semantics are used
    // Optionally still verify app_* for additional guarantees.
}

```

This keeps:

- **Economic semantics** tied to `creator` (ICA).
- **Cryptographic provenance** of metadata tied to `app_pubkey` when needed.
- Backwards compatibility for non-ICA accounts.

---