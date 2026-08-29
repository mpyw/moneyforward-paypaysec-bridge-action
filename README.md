<p align="center">
  <img src="https://github.com/user-attachments/assets/0285a2ec-77d4-43d1-85fb-e6ac814cc674" alt="moneyforward-paypaysec-bridge-action" width="640">
</p>

<h1 align="center">moneyforward-paypaysec-bridge-action</h1>

> [!CAUTION]
> **有志が個人利用目的で作成した非公式のツールです。**
> **利用には情報流出のリスクが伴います。**
> **ご自身でリスク管理できる方のみ利用してください。**
>
> PayPay 証券・マニュライフ生命・MoneyForward いずれとも関係はありません。

MoneyForward が対応していない口座を毎営業日スクレイピングして、MoneyForward の
手入力口座に反映する GitHub Action。

**ソースは 2 つあり、どちらも任意。** 設定したものだけ読む。

| ソース | 何を読むか |
|---|---|
| PayPay 証券 | 8 カテゴリの保有銘柄。銘柄ごとに 1 資産 |
| マニュライフ生命 | 外貨建一時払終身保険の解約時お支払金額。契約ごとに 1 資産 |

**ソースごとに書き込み先の手入力口座を分ける。** 片方が読めなかった日に、
もう片方の口座まで止まらないため。

```
── PayPay 証券の口座 ──
[米国株] テスト電機                  評価額 456,789 円 / 取得 400,000 円
[米国株] テスト商事                  評価額 234,567 円 / 取得 200,000 円
[ミニ] テスト電機                    評価額   3,210 円 / 取得   3,500 円
[投信ミ] テスト・グローバル・ファン…  評価額 345,678 円 / 取得 300,000 円

── マニュライフ生命の口座 ──
[保険] テスト終身保険                評価額 1,500,000 円 / 取得 1,400,000 円
```

数字と銘柄名は例。実際の残高はリポジトリにもテストにも書かない。

---

## 使いたいだけの人へ

