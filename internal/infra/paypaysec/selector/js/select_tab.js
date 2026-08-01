/**
 * Click the tab whose label matches, and report what happened.
 *
 * The 投資信託 tabs are Vue-rendered `<li><a>label</a></li>` with no id, role,
 * href or data attribute — the label text is the only stable handle. Matching on
 * text rather than nth-child means reordering the tabs cannot silently start
 * reading the wrong bucket, which is the failure that matters here: it would
 * produce a plausible number attributed to the wrong side.
 *
 * @param {{container: string, label: string, activeClass: string}} params
 * @returns {{clicked:boolean, active:string, available:string[]}}
 */
(params) => {
  const items = Array.from(document.querySelectorAll(`${params.container} li`));
  const labelOf = (li) => (li.innerText || "").trim();
  const activeLabel = () => {
    const on = items.find((li) => li.classList.contains(params.activeClass));
    return on ? labelOf(on) : "";
  };

  const available = items.map(labelOf);
  const hit = items.find((li) => labelOf(li) === params.label);
  if (!hit) return { clicked: false, active: activeLabel(), available };

  (hit.querySelector("a") || hit).click();
  return { clicked: true, active: activeLabel(), available };
}
