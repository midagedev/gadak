// Keyboard-aware bottom bar (DESIGN.md §4.2). In WKWebView the software
// keyboard overlays the layout viewport — the page does not resize — so a
// bottom-fixed composer disappears behind it. The VisualViewport API is the
// one honest measurement of the obscured band; this action translates the
// node up by exactly that band while the keyboard is up.
//
// Svelte action: <div use:keyboardInset>.

export function keyboardInset(node: HTMLElement) {
  const vv = window.visualViewport
  if (!vv) return
  const update = () => {
    const inset = Math.max(0, window.innerHeight - vv.height - vv.offsetTop)
    node.style.transform = inset > 0 ? `translateY(-${inset}px)` : ''
  }
  vv.addEventListener('resize', update)
  vv.addEventListener('scroll', update)
  update()
  return {
    destroy() {
      vv.removeEventListener('resize', update)
      vv.removeEventListener('scroll', update)
      node.style.transform = ''
    },
  }
}
