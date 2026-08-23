/**
 * Parse the i18n catalog sources into (key, locale, string) rows.
 * Understands both the locale-file form (`'key': 'value'`) and the
 * per-key `{ en, ko, ja }` objects. Used by dump-i18n-catalog.mjs.
 */

import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const HERE = dirname(fileURLToPath(import.meta.url))
export const I18N_DIR = join(HERE, '../src/lib/i18n')
export const LOCALES = ['en', 'ko', 'ja']

export function parseQuoted(src, i) {
  const q = src[i]
  if (q !== "'" && q !== '"') {
    throw new Error(`not a quote at ${i}: ${JSON.stringify(src.slice(i, i + 24))}`)
  }
  i++
  let out = ''
  while (i < src.length) {
    const c = src[i]
    if (c === '\\') {
      const n = src[i + 1]
      if (n === undefined) throw new Error('dangling escape')
      if (n === 'x') {
        out += String.fromCharCode(parseInt(src.slice(i + 2, i + 4), 16))
        i += 4
        continue
      }
      if (n === 'u') {
        if (src[i + 2] === '{') {
          const end = src.indexOf('}', i + 3)
          out += String.fromCodePoint(parseInt(src.slice(i + 3, end), 16))
          i = end + 1
          continue
        }
        out += String.fromCharCode(parseInt(src.slice(i + 2, i + 6), 16))
        i += 6
        continue
      }
      const map = {
        n: '\n',
        r: '\r',
        t: '\t',
        b: '\b',
        f: '\f',
        v: '\v',
        0: '\0',
        '\\': '\\',
        "'": "'",
        '"': '"',
        '/': '/',
      }
      out += map[n] ?? n
      i += 2
      continue
    }
    if (c === q) return { value: out, next: i + 1 }
    out += c
    i++
  }
  throw new Error('unterminated string')
}

function skipWsAndComments(src, i) {
  while (i < src.length) {
    const c = src[i]
    if (c === ' ' || c === '\t' || c === '\n' || c === '\r') {
      i++
      continue
    }
    if (c === '/' && src[i + 1] === '/') {
      while (i < src.length && src[i] !== '\n') i++
      continue
    }
    if (c === '/' && src[i + 1] === '*') {
      const end = src.indexOf('*/', i + 2)
      if (end < 0) throw new Error('unterminated block comment')
      i = end + 2
      continue
    }
    break
  }
  return i
}

/**
 * Pull `'key': <string>` entries out of a locale-file object body.
 * Comments immediately before a key are attached; `──` block comments
 * are emitted as section markers.
 */
export function extractLocaleEntries(src) {
  const m = src.match(/export const (en|ko|ja) = \{/)
  if (!m) throw new Error('no locale export')
  const brace = src.indexOf('{', m.index)
  let i = brace + 1
  const entries = []
  let pending = []
  const flushWsComments = () => {
    while (i < src.length) {
      const c = src[i]
      if (c === ' ' || c === '\t' || c === '\r' || c === '\n') {
        i++
        continue
      }
      if (c === '/' && src[i + 1] === '/') {
        let j = i
        while (j < src.length && src[j] !== '\n') j++
        pending.push(src.slice(i, j))
        i = j
        continue
      }
      if (c === '/' && src[i + 1] === '*') {
        const end = src.indexOf('*/', i + 2)
        if (end < 0) throw new Error('unterminated block comment')
        pending.push(src.slice(i, end + 2))
        i = end + 2
        continue
      }
      break
    }
  }
  while (i < src.length) {
    flushWsComments()
    if (src[i] === '}') break
    const section = pending.filter((c) => c.includes('──'))
    const comments = pending.filter((c) => !c.includes('──'))
    pending = []
    if (src[i] !== "'" && src[i] !== '"') {
      throw new Error(`expected key at ${i}: ${JSON.stringify(src.slice(i, i + 40))}`)
    }
    const key = parseQuoted(src, i)
    i = skipWsAndComments(src, key.next)
    if (src[i] !== ':') throw new Error(`expected colon after ${key.value}`)
    i = skipWsAndComments(src, i + 1)
    const val = parseQuoted(src, i)
    for (const s of section) entries.push({ kind: 'section', text: s })
    entries.push({ kind: 'entry', key: key.value, value: val.value, comments })
    i = skipWsAndComments(src, val.next)
    if (src[i] === ',') i++
  }
  return entries
}

/**
 * Pull `'key': { en, ko, ja }` entries out of a messages/*.ts object body.
 */
export function extractMessageEntries(src) {
  const m = src.match(/export const \w+ = \{/)
  if (!m) throw new Error('no messages export')
  const brace = src.indexOf('{', m.index)
  let i = brace + 1
  const entries = []
  while (i < src.length) {
    i = skipWsAndComments(src, i)
    if (src[i] === '}') break
    if (src[i] !== "'" && src[i] !== '"') {
      throw new Error(`expected key at ${i}: ${JSON.stringify(src.slice(i, i + 40))}`)
    }
    const key = parseQuoted(src, i)
    i = skipWsAndComments(src, key.next)
    if (src[i] !== ':') throw new Error(`expected colon after ${key.value}`)
    i = skipWsAndComments(src, i + 1)
    if (src[i] !== '{') throw new Error(`expected message object for ${key.value}`)
    i++
    const msg = {}
    while (i < src.length) {
      i = skipWsAndComments(src, i)
      if (src[i] === '}') {
        i++
        break
      }
      const identStart = i
      while (i < src.length && /[a-z]/.test(src[i])) i++
      const loc = src.slice(identStart, i)
      i = skipWsAndComments(src, i)
      if (src[i] !== ':') throw new Error(`expected colon after locale ${loc} in ${key.value}`)
      i = skipWsAndComments(src, i + 1)
      const val = parseQuoted(src, i)
      msg[loc] = val.value
      i = skipWsAndComments(src, val.next)
      if (src[i] === ',') i++
    }
    entries.push({ key: key.value, msg })
    i = skipWsAndComments(src, i)
    if (src[i] === ',') i++
  }
  return entries
}

function walkTsFiles(dir, acc = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) walkTsFiles(p, acc)
    else if (name.endsWith('.ts')) acc.push(p)
  }
  return acc
}

/**
 * Dump rows as `key\tlocale\tjson(string)` sorted by key then locale.
 * Auto-detects pre-merge locale files vs post-merge messages/.
 */
export function dumpRows() {
  const messagesDir = join(I18N_DIR, 'messages')
  const rows = []
  const messageFiles = existsSync(messagesDir) ? walkTsFiles(messagesDir) : []
  if (messageFiles.length > 0) {
    const files = messageFiles
    for (const f of files) {
      const src = readFileSync(f, 'utf8')
      if (!/export const \w+ = \{/.test(src)) continue
      for (const { key, msg } of extractMessageEntries(src)) {
        for (const loc of LOCALES) {
          if (typeof msg[loc] !== 'string') {
            throw new Error(`${f} ${key} missing ${loc}`)
          }
          rows.push(`${key}\t${loc}\t${JSON.stringify(msg[loc])}`)
        }
      }
    }
  } else {
    for (const loc of LOCALES) {
      const src = readFileSync(join(I18N_DIR, `${loc}.ts`), 'utf8')
      for (const item of extractLocaleEntries(src)) {
        if (item.kind !== 'entry') continue
        rows.push(`${item.key}\t${loc}\t${JSON.stringify(item.value)}`)
      }
    }
  }
  rows.sort()
  return rows
}