**このリポジトリをクローンする必要はない。**
[moneyforward-paypaysec-bridge-template](https://github.com/mpyw/moneyforward-paypaysec-bridge-template)
を private fork して、使うソースぶんの secrets を入れれば動く。手順はそちらの README にある。

このリポジトリを直接触るのは 1 箇所だけ — Gmail の同意フローで、
クローンせずに `go run` で叩く:

```bash
# client_secret.json を置いたディレクトリで
go run github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/cmd/mfpp@v3 \
  gmail authorize
```

これに [Go](https://go.dev/dl/) が要る（1.21 以降なら必要なものを自動で取ってくる）。
使うのはこの 1 回だけで、以降の同期はランナー側で動くので手元には残らない。
同意フローはループバックで完結するため、**ブラウザを手元で開ける環境**が要る。

---

## Action として

```yaml
- uses: mpyw/moneyforward-paypaysec-bridge-action@v3
  with:
    # 台帳。これだけは必須
    moneyforward-email: ${{ secrets.MONEYFORWARD_EMAIL }}
    moneyforward-password: ${{ secrets.MONEYFORWARD_PASSWORD }}
    gmail-credentials: ${{ secrets.GMAIL_CREDENTIALS }}

    # ソース。使うものだけ、その 3 つを全部
    paypaysec-username: ${{ secrets.PAYPAYSEC_USERNAME }}
    paypaysec-password: ${{ secrets.PAYPAYSEC_PASSWORD }}
    moneyforward-paypaysec-asset-id: ${{ secrets.MONEYFORWARD_PAYPAYSEC_ASSET_ID }}

    manulife-username: ${{ secrets.MANULIFE_USERNAME }}
    manulife-password: ${{ secrets.MANULIFE_PASSWORD }}
    moneyforward-manulife-asset-id: ${{ secrets.MONEYFORWARD_MANULIFE_ASSET_ID }}
    manulife-acquisition-yen: ${{ secrets.MANULIFE_ACQUISITION_YEN }}
```

**ソースの入力は「全部か、ゼロか」。** 半端に設定するとエラーで止まる。
変数名を 1 つ打ち間違えたときに、そのソースが黙って読まれなくなり、口座だけ
静かに古くなるほうが失敗より悪いから。**1 つもソースが無い場合もエラー**。

`actions/checkout` は要らない。composite action で、全ステップが自分の
`github.action_path` に閉じている。runner に Chrome が要る (`ubuntu-latest` にはある)。

> [!IMPORTANT]
> **`@v3` は動くポインタ。** このアクションは呼び出し側のジョブの中で、証券口座と
> MoneyForward の資格情報を環境に持って動く。タグ参照のままだと、そのコードは
> 利用者の関与なしに差し替わり得る。

気になるなら SHA-1 で固定する:

```yaml
- uses: mpyw/moneyforward-paypaysec-bridge-action@<commit sha>  # v3
```

代償はスクレイパ特有のもので、PayPay 証券や MoneyForward が DOM を変えた日に
セレクタの修正が届かず毎営業日失敗する。固定するなら Dependabot も入れておく。

`action.yml` の中で使う `actions/setup-go` は**こちら側で SHA 固定してある**。
利用者がこのアクションを SHA 固定しても、その先のタグ参照までは止められない
——止められない側が負う risk なので、止められる側で止めてある。

### 共通

台帳と、その他。**ソースが何であれ要る。**

| input | 環境変数 | 中身 |
|---|---|---|
| `moneyforward-email` | `MONEYFORWARD_EMAIL` | MoneyForward のログインメール。**必須** |
| `moneyforward-password` | `MONEYFORWARD_PASSWORD` | 同パスワード。**必須** |
| `gmail-credentials` | `GMAIL_CREDENTIALS` | `gmail.readonly` の authorized_user JSON。上の `go run` で作る |
| `allow-emptying-categories` | `ALLOW_EMPTYING_CATEGORIES` | 任意。カテゴリを丸ごと空にする削除を 1 回許可する |
| `go-version-file` | — | 任意。既定はこのアクション自身の `go.mod` |

### PayPay 証券

**3 つ全部か、ゼロか。** 全部無ければこのソースは読まれない。

| input | 環境変数 | 中身 |
|---|---|---|
| `paypaysec-username` | `PAYPAYSEC_USERNAME` | ログイン ID (メールアドレス) |
| `paypaysec-password` | `PAYPAYSEC_PASSWORD` | 同パスワード |
| `moneyforward-paypaysec-asset-id` | `MONEYFORWARD_PAYPAYSEC_ASSET_ID` | 書き込み先の手入力口座。URL に出る `account_id_hash` |
| `moneyforward-asset-id` | `MONEYFORWARD_ASSET_ID` | **旧名**。上の以前の名前で、まだ読む |

### マニュライフ生命

**上 3 つが全部か、ゼロか。** `manulife-acquisition-yen` は設定していても任意。

| input | 環境変数 | 中身 |
|---|---|---|
| `manulife-username` | `MANULIFE_USERNAME` | マイページのログイン ID |
| `manulife-password` | `MANULIFE_PASSWORD` | 同パスワード |
| `moneyforward-manulife-asset-id` | `MONEYFORWARD_MANULIFE_ASSET_ID` | 書き込み先の手入力口座 |
| `manulife-acquisition-yen` | `MANULIFE_ACQUISITION_YEN` | 任意。一時払で実際に払った円額（数字のみ）|

---

**口座 id は「台帳が主語、ソースが修飾語」**——`MONEYFORWARD_<ソース>_ASSET_ID`。
どれも MoneyForward の口座で、違うのは誰の保有が入るか。`MONEYFORWARD_ASSET_ID` は
口座が 1 つしかなかった頃の名前で、2 つ目ができた時点で意味が定まらなくなった。
**改名は非破壊**で旧名も読み続ける。両方設定されていて値が違えばエラー——
どちらの口座に書くかを推測させない。

**`manulife-acquisition-yen` は手で渡す。** サイトは払込保険料を米ドルでしか出さず、
今日のレートで換算したものは取得原価ではなく為替と一緒に動く数字になる。
省くと MoneyForward は取得原価＝評価額とみなし、**評価損益をちょうど 0** と表示する。
一時払だから「一度きりの円額」が存在して成立している。

**環境変数名は input 名を大文字にしてハイフンを `_` にしたもの**、という 1 本の規則に
揃えてある。対応表を覚える必要はない。

規則も対応関係もテストが縛っている (`internal/config/actionyml_test.go`)。
`action.yml` が宣言していて誰も読まない入力、逆に読まれるのに宣言されていない変数、
規則を外れた名前、`required: true` の付け忘れ——どれもテストが落ちる。

### 前提

**使う全サービスの OTP メールが、同じ Gmail の受信箱に届くこと。**
Gmail API で読むため。転送でも受信はできるが、OTP メールは**送信者アドレスで
絞っている**ので、転送で `From:` が書き換わる経路では動かない。

マニュライフ生命は OTP の送信先をログイン ID と別のアドレスに設定できるので、
そちらに寄せられる。なお送信元は `manulife.com` で、サイトの `manulife.co.jp` ではない。

**1 実行あたり OTP はソース数 + 1 通**（各ソース + MoneyForward）。両ソースを使うと
3 通になる。短時間に繰り返すと送信自体を止められることがある（実測で 5 回程度）。

### 何を書き換えるか

**指定した手入力口座の中身はこのアクションが管理する。保有に対応しない
エントリは削除される。** 他の口座には触らない。

だから**ソースごとに新しい空の口座を作ること**。既存の口座を指すと、手で入れた行も
「保有に対応しないエントリ」として消える。接頭辞の無い行は削除ガードの対象外——
人が作った行と、このプログラムが改名前に書いた行を、名前からは区別できないため。

安全側の制限:

- **読めなかったソースは、その口座に触らない。** 何も読めていないのだから
  突合しようがない。他のソースは通常どおり記録され、実行は最後にそのソース名を
  挙げて失敗する（通知メールは飛ぶ）。片方のサイトが落ちている日に、もう片方まで
  更新されないのは損なだけ

- **読まなかったカテゴリからは削除しない** (`portfolio.CheckCoverage`)。
  そのソースのページが 1 つでも読めなければそのソースの読み取りが失敗するので、
  計画が存在する時点で全ページが検証済み。それでも口座にあってこの実行が一度も
  見なかったカテゴリの行は「古い」ではなく「未検証」なので、消さずに落とす
- **カテゴリが丸ごと空になる読み取りは拒否する** (`portfolio.CheckCategoryEmptied`)。
  誤読が毎回とった形がこれ（保有 2 銘柄のカテゴリが 0 件と読まれ、両方消えた）。
  ページを取得して検証まで通っても、そのページの別ビューの数字が返ることがある。
  その特定の抜け道は投資信託を API から読むようにして閉じたが、この停止は
  次の抜け道がどんな形で来ても効く
- **本当に売り切ったときは 1 回だけ解除できる。** `allow-emptying-categories` を
  付けて手動実行する。**解除手段の無い停止は入れない** — 「limit を上げて再実行しろ」
  と言うだけで上げる手段が無かった以前の実装が、まさにそれだった
- **口座が持っていないカテゴリは読まない。** ミニアプリの投資信託は任意なので、
  ページ自身の判定で「無い」なら、そのターゲットを失敗にも空にもせず飛ばす。
  飛ばしたカテゴリは未カバーなので、上の 1 つ目の制限で削除されない（ログに 1 行出る）
- **カテゴリ内で銘柄が減るのは正常系。** 何銘柄減っても、そのカテゴリに 1 つでも
  残っていればそのまま反映される
- ページが読み込み中のプレースホルダを返している間は値を採用しない。
  非同期ロード中の 0 円表示は 3 ルート照合をすべて通ってしまうため、
  「合計が 0 で銘柄も 0 件」は整合しているだけで正しくない

---

## 開発

```bash
cp .envrc.example .envrc && $EDITOR .envrc && direnv allow
```

プログラムは環境変数しか読まない。CI で secrets が渡るのと同じ経路を手元でも使う。

パイプラインは壊れうる箇所が多く（ソースごとのログイン / OTP / スクレイプ / MF 書き込み）、
通しで動かすと全部同じ顔の失敗に見える。`mfpp debug` はそれを 1 ステップずつ
叩けるようにしたもの。

`debug probe` は**まだセレクタが 1 つも無いサイト**への入口。人が手でログインし、
そのあと何ページでも捕まえられる。マニュライフのセレクタは全部これで取った。

```bash
go run ./cmd/mfpp sync                          # 本体

go run ./cmd/mfpp debug probe --url URL --manual --save-session
                                                # サイトを知らない調査。手でログインし
                                                # 何ページでも捕まえる。通信も記録できる

go run ./cmd/mfpp debug paypaysec selectors     # 認証不要。セレクタの生存確認
go run ./cmd/mfpp debug paypaysec login         # ログイン。セッションを .debug/ に保存
go run ./cmd/mfpp debug paypaysec balance       # 保存したセッションで銘柄を読む
go run ./cmd/mfpp debug manulife login | read   # マニュライフ生命。read は契約を全部読む
go run ./cmd/mfpp debug mf login | list | sync  # MoneyForward 側
go run ./cmd/mfpp debug mf subclasses           # MF の 資産クラス 一覧と未マッピング検出
go run ./cmd/mfpp gmail check | search          # メールボックス

go test -tags=live ./internal/...               # 実サイトに対するセレクタ回帰確認
```

lint は `mise` が版を固定する。**素の `golangci-lint` を叩かないこと** — PATH 上の
別の版が走り、手元と CI が別のものを検査する:

```bash
mise install
mise exec -- golangci-lint run ./...
```

`--otp file` を付けるとメール経路が外れ、失敗の原因をブラウザ側に限定できる。

> [!WARNING]
> `mf add` と `mf sync` は**実口座に書き込む**。他の debug コマンドは読むだけ。

> [!CAUTION]
> `.debug/` には生きたセッション cookie と認証済みページの生 HTML が入る。
> cookie は単体でログインできる。gitignore 済みだが、作業が終わったら
> `rm -rf .debug` すること。

設計と、その理由になった実地の落とし穴は [CLAUDE.md](./CLAUDE.md) に。
脅威モデルと secret の扱いは [SECURITY.md](./SECURITY.md) に。

## License

MIT. [LICENSE](./LICENSE) を参照。
