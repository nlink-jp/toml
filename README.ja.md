# toml

[nlink-jp](https://github.com/nlink-jp) のツールのための TOML 1.1.0 デコーダ。

Go 標準ライブラリだけで書かれたライブラリです。TOML の文書を Go の値に読み込みます。
**既定で厳格**に読み、読み込み先に対応するフィールドがないキーは、黙って捨てずにエラーにします。
読み込み専用で、書き出しの機能はありません。

## インストール

```bash
go get github.com/nlink-jp/toml
```

## 使い方

```go
import "github.com/nlink-jp/toml"

cfg := defaultConfig() // ファイルに書かれていないキーはこの値のまま
md, err := toml.DecodeFile(path, &cfg)
var unknown *toml.UnknownKeysError
if errors.As(err, &unknown) {
	// unknown.Keys に、構造体にフィールドのないキーが文書の順にすべて入る。
	// 知っているキーはすべて読み込み済み
}
if md.IsDefined("model", "name") {
	// ファイルに model.name が書かれていた
}
```

| API | 働き |
|-----|------|
| `Decode(data string, v any, opts ...Option) (MetaData, error)` | 文書を `v`（nil でないポインタ）に読み込む。`v` が既に持っている値の上に重ねる。 |
| `DecodeFile(path string, v any, opts ...Option) (MetaData, error)` | ファイルから同じことをする。1 MiB を超える分は読まない。 |
| `AllowUnknownKeys()` | `v` に受け皿のないキーを、エラーにせず読み飛ばす。 |
| `MetaData.IsDefined(key ...string) bool` | 文書にそのキーが書かれていたかを返す。 |
| `LocalDateTime`、`LocalDate`、`LocalTime` | TOML のローカル日時の受け皿。オフセットつき日時は `time.Time` に入る。 |
| `Unmarshaler` | `UnmarshalTOML(any) error`。値の形が一つに決まらないキーのためのもの。 |
| `ParseError`、`DecodeError`、`UnknownKeysError`、`Key` | 3 種類のエラー。どれも `errors.As` で取り出せる。 |

どの TOML の値がどの Go の型に入るか、構造体のフィールドとキーをどう対応させるか、`BurntSushi/toml` とどこが違うかは、
[docs/ja/decoding.ja.md](docs/ja/decoding.ja.md) にまとめています。

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
