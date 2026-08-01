/**
 * List the tab-like and button-like controls on the page, each with a
 * best-effort CSS selector.
 *
 * Deliberately broad: this is used when a selector constant is still a guess,
 * so over-reporting beats missing the one control that matters.
 *
 * @returns {Array<{selector:string,tag:string,role:string,text:string,visible:boolean}>}
 */
() => {
  const QUERY = '[role="tab"], button, a[href="#"], a[href^="javascript"], [class*="tab" i], [id*="tab" i]';
  const MAX = 80;

  const selectorFor = (el) => {
    if (el.id) return "#" + CSS.escape(el.id);
    for (const attr of ["data-testid", "name", "aria-controls", "role"]) {
      const v = el.getAttribute(attr);
      if (v) return `${el.tagName.toLowerCase()}[${attr}="${v}"]`;
    }
    const cls = typeof el.className === "string" && el.className.trim()
      ? el.className.trim().split(/\s+/).slice(0, 2).map((c) => "." + CSS.escape(c)).join("")
      : "";
    return el.tagName.toLowerCase() + cls;
  };

  const seen = new Set();
  const out = [];
  for (const el of document.querySelectorAll(QUERY)) {
    const selector = selectorFor(el);
    const text = (el.innerText || el.value || "").trim().replace(/\s+/g, " ").slice(0, 60);
    const key = `${selector}|${text}`;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push({
      selector,
      tag: el.tagName.toLowerCase(),
      role: el.getAttribute("role") || "",
      text,
      visible: el.getClientRects().length > 0,
    });
    if (out.length >= MAX) break;
  }
  return out;
}
