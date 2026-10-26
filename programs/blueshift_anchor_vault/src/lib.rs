use anchor_lang::prelude::*;
use anchor_lang::system_program::{transfer, Transfer};

declare_id!("22222222222222222222222222222222222222222222");

#[program]
pub mod blueshift_anchor_vault {
    use super::*;

    /// 存款：把 lamports 从 signer 转入金库 PDA
    pub fn deposit(ctx: Context<VaultAction>, amount: u64) -> Result<()> {
        // 1. 金库必须为空（每个用户只能存一次，防止重复存款）
        require_eq!(
            ctx.accounts.vault.lamports(),
            0,
            VaultError::VaultAlreadyExists
        );

        // 2. 金额必须大于 0 字节账户的免租金最低余额，否则金库账户无法存活
        require_gt!(
            amount,
            Rent::get()?.minimum_balance(0),
            VaultError::InvalidAmount
        );

        // 3. 通过 System Program 的 Transfer CPI 转账（signer 自己签名）
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

    /// 取款：把金库中的全部 lamports 转回 signer
    pub fn withdraw(ctx: Context<VaultAction>) -> Result<()> {
        // 1. 金库里必须有钱
        require_neq!(ctx.accounts.vault.lamports(), 0, VaultError::InvalidAmount);

        // 2. 金库是 PDA，没有私钥，需要用 seeds + bump 让程序代为签名
        let signer_key = ctx.accounts.signer.key();
        let signer_seeds: &[&[&[u8]]] =
            &[&[b"vault", signer_key.as_ref(), &[ctx.bumps.vault]]];

        // 3. 把金库全部余额转回 signer
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
    /// 金库所有者；lamports 会变化，所以需要 mut
    #[account(mut)]
    pub signer: Signer<'info>,

    /// 金库 PDA：seeds = ["vault", signer]；由 System Program 拥有，只存 lamports
    #[account(
        mut,
        seeds = [b"vault", signer.key().as_ref()],
        bump,
    )]
    pub vault: SystemAccount<'info>,

    /// 用于 Transfer CPI；Anchor 会校验其地址与可执行性
    pub system_program: Program<'info, System>,
}

#[error_code]
pub enum VaultError {
    #[msg("Vault already exists")]
    VaultAlreadyExists,
    #[msg("Invalid amount")]
    InvalidAmount,
}
