import { describe, expect, test } from 'vitest'
import {
  adfCommandBlocks,
  COMMAND_MAX_BYTES,
  fencedCommandBlocks,
  isRunnableCommand,
  issueCommandBlocks,
} from './issue-commands'
import type { AdfNode } from './types'

/*
 * GDK-1162. The parser decides what gets a ▶, so its edges are the feature's
 * edges: a body that is mostly prose must not sprout buttons, and a block the
 * serve would refuse must not offer one.
 */

describe('fenced blocks', () => {
  test('several blocks, in document order, with their language tags', () => {
    const body = [
      'Reproduce it:',
      '',
      '```sh',
      'go test ./... -count=1',
      '```',
      '',
      'then',
      '',
      '```',
      'make typecheck',
      '```',
    ].join('\n')
    expect(fencedCommandBlocks(body)).toEqual([
      { text: 'go test ./... -count=1', lang: 'sh', runnable: true },
      { text: 'make typecheck', lang: '', runnable: true },
    ])
  })

  test('a tilde fence is a fence, and a backtick run does not close it', () => {
    const body = ['~~~bash', 'ls -la', '```', '~~~'].join('\n')
    expect(fencedCommandBlocks(body)).toEqual([
      { text: 'ls -la\n```', lang: 'bash', runnable: false },
    ])
  })

  test('a longer fence quotes a shorter one instead of being closed by it', () => {
    const body = ['````md', '```sh', 'rm -rf /', '```', '````'].join('\n')
    expect(fencedCommandBlocks(body)).toEqual([
      { text: '```sh\nrm -rf /\n```', lang: 'md', runnable: false },
    ])
  })

  test('an unterminated fence runs to the end rather than eating the document', () => {
    expect(fencedCommandBlocks('intro\n\n```\ngadak sync\n')).toEqual([
      { text: 'gadak sync', lang: '', runnable: true },
    ])
  })

  test('inline code and stray backticks are not fences', () => {
    expect(fencedCommandBlocks('use ``a`b`` and `ls` here')).toEqual([])
    expect(fencedCommandBlocks('no fences at all')).toEqual([])
    expect(fencedCommandBlocks('')).toEqual([])
    expect(fencedCommandBlocks(null)).toEqual([])
  })

  test('a closing fence with an info string does not close', () => {
    const body = ['```sh', 'echo one', '``` sh', 'echo two', '```'].join('\n')
    expect(fencedCommandBlocks(body)).toEqual([
      { text: 'echo one\n``` sh\necho two', lang: 'sh', runnable: false },
    ])
  })
})

describe('runnable', () => {
  test('one non-empty line is runnable; more than one is not', () => {
    expect(isRunnableCommand('go build ./...')).toBe(true)
    expect(isRunnableCommand('go build ./...\n')).toBe(true)
    expect(isRunnableCommand('cd web\nnpm run build')).toBe(false)
    expect(isRunnableCommand('  ')).toBe(false)
    expect(isRunnableCommand('')).toBe(false)
  })

  test('a lone carriage return is refused — on a terminal that byte is Enter', () => {
    expect(isRunnableCommand('echo hi\rls')).toBe(false)
  })

  test('the cap is bytes, not UTF-16 units — the serve counts bytes', () => {
    expect(isRunnableCommand('x'.repeat(COMMAND_MAX_BYTES))).toBe(true)
    expect(isRunnableCommand('x'.repeat(COMMAND_MAX_BYTES + 1))).toBe(false)
    // Three bytes a character: half the cap in characters is over it in bytes.
    expect(isRunnableCommand('가'.repeat(COMMAND_MAX_BYTES / 2))).toBe(false)
  })
})

describe('ADF blocks', () => {
  const doc = (...content: AdfNode[]): AdfNode => ({ type: 'doc', content })
  const code = (text: string, language?: string): AdfNode => ({
    type: 'codeBlock',
    ...(language ? { attrs: { language } } : {}),
    content: [{ type: 'text', text }],
  })

  test('code blocks anywhere in the tree, in document order', () => {
    const node = doc(
      { type: 'paragraph', content: [{ type: 'text', text: 'run this' }] },
      code('gadak sync', 'shell'),
      {
        type: 'panel',
        attrs: { panelType: 'info' },
        content: [code('gadak doctor')],
      },
    )
    expect(adfCommandBlocks(node)).toEqual([
      { text: 'gadak sync', lang: 'shell', runnable: true },
      { text: 'gadak doctor', lang: '', runnable: true },
    ])
  })

  test('no code blocks, and no node at all', () => {
    expect(adfCommandBlocks(doc({ type: 'paragraph' }))).toEqual([])
    expect(adfCommandBlocks(null)).toEqual([])
  })

  test('ADF wins over the flattened text of the same document', () => {
    // description_text is the flatten of description_adf, so reading both
    // would count one block twice.
    const node = doc(code('go vet ./...', 'sh'))
    expect(issueCommandBlocks(node, '```sh\ngo vet ./...\n```')).toEqual([
      { text: 'go vet ./...', lang: 'sh', runnable: true },
    ])
  })

  test('markdown bodies (Linear leaves description_adf empty) reach the fence parser', () => {
    expect(issueCommandBlocks(null, '```\ngadak serve\n```')).toEqual([
      { text: 'gadak serve', lang: '', runnable: true },
    ])
    // An ADF doc with real content and no code block is not a markdown body:
    // its flattened text must not be re-scanned for fences that are prose.
    const prose = doc({ type: 'paragraph', content: [{ type: 'text', text: '```\nls\n```' }] })
    expect(issueCommandBlocks(prose, '```\nls\n```')).toEqual([])
  })
})
