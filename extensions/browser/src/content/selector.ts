// Deterministic CSS-selector path — no heuristic guessing about which
// element is meant, but two things keep it human-readable:
//  1. Generated class names (CSS Modules "_cover_1gxhc_2", webpack hashed
//     suffixes "flex-module__f__rtWJH", styled-components/emotion "sc-xxxx")
//     are filtered out of each segment — they're noise, not identifiers.
//  2. An id or a stable test attribute (data-testid/data-test/data-cy/
//     data-qa) on the element itself or any ancestor stops the walk early —
//     no need to keep climbing once there's a real anchor.
const STABLE_TEST_ATTRS = ["data-testid", "data-test", "data-cy", "data-qa"];

function isGeneratedClass(cls: string): boolean {
  // CSS Modules: _name_hash(_index)?  e.g. "_cover_1gxhc_2"
  if (/^_[a-zA-Z][\w-]*_[a-z0-9]{4,8}(_\d+)?$/.test(cls)) return true;
  // webpack/CSS-modules hashed suffix: "flex-module__f__rtWJH"
  if (/__[A-Za-z0-9]{4,10}$/.test(cls)) return true;
  // styled-components / emotion generated: "sc-bZQynM", "css-1a2b3c"
  if (/^(sc|css)-[a-z0-9]{5,}$/i.test(cls)) return true;
  return false;
}

function stableAttrSelector(el: Element): string | null {
  for (const attr of STABLE_TEST_ATTRS) {
    const value = el.getAttribute(attr);
    if (value) return `[${attr}="${CSS.escape(value)}"]`;
  }
  return null;
}

export function cssSelector(el: Element): string {
  const path: string[] = [];
  let node: Element | null = el;

  while (node && node.nodeType === 1 && path.length < 8) {
    if (node.id) {
      path.unshift("#" + CSS.escape(node.id));
      break;
    }

    const attr = stableAttrSelector(node);
    if (attr) {
      path.unshift(node.tagName.toLowerCase() + attr);
      break;
    }

    let part = node.tagName.toLowerCase();
    const meaningfulClasses = Array.from(node.classList).filter((c) => !isGeneratedClass(c));
    if (meaningfulClasses.length) {
      part += "." + meaningfulClasses.map((c) => CSS.escape(c)).join(".");
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
