/**
 * Watch the figures for the repaint that follows a tab switch.
 *
 * Switching the 投資信託 tab is client-side: there is no navigation to wait on,
 * and the numbers for the newly selected bucket arrive about a second later.
 * Until they do, the page still shows the previous tab's numbers — which are
 * stable, and carry no loading overlay, so a wait that only asks "has the value
 * stopped changing?" is satisfied immediately by the wrong bucket's data.
 *
 * Nothing in the DOM distinguishes the two states. Measured against the live
 * page, both the tab's own `actived` class and the nav's bucket markers flip
 * 8ms after the click, while the figures change at ~1000ms. Anything derived
 * from the click is therefore useless as a signal that the data followed it.
 *
 * What does distinguish them is that the swap rewrites this part of the
 * document. A MutationObserver sees that directly: install before the click,
 * then wait for mutations to start and then stop.
 *
 * Two calls: `install` before the click, `poll` after.
 *
 * @param {{roots: string[], mode: string}} args
 * @returns {{installed:boolean}|{mutations:number, quietMs:number, watching:boolean}}
 */
(args) => {
  const KEY = "__mfppRepaintWatch";

  if (args.mode === "install") {
    const prev = window[KEY];
    if (prev && prev.observer) prev.observer.disconnect();

    const state = { mutations: 0, last: 0, observer: null };
    const observer = new MutationObserver((records) => {
      state.mutations += records.length;
      state.last = Date.now();
    });

    let watched = 0;
    for (const sel of args.roots) {
      for (const root of document.querySelectorAll(sel)) {
        observer.observe(root, {
          childList: true,
          subtree: true,
          characterData: true,
        });
        watched++;
      }
    }
    // Nothing to watch means the read that follows cannot be guarded this way.
    // Reported rather than papered over, so the caller decides.
    if (watched === 0) return { installed: false };

    state.observer = observer;
    state.start = Date.now();
    window[KEY] = state;
    return { installed: true };
  }

  const state = window[KEY];
  if (!state || !state.observer) return { mutations: 0, quietMs: 0, watching: false };

  return {
    mutations: state.mutations,
    // How long the document has been still. Measured from the last mutation, or
    // from installation when there has not been one — so a caller waiting for
    // quiet cannot mistake "never started" for "finished".
    quietMs: Date.now() - (state.last || state.start),
    watching: true,
  };
}
