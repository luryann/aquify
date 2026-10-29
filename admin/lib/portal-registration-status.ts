import type {
  DocumentSubmissionRecord,
  DocumentTypeRecord,
  RegistrationRecord,
} from './adminData.ts'
import { resolveDocumentStatus, selectLatestSubmission, type DocumentStatus } from './document-submissions.ts'

export type RegistrationDocumentRow = {
  id: string
  name: string
  category: string
  appliesTo: string
  effectiveStatus: DocumentStatus
  submittedAt?: string
  expiresAt?: string
}

export function summarizeRegistrationWaiver(registration: RegistrationRecord): {
  accepted: boolean
  count: number
  types: string[]
  lastAcceptedAt?: string
} {
  const count = registration.waiver_count ?? 0
  const types = [...(registration.waiver_types ?? [])].sort()
  return {
    accepted: Boolean(registration.waiver_accepted) || count > 0,
    count,
    types,
    lastAcceptedAt: registration.last_waiver_accepted_at,
  }
}

export function buildPortalRegistrationDocumentRows(input: {
  registration: RegistrationRecord
  swimmerId: string
  documentTypes: DocumentTypeRecord[]
  submissions: DocumentSubmissionRecord[]
  now?: Date
}): RegistrationDocumentRow[] {
  const now = input.now ?? new Date()

  return input.documentTypes
    .filter((type) => type.is_active)
    .filter((type) => type.category === 'waiver' || type.category === 'registration')
    .filter((type) => {
      if (type.applies_to === 'swimmer') return input.swimmerId.trim() !== ''
      return type.applies_to === 'household' || type.applies_to === 'all'
    })
    .map((type) => {
      const submission = selectLatestSubmission(
        input.submissions,
        type.id,
        type.applies_to === 'swimmer' ? input.swimmerId : undefined,
      )

      return {
        id: type.applies_to === 'swimmer' ? `${type.id}::${input.swimmerId}` : type.id,
        name: type.name,
        category: type.category,
        appliesTo: type.applies_to,
        effectiveStatus: resolveDocumentStatus(submission, now),
        submittedAt: submission?.submitted_at,
        expiresAt: submission?.expires_at,
      }
    })
}
