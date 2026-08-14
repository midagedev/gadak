/*
 * Omnibox + body-link routing. One owner so a /browse/KEY paste and a body
 * click cannot drift (spec 001: nothing silently swallowed; modeled links
 * open native panels).
 *
 * classifyOmnibox is sync so SearchBox can preventDefault on paste before
 * the clipboard text lands in the input.
 */

import { extractJql, looksLikeJql } from './jql'
import {
  classifyAtlassianLink,
  extractBrowseKey,
  extractWikiPageId,
  openContainedUrl,
} from './desktop-links'
import { config } from './config'
import { t } from './i18n'
import { issues } from '../stores/issues.svelte'
import { pages } from '../stores/pages.svelte'
import { selection } from '../stores/selection.svelte'
import { views } from '../stores/views.svelte'
import { write } from '../stores/write.svelte'
import { showIssueList } from './show-issue-list'
import type { ViewConfig } from './view-config'

export type OmniboxAction =
  | { kind: 'issue'; key: string }
  | { kind: 'issue-miss'; key: string }
  | { kind: 'page'; key: string }
  | { kind: 'filter'; id: string; input: string }
  | { kind: 'jql'; input: string }
  | { kind: 'contained'; url: string }
  | { kind: 'text' }

function sameSite(href: string): boolean {
  return classifyAtlassianLink(href, config().jiraBaseUrl || null).inApp
}

function applySourceConfig(config: ViewConfig, unsupported?: string[]): void {
  showIssueList(config)
  if (unsupported?.length) {
    write.toast(t('filter.jqlPartial', { clauses: unsupported.join('; ') }), 'info')
  }
}

/** Decide what a pasted / entered string is. No I/O, no store writes. */
export function classifyOmnibox(raw: string): OmniboxAction {
  const text = raw.trim()
  if (!text) return { kind: 'text' }

  const issueKey = extractBrowseKey(text)
  if (issueKey) {
    return issues.pool.has(issueKey)
      ? { kind: 'issue', key: issueKey }
      : { kind: 'issue-miss', key: issueKey }
  }

  const pageId = extractWikiPageId(text)
  if (pageId && pages.byKey.has(pageId)) {
    return { kind: 'page', key: pageId }
  }

  const extracted = extractJql(text)
  if (extracted.filterId && !extracted.jql) {
    return { kind: 'filter', id: extracted.filterId, input: text }
  }
  if (extracted.jql && (extracted.isUrl || looksLikeJql(text))) {
    return { kind: 'jql', input: text }
  }
  if (looksLikeJql(text) && !extracted.isUrl) {
    return { kind: 'jql', input: text }
  }

  if (pageId && sameSite(text)) return { kind: 'contained', url: text }
  if (extracted.isUrl && sameSite(text)) return { kind: 'contained', url: text }
  return { kind: 'text' }
}

/** Modeled /browse/KEY or mirrored wiki URL → native panel. False = leave it. */
export function tryOpenNativeLink(href: string): boolean {
  const issueKey = extractBrowseKey(href)
  if (issueKey) {
    if (issues.pool.has(issueKey)) {
      selection.select(issueKey)
      return true
    }
    return false
  }
  const pageId = extractWikiPageId(href)
  if (pageId && pages.byKey.has(pageId)) {
    pages.select(pageId)
    return true
  }
  return false
}

/**
 * Apply a classified omnibox action. True = handled (caller preventDefault /
 * skip FTS). False = ordinary text.
 */
export async function applyOmniboxAction(
  action: OmniboxAction,
  applyJql: (input: string) => Promise<boolean>,
): Promise<boolean> {
  switch (action.kind) {
    case 'text':
      return false
    case 'issue':
      selection.select(action.key)
      return true
    case 'issue-miss':
      write.toast(t('omnibox.issueMissing', { key: action.key }), 'info')
      return true
    case 'page':
      pages.select(action.key)
      return true
    case 'filter': {
      const src = views.source.find((v) => v.external_id === action.id)
      if (src) {
        applySourceConfig(src.config, src.unsupported)
        return true
      }
      await applyJql(action.input)
      return true
    }
    case 'jql':
      await applyJql(action.input)
      return true
    case 'contained':
      openContainedUrl(action.url)
      return true
  }
}

export async function handleOmniboxInput(
  raw: string,
  applyJql: (input: string) => Promise<boolean>,
): Promise<boolean> {
  return applyOmniboxAction(classifyOmnibox(raw), applyJql)
}
