/**
 * Find the contract whose 種類-証券番号 is the one asked for, and click it.
 *
 * The click is the page's own — element.click(), which invokes the card's
 * onclick and lets it set location.href. Two other ways were tried against the
 * live site and both failed silently:
 *
 *   - chromedp's Click aims a mouse event at the element's centre. It reported
 *     success and the page did not move; this site layers a Salesforce
 *     embedded-service widget over its pages, and a coordinate can land on an
 *     overlay.
 *   - Scheduling the click on a timeout, so the evaluation could return before
 *     the navigation, did not fire the handler either.
 *
 * What works is this, and it is what the first successful read used. The
 * theoretical hazard — an evaluation waiting to return from an execution
 * context a navigation has destroyed — has not been observed here, and the
 * budget around the call now reports itself if it ever is.
 *
 * Finding and clicking stay together, and have to: the list is re-rendered on
 * every visit and each rendering mints a fresh token for the card's onclick, so
 * a click aimed at a card found a moment earlier could open a different
 * contract.
 *
 * The mark is left on the card that was clicked. Nothing needs it; it makes a
 * page dump say which card this was aimed at, which is the question every
 * failure here has started from.
 *
 * Matching is on the number's text rather than on its label. The label is
 * punctuated differently on different pages here, and the rule for coping with
 * that lives in Go where it is tested; asking this file to know it as well
 * would be two statements of one rule.
 *
 * Returns how many cards matched. Anything but one is the caller's to refuse:
 * none means the contract has left the list, and more than one means the number
 * does not identify a contract after all.
 *
 * @param {{card:string, table:string, mark:string}} selectors
 * @param {string} number the 種類-証券番号 to mark
 * @returns {{matched:number}}
 */
function (selectors, number) {
  const normalise = (s) => (s || '').replace(/\s+/g, ' ').trim();
  const text = (el) => normalise(el ? el.textContent : '');
  const shown = (el) => !!(el && el.getClientRects().length);

  // Any mark left by an earlier attempt, so a stale one cannot be clicked.
  document.querySelectorAll('[' + selectors.mark + ']').forEach((el) => {
    el.removeAttribute(selectors.mark);
  });

  const wanted = normalise(number);
  const hits = [];
  document.querySelectorAll(selectors.card).forEach((card) => {
    if (!shown(card)) {
      return;
    }
    const table = card.querySelector(selectors.table);
    if (!table) {
      return;
    }
    for (const cell of table.querySelectorAll('td')) {
      if (text(cell) === wanted) {
        hits.push(card);
        return;
      }
    }
  });

  if (hits.length === 1) {
    hits[0].setAttribute(selectors.mark, '');
    hits[0].click();
  }
  return { matched: hits.length };
}
