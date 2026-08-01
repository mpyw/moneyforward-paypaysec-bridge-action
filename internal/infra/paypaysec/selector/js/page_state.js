/**
 * Report whether the page has finished loading its figures.
 *
 * The 投資信託 view is a Vue app that fetches its numbers after the document is
 * ready, and it renders 0円 in the meantime. That placeholder parses perfectly
 * well, so reading too early yields a confident, wrong zero — which is exactly
 * what happened before this check existed.
 *
 * @param {{loading: string, total: string}} selectors
 * @returns {{loading:boolean, present:boolean, text:string}}
 */
(selectors) => {
  const overlay = document.querySelector(selectors.loading);
  const loading = !!overlay && overlay.getClientRects().length > 0;

  const el = document.querySelector(selectors.total);
  return {
    loading,
    present: !!el,
    text: el ? (el.textContent || "").trim().replace(/\s+/g, "") : "",
  };
}
