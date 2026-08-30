export function mountCommentBox(host: HTMLElement) {
  const box = document.createElement('textarea')
  host.append(box)
  box.focus() // called before the element is laid out
  return box
}
