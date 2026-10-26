# Blueshift Anchor Vault

[Blueshift Anchor 金库挑战](https://learn.blueshift.gg/zh-CN/challenges/anchor-vault) 的参考实现：每个用户有一个由 PDA 托管的 SOL 金库，可以存入（deposit）和全部取出（withdraw）。

## 核心知识点

| 概念 | 代码位置 |
| --- | --- |
| PDA 派生：`seeds = [b"vault", signer]` + `bump` | `VaultAction.vault` |
| `SystemAccount`：只存 lamports、归 System Program 所有的账户 | `VaultAction.vault` |
| 普通 CPI：`CpiContext::new`（signer 自己签名） | `deposit` |
| PDA 签名 CPI：`CpiContext::new_with_signer` + `ctx.bumps.vault` | `withdraw` |
| 免租金最低余额：`Rent::get()?.minimum_balance(0)` | `deposit` |
| 自定义错误：`#[error_code]` + `require_*!` 宏 | `VaultError` |

## 构建 & 测试

```bash
anchor build          # 产物：target/deploy/blueshift_anchor_vault.so
anchor test --skip-build
```

提交挑战：在挑战页面点击 "take challenge"，上传 `target/deploy/blueshift_anchor_vault.so`。

## 说明

- 挑战要求程序 ID 为 `22222222222222222222222222222222222222222222`，这个地址没有私钥，无法 `anchor deploy`。
  所以 `Anchor.toml` 中用 `[[test.genesis]]` 让本地验证器在启动时直接把 `.so` 加载到该地址。
- 如果 `anchor build` 报 `feature edition2024 is required`：Solana platform-tools 内置的 cargo 版本较旧，
  新版依赖不兼容。解决方法：
  ```bash
  CARGO_RESOLVER_INCOMPATIBLE_RUST_VERSIONS=fallback cargo update -p blake3 --precise 1.5.5
  ```
  （本项目已在程序 `Cargo.toml` 中设置 `rust-version = "1.84"`，项目里也已包含锁定好的 `Cargo.lock`。）

---

# Go 客户端：Codama + solana-go v2

这一部分演示：**Anchor 程序 → IDL → Codama → Go 客户端 → 用 solana-go v2 与链上交互**。

```
programs/…/lib.rs ──anchor build──▶ target/idl/blueshift_anchor_vault.json
                                          │
                       scripts/generate-go-client.mjs (Codama)
                                          ▼
                              clients/go/generated/*.go   ◀── 自动生成，勿手改
                                          │
                              clients/go/cmd/demo/main.go ◀── 用 solana-go v2 调用
```

## 目录

| 路径 | 说明 |
| --- | --- |
| `scripts/generate-go-client.mjs` | Codama 生成脚本：`rootNodeFromAnchor` 读 IDL，`@codama/renderers-go` 渲染 Go 代码 |
| `clients/go/generated/` | 生成的 Go 包 `blueshift_anchor_vault` |
| `clients/go/cmd/demo/main.go` | 交互 demo：空投 → deposit → 重复 deposit（报错）→ withdraw |

生成的文件：

| 文件 | 内容 |
| --- | --- |
| `instructions.go` | `ProgramID`、`Instruction` 包装类型（实现 `solana.Instruction`）、`DecodeInstruction` |
| `instruction_deposit.go` / `instruction_withdraw.go` | 指令 builder：`SetAmount` / `SetSignerAccount` / `SetVaultAccount` … `ValidateAndBuild()`，带 Anchor 8 字节 discriminator |
| `pdas.go` | `FindVaultPDA(signer)`：IDL 里的 `seeds = ["vault", signer]` 直接变成了 Go 函数 |
| `errors.go` | `BlueshiftAnchorVaultError`：`VaultAlreadyExistsError = 6000`、`InvalidAmountError = 6001` |

## 关于 solana-go v2

`@codama/renderers-go` 目前生成的 import 是 v1 的 `github.com/gagliardetto/solana-go`。
v2 的 API 与 v1 一致，只是模块路径变成了 `github.com/solana-foundation/solana-go/v2`，
所以生成脚本最后一步把 import 改写成 v2（`github.com/gagliardetto/binary` 在 v2 中仍在使用，保持不变）。

## 运行

```bash
# 1. 编译程序，得到 .so 和 IDL
anchor build

# 2. 生成 Go 客户端
yarn install
yarn generate:go

# 3. 启动本地验证器，把 .so 直接加载到 2222… 地址（这个地址没有私钥，不能 anchor deploy）
solana-test-validator --reset \
  --bpf-program 22222222222222222222222222222222222222222222 target/deploy/blueshift_anchor_vault.so

# 4. 另开一个终端运行 demo
cd clients/go
go run ./cmd/demo
```

可以用 `RPC_URL` / `WS_URL` 环境变量连接其他集群。

预期输出：

```
用户钱包: EgRBEk3L…
空投 2 SOL 完成
金库 PDA:  J7iQGzHt… (bump 255)
  用户余额: 2.000000 SOL
  金库余额: 0.000000 SOL

[deposit] 存入 1 SOL, tx: 56Ric7a8…
  用户余额: 0.999995 SOL
  金库余额: 1.000000 SOL

[deposit again] 被程序拒绝: Vault already exists (code 6000)

[withdraw] 取回全部 lamports, tx: 3j2PSspm…
  用户余额: 1.999990 SOL
  金库余额: 0.000000 SOL
```

## Demo 里用到的 solana-go v2 API

| 步骤 | API |
| --- | --- |
| 连接 | `rpc.New(rpc.LocalNet_RPC)`、`ws.Connect(ctx, rpc.LocalNet_WS)` |
| 钱包 / 空投 | `solana.NewWallet()`、`client.RequestAirdrop` |
| 组装交易 | `client.GetLatestBlockhash` + `solana.NewTransaction(ixs, blockhash, solana.TransactionPayer(...))` |
| 签名 | `tx.Sign(func(pubkey) *solana.PrivateKey { … })` |
| 发送 | `client.SendTransactionWithOpts`（预检失败时返回 `*jsonrpc.RPCError`） |
| 确认 | `wsClient.SignatureSubscribe(sig, rpc.CommitmentConfirmed)` |
| 自定义错误 | 从 `RPCError.Data` 取 `{"InstructionError":[0,{"Custom":6000}]}`，转换为生成的 `BlueshiftAnchorVaultError` |
