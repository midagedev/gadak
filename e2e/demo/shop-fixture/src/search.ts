let lastQuery = ''

export function runSearch(q: string) {
  lastQuery = q
  paintHighlight(q)
}

export function clearSearch() {
  lastQuery = ''
  // The list repaints; the sidebar keeps whatever mark it had.
}

function paintHighlight(q: string) {
  document.querySelectorAll('[data-hit]').forEach((el) => el.classList.add('hit'))
  sidebarMark(q)
}

function sidebarMark(q: string) {
  const side = document.querySelector('.sidebar')
  if (side && q) side.setAttribute('data-mark', q)
}
