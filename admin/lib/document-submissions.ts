type SubmissionLike = {
  document_type_id: string
  swimmer_id?: string | null
  created_at?: string
  updated_at?: string
  submitted_at?: string
  verified_at?: string
}

type PortalDocumentActionInput = {
  effectiveStatus: string
  typeCategory: string
}

export type PortalDocumentActionKind = 'upload' | 'none'

/** Status shown to families for a document requirement; 'missing' means nothing was submitted. */
export type DocumentStatus = 'verified' | 'submitted' | 'pending' | 'rejected' | 'expired' | 'missing'

/** Derives the display status from the latest submission, treating a lapsed, unverified document as expired. */
export function resolveDocumentStatus(
  submission: { status: string; expires_at?: string | null } | null,
  now: Date = new Date(),
): DocumentStatus {
  if (!submission) return 'missing'
  if (submission.expires_at && new Date(submission.expires_at).getTime() < now.getTime() && submission.status !== 'verified') {
    return 'expired'
  }
  switch (submission.status) {
    case 'verified':
    case 'submitted':
    case 'pending':
    case 'rejected':
      return submission.status
    default:
      return 'pending'
  }
}

/**
 * True when the family still has to upload something. 'pending' means a
 * submission exists with no attached file (an interrupted upload).
 */
export function documentNeedsAction(status: string): boolean {
  return status === 'missing' || status === 'pending' || status === 'rejected' || status === 'expired'
}

function normalizeSwimmerId(swimmerId?: string | null): string {
  return (swimmerId ?? '').trim()
}

function submissionTimestamp(submission: SubmissionLike): number {
  const candidates = [
    submission.updated_at,
    submission.verified_at,
    submission.submitted_at,
    submission.created_at,
  ]
  for (const candidate of candidates) {
    if (!candidate) continue
    const parsed = Date.parse(candidate)
    if (!Number.isNaN(parsed)) {
      return parsed
    }
  }
  return 0
}

export function selectLatestSubmission<T extends SubmissionLike>(
  submissions: T[],
  documentTypeId: string,
  swimmerId?: string | null,
): T | null {
  const targetSwimmerId = normalizeSwimmerId(swimmerId)
  const matches = submissions.filter((submission) => (
    submission.document_type_id === documentTypeId &&
    normalizeSwimmerId(submission.swimmer_id) === targetSwimmerId
  ))

  if (matches.length === 0) {
    return null
  }

  return matches.reduce((latest, next) => (submissionTimestamp(next) > submissionTimestamp(latest) ? next : latest))
}

export function getPortalDocumentAction({ effectiveStatus }: PortalDocumentActionInput): PortalDocumentActionKind {
  return documentNeedsAction(effectiveStatus) ? 'upload' : 'none'
}
