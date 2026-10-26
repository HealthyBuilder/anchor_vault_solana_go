# Anchor Vault + solana-go v2

An end-to-end demo: **Anchor program → IDL → Codama → Go client → on-chain interaction with [solana-go v2](https://github.com/solana-foundation/solana-go)**.

The program is a reference solution to the [Blueshift Anchor Vault challenge](https://learn.blueshift.gg/en/challenges/anchor-vault): every user gets a PDA-held SOL vault they can deposit into and withdraw everything from.

```
programs/…/lib.rs ──anchor build──▶ target/idl/blueshift_anchor_vault.json
                                          │
                       scripts/generate-go-client.mjs (Codama)
                                          ▼
                              clients/go/generated/*.go   ◀── generated, do not edit
                                          │
                              clients/go/cmd/demo/main.go ◀── calls it with solana-go v2
```

## Layout

| Path | Description |
| --- | --- |
| `programs/blueshift_anchor_vault/src/lib.rs` | The Anchor vault program (`deposit` / `withdraw`) |
| `scripts/generate-go-client.mjs` | Codama script: `rootNodeFromAnchor` reads the IDL, `@codama/renderers-go` renders Go code |
| `clients/go/generated/` | Generated Go package `blueshift_anchor_vault` |
| `clients/go/cmd/demo/main.go` | Demo: airdrop → deposit → second deposit (error) → withdraw |
| `tests/blueshift_anchor_vault.ts` | Anchor TypeScript tests |

## The program

| Concept | Where |
| --- | --- |
| PDA derivation: `seeds = [b"vault", signer]` + `bump` | `VaultAction.vault` |
| `SystemAccount`: an account owned by the System Program that only holds lamports | `VaultAction.vault` |
| Plain CPI: `CpiContext::new` (signed by the user) | `deposit` |
| PDA-signed CPI: `CpiContext::new_with_signer` + `ctx.bumps.vault` | `withdraw` |
| Rent-exempt minimum: `Rent::get()?.minimum_balance(0)` | `deposit` |
| Custom errors: `#[error_code]` + `require_*!` macros | `VaultError` |

## The generated Go client

| File | Contents |
| --- | --- |
| `instructions.go` | `ProgramID`, the `Instruction` wrapper (implements `solana.Instruction`), `DecodeInstruction` |
| `instruction_deposit.go` / `instruction_withdraw.go` | Instruction builders: `SetAmount` / `SetSignerAccount` / `SetVaultAccount` … `ValidateAndBuild()`, with the 8-byte Anchor discriminator |
| `pdas.go` | `FindVaultPDA(signer)`: the IDL's `seeds = ["vault", signer]` turned into a Go function |
| `errors.go` | `BlueshiftAnchorVaultError`: `VaultAlreadyExistsError = 6000`, `InvalidAmountError = 6001` |

### Targeting solana-go v2

`@codama/renderers-go` currently emits v1 imports (`github.com/gagliardetto/solana-go`).
v2 keeps the same API under the new module path `github.com/solana-foundation/solana-go/v2`,
so the last step of the generator script rewrites the imports to v2.
`github.com/gagliardetto/binary` is still used by v2 and is left unchanged.

## Running

Prerequisites: Rust, Solana CLI, Anchor 0.31, Node.js + Yarn, Go 1.25+.

```bash
# 1. Build the program: produces the .so and the IDL
anchor build

# 2. Generate the Go client
yarn install
yarn generate:go

# 3. Start a local validator with the .so loaded at 2222…
#    (this address has no private key, so `anchor deploy` cannot be used)
solana-test-validator --reset \
  --bpf-program 22222222222222222222222222222222222222222222 target/deploy/blueshift_anchor_vault.so

# 4. In another terminal, run the demo
cd clients/go
go run ./cmd/demo
```

Set `RPC_URL` / `WS_URL` to point the demo at another cluster.

Expected output:

```
User wallet:  EgRBEk3L…
Airdropped 2 SOL
Vault PDA:   J7iQGzHt… (bump 255)
  User balance:  2.000000 SOL
  Vault balance: 0.000000 SOL

[deposit] deposited 1 SOL, tx: 56Ric7a8…
  User balance:  0.999995 SOL
  Vault balance: 1.000000 SOL

[deposit again] rejected by the program: Vault already exists (code 6000)

[withdraw] withdrew all lamports, tx: 3j2PSspm…
  User balance:  1.999990 SOL
  Vault balance: 0.000000 SOL
```

To run the Anchor TypeScript tests instead (`Anchor.toml` loads the program at `2222…` through `[[test.genesis]]`):

```bash
anchor test --skip-build
```

## solana-go v2 APIs used in the demo

| Step | API |
| --- | --- |
| Connect | `rpc.New(rpc.LocalNet_RPC)`, `ws.Connect(ctx, rpc.LocalNet_WS)` |
| Wallet / airdrop | `solana.NewWallet()`, `client.RequestAirdrop` |
| Build a transaction | `client.GetLatestBlockhash` + `solana.NewTransaction(ixs, blockhash, solana.TransactionPayer(...))` |
| Sign | `tx.Sign(func(pubkey) *solana.PrivateKey { … })` |
| Send | `client.SendTransactionWithOpts` (returns a `*jsonrpc.RPCError` when preflight fails) |
| Confirm | `wsClient.SignatureSubscribe(sig, rpc.CommitmentConfirmed)` |
| Custom errors | Read `{"InstructionError":[0,{"Custom":6000}]}` from `RPCError.Data` and convert it to the generated `BlueshiftAnchorVaultError` |

## Troubleshooting

If `anchor build` fails with `feature edition2024 is required`, the cargo bundled with Solana platform-tools is too old for a newer dependency. Pin it with:

```bash
CARGO_RESOLVER_INCOMPATIBLE_RUST_VERSIONS=fallback cargo update -p blake3 --precise 1.5.5
```

(The program's `Cargo.toml` already sets `rust-version = "1.84"`, and the committed `Cargo.lock` is pinned.)

If the demo fails with `InvalidProgramForExecution` or the slot number looks far too high, another RPC server is already listening on port 8899. Start the validator on other ports and point the demo at them:

```bash
solana-test-validator --reset --rpc-port 8999 --faucet-port 9910 \
  --bpf-program 22222222222222222222222222222222222222222222 target/deploy/blueshift_anchor_vault.so

RPC_URL=http://127.0.0.1:8999 WS_URL=ws://127.0.0.1:9000 go run ./cmd/demo
```
