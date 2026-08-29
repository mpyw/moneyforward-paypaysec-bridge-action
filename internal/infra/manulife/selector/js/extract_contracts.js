/**
 * Read the contract cards from the マイページ top page as raw text.
 *
 * The list is the one part of this site that can be addressed by class name, so
 * this is a straightforward walk. What it is not allowed to do is decide
 * anything: which contract to open, and whether the one that opened is the one
 * that was asked for, are judgements about money and belong in Go.
 *
 * The card's own onclick carries the token the detail page is reached by, and
 * it is deliberately not returned. It is a continuation minted for this
 * rendering, so a caller that held one would be holding something that expires
 * without saying so — clicking the card is the only correct way in.
 *
 * Labels come back exactly as written. This site punctuates the same label
 * three different ways across two pages, so normalising here would hide from Go
 * the one thing it has to be careful about; TrimLabel does that, once, where it
 * can be tested.
 *
 * @param {{card:string, title:string, table:string}} selectors
 * @returns {{cards: Array<{title:string, fields:Array<{label:string,value:string}>}>}}
 */
function (selectors) {
  const text = (el) => (el ? (el.textContent || '').replace(/\s+/g, ' ').trim() : '');

  // A card the browser is not showing is not on offer. The site renders the
  // navigation twice — once for wide screens and once for narrow — so
  // "present in the markup" and "on the page" are different questions here.
  const shown = (el) => !!(el && el.getClientRects().length);

  const cards = [];
  document.querySelectorAll(selectors.card).forEach((card) => {
    if (!shown(card)) {
      return;
    }
    const fields = [];
    const table = card.querySelector(selectors.table);
    if (table) {
      table.querySelectorAll('tr').forEach((row) => {
        const label = row.querySelector('th');
        const value = row.querySelector('td');
        if (label && value) {
          fields.push({ label: text(label), value: text(value) });
        }
      });
    }
    cards.push({ title: text(card.querySelector(selectors.title)), fields: fields });
  });
  return { cards: cards };
}
