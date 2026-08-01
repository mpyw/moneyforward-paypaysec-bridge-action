/**
 * Read one 銘柄's own figures from its detail page.
 *
 * The holdings list is not enough for this: it rounds the profit figure on the
 * 株 template ("+3.7万"), so an acquisition cost derived from it is out by up to
 * several hundred yen. The detail page carries the acquisition amount directly,
 * and unrounded.
 *
 * @param {{value:string, acquisition:string, gain:string}} selectors
 * @returns {{valueRaw:string, acquisitionRaw:string, gainRaw:string,
 *            valuePresent:boolean, acquisitionPresent:boolean}}
 */
(selectors) => {
  const amount = (el) => (el ? (el.textContent || "").replace(/\s+/g, "").trim() : "");
  const read = (sel) => {
    const el = document.querySelector(sel);
    return { present: !!el, raw: amount(el) };
  };

  const value = read(selectors.value);
  const acquisition = read(selectors.acquisition);
  const gain = read(selectors.gain);

  return {
    valuePresent: value.present,
    valueRaw: value.raw,
    acquisitionPresent: acquisition.present,
    acquisitionRaw: acquisition.raw,
    gainRaw: gain.raw,
  };
}
