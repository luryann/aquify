/** "All ages", "Up to 12", "13+" or "9-12" for a group's age bounds. */
export function formatAgeRange(minAge: number | null | undefined, maxAge: number | null | undefined): string {
  if (minAge == null && maxAge == null) return 'All ages'
  if (minAge == null) return `Up to ${maxAge}`
  if (maxAge == null) return `${minAge}+`
  return `${minAge}-${maxAge}`
}
