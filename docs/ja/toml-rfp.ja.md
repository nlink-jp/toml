# RFP: toml

> Generated: 2026-11-03
> Status: Draft

## 1. Problem Statement

nlink-jp の Go 製ツールは、利用者が手で書く `~/.config/<tool>/config.toml` を読むために、組織外で作られた TOML ライブラリ（`github.com/BurntSushi/toml`）に依存している。2026-11-03 時点で、稼働中の 29 リポがこれに依存している。この依存は、サプライチェーン対策としての組織方針（第三者の依存を持たず、標準ライブラリ・一次 SDK・REST 直叩きだけを使う。ADR-024）に反する。依存を避けるために設定を JSON にし、sectioned TOML の組織規約から外れたリポもある（mcp-bridge）。また、知らないキーをエラーにする厳格な読み込みを、各リポが `Undecoded()` を使って個別に書いており、実装がばらついている。

本ライブラリは、TOML 1.1.0 の読み込みを Go 標準ライブラリだけで実装し、厳格な読み込みを既定の動作にする。それによって、組織の Go ツールから TOML の外部依存を取り除く。利用者は組織内の Go ツールの開発者であり、最終的な受益者は設定ファイルを書くツールの利用者である。

## 2. Functional Specification

### Commands / API Surface

```go
package toml // github.com/nlink-jp/toml

func Decode(data string, v any, opts ...Option) (MetaData, error)
func DecodeFile(path string, v any, opts ...Option) (MetaData, error)

type MetaData struct{ /* … */ }
func (MetaData) IsDefined(key ...string) bool

type Key []string // String() は "a.b.c"

type Option func(*config)
func AllowUnknownKeys() Option // 厳格な読み込みを明示的に外す

// エラーはいずれも errors.As で取り出せる
type ParseError struct{ Line, Column int; Msg string }  // 構文エラー（位置つき）
type UnknownKeysError struct{ Keys []Key }               // 知らないキー（全件）
type DecodeError struct{ Key Key; Line int; Msg string } // 型の不一致・桁あふれ
```

- `Decode` と `DecodeFile` の引数は `BurntSushi/toml` と同じ形にする。移行は import の書き換えと、未知キー確認の削除で済むようにする。
- `Undecoded()` は用意しない。知らないキーがあれば、読み込み自体が `UnknownKeysError` で失敗する。呼び出し側はキーの一覧を取り出し、ツール独自の文面やエラーコードに組み立てられる。
- 既定は厳格な読み込みとする。緩く読むのは `AllowUnknownKeys()` を明示したときだけ。

### Input / Output

- 入力: メモリ上の文字列、またはファイルのパス。TOML 1.1.0 の文書（1.0.0 の文書はそのまま 1.1.0 として読める）。
- 出力: Go の値。対応する受け皿は次のとおり。
  - string、bool、整数（int 系・uint 系。桁あふれはエラー）、float 系（整数を float に入れるのは可）
  - スライス、`map[string]T`、構造体（入れ子あり、埋め込み構造体は親のフィールドとして扱う）、ポインタ、`any`（`map[string]any` と `[]any` に入る）
  - `encoding.TextUnmarshaler` を実装した型
  - 日時 4 種類（オフセットつき日時・ローカル日時・ローカル日付・ローカル時刻）。Go 側の受け皿の型は Phase 2 の設計で決める。
- 既存の値の上に重ねて読む。書かれていないキーは既定値のまま残す。スライスは丸ごと置き換え、map は書かれたキーだけを上書きする。
- 構造体のフィールドとキーの対応は `BurntSushi/toml` に合わせる。タグの名前を優先し、タグがなければフィールド名と大文字小文字を区別せずに照合する。タグの `,omitempty` などのオプションは無視する。
- 入力サイズと入れ子の深さに上限を設ける。具体的な値は設計で決める。
- 書き出し（Encoder / Marshal）は提供しない。

### Configuration

なし（ライブラリであり、それ自体の設定は持たない）。

### External Dependencies

なし。Go 標準ライブラリだけを使う。テストでも第三者のモジュールは go.mod に載せない。

## 3. Design Decisions

- **言語は Go。** 利用側はすべて Go である。Python のツールは標準ライブラリの `tomllib` で足りているため対象外とする。
- **対象は TOML 1.1.0。** 現行の `BurntSushi/toml` v1.6.0 は TOML 1.1.0 を既定で受け付ける。1.0 止まりにすると、1.1 の構文を使った既存の設定ファイルが移行後に読めなくなる。1.1 で増えたのは、時刻の秒の省略、`\xHH` と `\e` のエスケープ、インラインテーブル内の改行と末尾カンマである。
- **2 段階の読み込み。** まず文書を、位置情報つきの木に構文解析する。次にその木を Go の値に当てはめる。知らないキーの検出、`IsDefined`、行番号つきのエラーを、すべて同じ木から作る。
- **厳格な読み込みを既定にする。** 手で書く設定の打ち間違いが黙って無視される事故を防ぐ（組織の strict decode の慣行）。
- **適合試験は toml-lang/toml-test のデータを取り込む。** 正しい文書と誤った文書、約 1,000 件のデータ集である。コミットを固定して `testdata/` に置き、全ファイルの SHA256 一覧とライセンス表記を同梱する。テスト時に読むだけで、出荷物にも go.mod にも入らない。ADR-024 が禁じる「組織外で書かれたコードの出荷」には当たらない。照合の仕組みは自前で書く。
- **ファジング。** Go 標準のファジングで、不正な入力でパニックしないことを確かめる。
- **組織内の位置づけ。** lib-series に置き、pathguard と同様に組織の Go ツール群の土台とする。移行が終われば、mcp-bridge が JSON を選んだ理由はなくなる。ただし、mcp-bridge を TOML に戻すかどうかは別の判断とする。

