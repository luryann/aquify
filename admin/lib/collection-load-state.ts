export function shouldShowCollectionEmptyState(loading: boolean, error: string | null, count: number): boolean {
  return !loading && !error && count === 0
}

export function shouldShowCollectionContent(loading: boolean, error: string | null, count: number): boolean {
  return !loading && !error && count > 0
}
