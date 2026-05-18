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

  return matches
    .slice()
    .sort((a, b) => submissionTimestamp(b) - submissionTimestamp(a))[0] ?? null
}

export function getPortalDocumentAction({ effectiveStatus }: PortalDocumentActionInput): PortalDocumentActionKind {
  if (effectiveStatus === 'missing' || effectiveStatus === 'rejected' || effectiveStatus === 'expired') {
    return 'upload'
  }
  return 'none'
}
