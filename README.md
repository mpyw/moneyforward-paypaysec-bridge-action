# moneyforward-paypaysec-bridge-action

MoneyForward が PayPay 証券に非対応なので、保有銘柄を毎営業日スクレイピングして
MoneyForward の手入力口座に反映する GitHub Action。

銘柄ごとに 1 つの資産として登録される。

```
[米国株] テスト電機                  評価額 456,789 円 / 取得 400,000 円
[米国株] テスト商事                  評価額 234,567 円 / 取得 200,000 円
[ミニ] テスト電機                    評価額   3,210 円 / 取得   3,500 円
[投信ミ] テスト・グローバル・ファン…  評価額 345,678 円 / 取得 300,000 円
[投信ミ] テストAIファンド            評価額   5,432 円 / 取得   5,800 円
```

数字と銘柄名は例。実際の残高はリポジトリにもテストにも書かない。

---

## 使いたいだけの人へ

**このリポジトリをクローンする必要はない。**
[moneyforward-paypaysec-bridge-template](https://github.com/mpyw/moneyforward-paypaysec-bridge-template)
を private fork して、secrets を 6 つ入れれば動く。手順はそちらの README にある。

このリポジトリを直接触るのは 1 箇所だけ — Gmail の同意フローで、
クローンせずに `go run` で叩く:

```bash
# client_secret.json を置いたディレクトリで
go run github.com/mpyw/moneyforward-paypaysec-bridge-action/cmd/mfpp@v1 \
  debug gmail authorize
```

---

## Action として

```yaml
- uses: mpyw/moneyforward-paypaysec-bridge-action@v1
  with:
    paypaysec-username: ${{ secrets.PAYPAY_SEC_USERNAME }}
    paypaysec-password: ${{ secrets.PAYPAY_SEC_PASSWORD }}
    moneyforward-email: ${{ secrets.MF_EMAIL }}
    moneyforward-password: ${{ secrets.MF_PASSWORD }}
    account-id-hash: ${{ secrets.MF_ASSET_ID }}
    gmail-credentials: ${{ secrets.GMAIL_CREDENTIALS_JSON }}
```

`actions/checkout` は要らない。composite action で、全ステップが自分の
`github.action_path` に閉じている。runner に Chrome が要る (`ubuntu-latest` にはある)。

| input | 中身 |
|---|---|
| `paypaysec-username` / `paypaysec-password` | PayPay 証券のログイン ID (メールアドレス) とパスワード |
| `moneyforward-email` / `moneyforward-password` | MoneyForward のログイン情報 |
| `account-id-hash` | 書き込み先の手入力口座。MoneyForward の URL に出る `account_id_hash` |
| `gmail-credentials` | `gmail.readonly` の authorized_user JSON。上の `go run` で作る |
| `go-version-file` | 任意。既定はこのアクション自身の `go.mod` |

入力の集合は `internal/config` がそのまま契約になっている。
`config_test.go` が「domain が必須と言う名前」と「`Load` が実際に読む名前」を
突き合わせるので、片方だけ増えることがない。

### 前提

**PayPay 証券と MoneyForward の登録メールアドレスが、同じ Gmail の受信箱に届くこと。**
OTP を Gmail API で読むため。転送でも受信はできるが、OTP メールは**送信者アドレスで
絞っている**ので、転送で `From:` が書き換わる経路では動かない。

### 何を書き換えるか

**指定した手入力口座の中身はこのアクションが管理する。保有銘柄に対応しない
エントリは削除される。** 他の口座には触らない。

安全側の制限が 2 つある:

- 1 回の実行で台帳の 1/3 を超える削除は拒否する (`portfolio.CheckBlastRadius`)。
  スクレイプが部分的に失敗して「保有ゼロ」と読めたときに、実在する銘柄を
  消させないため
- ページが読み込み中のプレースホルダを返している間は値を採用しない。
  投資信託ページは非同期ロード中に全項目 0 円を表示し、それは 3 ルート照合を
  すべて通ってしまう

---

## 開発

```bash
cp .envrc.example .envrc && $EDITOR .envrc && direnv allow
```

プログラムは環境変数しか読まない。CI で secrets が渡るのと同じ経路を手元でも使う。

パイプラインは壊れうる箇所が 4 つ（ログイン / OTP / 残高スクレイプ / MF 書き込み）
あり、通しで動かすと全部同じ顔の失敗に見える。`mfpp debug` はそれを 1 ステップずつ
叩けるようにしたもの。

```bash
go run ./cmd/mfpp sync                          # 本体

go run ./cmd/mfpp debug paypaysec selectors     # 認証不要。セレクタの生存確認
go run ./cmd/mfpp debug paypaysec login         # ログイン。セッションを .debug/ に保存
go run ./cmd/mfpp debug paypaysec balance       # 保存したセッションで銘柄を読む
go run ./cmd/mfpp debug paypaysec probe --url URL
go run ./cmd/mfpp debug mf login | list | sync  # MoneyForward 側
go run ./cmd/mfpp debug gmail check | search    # メールボックス

go test -tags=live ./internal/...               # 実サイトに対するセレクタ回帰確認
```

`--otp file` を付けるとメール経路が外れ、失敗の原因をブラウザ側に限定できる。

> ⚠ `mf add` と `mf sync` は実口座に書き込む。他の debug コマンドは読むだけ。
>
> ⚠ `.debug/` には生きたセッション cookie と認証済みページの生 HTML が入る。
> gitignore 済みだが、作業が終わったら `rm -rf .debug` すること。

設計と、その理由になった実地の落とし穴は [CLAUDE.md](./CLAUDE.md) に。
脅威モデルと secret の扱いは [SECURITY.md](./SECURITY.md) に。

## License

MIT. [LICENSE](./LICENSE) を参照。
