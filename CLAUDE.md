# moneyforward-paypaysec-bridge-action

MoneyForward が PayPay 証券に非対応なので、PayPay 証券の残高を毎営業日スクレイピングして MoneyForward の「手入力資産」に反映する GitHub Action。

利用者向けのフォーク元は別リポジトリ (`moneyforward-paypaysec-bridge-template`)。
ここには `action.yml` と Go の実装、開発者向けの文書しか置かない。cron を持つ
`sync.yml` はテンプレート側の持ち物で、ここには無い。

## ゴール

- PayPay 証券（Web）の残高を平日大引け後（JST 15:30）に取得
- **8 つのターゲット**の 評価額合計 を合算（内訳は「PayPay 証券スクレイピング」節）
- MoneyForward 上では **1 つの手入力資産** に合算残高を書き込む
- GitHub Actions の Free 枠で完結、追加課金 0 円
- 設計はいつでも public 化できるセキュアな構成。「artifact によるセッション受け渡し」「個人情報のハードコード」「ログへの secret 出力」を含まない

## アーキテクチャ (シングルジョブ)

PayPay 証券・MoneyForward どちらも **ID/PW + メール OTP** でログインする。CI runner は
ステートレスなので毎回 OTP が発行される。OTP は **Gmail API から直接読む**。

```
[cron 平日 15:30 JST — 呼び出し側の sync.yml]
  └─> action.yml (composite) = mfpp sync
       Phase 1: PayPay 証券
         1. chromedp でログイン → ID/PW 投入 → OTP メール発行をトリガー
         2. Gmail API をポーリングし、送信時刻より新しいメールから 6 桁を抽出
         3. ::add-mask::$OTP → 6 桁を 1 桁ずつ入力 → ログイン完了
         4. 8 ターゲットから銘柄単位で 評価額 と 取得価額 を取得

       Phase 2: MoneyForward
         5. chromedp でログイン → 同様に Gmail API から OTP 取得
         6. 手入力口座 (MONEYFORWARD_ASSET_ID) の中身を銘柄単位で突合
            insert or update / delete。書き込みごとに読み戻して検証
```

### 設計判断と理由

- **OTP は Gmail API から直接読む**: 外部サービスに中継させて GitHub Variables に
  書かせる形も採れるが、そうすると third-party・PAT・書き込み権限が要り、
  「変数にあるのは今回のコードか前回のか」を毎回判断させられる。直接読めば
  受信時刻とログイン送信時刻を比較するだけで済む
- **サービスアカウント不可**: ドメイン全体の委任は Workspace 専用。個人 Gmail は
  ユーザーの refresh token でしか読めないので、Workload Identity Federation は使えない
- **スコープは gmail.readonly のみ**: `gcloud auth application-default login` は
  cloud-platform を強制するため使わず、同意フローを自前で持つ (`mfpp gmail authorize`)。
  CI に置く資格情報が漏れたとき、影響が「メール読み取り」に留まる
- **artifact 不使用**: public repo では誰でも DL 可、private でも collaborator 全員可で
  90 日保持。短命データを永続化しない
- **シングルジョブ**: job 間のセッション受け渡しが不要
- **passkey 不採用**: macOS Chrome の Virtual Authenticator が platform authenticator に
  優先されない。OTP 経路 1 本のほうが再現性が高い
- **個人情報をリポジトリに残さない**: メールアドレス・トークンはコードにも文書にも書かない

## PayPay 証券スクレイピング (2026-08-01 実口座で確定)

### ターゲット 8 件

| Key | 取得元 | bucket | 読み方 |
|---|---|---|---|
| `japan` / `japan-etf` / `usa` / `usa-etf` | `/trade?country=…` | app | ページ |
| `robo` | `/trade?reserve_mode=1` | app | ページ |
| `miniapp` | `/trade?country=pp` | miniapp | ページ |
| `toushin-app` | `/investment_trust/` の API | app | **API** |
| `toushin-miniapp` | 同上（別パス） | miniapp | **API** |

