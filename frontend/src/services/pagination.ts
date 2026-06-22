// Shared helpers for the API's offset-token pagination. The nested PageRequest
// binds via dot notation; the kratos form codec accepts the proto field names
// (page.page_size / page.page_token).

export interface PageParams {
  pageSize?: number
  pageToken?: string
}

type QueryValue = string | number | boolean | undefined | null

// pageQuery merges pagination params into an optional base query (filters).
export function pageQuery(
  page?: PageParams,
  base?: Record<string, QueryValue>,
): Record<string, QueryValue> {
  const q: Record<string, QueryValue> = { ...(base ?? {}) }
  if (page?.pageSize) q['page.page_size'] = page.pageSize
  if (page?.pageToken) q['page.page_token'] = page.pageToken
  return q
}
