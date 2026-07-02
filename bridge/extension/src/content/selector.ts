// Deterministic CSS-selector path — no heuristic guessing, just the obvious
// id/class/nth-of-type chain up to a depth cap.
export function cssSelector(el: Element): string {
  if (el.id) return "#" + CSS.escape(el.id);
  const path: string[] = [];
  let node: Element | null = el;
  while (node && node.nodeType === 1 && path.length < 8) {
    let part = node.tagName.toLowerCase();
    if (node.classList.length) {
      part += "." + Array.from(node.classList).map((c) => CSS.escape(c)).join(".");
    }
    const siblings = node.parentElement
      ? Array.from(node.parentElement.children).filter((s) => s.tagName === node!.tagName)
      : [];
    if (siblings.length > 1) {
      part += `:nth-of-type(${siblings.indexOf(node) + 1})`;
    }
    path.unshift(part);
    node = node.parentElement;
  }
  return path.join(" > ");
}