**投資信託の 2 件だけは API を叩く** (`infra/paypaysec/investapi`)。この 2 つは
同一 URL の 2 ビューで、ページからはどちらの数字かを区別できない（「投資信託だけ
API から読む理由」節）。他の 6 件は jQuery のサーバレンダリングで JSON API を
持たないため、ページから読む。

`Target.ViaAPI` がその分岐で、同一 URL に 2 件並んだら API 経由であることを
`target_test.go` が要求する。

### 3 ルート照合

各ページで以下を全部取り、**食い違ったら値を返さずエラー**にする (`Reading.Amount`)。

1. `#SECURITIES_VALUE_TOTAL` = 評価額合計（主）
2. `#TOTAL_ACQUISITION_FEE_TAX_TOTAL` (投資元本) + `#gross_profit_total` (含み益)
   → 定義上 1 と一致するはずの**算術チェック**
3. `.brand_invest` の全銘柄合計（ページが銘柄一覧を持つ場合）

**`TOTAL_ACQUISITION_FEE_TAX_TOTAL` は取得原価であって評価額ではない。**
単体で採用すると含み益の分だけ過少計上する。

### 投資信託だけ API から読む理由

同一 URL の 2 ビューをタブで切り替える。**クリックは同期で即座、数字は約 1 秒後**——
実測で `actived` も `menu_1_mini` も **8ms**、数字が変わるのは **1028ms**。
その間ページは「新しいタブが選ばれた状態」で「前のタブの数字」を表示している。
DOM にこの 2 状態を区別する材料がない。

3 通り試して 3 通り漏れた:

| 待ち方 | 結果 |
|---|---|
| MutationObserver + 5 秒の下限 | 保有 2 銘柄が 0 件と読まれ、両方削除 |
| 下限を 10 秒に倍増 | 同じ事故が再発（v3.2.1 の実行） |
| そのタブ自身の `pc_invest_top` レスポンスを待つ | レスポンスと描画の間でまだ競合 |

**時間で待つ実装は必ず期限切れになり、切れた先で前のタブの値を読む。**
だからページを読むのをやめた。API では bucket が「ビュー」ではなく
**エンドポイント**なので、観測すべき状態が存在しない。

| bucket | top | init（銘柄名） | APP_ID |
|---|---|---|---|
| アプリ | `/v2/invest/brand/pc_invest_top` | `/v2/…/pc_invest_init` | 3 |
| ミニアプリ | `/v3/invest/brand/pc_invest_top` | `/v3/…/pc_invest_init` | 6 |

**ページは bucket ごとに transport を持ち、各 transport が自分の既定フィールドを持つ。**
`MINI_CLIENT_SEQ_NO` を取る `pc_invest_info` は v2 のパスだが、ミニアプリのときは
**ミニアプリの transport 経由**で呼ばれる = `APP_ID: 6` かつ `MINI_CLIENT_SEQ_NO: ""`。
アプリの transport（`APP_ID: 3`）で聞くのは別の質問で、その答えを v3 top に渡すと
**`STATUS: 9`「システムの不具合」** で拒否される（原因を何も言わない）。
ミニアプリを持たない口座では `MINI_CLIENT_SEQ_NO` が **0** で返る。数値フィールドなので
空文字判定だけでは抜ける。`init` → `top` の順もページに合わせる（ステートフルな PHP）。

- **契約はページ自身の JS バンドルから読んだ**（推測していない）。未認証の
  `GET /investment_trust/` がバンドル名を返し、バンドルがパス・定数・
  `MINI_CLIENT_SEQ_NO` の出どころ (`pc_invest_info`) を全部書いている
- **`Referer` が無いとミニアプリの v3 は答えない。** ページの中から投げた同じボディは
  200 で通り、Go から投げると `STATUS 9`。差分は **`Referer` だけ**で、ブラウザの
  User-Agent は関係ない（実測で 4 通り試した）。全呼び出しに付ける — v2 は要求しないが、
  例外は維持するコストがあり、主張自体はどちらでも正しい
