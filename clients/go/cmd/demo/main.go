// Interact with the Anchor vault program using the Codama-generated Go client
// and solana-go v2.
//
// Steps:
//  1. Create a new wallet and airdrop SOL to it
//  2. Derive the vault address with the generated FindVaultPDA
//  3. deposit: put 1 SOL into the vault
//  4. deposit again: the program returns the custom error VaultAlreadyExists,
//     decoded into the generated error type
//  5. withdraw: take all lamports back out of the vault
//
// Start a local validator first (see README):
//
//	go run ./cmd/demo
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/solana-foundation/solana-go/v2"
	"github.com/solana-foundation/solana-go/v2/rpc"
	"github.com/solana-foundation/solana-go/v2/rpc/jsonrpc"
	"github.com/solana-foundation/solana-go/v2/rpc/ws"

	vault "github.com/HealthyBuilder/anchor_vault_solana_go/clients/go/generated"
)

func main() {
	ctx := context.Background()

	rpcURL := envOr("RPC_URL", rpc.LocalNet_RPC)
	wsURL := envOr("WS_URL", rpc.LocalNet_WS)

	client := rpc.New(rpcURL)
	wsClient, err := ws.Connect(ctx, wsURL)
	if err != nil {
		log.Fatalf("connect ws: %v", err)
	}
	defer wsClient.Close()

	// ---------- 1. New wallet + airdrop ----------
	user := solana.NewWallet()
	fmt.Println("User wallet: ", user.PublicKey())

	sig, err := client.RequestAirdrop(ctx, user.PublicKey(), 2*solana.LAMPORTS_PER_SOL, rpc.CommitmentConfirmed)
	if err != nil {
		log.Fatalf("airdrop: %v", err)
	}
	if err := waitConfirmed(ctx, wsClient, sig); err != nil {
		log.Fatalf("airdrop confirm: %v", err)
	}
	fmt.Println("Airdropped 2 SOL")

	// ---------- 2. Derive the vault PDA (seeds = ["vault", signer]) ----------
	vaultPDA, bump, err := vault.FindVaultPDA(user.PublicKey())
	if err != nil {
		log.Fatalf("find vault pda: %v", err)
	}
	fmt.Printf("Vault PDA:   %s (bump %d)\n", vaultPDA, bump)
	printBalances(ctx, client, user.PublicKey(), vaultPDA)

	// ---------- 3. deposit 1 SOL ----------
	depositIx, err := vault.NewDepositInstructionBuilder().
		SetAmount(solana.LAMPORTS_PER_SOL).
		SetSignerAccount(user.PublicKey()).
		SetVaultAccount(vaultPDA).
		SetSystemProgramAccount(solana.SystemProgramID).
		ValidateAndBuild()
	if err != nil {
		log.Fatalf("build deposit: %v", err)
	}

	sig, err = sendTx(ctx, client, wsClient, user, depositIx)
	if err != nil {
		log.Fatalf("deposit: %v", err)
	}
	fmt.Println("\n[deposit] deposited 1 SOL, tx:", sig)
	printBalances(ctx, client, user.PublicKey(), vaultPDA)

	// ---------- 4. deposit again: expected to fail ----------
	// The Instruction returned by Build() aliases the builder, so it can be reused as is.
	_, err = sendTx(ctx, client, wsClient, user, depositIx)
	var vaultErr vault.BlueshiftAnchorVaultError
	if errors.As(err, &vaultErr) {
		fmt.Printf("\n[deposit again] rejected by the program: %s (code %d)\n", vaultErr, vaultErr.Code())
	} else {
		log.Fatalf("expected VaultAlreadyExists, got: %v", err)
	}

	// ---------- 5. withdraw ----------
	withdrawIx, err := vault.NewWithdrawInstructionBuilder().
		SetSignerAccount(user.PublicKey()).
		SetVaultAccount(vaultPDA).
		SetSystemProgramAccount(solana.SystemProgramID).
		ValidateAndBuild()
	if err != nil {
		log.Fatalf("build withdraw: %v", err)
	}

	sig, err = sendTx(ctx, client, wsClient, user, withdrawIx)
	if err != nil {
		log.Fatalf("withdraw: %v", err)
	}
	fmt.Println("\n[withdraw] withdrew all lamports, tx:", sig)
	printBalances(ctx, client, user.PublicKey(), vaultPDA)
}

// sendTx builds, signs and sends a transaction, then waits for it to be confirmed.
// A custom program error returned during preflight (simulation) is converted
// into the generated BlueshiftAnchorVaultError.
func sendTx(ctx context.Context, client *rpc.Client, wsClient *ws.Client, payer *solana.Wallet, ixs ...solana.Instruction) (solana.Signature, error) {
	recent, err := client.GetLatestBlockhash(ctx, rpc.CommitmentConfirmed)
	if err != nil {
		return solana.Signature{}, err
	}

	tx, err := solana.NewTransaction(ixs, recent.Value.Blockhash, solana.TransactionPayer(payer.PublicKey()))
	if err != nil {
		return solana.Signature{}, err
	}
	if _, err := tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(payer.PublicKey()) {
			return &payer.PrivateKey
		}
		return nil
	}); err != nil {
		return solana.Signature{}, err
	}

	sig, err := client.SendTransactionWithOpts(ctx, tx, rpc.TransactionOpts{
		PreflightCommitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		if code, ok := customErrorCode(err); ok {
			return sig, vault.BlueshiftAnchorVaultError(code)
		}
		return sig, err
	}
	return sig, waitConfirmed(ctx, wsClient, sig)
}

// waitConfirmed subscribes to the signature over WebSocket until the transaction is confirmed.
func waitConfirmed(ctx context.Context, wsClient *ws.Client, sig solana.Signature) error {
	sub, err := wsClient.SignatureSubscribe(sig, rpc.CommitmentConfirmed)
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	got, err := sub.Recv(ctx)
	if err != nil {
		return err
	}
	if got.Value.Processed != nil && got.Value.Processed.Err != nil {
		return fmt.Errorf("transaction failed: %v", got.Value.Processed.Err)
	}
	return nil
}

// customErrorCode extracts the program's custom error code from a failed preflight RPC error:
//
//	{"err": {"InstructionError": [0, {"Custom": 6000}]}, "logs": [...]}
func customErrorCode(err error) (uint32, bool) {
	var rpcErr *jsonrpc.RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Data == nil {
		return 0, false
	}
	raw, _ := json.Marshal(rpcErr.Data)

	var data struct {
		Err struct {
			InstructionError [2]json.RawMessage
		}
	}
	if json.Unmarshal(raw, &data) != nil {
		return 0, false
	}
	var custom struct{ Custom *uint32 }
	if json.Unmarshal(data.Err.InstructionError[1], &custom) != nil || custom.Custom == nil {
		return 0, false
	}
	return *custom.Custom, true
}

func printBalances(ctx context.Context, client *rpc.Client, user, vaultPDA solana.PublicKey) {
	fmt.Printf("  User balance:  %.6f SOL\n", sol(balance(ctx, client, user)))
	fmt.Printf("  Vault balance: %.6f SOL\n", sol(balance(ctx, client, vaultPDA)))
}

func balance(ctx context.Context, client *rpc.Client, key solana.PublicKey) uint64 {
	out, err := client.GetBalance(ctx, key, rpc.CommitmentConfirmed)
	if err != nil {
		log.Fatalf("get balance %s: %v", key, err)
	}
	return out.Value
}

func sol(lamports uint64) float64 {
	return float64(lamports) / float64(solana.LAMPORTS_PER_SOL)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
