# toml リファレンスマニュアル

> 版: 0.1.1

## 概要

`github.com/nlink-jp/toml` は、TOML 1.1.0 の文書を Go の値に読み込むライブラリです。
Go の標準ライブラリだけを使い、書き出しの機能はありません。

```
go get github.com/nlink-jp/toml
```

ふつうのアンマーシャラーと違う点が 3 つあります。

- **渡した値の上に読み込みます。** 設定の構造体に既定値を入れてから、ファイルを上に重ねて読み込みます。
  ファイルに書かれていないキーは、既定値のまま残ります。
- **既定で厳格に読みます。** 値に対応するフィールドがないキーはエラー（`UnknownKeysError`）にし、黙って捨てません。
- **構造体のフィールドとキーの対応は `BurntSushi/toml` と同じです。** ツールをこのライブラリに移しても、設定ファイルの意味は変わりません。

このマニュアルでは、公開されている名前をすべて説明します。
どの TOML の値がどの Go の型に入るか、フィールドとキーをどう対応させるか、`BurntSushi/toml` とどこが違うかといった細かい規則は、
[decoding.ja.md](decoding.ja.md) にまとめています。

```go
import (
	"errors"

	"github.com/nlink-jp/toml"
)

cfg := defaultConfig()
md, err := toml.DecodeFile(path, &cfg)
var unknown *toml.UnknownKeysError
switch {
case errors.As(err, &unknown):
	// unknown.Keys: cfg にフィールドのないキーすべて。ほかはすべて読み込み済み
case err != nil:
	// *toml.ParseError、*toml.DecodeError、またはファイルを開くときのエラー
}
if md.IsDefined("model", "name") {
	// ファイルに model.name が書かれていた
}
```

---

## 関数

### `Decode(data string, v any, opts ...Option) (MetaData, error)`

TOML の文書 `data` を `v` に読み込みます。`v` は nil でないポインタでなければなりません。

- 文書に書かれていない値は、`v` に既にある値のまま残ります。入れ子の構造体、既に設定されたポインタの先、map の項目でも同じです。
  ただしスライスと Go の配列は、文書の配列で置き換えます。
- nil のポインタは、文書がそこに書いたときだけ新しい値を作ります。そのため、ポインタのフィールドで「書かれた」と「書かれていない」を区別できます。
- エラーは次のいずれかです。
  - `*ParseError` — `data` が TOML 1.1.0 ではない。
  - `*DecodeError` — 値が受け皿に合わない。読み込みはそこで止まるので、`v` には文書の一部だけが入っていることがある。
  - `*UnknownKeysError` — `v` に受け皿のないキーが文書にある。ほかにエラーがないときだけ返す。知っているキーはすべて読み込み済み。
  - `v` が nil でないポインタではないときは、ふつうのエラー。
- `MetaData` は、文書を構文解析できたときは常に返します。`*DecodeError` や `*UnknownKeysError` と一緒に返す場合も含みます。

```go
var cfg Config
md, err := toml.Decode(`name = "x"`, &cfg)
```

### `DecodeFile(path string, v any, opts ...Option) (MetaData, error)`

`path` のファイルを読み、`Decode` と同じように読み込みます。
読むのは 1 MiB と 1 バイトまでで、それより大きいファイルは全体を読まずに `*ParseError` にします。
ファイルを開くときや読むときのエラーは、`os` パッケージが返したものをそのまま返すので、`errors.Is(err, fs.ErrNotExist)` で判定できます。

```go
md, err := toml.DecodeFile(filepath.Join(home, ".config", "tool", "config.toml"), &cfg)
if errors.Is(err, fs.ErrNotExist) {
	// 設定ファイルがない: 既定値のまま使う
}
```

---

## オプション

### `type Option`

```go
type Option func(*config)
```

読み込み方を変えます。`Decode` と `DecodeFile` には、いくつでも渡せます。

### `AllowUnknownKeys() Option`