- **バンドルはボディまでしか教えてくれなかった。** 「ページの中から同じ呼び出しをして
  比べる」(`debug paypaysec invest --via-page`) が、ボディの問題か否かを 1 回で切り分ける。
  **CI に投げて 1 回 1 推測（毎回ログイン + 全走査）を 3 リリース続けたのが間違い**で、
  ローカルで回せる経路を先に作るべきだった
- **ミニアプリを持たない口座は「読み取り失敗」でも「空」でもない第 3 の答えにする。**
  ページ自身の判定 `"" != (MINI_CLIENT_SEQ_NO && INV_TRUST_USABLE)` を使い、偽なら
  **そのターゲットを読まない**（`ErrNoMiniApp`）。失敗にすると持っていない人の実行が
  毎営業日落ちる。空にすると**そのカテゴリの全削除を許可**してしまう。読まなければ
  カテゴリが未カバーになり `CheckCoverage` が削除を拒否する。ログに 1 行出す
  （黙って古くなるのは、失敗よりたちが悪い。失敗はメールが飛ぶ）
- **`PP_KYC` には何もさせていない。** バンドルは `hasMiniApp && PP_KYC` でタブメニューを
  出し、`PP_KYC == 0` でアプリ側の `/portfolio` ルートを塞ぐ。ここから「PP_KYC は
  アプリ bucket を gate する」と読むのは**推測**で、分かるのは画面の話だけ。エンドポイントが
  何を返すかは未観測（この口座は `PP_KYC="1"` なので観測できない）。値は debug
  コマンドが表示するだけに留めてある
- **`LOGIN_STATUS === 1` はサインアウト済みで、その応答は保有を持たない。**
  額面どおり受けると「カテゴリが空になった」に見え、このプログラムはそれを削除する。
  `STATUS !== 0` と併せて数値を読む前に弾く
- **取得原価は導出**: `SECURITIES_VALUE - SUM_GROSS_PROFIT`。整数演算で、
  ページ側が万の小数 1 桁に丸めていたのより正確
- **銘柄名は init のカタログ由来**で、これは保有していない銘柄も返す。保有は top の
  `INVEST_BRAND_ARRAY` だけ。**カタログを保有と読むと数百件を発明する**
- **スカラーは「数値」でも「数値の文字列」でも来る。** PHP + DB ドライバの API なので、
  引用符が付くかは値の出どころで決まり、意味では決まらない。`MINI_CLIENT_SEQ_NO` は
  バンドルが文字列扱いしているが実際は数値で来た。`laxInt64` / `laxString` が両方受ける。
  **緩めるが推測はしない** — 数値として読めない文字列はエラーで、0 にはしない
  （0 は金額であり、このプログラムは金額に対して動く）
- **`INVEST_BRAND_ARRAY` は「brand id キーのオブジェクト」と「素の配列」の 2 形で来る。**
  PHP の配列なので、キーが疎ならオブジェクト、密なら配列。**形はエンドポイントではなく
  口座の保有内容で決まる**ので、どちらも仮定できない。初回の実運用でミニアプリは
  オブジェクト、アプリは配列だった（`brandList` が両方受ける）。配列形はキーを持たない
  ので、カタログとの突合は `BRAND_ID` にフォールバックする。空 (`[]`) も同じ形なので、
  受け付けないと「保有 0 件」が全部失敗になる
- 名前が引けない保有はエラー。台帳は名前をキーにするので、空名で記録すると
  次回マッチせずもう 1 行作る
- セッションは chromedp の cookie jar を借りる (`cookiestore.HTTPClientFor`)。
  投資信託に到達しなかった実行が代金を払わないよう遅延生成する
- **他の 6 件は同じことができない。** `/trade` は jQuery のサーバレンダリングで
  JSON API を持たない。「API に寄せる」は投資信託に限った話

### 踏んだ罠（再発防止のため記録）

