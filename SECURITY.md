# Security model

このリポジトリは個人ユース自動化だが、いつでも public 化できることを設計目標にしている。下記の脅威モデル / secret 取り扱い / 漏洩時手順をベースに監査する。

## ローカル開発時の資材 (`.debug/`)

`mfpp debug` は `.debug/` 以下に次を作る。すべて gitignore 済み、パーミッション 0600/0700。

| ファイル | 中身 | 危険度 |
|---|---|---|
| `cookies.json` | 証券口座の**生きたセッション** | 高。これ単体でログインできる |
| `profile/` | Chrome プロファイル | 中 |
| `*.html` | 認証済みページの生 HTML（残高・氏名等） | 中 |
| `otp.txt` | 手渡し OTP。読み取り後に自動削除 | 低（短命） |

**作業が終わったら `rm -rf .debug`。** 

## 守る資産

| 資産 | 漏洩した場合の影響 |
|---|---|
| PayPay 証券ログイン認証情報 | 他人が口座に直接アクセスし得る。最重要 |
| MoneyForward ログイン認証情報 | 全資産情報、生活パターンが見える。極めて重要 |
| `GMAIL_CREDENTIALS_JSON` | 受信箱を読める。OTP を含むため各サービスのパスワードリセットに連鎖し得る |
| OTP メール本文 | 単独では失効するが、複数集まると挙動分析に使われる |
| `.debug/cookies.json` | 生きたセッションそのもの。単体でログイン可能 |

## 脅威モデル

### 想定する攻撃者

1. **public 化後のサードパーティ閲覧者**: workflow run ログ、コードを read
2. **GitHub アカウントへの侵入者**: Secrets を抜くのは技術的に難しいが、workflow を
   改変して secret を出力させることは可能
3. **Gmail への侵入者**: OTP メールを読まれる → 認証フローのうちの一手を奪う
4. **開発機への侵入者**: `.envrc` と `.debug/` に平文の認証情報とセッションがある

### 緩和策

- **public repo 安全**: artifact 不使用、ログに secret を出さない、コードに個人情報なし
- **実データを置かない**: 残高・銘柄名・OTP コードは、コメントにもテストにも README にも
  実物を書かない。合成値を使う。実口座の銘柄と評価額が README とテストに入っていた
  (2026-08-01 に除去)。「ログに出さない」を徹底しても、ツリーに書いてあれば意味がない
- **secret 露出最小化**: workflow で扱う値は `::add-mask::` を明示適用。残高と OTP は
  プログラム側が読み取り時点で mask する
- **Gmail スコープ最小化**: `gmail.readonly` のみ。`gcloud auth application-default login`
  は `cloud-platform` を強制するので使わず、同意フローを自前で持つ
  (`mfpp gmail authorize`)。漏洩時の影響を「メール読み取り」に留めるため。
  資格情報の解決は secret → ローカルファイルの 2 段だけで、**ADC へのフォールバックは
  持たない**。持つと、ADC が転がっているマシンで黙って cloud-platform 資格情報を
  使ってしまい、この判断そのものが無効になる
- **サービスアカウント不使用**: そもそも個人 Gmail には使えないが、仮に使えたとしても
  ユーザー資格情報のほうが権限が狭い
- **GitHub 側の権限を持たない**: OTP を Gmail から直接読むので、PAT も Variables 書き込み
  権限も不要。workflow の permissions は `contents: read` のみ
- **OTP の鮮度検証**: メールの受信時刻がログイン送信時刻より後であることを確認する。
  前回実行時のメールが受信箱に残っていても拾わない
- **chromedp verbose ログ無効**: 本番で `log.Printf` を最小化し、エラーメッセージにも
  機密情報を入れない

### ローカル開発時の資材 (`.debug/`)

`mfpp debug` は `.debug/` 以下に次を作る。すべて gitignore 済み、パーミッション 0600/0700。

| ファイル | 中身 | 危険度 |
|---|---|---|
| `cookies.json` | 生きたセッション | 高。これ単体でログインできる |
| `profile/` | Chrome プロファイル | 中 |
| `*.html` | 認証済みページの生 HTML（残高・氏名等） | 中 |
| `otp.txt` | 手渡し OTP。読み取り後に自動削除 | 低（短命） |

**作業が終わったら `rm -rf .debug`。**

同じく gitignore 済みでローカルにのみ置くもの:

| ファイル | 中身 |
|---|---|
| `.envrc` | 各サービスの認証情報 (direnv が環境に置く) |
| `client_secret.json` | OAuth クライアント。同意フローの一度きりにしか使わない |
| `gmail-credentials.json` | Gmail のユーザー資格情報 (refresh token) |

## Secret ローテーション手順

すべて `gh secret set` 経由。値はローカルで libsodium sealed box に暗号化されてから送られる。
値を引数に書かないこと (コマンド履歴と `ps` に残る) — 対話入力か stdin を使う。

| Secret | ローテ頻度 | 手順 |
|---|---|---|
| `PAYPAY_SEC_PASSWORD` | 半年 / 漏洩疑い時即 | PayPay 証券の Web でパスワード変更 → `gh secret set PAYPAY_SEC_PASSWORD` |
| `MF_PASSWORD` | 半年 / 漏洩疑い時即 | MF でパスワード変更 → `gh secret set MF_PASSWORD` |
| `MF_ASSET_ID` | 資産削除/再作成時のみ | 新 ID をメモして `gh secret set MF_ASSET_ID` |
| `PAYPAY_SEC_USERNAME` / `MF_EMAIL` | アドレス変更時のみ | `gh secret set <name>` |

