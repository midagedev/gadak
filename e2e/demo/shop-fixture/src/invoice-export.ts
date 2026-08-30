const PREVIEW_COLUMNS = ['date', 'customer', 'net', 'tax', 'gross']
const EXPORT_COLUMNS = ['date', 'customer', 'net', 'gross']

export function previewRows(rows: Row[]) {
  return rows.map((r) => PREVIEW_COLUMNS.map((c) => r[c]))
}

export function exportRows(rows: Row[]) {
  return rows.map((r) => EXPORT_COLUMNS.map((c) => r[c]))
}