- **金額は入れ子要素**: `<span id="…">34<span>万</span>5678<span>円</span></span>`。
  `innerText` は改行を挟むので正規表現が効かず、**ページ内の無関係な「0円」に誤マッチ**して
  25万1234円の口座を 0 と報告していた。要素指定 + `textContent` で読む
- **投資信託は数値を非同期ロードし、その間 0円 を表示**。このプレースホルダは正常にパース
  できてしまうので、`.loading_page` の消滅と値の安定を待つ (`pagescan.settle`)
- **万表記のパースは Go 側** (`ParseYen`)。端数は 4 桁必須（`60万0000` とゼロパディングされる）。
  `1万23` のような曖昧形は 10,023 か 12,300 か決められないのでエラーにする
- **内側の待ちの合計は外側の deadline より小さく保つ。** `targetTimeout` が 30 秒の
  ままだと settle 20 + settle 20 を包めず、外側が先に切れて「どの待ちで詰まったか」を
  言わない context deadline に化ける
- **同一性は動かない値で判定する。** 「一覧の値」と「個別ページの値」の一致を
  求めていたが、2 つは数秒差で取得するので価格が動けば必ず食い違い、1 日に 2 回
  実行全体が落ちた。着地した URL で判定する

### OTP ページ

- **6 桁が 1 桁ずつ別 input** (`#code1`〜`#code6`)。`#code2` 以降は初期 `readonly` で、
  ページの `keyPress()` が順次アンロックする。**値の直接代入では送信ボタンが出ない**ので
  1 文字ずつキーイベントを送る
- **送信ボタンが 2 つある**。可視な `#btn_sms_success_two` は onclick を持たないダミー。
  本物は `#btn_sms_success` で 6 桁揃うまで `display:none`
- `#otp-prefix` に 2 文字のアルファベット（例 `AB-`）。メール側と一致することを人間が
  確認するフィッシング対策。自動化ではこの確認が飛ぶため、`LoginResult.OTPPrefix` に
  保持だけしてある

## ディレクトリ構成

```
action.yml              # composite action。inputs が internal/config の写し
.github/workflows/
  ci.yml                # push ごと。actionlint / vet / race / lint / 実ブラウザ
cmd/
  mfpp/                 # 単一エントリポイント (薄い main のみ)
internal/
  application/          # 判断。infra も cli も import しない (layer_test.go が強制)
    domain/
      money/            #   ParseYen
      valuation/        #   3 ルート照合。信じてよい合計か
      portfolio/        #   Reconcile / Plan / 書き込みの成否 / 削除の妥当性
                        #     CheckCoverage: 読んだカテゴリからしか消さない
                        #     CheckCategoryEmptied: 丸ごと空になる削除は拒否（解除可）
      assetname/        #   資産名の生成と一意性
      asset/            #   サイト間を渡る単位と Kind
      secret/           #   ジョブが必要とする資格情報の集合
      credential/       #   無人で使える資格情報とは何か
    port/               # 副作用そのものの interface。1 メソッド 1 通信
    usecase/
      syncassets/       #   mfpp sync の段取り全部
      authorizegmail/   #   mfpp gmail authorize
  cli/                  # 配送手段。フラグと表示
    credentials/        #   Gmail 資格情報の解決順序 (secret → ファイル)
    commands/           #   ディレクトリ構成 = コマンドの木
      root.go           #     mfpp 直下に何がぶら下がるか
      sync/             #     wire の injector と provider
      gmail/            #     authorize / check / search。debug ではない
      debug/            #     開発用ハーネス
        session/        #       共有フラグ / 永続プロファイル付き Chrome
        paypaysec/ moneyforward/
  infra/                # 副作用の実装
    adapter/            #   port の実装。サイトパッケージと繋ぐ
    actionslog/         #   Actions ログのマスキング
    helpers/steperr/    #   どの段階で失敗したか
    chrome/browser/     #   chromedp ラッパ + probe + ページダンプ
    chrome/pagescript/  #   埋め込み JS の読み込みと引数適用
    chrome/cookiestore/ #   セッション cookie の保存と復元
    gmail/              #   Gmail API 読み取り専用クライアント
    gmail/consent/      #   OAuth 同意フロー (ループバック + PKCE)
    otp/                #   OTP 取得 Source (Gmail / ファイル手渡し)
    moneyforward/       #   MF ログイン (chromedp)
      selector/         #     確定セレクタと OTP メールの見つけ方
      manualasset/      #     手入力口座の銘柄 CRUD (HTTP)
    paypaysec/          #   PayPay 証券: 数値の照合
      selector/         #     確定セレクタ・8 ターゲット・埋め込み JS
      pagescan/         #     ページ操作。テキストだけ返す (chromedp)
      investapi/        #     投資信託の 2 bucket。ページの叩く API を直接叩く
                        #       contract  パス・定数・リクエストボディ（バンドル由来）
                        #       transport 1 回の POST と必須の Referer
                        #       response  応答の形と、数値を読む前の判定
                        #       decode    JSON 型を約束しないサービスへの対処
                        #       account   その bucket が存在するのか
                        #       holdings  保有と銘柄名の突合
```

