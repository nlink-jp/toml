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

## 作った理由

nlink-jp は、組織の外で書かれたコードを出荷しません。Go 製ツールは
`config.toml` を第三者製のライブラリで読んでいました。このライブラリはその置き換えです。

## ライセンス

MIT — [LICENSE](LICENSE) を参照してください。
