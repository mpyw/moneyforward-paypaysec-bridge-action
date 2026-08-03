---
name: release
description: このアクションのリリースを切り，template と private fork まで反映するとき
---

# リリースと 3 リポジトリへの反映

利用者は `@v3` を参照する。**`v3` は可動タグで，動かし忘れると修正が誰にも届かない。**
これはスクレイパなので，セレクタ修正が届かないことは毎営業日の失敗を意味する。

## 手順

```bash
# 1. ゲート（verify スキル）を全部通す
# 2. コミットして push
git push origin main

# 3. パッチを打ち，メジャータグを動かす
git tag vX.Y.Z && git push origin vX.Y.Z
git tag -f v3   && git push -f origin v3      # ★ 忘れない

# 4. CI が緑になるまで待つ
gh run watch "$(gh run list --workflow ci.yml --limit 1 --json databaseId --jq '.[0].databaseId')" --exit-status

# 5. template に変更があれば push
# 6. private fork に取り込む
git clone git@github.com:mpyw/moneyforward-paypaysec-bridge-private.git /tmp/pf && cd /tmp/pf
git remote add upstream https://github.com/mpyw/moneyforward-paypaysec-bridge-template.git
git fetch upstream && git merge upstream/main -m "Merge upstream" && git push origin main

# 7. 実口座で通す
gh workflow run sync.yml --repo mpyw/moneyforward-paypaysec-bridge-private
```

## 破壊的変更のとき

input 名・環境変数名を変えたらメジャーを上げる。

- 新しいメジャータグを打ち，**古い系列のタグは消す**（古い版に固定されると，直って
  いないバグを含む版を使わせることになる）
- **GitHub Secrets は値を読み出せないのでリネームできない。** 新名で登録し，
  **新名が揃ったことを確認してから**旧名を消す。逆順でやって secret が 1 件になった
  ことがある
- ローカルの `.envrc` も同時に直す。忘れると新名の値が空のまま登録される

## 落とし穴

- **`git rev-parse` は解決できない ref でエラー終了せず，引数をそのまま返す。**
  SHA が要るときは `git rev-parse --verify vX.Y.Z^{commit}` を使い，40 桁 hex か
  確かめてから使う。確かめずに使って，SHA 固定の例にタグ名を書いて push している

  ```bash
  SHA=$(git rev-parse --verify vX.Y.Z^{commit})
  [ ${#SHA} -eq 40 ] || { echo "解決できていない"; exit 1; }
  ```

- **文書にリテラル SHA を書かない。** template の固定例に書いてあった時期があり，
  リリースのたびに書き換える手順が 2 回連続で漏れて，例が「実在する銘柄を消すバグ入りの
  版」を指したまま残った。いまは引くコマンドを載せてある
- **タグを消しても proxy.golang.org からは消えない。** `go run …@vX.Y.Z` は
  永久に解決する。公開前の監査はタグを打つ前に済ませる
- **Dependabot が main に直接コミットすることがある。** push が rejected されたら
  `git rebase origin/main` してから，依存更新込みでテストを通し直す
