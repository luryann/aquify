export type MfaFactorSummary = {
  id: string
  factor_type: string
  is_verified: boolean
}

export type MfaBootstrapState =
  | { kind: 'enroll-totp' }
  | { kind: 'setup-backup'; totpFactorId: string }
  | { kind: 'verify'; mode: 'totp' | 'backup'; totpFactorId?: string; backupFactorId?: string }

export function resolveMfaBootstrapState(factors: MfaFactorSummary[]): MfaBootstrapState {
  const totpFactors = factors.filter((factor) => factor.factor_type === 'totp')
  const backupFactors = factors.filter((factor) => factor.factor_type === 'backup_code')
  const verifiedTotp = totpFactors.find((factor) => factor.is_verified)
  const verifiedBackup = backupFactors.find((factor) => factor.is_verified)

  if (verifiedTotp) {
    const backup = backupFactors[0]
    if (!backup) {
      return { kind: 'setup-backup', totpFactorId: verifiedTotp.id }
    }
    return {
      kind: 'verify',
      mode: 'totp',
      totpFactorId: verifiedTotp.id,
      backupFactorId: backup.id,
    }
  }

  if (verifiedBackup) {
    return {
      kind: 'verify',
      mode: 'backup',
      backupFactorId: verifiedBackup.id,
      totpFactorId: totpFactors[0]?.id,
    }
  }

  return { kind: 'enroll-totp' }
}
