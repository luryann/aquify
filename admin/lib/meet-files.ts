// Display helpers for the meet-scoped files (reports and exports) shown to
// parents and swimmers on the meet detail pages.

export function formatMeetFileSize(sizeBytes?: number): string {
  if (!sizeBytes) return 'Unknown size'
  if (sizeBytes < 1024) return `${sizeBytes} B`
  return `${Math.round(sizeBytes / 1024)} KB`
}

export function formatMeetFileType(exportType: string): string {
  if (exportType === 'eligibility_report') return 'Eligibility report'
  if (exportType === 'entry_report') return 'Entry report'
  if (exportType === 'event_report') return 'Event report'
  return exportType.replace(/_/g, ' ')
}