値に受け皿のないキーを、`UnknownKeysError` にせず読み飛ばします。
文書がほかの読み手のための設定も含みうる場合や、移行中の既存ツールを緩い読み方のまま保ちたい場合に使います。

```go
_, err := toml.Decode(doc, &cfg, toml.AllowUnknownKeys())
```

---

## MetaData

### `type MetaData`

```go
type MetaData struct { /* 非公開 */ }
```

読み込んだ文書について説明します。ゼロ値はどの文書も表しません。

### `(MetaData) IsDefined(key ...string) bool`

文書にそのキーが書かれていたかを返します。キーのパートを 1 つずつ引数に渡します。
値なら真です。表なら、ヘッダー、ドット付きキー、より深いヘッダーのどれで作られた場合でも真です。
配列テーブルの中のキーには届きません。引数がまったくなければ偽です。
空のパート `""` は 1 つのキーとして扱い、文書に `"" = …` と書かれていれば真です。

```go
md.IsDefined("model", "name")                            // [model] name = …
md.IsDefined(strings.Split("sandbox.enabled", ".")...)  // 手元にあるドット区切りの名前
```

---

## 独自の読み込み

### `type Unmarshaler`

```go
type Unmarshaler interface {
	UnmarshalTOML(any) error
}
```

TOML の値を自分で読み込む型が、ポインタで実装します。
値の形が一つに決まらないキーのためのものです。例えば、コマンドを argv の配列でも 1 つのシェル用文字列でも受けたい場合に使います。
`UnmarshalTOML` には、`any` に入れたときと同じ形の値を渡します。

| TOML | 受け取る Go の値 |
|------|-----------------|
| 表 | `map[string]any` |
| 配列（配列テーブルも） | `[]any` |
| 文字列 | `string` |
| 整数 | `int64` |
| 浮動小数点数 | `float64` |
| 真偽値 | `bool` |
| オフセットつき日時 | `time.Time` |
| ローカル日時・日付・時刻 | `LocalDateTime`・`LocalDate`・`LocalTime` |

この受け皿の下にあるキーは、すべて読み込み済みとして扱います。
返したエラーは `*DecodeError` に包まれ、Unwrap で取り出せます。

### `encoding.TextUnmarshaler`

このパッケージの型ではありませんが、扱います。
ポインタが `UnmarshalText` を実装している受け皿には、文字列ならその中身を、日時なら TOML の書式で書いた文字列を渡します。
それ以外の値は `*DecodeError` にします。返したエラーは `Unmarshaler` と同じように包みます。
`time.Time` は `TextUnmarshaler` なので、RFC 3339 の書式の文字列も入ります。

### `time.Duration`

`"1m30s"` のような文字列は、`time.ParseDuration` で `time.Duration` に読み込みます。整数はナノ秒として扱います。

---

## 日時の型

オフセットつき日時は `time.Time` に入れ、書かれたオフセットを保ちます（名前のない固定のタイムゾーン。オフセット 0 は `time.UTC`）。
ローカルの 3 種類はある瞬間を指していないので、それぞれ専用の型に入れます。
`time.Time` に入れようとすると `*DecodeError` になり、メッセージで正しい型を示します。

### `type LocalDate`

```go
type LocalDate struct {
	Year  int
	Month time.Month
	Day   int
}
```

時刻もオフセットもない、ある日です（`1979-05-27`）。

#### `(LocalDate) String() string`

TOML と同じ書式で日付を書きます（`1979-05-27`）。

### `type LocalTime`

```go
type LocalTime struct {
	Hour, Minute, Second int
	Nanosecond           int
}
```

日付もオフセットもない、一日の中の時刻です（`07:32:00.5`）。
`Second` は 60（うるう秒）になりえます。
文書で秒が省かれていれば（TOML 1.1 で書ける `07:32`）0 になります。ナノ秒より細かい小数は切り捨てます。

#### `(LocalTime) String() string`

TOML と同じ書式で時刻を書きます（`07:32:00`）。小数は、あるときだけ付けます。

### `type LocalDateTime`

```go
type LocalDateTime struct {
	Date LocalDate
	Time LocalTime
}
```

