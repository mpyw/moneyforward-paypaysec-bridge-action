/**
 * Report which tab is currently active, without touching anything.
 *
 * Used to confirm a click actually took effect. The tabs are client-side, so
 * the click returns before Vue has re-rendered; asking again afterwards is the
 * only way to know the intended bucket is the one on screen.
 *
 * @param {{container: string, activeClass: string}} params
 * @returns {{active:string, available:string[]}}
 */
(params) => {
  const items = Array.from(document.querySelectorAll(`${params.container} li`));
  const labelOf = (li) => (li.innerText || "").trim();
  const on = items.find((li) => li.classList.contains(params.activeClass));
  return { active: on ? labelOf(on) : "", available: items.map(labelOf) };
}