### Out of Scope

- 書き出し（Encoder / Marshal）
- コメントを保ったままの編集・書き換え
- `io.Reader` からの逐次読み込み
- TOML 1.1.0 以外の版の切り替え
- 検証用 CLI
- Python 側の対応（`tomli` の除去を含む）
- 開発中の比較対象として `BurntSushi/toml` を使うこと
- 既存 29 リポの移行（後続作業として別に計画する）

## 4. Development Plan

### Phase 1: Core

構文解析器（文書 → 位置情報つきの木）を作る。

- TOML 1.1.0 の全構文を読めるようにする。
- toml-test のデータを、コミットを固定して取り込む。SHA256 一覧とライセンス表記も置く。
  - 正しい文書: 解析結果を期待値（型つき JSON）と突き合わせる。JSON は標準の `encoding/json` で読む。
  - 誤った文書: すべて位置つきのエラーで拒否されることを確かめる。
- ファジングを入れ、入力サイズと入れ子の深さの上限を入れる。
- **レビューの区切り:** 適合試験を全件通過すること。

### Phase 2: Features

Go の値への当てはめと、公開 API を作る。

- `Decode`、`DecodeFile`、`MetaData.IsDefined`、`AllowUnknownKeys()`、3 種類のエラー型。
- 型の対応、既定値の上に重ねる動作、ポインタ、`any`、`TextUnmarshaler`、桁あふれの検出。
- 日時 4 種類の Go 側の受け皿を決め、設計文書に書く。
- 既存リポの使い方を再現した互換テストを書く。
  - 既定値を入れた構造体への読み込み
  - `*int` と `*bool`
  - `Duration`（`TextUnmarshaler`）
  - `IsDefined` による設定値の出どころ表示
  - 未知キーの一覧から独自エラーを組み立てる
- **レビューの区切り:** 第三者検証パス（サブエージェント）。指摘は振り分けてから扱い、自動では適用しない。

### Phase 3: Release

- README.md と README.ja.md、AGENTS.md、CHANGELOG.md を書く。
- 組織 ADR-026 で「TOML の読み込みは `nlink-jp/toml` を使う」と定める。
- v0.1.0 としてリリースする。移行で何リポか使い、API が固まってから v1.0.0 にする。
- lib-series にサブモジュールとして登録し、org プロフィールを更新する。check-org を通し、知見を knowledge に還元する。

### Follow-up（本 RFP の外）: 既存リポの移行

- 29 リポへの一括変更として計画を立て、分担して進める。ADR-025 に従い、各リポの中から変更する。
- 現在は知らないキーを読み飛ばしているリポ（`DecodeFile` は 31 箇所、未知キー確認は 19 箇所）について、厳格に読むか `AllowUnknownKeys()` を付けるかをリポごとに決める。厳格にする場合は、互換を崩す変更としてリリースノートに書く。
- 各リポを個別にリリースする。

## 5. Required API Scopes / Permissions

なし。外部サービスも資格情報も使わない。

## 6. Series Placement

Series: lib-series
Reason: シリーズをまたいで使う土台のライブラリであり、pathguard と同じ置き場所になる。作業は `_wip/toml/` で始め、リリース時に `lib-series/toml` へ登録する。

## 7. External Platform Constraints

なし。Go の最低版は pathguard に合わせて 1.23 とする。

---

## Discussion Log

- **発端:** サプライチェーン対策の確認で、TOML ローダーが毎回引っかかる。自前のライブラリとして実装する案が出された。
- **現状調査（2026-11-03）:**
  - 外部の TOML ライブラリに依存するのは Go の稼働中 29 リポ（`BurntSushi/toml` v1.4.0〜v1.6.0）。ほかに `markdown-viewer` が `pelletier/go-toml/v2` を間接依存として持つ。
  - Python は標準の `tomllib` を使っている（`tomli` を代わりに読む箇所が 1 つ）。
  - 書き出しの利用はゼロ。使われている API は `DecodeFile` 31、`Undecoded()` 19、`Decode` 4、`IsDefined` 2。
- **残る外部依存:** TOML を外しても、小さなツールの多くには `spf13/cobra` 系の依存が残る。これは別の議題とした。
- **名前:** パッケージ名を `toml` にすると、移行が import の書き換えで済む。リポジトリ名は `nlink-jp/toml` とした。
- **API:** 引数は現行ライブラリと同じ形にした。`Undecoded()` を廃し、厳格な読み込みを既定にして、`UnknownKeysError` でキー一覧を返す形にした。
- **今は緩く読んでいるリポの扱い:** 互換を崩す変更は確認してから入れる方針に従い、ライブラリの既定は厳格にしたうえで、移行時にリポごとに `AllowUnknownKeys()` を付けるかどうか決めることにした。
- **対象版の修正:** 当初は TOML 1.0 を想定していた。手元の v1.6.0 が 1.1.0 を既定で受け付けると確認できたため、退行を避けて 1.1.0 を対象にした。
- **試験データ:** 適合試験データを取り込む案（A）と、すべて自前で書く案（B）を比べた。網羅性と見落としの少なさを重視し、データだけを固定・ハッシュ一覧つきで取り込む A に決めた。
- **リリース:** 移行で API が固まるまでは v0.x とする。
