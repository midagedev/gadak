#!/usr/bin/env node
/**
 * Dump every catalog (key, locale, string) as sorted TSV on stdout.
 * Used to prove a catalog structure migration did not change copy.
 *
 *   node web/scripts/dump-i18n-catalog.mjs > /tmp/i18n-dump.tsv
 */
import { dumpRows } from './i18n-catalog-parse.mjs'

const rows = dumpRows()
process.stdout.write(rows.join('\n') + '\n')
process.stderr.write(`${rows.length} rows\n`)
