// A small dependency-free line diff (LCS) used to compare the pending KumoMTA
// policy against the running one. Returns a unified sequence of lines, each
// tagged as unchanged context, added (in the new text), or removed (only in the
// old text).

export type DiffLineType = 'context' | 'add' | 'del'

export interface DiffLine {
  type: DiffLineType
  text: string
  /** 1-based line number in the old text (null for added lines). */
  oldNumber: number | null
  /** 1-based line number in the new text (null for removed lines). */
  newNumber: number | null
}

export interface DiffResult {
  lines: DiffLine[]
  added: number
  removed: number
}

// Guard against pathological memory use on very large inputs (the LCS table is
// O(n*m)). Real KumoMTA policies are a few hundred lines; well under this.
const MAX_LINES = 6000

function splitLines(text: string): string[] {
  return text.length ? text.split('\n') : []
}

export function diffLines(oldText: string, newText: string): DiffResult {
  const a = splitLines(oldText)
  const b = splitLines(newText)

  // Fallback for oversized inputs: emit the new text as all-context so the view
  // still renders something useful rather than exhausting memory.
  if (a.length > MAX_LINES || b.length > MAX_LINES) {
    return {
      lines: b.map((text, i) => ({ type: 'context', text, oldNumber: null, newNumber: i + 1 })),
      added: 0,
      removed: 0,
    }
  }

  const n = a.length
  const m = b.length
  // dp[i][j] = length of the LCS of a[i:] and b[j:].
  const dp: number[][] = Array.from({ length: n + 1 }, () => new Array(m + 1).fill(0))
  for (let i = n - 1; i >= 0; i -= 1) {
    for (let j = m - 1; j >= 0; j -= 1) {
      dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1])
    }
  }

  const lines: DiffLine[] = []
  let added = 0
  let removed = 0
  let i = 0
  let j = 0
  let oldNo = 1
  let newNo = 1

  const pushContext = () => {
    lines.push({ type: 'context', text: a[i], oldNumber: oldNo, newNumber: newNo })
    i += 1
    j += 1
    oldNo += 1
    newNo += 1
  }
  const pushDel = () => {
    lines.push({ type: 'del', text: a[i], oldNumber: oldNo, newNumber: null })
    i += 1
    oldNo += 1
    removed += 1
  }
  const pushAdd = () => {
    lines.push({ type: 'add', text: b[j], oldNumber: null, newNumber: newNo })
    j += 1
    newNo += 1
    added += 1
  }

  while (i < n && j < m) {
    if (a[i] === b[j]) pushContext()
    else if (dp[i + 1][j] >= dp[i][j + 1]) pushDel()
    else pushAdd()
  }
  while (i < n) pushDel()
  while (j < m) pushAdd()

  return { lines, added, removed }
}