**依存の向きは `application/layer_test.go` が強制する。** `application/**` は
`infra/**` も `cli/**` も import しない。doc コメントに書いただけの規則は、
何も考えずに追加された import で破られる — 実際、use case が自分のポートを
スクレイパの構造体で宣言していたのはそれ。

**port は副作用そのものの単位。** SignIn / Recorded / Create / Update / Delete。
「突合せよ」ではない。そういう形の interface はユースケースごと実装者に渡してしまい、
書き込みの順序も読み戻しの要否も infra に住むことになる。

**ログインとデータ経路は別パッケージ。** 両サイトとも認証は chromedp、その後の
読み書きは別手段 (MF は HTTP、PayPay はページ読み取り)。同じ名前空間に置くと
`Origin` や `fieldToken`、`settle` が他方から届いてしまう。

**PayPay 側の境界は「ページはテキストしか返さない」。** `pagescan` は文字列を返し、
数値化と照合は `paypaysec` と `domain/valuation` が行う。投資信託は `investapi` が
数値のまま返すが、`investread.go` が同じ `Reading` に詰めるので、3 ルート照合・
マスク・ログ行は下流で共通。

ページスクリプトは `pagescan/script_test.go` が実ブラウザに fixture ページを
読ませて駆動する (外部通信なし)。長く「テストが届かない」前提で書かれていたが届く。

サブコマンド:

```
mfpp sync               # 本体ジョブ
mfpp debug paypaysec …  # selectors / login / balance / probe
mfpp debug mf …         # login / portfolio / list / add / sync / fetch / probe
mfpp gmail …            # authorize / check / search
```

`mf add` と `mf sync` は実口座に書き込む。他の debug コマンドは読むだけ。

`mf add` と `mf sync` は実口座に書き込む。他の debug コマンドは読むだけ。

## 認証情報

### アクションの入力

**このリポジトリは secrets を持たない。** public で、cron も無い。下記は
`action.yml` の入力で、値は利用者側の fork の Secrets から渡る。

| 環境変数 | action.yml の input | 用途 |
|---|---|---|
| `PAYPAYSEC_USERNAME` / `PAYPAYSEC_PASSWORD` | `paypaysec-username` / `-password` | PayPay 証券ログイン |
| `MONEYFORWARD_EMAIL` / `MONEYFORWARD_PASSWORD` | `moneyforward-email` / `-password` | MoneyForward ログイン (ID/PW) |
| `MONEYFORWARD_ASSET_ID` | `moneyforward-asset-id` | MF 手入力口座の account_id_hash |
| `GMAIL_CREDENTIALS` | `gmail-credentials` | Gmail API のユーザー資格情報 |

**名前の規則は 1 本だけ: 環境変数 = input を大文字にしてハイフンを `_` に。**
対応表を覚える必要はない。規則を外れた名前はテストが落とす。