オフセットのない日付と時刻です（`1979-05-27T07:32:00`）。

#### `(LocalDateTime) String() string`

TOML と同じ書式で日時を書きます（`1979-05-27T07:32:00`）。

#### `(LocalDateTime) In(loc *time.Location) time.Time`

呼び出し側がタイムゾーンを決めたあとで、この日時が `loc` で指す瞬間を返します。
うるう秒は、`time.Date` の正規化に従って次の分の 0 秒になります。

```go
start := cfg.Start.In(time.Local)
```

---

## エラー

どのエラーも `errors.As` で取り出せます。`fmt.Errorf("…: %w", err)` で包んだ後でも取り出せます。
メッセージは、文書の中身にかかわらず小さく保ちます。文書から引用する部分は 40 文字まで、受け皿のメソッドのエラーは 200 文字までです。

### `type ParseError`

```go
type ParseError struct {
	Line, Column int
	Msg          string
}
```

文書が TOML 1.1.0 ではないことを表します。構文の誤り、キーや表の二重定義、範囲外の値、不正な UTF-8、上限を超えた文書がこれに当たります。
`Line` と `Column` は 1 から数えます。`Column` はバイト数ではなく文字数で、先頭の BOM は 1 文字に数えません。

#### `(*ParseError) Error() string`

`toml: line L, column C: msg`

### `type DecodeError`

```go
type DecodeError struct {
	Key  Key
	Line int
	Msg  string
	// 非公開: 包んだエラー
}
```

文書は正しいが、`Key` の値が Go の受け皿に合わないことを表します。次の場合です。

- 型が違う
- 数値が範囲外、または正確に表せない
- 大文字小文字だけが違う 2 つのキーが同じフィールドに行き着いた
- 受け皿の `UnmarshalTOML` や `UnmarshalText`、`time.Parse`、`time.ParseDuration` がエラーを返した

`Line` は、その値が書かれている行です。文書全体が受け皿に合わない場合（表を `int` に読み込もうとしたなど）は、`Key` は空です。

#### `(*DecodeError) Error() string`

`toml: line L: key K: msg`。`Key` が空のときは `toml: line L: msg`。

#### `(*DecodeError) Unwrap() error`

受け皿のメソッド、`time.Parse`、`time.ParseDuration` のエラーを返します。それ以外のときは nil です。

### `type UnknownKeysError`

```go
type UnknownKeysError struct {
	Keys []Key
}
```

値に受け皿のないキーを、文書に書かれた順にすべて挙げます。
受け皿のない表は、その下のキーもすべて合わせて挙げます。そのため、削除した表の下のキーも、フルネームで見つけられます（`workspace` と `workspace.workspace_dir`）。
配列テーブルの中のキーは、添字なしの名前で 1 回だけ挙げます。
map、`any`、`Unmarshaler` の受け皿はどのキーも受けますが、map の中の構造体は別です。

#### `(*UnknownKeysError) Error() string`

キーが 1 つなら `toml: unknown key: K`、2 つ以上なら `toml: unknown keys: K1, K2, …` です。
名前を挙げるのは 10 個までで、残りは数だけ示します（`and 3 more`）。

### `type Key`

```go
type Key []string
```

値への道筋を、キーのパートごとに 1 要素で表します。`[model] name = …` は `Key{"model", "name"}` です。
配列テーブルの添字は含みません。

#### `(Key) String() string`

TOML の書き方でキーを書きます。パートをドットでつなぎ、裸のキーにできないパートだけを引用符で囲みます（`model.name`、`characters."少女".pitch`）。
ツールが使っているドット区切りの名前と、そのまま比べられます。

---

## 上限

| 上限 | 値 |
|------|-----|
| 文書の大きさ | 1 MiB |
| 表と配列の入れ子 | 128 段 |
| 1 つのキーのパート数 | 128 |

上限を超えた文書は `*ParseError` にします。自分自身を指すポインタ型（`type P *P`）は `*DecodeError` にします。
