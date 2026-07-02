export const SWIMMER_BIO_MAX_LENGTH = 400

export function normalizeSwimmerBio(value: string): string {
  return value.trim()
}

export function validateSwimmerBio(value: string): string | null {
  if (normalizeSwimmerBio(value).length > SWIMMER_BIO_MAX_LENGTH) {
    return `Bio must be ${SWIMMER_BIO_MAX_LENGTH} characters or fewer.`
  }
  return null
}
