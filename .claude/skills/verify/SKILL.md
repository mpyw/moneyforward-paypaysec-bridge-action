---
name: verify
description: このリポジトリに変更を加えて push する前に回す検査一式。CI と同じものをローカルで先に通す
---

# push 前のゲート

CI と同じ検査。**1 つでも赤ければ push しない。**

```bash
gofmt -l .                                              # 出力が空であること
go vet ./... && go vet -tags live ./... && go vet -tags wireinject ./...
go run github.com/google/wire/cmd/wire ./internal/cli/commands/...
git diff --quiet -- '*wire_gen.go' || echo "★ wire_gen.go が古い"
golangci-lint run ./... && golangci-lint run --build-tags wireinject ./...
TZ=UTC go test -race ./...
go run github.com/rhysd/actionlint/cmd/actionlint@latest
```

## それぞれが実際に捕まえたもの

- **`go vet` を 3 タグぶん** — `live` と `wireinject` は既定ビルドから外れる。片方だけ
  通して push し，CI で落ちたことがある
- **wire の再生成** — `wire.go` の **doc コメントを直すだけ**でも `wire_gen.go` が古くなる
  （生成物がコメントを写す）。ビルドもテストも通るので，これだけは差分で見るしかない
- **golangci を 2 タグぶん** — 既定ビルドは `wire.go` を見ないので `wire.NewSet` が
  `unused` に見え，`wireinject` ビルドは `wire_gen.go` を見ない。**どちらか片方では
  パッケージ全体を覆えない**
- **`TZ=UTC`** — 壁時計に依存したテストが JST の夕方に通り UTC の朝に落ちた
- **actionlint** — ステップ名に `::` を書いて YAML を壊したことがある。**有効な YAML で
  無効なワークフロー**になり，`gh workflow list` には active と出たまま dispatch もできない

## サブコマンドの起動確認

```bash
go build -o /tmp/mfpp ./cmd/mfpp
while read -r line; do
  eval "set -- $line"          # ★ zsh は $line を単語分割しない。eval が要る
  /tmp/mfpp "$@" --help >/dev/null 2>&1 || echo "FAILED: $*"
done <<'EOF'
sync
gmail authorize
gmail check
gmail search
debug paypaysec selectors
debug paypaysec login
debug paypaysec balance
debug paypaysec probe
debug mf login
debug mf portfolio
debug mf add
debug mf sync
debug mf list
debug mf fetch
debug mf probe
EOF
```

`eval` を省くと `"debug mf"` が 1 引数として渡り，**全部通ったように見えて何も検査
していない**。一度これで「全サブコマンド確認済み」と報告している。

## やってはいけないこと

- **未コミットの変更があるファイルに `git checkout --` を打たない。** 変異検証のあと
  戻すつもりで，修正ごと消したことがある。変異は `python3` で入れて同じ方法で戻す
- **`&&` で繋がずに commit しない。** `actionlint` が落ちているのに次行の commit が
  走って，壊れた YAML を push したことがある
