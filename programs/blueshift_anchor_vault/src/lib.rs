use anchor_lang::prelude::*;
use anchor_lang::system_program::{transfer, Transfer};

declare_id!("22222222222222222222222222222222222222222222");

#[program]
pub mod blueshift_anchor_vault {
    use super::*;

    /// Deposit: transfer lamports from the signer into the vault PDA.
    pub fn deposit(ctx: Context<VaultAction>, amount: u64) -> Result<()> {
        // 1. The vault must be empty (one deposit per user, prevents double deposits).
        require_eq!(
            ctx.accounts.vault.lamports(),
            0,
            VaultError::VaultAlreadyExists
        );

        // 2. The amount must exceed the rent-exempt minimum of a 0-byte account,
        //    otherwise the vault account cannot exist.
        require_gt!(
            amount,
            Rent::get()?.minimum_balance(0),
            VaultError::InvalidAmount
        );

        // 3. Transfer via a System Program CPI (signed by the signer itself).
        transfer(
            CpiContext::new(
                ctx.accounts.system_program.to_account_info(),
                Transfer {
                    from: ctx.accounts.signer.to_account_info(),
                    to: ctx.accounts.vault.to_account_info(),
                },
            ),
            amount,
        )?;

        Ok(())
    }

    /// Withdraw: transfer all lamports in the vault back to the signer.
    pub fn withdraw(ctx: Context<VaultAction>) -> Result<()> {
        // 1. The vault must hold some lamports.
        require_neq!(ctx.accounts.vault.lamports(), 0, VaultError::InvalidAmount);

        // 2. The vault is a PDA with no private key; the program signs for it with seeds + bump.
        let signer_key = ctx.accounts.signer.key();
        let signer_seeds: &[&[&[u8]]] =
            &[&[b"vault", signer_key.as_ref(), &[ctx.bumps.vault]]];

        // 3. Transfer the whole vault balance back to the signer.
        transfer(
            CpiContext::new_with_signer(
                ctx.accounts.system_program.to_account_info(),
                Transfer {
                    from: ctx.accounts.vault.to_account_info(),
                    to: ctx.accounts.signer.to_account_info(),
                },
                signer_seeds,
            ),
            ctx.accounts.vault.lamports(),
        )?;

        Ok(())
    }
}

#[derive(Accounts)]
pub struct VaultAction<'info> {
    /// Vault owner; its lamports change, so it must be mut.
    #[account(mut)]
    pub signer: Signer<'info>,

    /// Vault PDA: seeds = ["vault", signer]; owned by the System Program, holds only lamports.
    #[account(
        mut,
        seeds = [b"vault", signer.key().as_ref()],
        bump,
    )]
    pub vault: SystemAccount<'info>,

    /// Used for the Transfer CPI; Anchor checks its address and that it is executable.
    pub system_program: Program<'info, System>,
}

#[error_code]
pub enum VaultError {
    #[msg("Vault already exists")]
    VaultAlreadyExists,
    #[msg("Invalid amount")]
    InvalidAmount,
}