必須名の一覧は `domain/secret`。`config_test.go` が domain と `Load` を突き合わせ、
`actionyml_test.go` が `action.yml` と突き合わせる。**3 者のどれかがずれたら落ちる。**
GitHub Variables は使わない。

### ローカルにのみ置くもの (リポジトリには書かない)

- `.envrc` — 上記と同じ値を direnv が環境に置く (`.envrc.example` が雛形)
- `client_secret.json` — OAuth クライアント。同意フローの一度きりにしか使わない
- `gmail-credentials.json` — `mfpp gmail authorize` が作る資格情報

## 開発

- Go 1.27+ (go.mod がこれを宣言する。CI も action.yml も `go-version-file` で追随する)
  - **golangci-lint は go.mod の言語バージョン以上の Go でビルドされたものが要る。**
    古いと `can't load config` で**何も検査せずに**終わる。CI のピン (`.github/workflows/ci.yml`)
    と手元を同じ版に揃える
- chromedp 用に Chromium / Google Chrome をローカルにインストール
- `.envrc` 等の機密ファイルはリポジトリにコミットしない (`.gitignore` 必須)
- **ローカル開発のための経路をアプリケーションに持たせない**。プログラムは環境変数
  しか読まず、ファイルを探さない。値を環境に置くのは direnv の仕事 (`.envrc.example`)。
  CI で通る経路と手元で通る経路が同じものになる
- CI は `ubuntu-latest` runner (Chromium プリインストール) で headless 実行

## セットアップ

