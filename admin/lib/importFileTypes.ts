export const SUPPORTED_IMPORT_FILE_TYPES = ['SD3', 'CL2', 'ZIP', 'HY3', 'HYV', 'EV3'] as const
export const IMPORT_ACCEPT_EXTENSIONS = '.sd3,.cl2,.zip,.hy3,.hyv,.ev3'

const DISPLAY_IMPORT_FILE_TYPES = [...SUPPORTED_IMPORT_FILE_TYPES, 'UNKNOWN'] as const

export type SupportedImportFileType = (typeof SUPPORTED_IMPORT_FILE_TYPES)[number]
export type DisplayImportFileType = (typeof DISPLAY_IMPORT_FILE_TYPES)[number]

export const IMPORT_FILE_TYPE_CFG: Record<DisplayImportFileType, { color: string; bg: string }> = {
  SD3: { color: 'var(--info)', bg: 'var(--info-bg)' },
  CL2: { color: 'var(--accent)', bg: 'var(--accent-subtle)' },
  ZIP: { color: 'var(--warning)', bg: 'var(--warning-bg)' },
  HY3: { color: 'var(--success)', bg: 'var(--success-bg)' },
  HYV: { color: 'var(--success)', bg: 'rgba(27, 122, 55, 0.14)' },
  EV3: { color: 'var(--accent)', bg: 'rgba(17, 91, 153, 0.14)' },
  UNKNOWN: { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
}

export function normalizeImportFileType(fileType: string): DisplayImportFileType {
  const normalized = fileType.trim().toUpperCase()
  return DISPLAY_IMPORT_FILE_TYPES.includes(normalized as DisplayImportFileType)
    ? normalized as DisplayImportFileType
    : 'UNKNOWN'
}
