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
         6. 手入力口座 (MF_ASSET_ID) の中身を銘柄単位で突合
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

| Key | URL / タブ | bucket |
|---|---|---|
| `japan` / `japan-etf` / `usa` / `usa-etf` | `/trade?country=…` | app |
| `robo` | `/trade?reserve_mode=1` | app |
| `miniapp` | `/trade?country=pp` | miniapp |
| `toushin-app` | `/investment_trust/` + タブ「PayPay証券アプリ」 | app |
| `toushin-miniapp` | `/investment_trust/` + タブ「ミニアプリ」 | miniapp |

投資信託だけは **同一 URL でタブ切り替え**。タブは id も role も href も持たない Vue の
`<li><a>ラベル</a></li>` なので、**ラベル文字列で選択**してアクティブ化を確認してから読む。
順序依存の `nth-child` は、並び替わったときに「もっともらしい数字を別 bucket に加算する」
壊れ方をするため使わない。

### 3 ルート照合

各ページで以下を全部取り、**食い違ったら値を返さずエラー**にする (`Reading.Amount`)。

1. `#SECURITIES_VALUE_TOTAL` = 評価額合計（主）
2. `#TOTAL_ACQUISITION_FEE_TAX_TOTAL` (投資元本) + `#gross_profit_total` (含み益)
   → 定義上 1 と一致するはずの**算術チェック**
3. `.brand_invest` の全銘柄合計（ページが銘柄一覧を持つ場合）

**`TOTAL_ACQUISITION_FEE_TAX_TOTAL` は取得原価であって評価額ではない。**
単体で採用すると含み益の分だけ過少計上する。

### 踏んだ罠（再発防止のため記録）

- **金額は入れ子要素**: `<span id="…">33<span>万</span>9780<span>円</span></span>`。
  `innerText` は改行を挟むので正規表現が効かず、**ページ内の無関係な「0円」に誤マッチ**して
  21万0987円の口座を 0 と報告していた。要素指定 + `textContent` で読む
- **投資信託は数値を非同期ロードし、その間 0円 を表示**。このプレースホルダは正常にパース
  できてしまうので、`.loading_page` の消滅と値の安定を待つ (`waitForFigures`)
- **万表記のパースは Go 側** (`ParseYen`)。端数は 4 桁必須（`60万0000` とゼロパディングされる）。
  `1万23` のような曖昧形は 10,023 か 12,300 か決められないのでエラーにする

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
      portfolio/        #   Reconcile / Plan / 書き込みの成否 / 削除の上限
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
      debug/            #     開発用ハーネス
        session/        #       共有フラグ / 永続プロファイル付き Chrome
        paypaysec/ moneyforward/ gmail/
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
`Origin` や `fieldToken`、`waitForFigures` が他方から届いてしまう。

**PayPay 側の境界は「ページはテキストしか返さない」。** `pagescan` は文字列を返し、
数値化と照合は `paypaysec` と `domain/valuation` が行う。

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

### GitHub Secrets

| Secret | 用途 |
|---|---|
| `PAYPAY_SEC_USERNAME` / `PAYPAY_SEC_PASSWORD` | PayPay 証券ログイン |
| `MF_EMAIL` / `MF_PASSWORD` | MoneyForward ログイン (ID/PW) |
| `MF_ASSET_ID` | MF 手入力口座の account_id_hash。この中に銘柄が並ぶ |

| `GMAIL_CREDENTIALS_JSON` | Gmail API のユーザー資格情報 (authorized_user JSON) |

GitHub Variables は使わない。

### ローカルにのみ置くもの (リポジトリには書かない)

- `.envrc` — 上記と同じ値を direnv が環境に置く (`.envrc.example` が雛形)
- `client_secret.json` — OAuth クライアント。同意フローの一度きりにしか使わない
- `gmail-credentials.json` — `mfpp gmail authorize` が作る資格情報

## 開発

- Go 1.22+
- chromedp 用に Chromium / Google Chrome をローカルにインストール
- `.envrc` 等の機密ファイルはリポジトリにコミットしない (`.gitignore` 必須)
- **ローカル開発のための経路をアプリケーションに持たせない**。godotenv を消して
  direnv にしたのはこれ。プログラムは環境変数しか読まないので、CI で通る経路と
  手元で通る経路が同じものになる
- CI は `ubuntu-latest` runner (Chromium プリインストール) で headless 実行

## セットアップ

利用者向けの手順はテンプレート側の SETUP.md にある。ここは開発用:

```bash
cp .envrc.example .envrc && $EDITOR .envrc && direnv allow
go run ./cmd/mfpp gmail authorize   # gmail-credentials.json を作る
```

`gh secret set` はこのリポジトリではなく、利用者自身の fork に対して行う。
このリポジトリは public で、secrets を持たない。

## 注意点

- **DOM 変更耐性**: PayPay 証券・MF のフロントが変わるとセレクタ修正が必要。セレクタは `internal/infra/paypaysec` / `internal/infra/moneyforward` に集約。失敗時は workflow が異常終了 → GitHub の通知メールで気づく運用
- **PayPay 証券への配慮**: スクレイピングは 1 日 1 回に限定、リトライは最大 1 回。robots.txt / 規約抵触の懸念があれば即停止
- **secrets の取り扱い**: chromedp の verbose ログは本番で無効化。OTP・cookies・残高金額はワークフローログで `::add-mask::` を明示適用。エラーメッセージから機密情報を redact
- **個人情報のハードコード禁止**: メールアドレス・トークン・パスワードはコードにも設定ファイルにも書かない。すべて GitHub Secrets とローカルの gitignore 済みファイルに閉じ込める
- **実データもハードコード禁止**: 残高・銘柄名・OTP コードも同じ。「CONFIRMED」の根拠として実ページの数字をコメントに写すのは自然だが、それが公開されれば口座の中身そのもの。セレクタと構造だけ残し、数字は合成値に置き換える
- **OTP race**: 1 ジョブ内で PayPay → MF の順で OTP を受け取る。phase ごとに `phaseStart` を取って ts 比較するので、片方の古い OTP がもう片方に流れ込む事故は起きない
