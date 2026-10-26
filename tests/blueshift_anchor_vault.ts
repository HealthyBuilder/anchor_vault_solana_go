import * as anchor from "@coral-xyz/anchor";
import { Program } from "@coral-xyz/anchor";
import { LAMPORTS_PER_SOL, PublicKey, SystemProgram } from "@solana/web3.js";
import { expect } from "chai";
import { BlueshiftAnchorVault } from "../target/types/blueshift_anchor_vault";

describe("blueshift_anchor_vault", () => {
  const provider = anchor.AnchorProvider.env();
  anchor.setProvider(provider);

  const program = anchor.workspace
    .blueshiftAnchorVault as Program<BlueshiftAnchorVault>;
  const signer = provider.wallet.publicKey;

  // 金库 PDA：seeds = ["vault", signer]
  const [vault] = PublicKey.findProgramAddressSync(
    [Buffer.from("vault"), signer.toBuffer()],
    program.programId
  );
  const accounts = { signer, vault, systemProgram: SystemProgram.programId };

  it("拒绝低于免租金额的存款", async () => {
    try {
      await program.methods.deposit(new anchor.BN(1)).accounts(accounts).rpc();
      expect.fail("应当失败");
    } catch (e: any) {
      expect(e.error.errorCode.code).to.equal("InvalidAmount");
    }
  });

  it("存入 1 SOL", async () => {
    const amount = 1 * LAMPORTS_PER_SOL;
    await program.methods.deposit(new anchor.BN(amount)).accounts(accounts).rpc();
    expect(await provider.connection.getBalance(vault)).to.equal(amount);
  });

  it("重复存款失败", async () => {
    try {
      await program.methods
        .deposit(new anchor.BN(LAMPORTS_PER_SOL))
        .accounts(accounts)
        .rpc();
      expect.fail("应当失败");
    } catch (e: any) {
      expect(e.error.errorCode.code).to.equal("VaultAlreadyExists");
    }
  });

  it("取出全部余额", async () => {
    await program.methods.withdraw().accounts(accounts).rpc();
    expect(await provider.connection.getBalance(vault)).to.equal(0);
  });

  it("空金库取款失败", async () => {
    try {
      await program.methods.withdraw().accounts(accounts).rpc();
      expect.fail("应当失败");
    } catch (e: any) {
      expect(e.error.errorCode.code).to.equal("InvalidAmount");
    }
  });
});
