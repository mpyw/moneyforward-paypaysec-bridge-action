# Repository instructions

This GitHub Action reads configured financial sources and syncs each to its own MoneyForward manual asset account. MoneyForward login is required; each source is optional, but a run with none configured is an error. Read [implementation notes](design/implementation.md) for the source flows and their observed failure cases, and [SECURITY.md](SECURITY.md) before touching credentials, browser state, logs, or release steps.

Never put real account balances, security codes, credentials, or personal holdings into source, tests, documentation, or logs. Use synthetic values. Use the relevant skill under `.agents/skills/` for verification, release, or scraping work; the `verify` skill describes the push gate. Preserve source-specific account routing and the order of masking versus logging.
