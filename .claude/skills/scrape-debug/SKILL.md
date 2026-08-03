---
name: scrape-debug
description: 実サイトの読み取りが失敗した・数字が合わないときに，安全に調べる手順
---

# スクレイピング不調の調べ方

## まず: 何を触るか決める

| コマンド | 書き込み | OTP |
|---|---|---|
| `debug paypaysec selectors` | なし | 不要 |
| `debug paypaysec login / balance / probe` | なし | ログイン 1 回 |
| `debug mf login / list / portfolio / fetch / probe` | なし | ログイン 1 回 |
| `debug mf add / sync` | **実口座に書く** | ログイン 1 回 |
| `sync` | **実口座に書く** | 2 回 |

**MoneyForward に触らずに済むなら触らない。** PayPay 側の読み取り調査は
`debug paypaysec balance` だけで完結する。

## 資格情報が手元に無いとき

```bash
cp .envrc.example .envrc && $EDITOR .envrc && direnv allow
```

`gmail-credentials.json` が無ければ OTP を自動取得できない。**手渡しにする:**

```bash
mfpp debug paypaysec login --otp file
# → .debug/otp.txt に 6 桁を書けと言われる。メールを見て
echo 123456 > .debug/otp.txt
```

以後 `debug paypaysec balance` はセッションを使い回すので OTP は不要。

## 叩きすぎない

**両サービスとも，短時間にログインを繰り返すと OTP メールの送信自体を止める**
（5 回程度で遭遇。同日中に復活した）。

**セッション切れは「全ターゲットが失敗」に見える。**

```
! japan: #SECURITIES_VALUE_TOTAL not found — the session is probably not authenticated
```

これを一度セレクタの不具合と誤認した。**全部落ちていたら，まずセッションを疑う。**

## タイミングの罠

数字が「0 と読めた」ときは，ページが嘘をついていないか疑う。

- **投資信託は非同期ロード中に全項目 0円 を表示する。** 3 ルート照合を全部通る
- **タブ切替はクリック同期で即座，データ到着は約 1 秒後。** 実測: `actived` も
  `menu_1_mini` も **8ms** で切り替わり，数字は **1028ms** で変わる。
  **クリックに由来する状態は，データが古いうちから正しく見える**
- したがって「値が変化しなくなった」は「新しいデータが来た」ではない

計測のしかた（モジュール内に使い捨てを置いて chromedp で駆動する）:

```go
// 保存済み cookie を読んでページを開き，クリック後の状態を 100ms 間隔で出す
```

**推測でセレクタを足さない。** 静的なダンプでは「クリック同期」と「データ同期」を
区別できない。時系列で測る。

## 同一性は動かない値で判定する

「一覧の値」と「個別ページの値」の一致を求めていた時期があり，**価格が数円動いただけで
実行全体が落ちた**（1 日に 2 回）。2 つは数秒差で取得するので，動く値は同一性の証拠に
ならない。着地した URL のような，見ている間に変わらないもので判定する。

## 後片付け

```bash
rm -rf .debug
```

`.debug/` には**生きたセッション cookie** と**認証済みページの生 HTML**（残高・銘柄名
入り）が入る。gitignore 済みだが，作業が終わったら必ず消す。

`probe` のダンプも同じ。実データを含むので，そこから何かを文書やコメントに書き写す
ときは合成値に置き換える。
