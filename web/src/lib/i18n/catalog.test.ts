import { describe, expect, test } from 'vitest'
import { en } from './en'
import { ko } from './ko'

describe('catalog contracts', () => {
  test('empty-project label says every project (en/ko catalogs)', () => {
    expect(en['settings.sourcesNoProjects']).toMatch(/every project/)
    expect(ko['settings.sourcesNoProjects']).toContain('모든 프로젝트')
  })
})
