/**
 * Return the name of the first candidate whose selector matches a visible
 * element, or "" when none of them do.
 *
 * Visibility is getClientRects().length > 0 rather than a style check: it is
 * what actually decides whether chromedp can click the thing.
 *
 * @param {Object<string,string>} candidates name -> CSS selector
 * @returns {string}
 */
(candidates) => {
  for (const name of Object.keys(candidates)) {
    const el = document.querySelector(candidates[name]);
    if (el && el.getClientRects().length > 0) return name;
  }
  return "";
}
