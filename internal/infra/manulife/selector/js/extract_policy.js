/**
 * Read a contract's detail page as raw text: the summary block, and every
 * labelled row.
 *
 * Nothing is parsed and nothing is chosen. Go decides which labels it wants,
 * and refuses when a label it needs turns up more than once — which is the
 * whole reason every match is returned rather than the first.
 *
 * Two things this file exists to get right:
 *
 * 1. Only what is visible. The page carries sections for products the customer
 *    does not hold — a 変額保険 block was present on a contract that is not one,
 *    with a zero-balance and a non-zero-balance panel both display:none. Their
 *    figures parse perfectly. getClientRects() is used rather than a style
 *    check because it accounts for every way an ancestor can hide a subtree,
 *    and a hand-rolled walk up the tree got that wrong on this very page.
 *
 * 2. No element ids. Salesforce Visualforce numbers them by position in a
 *    component tree — j_id0:j_id2:j_id257:0:… — and the indices move with the
 *    contents. Rows are found by their label's text instead.
 *
 * @param {{summary:string, summaryRow:string, valueMarker:string, valueText:string}} selectors
 * @returns {{summary: Array<{label:string,value:string}>,
 *            rows: Array<{label:string,value:string}>}}
 */
function (selectors) {
  const text = (el) => (el ? (el.textContent || '').replace(/\s+/g, ' ').trim() : '');
  const shown = (el) => !!(el && el.getClientRects().length);

  const summary = [];
  document.querySelectorAll(selectors.summary).forEach((box) => {
    if (!shown(box)) {
      return;
    }
    box.querySelectorAll(selectors.summaryRow).forEach((row) => {
      if (!shown(row)) {
        return;
      }
      const value = row.querySelector(selectors.valueMarker);
      if (!value) {
        return;
      }
      // The label is the row's other child. Taking the row's whole text and
      // subtracting the value would depend on their order in the markup.
      let label = null;
      for (const child of row.children) {
        if (child !== value && !child.contains(value)) {
          label = child;
          break;
        }
      }
      summary.push({ label: text(label), value: text(value) });
    });
  });

  const rows = [];
  document.querySelectorAll('tr').forEach((row) => {
    if (!shown(row)) {
      return;
    }
    const label = row.querySelector('th');
    const cell = row.querySelector('td');
    if (!label || !cell) {
      return;
    }
    // The figure sits in its own span where the page has one. Falling back to
    // the cell is what makes the labelled rows outside the money tables — the
    // 【基本情報】 block, where 保険種類 lives — readable by the same walk.
    const value = cell.querySelector(selectors.valueText) || cell;
    if (!shown(value)) {
      return;
    }
    rows.push({ label: text(label), value: text(value) });
  });

  return { summary: summary, rows: rows };
}
