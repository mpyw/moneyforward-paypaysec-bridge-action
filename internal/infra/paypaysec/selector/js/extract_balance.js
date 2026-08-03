/**
 * Read every amount on the page as raw text. No parsing happens here.
 *
 * Amounts are rendered as nested elements —
 * <span id="…">34<span>万</span>5678<span>円</span></span> — so innerText puts
 * newlines between the parts and a naive text pattern reads either nothing or,
 * worse, some other "0円" elsewhere on the page. textContent of the specific
 * element is what works.
 *
 * Turning these strings into numbers is deliberately left to Go (ParseYen),
 * where the 万 and signed forms can be handled strictly and unit-tested.
 *
 * This file is driven by pagescan/script_test.go, which loads a fixture page
 * into a real browser and checks what Go decodes. Change a key here and that
 * test says so.
 *
 * Three sources are reported, because a single one has no way to be caught
 * being wrong:
 *
 *   1. The 評価額合計 element. Primary, and only a cross-check now that the
 *      per-holding figures are what actually gets recorded.
 *   2. 投資元本 and 含み益, which must sum to it by definition. Note that
 *      投資元本 alone is the cost basis, not the market value.
 *   3. The per-holding rows. These carry the 銘柄 names, which is what each
 *      balance is ultimately recorded under.
 *
 * @param {{total:string, acquisition:string, gain:string, heading:string,
 *           headingText:string, container:string, row:string, name:string,
 *           invest:string, gain_cell:string}} selectors
 * @returns {object} see Reading in balance.go
 */
(selectors) => {
  // Collapse internal whitespace rather than stripping it: amounts are split
  // across child elements, and the 投資信託 template pads its cells
  // (" 345,678円 "), but names have meaningful spaces inside them.
  const text = (el) => (el ? (el.textContent || "").replace(/\s+/g, " ").trim() : "");

  // Amount cells have no meaningful internal spacing, and "33 万 9780 円" has to
  // come back as one token.
  const amount = (el) => text(el).replace(/\s+/g, "");

  const read = (selector) => {
    const el = document.querySelector(selector);
    return { present: !!el, raw: amount(el) };
  };

  const total = read(selectors.total);
  const acquisition = read(selectors.acquisition);
  const gain = read(selectors.gain);

  // Read row by row rather than by pairing two flat querySelectorAll lists by
  // index: a row missing its gain cell would shift every later pairing, quietly
  // attaching one holding's profit to the next holding's name.
  //
  // Every row is returned, never a sample — a truncated list would understate
  // an account with many holdings while looking perfectly healthy.
  // Scope to the 保有銘柄 section. The same row class marks up the site's whole
  // tradeable-brand catalogue, so an unscoped query walks hundreds of brands the
  // account does not hold.
  const holdingsRoot = () => {
    const heading = Array.from(document.querySelectorAll(selectors.heading))
      .find((el) => (el.textContent || "").trim() === selectors.headingText);
    if (!heading) return null;
    let node = heading.nextElementSibling;
    while (node && !node.matches(selectors.container)) node = node.nextElementSibling;
    return node;
  };

  // Whether the section exists at all, separately from whether it has rows.
  //
  // "This category holds nothing" and "this page did not render its holdings"
  // both arrive as an empty list, and the second one, paired with a zero total,
  // agrees with every cross-check there is. Nothing downstream can tell them
  // apart without this.
  const root = holdingsRoot();
  const rows = root ? Array.from(root.querySelectorAll(selectors.row)) : [];
  const holdings = rows.map((row) => {
    const link = row.querySelector("a");
    // The 株 template puts the name in a sibling div; 投資信託 uses a <p> inside
    // the anchor and repeats it in the title attribute.
    const named = text(row.querySelector(selectors.name));
    return {
      name: named || (link ? (link.getAttribute("title") || "").trim() : ""),
      ref: link ? link.getAttribute("href") || "" : "",
      investText: amount(row.querySelector(selectors.invest)),
      gainText: amount(row.querySelector(selectors.gain_cell)),
    };
  });

  return {
    totalPresent: total.present,
    totalRaw: total.raw,
    acquisitionPresent: acquisition.present,
    acquisitionRaw: acquisition.raw,
    gainPresent: gain.present,
    gainRaw: gain.raw,
    holdingsSection: !!root,
    holdings,
  };
}