利用者向けの手順は
[template の SETUP.md](https://github.com/mpyw/moneyforward-paypaysec-bridge-template/blob/main/SETUP.md)
にある。ここは開発用:

```bash
cp .envrc.example .envrc && $EDITOR .envrc && direnv allow
go run ./cmd/mfpp gmail authorize   # gmail-credentials.json を作る
```

`gh secret set` はこのリポジトリではなく、利用者自身の fork に対して行う。
このリポジトリは public で、secrets を持たない。

## タグの現状

| タグ | 何 |
|---|---|
| `v3` | 可動タグ。利用者が参照する |
| `v3.1.0` | 実体。`main` と同一 |
| `v1.0.4` | **旧モジュールパス（`/v3` なし）専用。main から辿れない** |

`v1.0.4` は `main` の履歴に無い。タグからのみ到達でき、それで十分（タグは ref なので
GC されない）。ブランチを置いていないのは、更新される場所に見せないため——**これは
終着点**で、中身は当時の `/v3` 最新と同じコード、モジュールパスだけ旧形に戻してある。

存在理由:

- `v1.0.0`〜`v1.0.2` は proxy.golang.org にキャッシュされており、**タグを消しても
  消えない**。旧パスを使い続ける人は v1.0.2 —— 旧命名と、実在する銘柄を消し得る
  読み取り不具合を含む版 —— を掴む
- **撤回はそのモジュールパスの最新版の go.mod からしか読まれない。** だから旧パスに
  新しい版が要った
- **v1.0.4 自身は撤回していない。** 撤回すると `@latest` が選べる版を失い、「古い
  パスを使った」が「動かない」になる。狙いは逆で，間違えても最新のコードが動くこと

再現するなら: `main` から作業ブランチを切り、`/v3` を全ファイルから外し、go.mod に
`retract` と `// Deprecated:` を足して、タグだけ push してブランチを捨てる。

## リリース手順

**メジャーを上げたらモジュールパスの `/vN` も上げる。** Go は v2 以降のモジュールに
パス末尾の major を要求する。付け忘れると `go run …@v3` が
`no matching versions` になり、`@latest` は**付いていなかった頃の最後の版**
（このリポジトリでは v1.0.2）を返し続ける。タグを消しても proxy には残るので、
利用者は旧命名・修正前のバグを含む版を黙って掴む。action の `uses: @v3` は git ref
なので影響を受けず、**Go 側だけが壊れる**。気づきにくい。


利用者は `@v3` を参照する。`v2` は**可動タグ**で、置き忘れると修正が誰にも届かない
——スクレイパなので、セレクタの修正が届かないことは毎営業日の失敗を意味する。

```bash
# ゲートを通してから
gofmt -l . ; go vet ./... && go vet -tags live ./... && go vet -tags wireinject ./...
go run github.com/google/wire/cmd/wire ./internal/cli/commands/...   # 差分が出ないこと
golangci-lint run ./... && golangci-lint run --build-tags wireinject ./...
TZ=UTC go test -race ./...
go run github.com/rhysd/actionlint/cmd/actionlint@latest

git tag vX.Y.Z && git push origin vX.Y.Z
git tag -f v3  && git push -f origin v3      # ★ これを忘れない
```

そのうえで template の変更があれば push し、private fork に merge する。

**template の SHA 固定の例にリテラルを書かないこと。** 以前は書いてあって、
リリースのたびに書き換える手順になっていた。2 回連続で忘れ、例が「タブ競合で
実在する銘柄を消すバージョン」を指したまま残った。いまは引くコマンドを載せてある。

SHA が要るときは `git rev-parse --verify vX.Y.Z^{commit}` を使い、40 桁 hex かを
確かめてから使う。`git rev-parse` は**解決できない ref でエラー終了せず引数を
そのまま返す**ので、確かめないと SHA のつもりでタグ名を書く。一度やっている。

## 注意点

- **DOM 変更耐性**: PayPay 証券・MF のフロントが変わるとセレクタ修正が必要。セレクタは `internal/infra/paypaysec` / `internal/infra/moneyforward` に集約。失敗時は workflow が異常終了 → GitHub の通知メールで気づく運用
- **PayPay 証券への配慮**: スクレイピングは 1 日 1 回に限定、リトライは最大 1 回。robots.txt / 規約抵触の懸念があれば即停止
- **secrets の取り扱い**: chromedp の verbose ログは本番で無効化。OTP・cookies・残高金額はワークフローログで `::add-mask::` を明示適用。エラーメッセージから機密情報を redact
- **個人情報のハードコード禁止**: メールアドレス・トークン・パスワードはコードにも設定ファイルにも書かない。すべて GitHub Secrets とローカルの gitignore 済みファイルに閉じ込める
- **実データもハードコード禁止**: 残高・銘柄名・OTP コードも同じ。「CONFIRMED」の根拠として実ページの数字をコメントに写すのは自然だが、それが公開されれば口座の中身そのもの。セレクタと構造だけ残し、数字は合成値に置き換える
- **OTP race**: 1 ジョブ内で PayPay → MF の順で OTP を受け取る。phase ごとに `phaseStart` を取って ts 比較するので、片方の古い OTP がもう片方に流れ込む事故は起きない
- **OTP メールの鮮度比較は秒精度で行う**。`internalDate` はミリ秒のフィールドだが中身は
  秒に切り捨てられており、`time.Now()` と厳密比較すると**同じ秒に届いたメールを永久に
  弾く**（タイムスタンプは変わらないので待っても直らない）
- **OTP メールを件名で絞らない**。件名はロケール依存で、MoneyForward は US ランナーからの
  ログインに英語で返す。送信者で絞り、本文の構造（コードだけの行）で判別する
- **`::add-mask::` はログと同じストリームに出す**。ディレクティブを stdout、ログを stderr に
  書くと、ランナーは 2 本のパイプの順序を保証しないので「登録してから出力」が逆順で届く。
  実測で stderr のディレクティブも効く
- **計画と結果を分けて報告する**。拒否のチェックは計画表示の後に走るので、1 回しか報告
  しないと**拒否された実行が成功したように見える**

### 開発の進め方

`.claude/skills/` に手順を置いてある。

| skill | いつ |
|---|---|
| `verify` | push 前のゲート（CI と同じ検査。wire 再生成と 2 タグぶんの lint を含む） |
| `release` | タグを切って template と private fork まで反映するとき |
| `scrape-debug` | 実サイトの読み取りが失敗した・数字が合わないとき |
