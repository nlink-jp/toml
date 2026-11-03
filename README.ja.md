# toml

[nlink-jp](https://github.com/nlink-jp) のツールのための TOML 1.1.0 デコーダ。

Go 標準ライブラリだけで書かれたライブラリです。TOML の文書を Go の値に読み込みます。
**既定で厳格**に読み、読み込み先に対応するフィールドがないキーは、黙って捨てずにエラーにします。
読み込み専用で、書き出しの機能はありません。

> **状態: 開発中。** 下の API は予定のもので、まだ実装されていません。
> [RFP](docs/ja/toml-rfp.ja.md) を参照してください。

## インストール

```bash
go get github.com/nlink-jp/toml
```

## 使い方（予定）

```go
import "github.com/nlink-jp/toml"

cfg := defaultConfig() // ファイルに書かれていないキーはこの値のまま
md, err := toml.DecodeFile(path, &cfg)
var unknown *toml.UnknownKeysError
if errors.As(err, &unknown) {
	// unknown.Keys に、構造体にフィールドのないキーがすべて入る
}
if md.IsDefined("model", "name") {
	// ファイルに model.name が書かれていた
}
```

読み込み先が知らないキーも受け付けたいときは、`toml.AllowUnknownKeys()` を渡します。

## 読めるもの

- TOML 1.1.0 のすべての構文を読めます。1.0.0 の文書もそのまま読めます。
  [toml-test](https://github.com/toml-lang/toml-test) の 1.1.0 向けの文書はすべて通ります。
- 読める文書は 1 MiB までで、表と配列の入れ子は 128 段までです。
- エラーには行と列を付けます。列は文字数で数えます。
- ヘッダーが親として暗に作っただけの表には、ドット付きキーで書き込めます。
  例えば `[a.b.c]` の後の `[a]` で、`b.d = 1` と書けます。仕様はこれを
  許していますが、他の TOML 実装の中にはもっと厳しく読むものがあり、
  そうしたファイルは他の実装では読めないことがあります。

## 作った理由

nlink-jp は、組織の外で書かれたコードを出荷しません。Go 製ツールは
`config.toml` を第三者製のライブラリで読んでいました。このライブラリはその置き換えです。

## ライセンス

MIT — [LICENSE](LICENSE) を参照してください。
