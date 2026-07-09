// Client-side mirror of the backend subject-classification matcher
// (backend/internal/biz/subject_classifier_usecase.go). It powers the on-page
// "simulate a match" tool: regex matching and priority ordering are exact;
// similarity is an approximation of PostgreSQL's pg_trgm (shown as such in the
// UI) since the true score is computed in the database.
import type { SubjectClassification, SubjectMatchType } from '@/types'

// Default pg_trgm similarity threshold. The live matcher reads this from global
// settings; 0.3 is the pg_trgm default and a sensible preview value.
export const DEFAULT_SIMILARITY_THRESHOLD = 0.3

// normalizeSubject mirrors Go's NormalizeSubject: lowercase, strip repeated
// reply/forward prefixes, then keep only letters with every run of
// digits/punctuation/whitespace collapsed to a single space.
export function normalizeSubject(input: string): string {
  let s = input.trim().toLowerCase()
  const prefix = /^\s*(re|fwd|fw)\s*:\s*/i
  for (;;) {
    const t = s.replace(prefix, '').trim()
    if (t === s) break
    s = t
  }
  let out = ''
  let space = true // suppress leading space
  for (const ch of s) {
    if (/\p{L}/u.test(ch)) {
      out += ch
      space = false
    } else if (!space) {
      out += ' '
      space = true
    }
  }
  return out.trim()
}

// trigrams returns the distinct pg_trgm-style trigram set for a normalized
// string: each word is padded with two leading blanks and one trailing blank,
// then split into 3-character windows.
function trigrams(normalized: string): Set<string> {
  const set = new Set<string>()
  for (const word of normalized.split(' ')) {
    if (!word) continue
    const padded = `  ${word} `
    for (let i = 0; i + 3 <= padded.length; i++) {
      set.add(padded.slice(i, i + 3))
    }
  }
  return set
}

// similarity approximates pg_trgm.similarity(): the Jaccard index of the two
// trigram sets. Not bit-identical to the database, but close enough to preview
// which similarity rule would win.
export function similarity(a: string, b: string): number {
  const ta = trigrams(a)
  const tb = trigrams(b)
  if (ta.size === 0 && tb.size === 0) return 0
  let inter = 0
  for (const g of ta) if (tb.has(g)) inter++
  const union = ta.size + tb.size - inter
  return union === 0 ? 0 : inter / union
}

// compileRule converts a rule's (Go/RE2) regex pattern to a JS RegExp, mapping a
// leading inline flag group like (?i) / (?is) to JS flags (JS has no inline
// flag syntax). Returns null when the pattern cannot be compiled.
export function compileRule(pattern: string): RegExp | null {
  let src = pattern
  let flags = ''
  const inline = src.match(/^\(\?([ims]+)\)/)
  if (inline) {
    if (inline[1].includes('i')) flags += 'i'
    if (inline[1].includes('m')) flags += 'm'
    if (inline[1].includes('s')) flags += 's'
    src = src.slice(inline[0].length)
  }
  try {
    return new RegExp(src, flags)
  } catch {
    return null
  }
}

export type MatchReason =
  | { kind: 'regex' }
  | { kind: 'similarity'; score: number }
  | { kind: 'regex-error' }

export interface RuleMatch {
  rule: SubjectClassification
  reason: MatchReason
}

export interface SimulationResult {
  // The rule the live matcher would pick (highest priority; regex wins ties),
  // or null when nothing matches (the subject would fall through to AI/none).
  winner: RuleMatch | null
  // Every rule that matched, ordered as the matcher evaluates them: priority
  // descending, regex before similarity on a tie, then similarity score.
  matches: RuleMatch[]
  // Regex rules whose pattern failed to compile (surfaced as a warning).
  invalid: SubjectClassification[]
}

function matchType(rule: SubjectClassification): SubjectMatchType {
  return rule.matchType === 'regex' ? 'regex' : 'similarity'
}

// simulate resolves which rule would classify `subject`, mirroring the backend:
// evaluate all rules, keep those that match, and choose the highest priority —
// an equal-priority tie is resolved in favour of the explicit regex rule.
export function simulate(
  subject: string,
  rules: SubjectClassification[],
  threshold = DEFAULT_SIMILARITY_THRESHOLD,
): SimulationResult {
  const norm = normalizeSubject(subject)
  const matches: RuleMatch[] = []
  const invalid: SubjectClassification[] = []

  for (const rule of rules) {
    if (!rule.label) continue // pending/unlabeled rules never match
    if (matchType(rule) === 'regex') {
      const re = compileRule(rule.subject)
      if (!re) {
        invalid.push(rule)
        continue
      }
      if (re.test(subject)) matches.push({ rule, reason: { kind: 'regex' } })
    } else {
      if (!norm) continue
      const score = similarity(norm, rule.subjectNormalized)
      if (score >= threshold) matches.push({ rule, reason: { kind: 'similarity', score } })
    }
  }

  // Evaluation order: priority desc; regex before similarity on a tie; then the
  // stronger similarity score. This makes matches[0] the winner.
  matches.sort((a, b) => {
    if (b.rule.priority !== a.rule.priority) return b.rule.priority - a.rule.priority
    const ak = a.reason.kind === 'regex' ? 1 : 0
    const bk = b.reason.kind === 'regex' ? 1 : 0
    if (ak !== bk) return bk - ak
    const as = a.reason.kind === 'similarity' ? a.reason.score : 0
    const bs = b.reason.kind === 'similarity' ? b.reason.score : 0
    return bs - as
  })

  return { winner: matches[0] ?? null, matches, invalid }
}