## OTP の取り扱い

GitHub Variables は使わない。OTP は Gmail API から直接読み、メールの受信時刻が
ログイン送信時刻より後であることを確認してから採用する。前回実行時のメールが受信箱に
残っていても拾わない。

`GMAIL_CREDENTIALS_JSON` は `gmail.readonly` のみのユーザー資格情報で、失効しない
refresh token を含む。実質的に受信箱への恒久的な読み取り鍵なので、パスワードと同じ
扱いをする。

「失効しない」は **OAuth 同意画面の公開ステータスが「本番環境」であること**に
依存する (2026-08-02 に設定済み)。「テスト」に戻すと refresh token は 7 日で切れ、
cron は 1 週間後に静かに死ぬ。原因が発行から 1 週間離れているので、そうと知らないと
追いにくい。

### なぜ OIDC / Workload Identity Federation にできないか

WIF が渡すのは Google Cloud の identity だが、個人 Gmail のメールボックスを読める
Google Cloud identity は存在しない。サービスアカウント + ドメイン全体の委任は
Workspace ドメイン専用で、個人アカウントには適用できない。個人の受信箱に到達する
経路はユーザー同意で得た refresh token だけなので、この secret は消せない。

スコープもこれ以上絞れない。`gmail.metadata` は本文を返さず 6 桁が読めないため、
`gmail.readonly` が機能する最小になる。

残る緩和策は **OTP 専用の Google アカウントを作り、PayPay 証券と MoneyForward の
登録アドレスをそちらに移すこと**。refresh token が漏れたときに読まれる範囲が
「全メール」から「OTP メールだけ」になる。無料で、Workspace も要らない。未実施。

## 漏洩時のリボーク手順

### PayPay / MF 認証情報が漏洩した疑い

1. PayPay 証券 / MF 上でパスワード変更 (それぞれ Web から)
2. `gh secret set <name>` で新値を Secrets に反映
3. 当該日の workflow を `gh workflow run sync.yml` で再実行して動作確認
4. Audit log で漏洩源を特定

### Gmail 資格情報が漏洩した疑い

1. https://myaccount.google.com/permissions で該当アプリの連携を取り消す
   (これで refresh token が即座に無効化される)
2. `go run ./cmd/mfpp gmail authorize` で再発行
3. `gh secret set GMAIL_CREDENTIALS_JSON < gmail-credentials.json`
4. `go run ./cmd/mfpp gmail check` で疎通確認

取り消し中は OTP が読めないので、その日の workflow は失敗して構わない。

### ローカルの `.debug/` や `.envrc` が漏洩した疑い

1. `.debug/cookies.json` は生きたセッション。各サービスでパスワードを変更すれば
   セッションも無効化される
2. `.envrc` の内容はすべてローテーション対象。上の手順に従う
3. `rm -rf .debug .envrc .env client_secret.json gmail-credentials.json`


## Public 化前チェックリスト

このリポジトリは 2026-08-02 に履歴を捨てて作り直した。実ポートフォリオと OTP
受信アドレスを含んでいた旧リポジトリ (`moneyforward-paypaysec-bridge`) は削除済み。
force push や `git filter-repo` では到達不能コミットが API から読めるままなので、
リポジトリごと消すのが唯一の確実な方法だった。

- [ ] `.gitignore` に `.envrc`, `.env`, `*.pem`, `*.key`, `cookies.json`, `localStorage.json`, `chrome-data/` が入っているか
- [ ] リポジトリ全文 grep で個人メールアドレスがコードに残っていないか (`grep -rn '@gmail\.com\|@[a-z-]*\.co\.jp'`)
- [ ] secret を含む log 行が CLAUDE.md / README / コメントに残っていないか
- [ ] workflow が `::add-mask::` を全 secret + OTP に適用しているか
- [ ] artifact ステップが追加されていないか (`actions/upload-artifact` 等が無いことを確認)
- [ ] `permissions:` ブロックが `actions: read` / `contents: read` 最小限になっているか
- [ ] private 状態で 1 週間以上 cron が安定稼働した実績があるか
- [ ] action を `go run` からリリース資産に切り替えたか
      (public にすると `uses: …@v1` はタグを信じるだけになる。チェックサムで
      固定できるバイナリ配布に替える。private のうちは資産ダウンロードのほうが
      手数が多く、起動時間も 1 日 1 回のジョブでは効かないので入れていない)
- [ ] 実残高・銘柄名・OTP コードが現ツリーに再混入していないか。
      **ファイル一覧ではなく中身を見ること。** `git status` は何が入るかしか
      見ないので、2026-08-02 にはテストのコメントに残った実銘柄名を通していた
      ```bash
      git grep -nE '<実銘柄名>|<実金額>'
      git grep -nE '[0-9]{6,7}' -- '*.md' '*.go' | grep -v _test.go
      gitleaks detect --no-git -v
      ```
- [ ] 実行ログに銘柄名が出ることを利用者に伝えてあるか
      (金額と secret はマスクされるが、**銘柄名はマスクされない**。public fork で
      動かすと保有銘柄が公開される)
