/**
 * Pulls the file name out of a `Content-Disposition` header. Handles the
 * RFC 6266 forms the API emits: a plain token, a quoted string (which may hold
 * `;` or escaped quotes) and the `filename*=UTF-8''…` form used for non-ASCII
 * names, which wins when both are present. Returns `fallback` when no usable
 * name is found.
 */
export function filenameFromContentDisposition(header: string | null | undefined, fallback: string): string {
  if (!header) return fallback

  const star = /filename\*\s*=\s*([^;]+)/i.exec(header)?.[1]?.trim()
  if (star) {
    const encoded = star.replace(/^[^']*'[^']*'/, '')
    try {
      const decoded = decodeURIComponent(encoded).replace(/["\\/]/g, '').trim()
      if (decoded) return decoded
    } catch {
      // malformed escape: fall through to the plain form
    }
  }

  const quoted = /filename\s*=\s*"((?:[^"\\]|\\.)*)"/i.exec(header)?.[1]
  if (quoted !== undefined) {
    const name = quoted.replace(/\\(.)/g, '$1').replace(/[\\/]/g, '').trim()
    if (name) return name
  }

  const token = /filename\s*=\s*([^;\s"]+)/i.exec(header)?.[1]
  return token?.replace(/[\\/]/g, '') || fallback
}
