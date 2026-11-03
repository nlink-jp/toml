package toml_test

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nlink-jp/toml"
)

// A config struct filled with defaults first and decoded second keeps every
// default the document does not set.
func ExampleDecode() {
	type Config struct {
		Model struct {
			Name        string  `toml:"name"`
			Temperature float64 `toml:"temperature"`
		} `toml:"model"`
		Timeout time.Duration `toml:"timeout"`
	}
	cfg := Config{Timeout: 30 * time.Second}
	cfg.Model.Temperature = 0.7

	doc := `
timeout = "2m"

[model]
name = "gemma"
`
	if _, err := toml.Decode(doc, &cfg); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(cfg.Model.Name, cfg.Model.Temperature, cfg.Timeout)
	// Output: gemma 0.7 2m0s
}

// Unknown keys come back together, in document order, after every known key
// has been decoded. A tool can name the ones it has removed.
func ExampleUnknownKeysError() {
	var cfg struct {
		Log struct {
			Level string `toml:"level"`
		} `toml:"log"`
	}
	removed := map[string]string{"workspace.workspace_dir": "removed in v0.4; delete it"}

	doc := `
[workspace]
workspace_dir = "/tmp/x"

[log]
level = "debug"
levle = "info"
`
	_, err := toml.Decode(doc, &cfg)
	var unknown *toml.UnknownKeysError
	if errors.As(err, &unknown) {
		for _, k := range unknown.Keys {
			why, ok := removed[k.String()]
			if !ok {
				why = "not a setting this tool reads"
			}
			fmt.Printf("%s: %s\n", k, why)
		}
	}
	fmt.Println("log.level =", cfg.Log.Level)
	// Output:
	// workspace: not a setting this tool reads
	// workspace.workspace_dir: removed in v0.4; delete it
	// log.levle: not a setting this tool reads
	// log.level = debug
}

func ExampleAllowUnknownKeys() {
	var cfg struct {
		Name string `toml:"name"`
	}
	_, err := toml.Decode("name = 'x'\nfuture_setting = 1\n", &cfg, toml.AllowUnknownKeys())
	fmt.Println(cfg.Name, err)
	// Output: x <nil>
}

// A pointer field tells "the file set it" from "absent"; IsDefined answers
// the same question by key, for reporting where a setting came from.
func ExampleMetaData_IsDefined() {
	var cfg struct {
		Model struct {
			Name      string `toml:"name"`
			MaxTokens *int   `toml:"max_tokens"`
		} `toml:"model"`
	}
	md, _ := toml.Decode("[model]\nname = 'gemma'\n", &cfg)
	for _, key := range []string{"model.name", "model.max_tokens"} {
		fmt.Println(key, md.IsDefined(strings.Split(key, ".")...))
	}
	fmt.Println(cfg.Model.MaxTokens == nil)
	// Output:
	// model.name true
	// model.max_tokens false
	// true
}

func ExampleDecodeError() {
	var cfg struct {
		Server struct {
			Port int `toml:"port"`
		} `toml:"server"`
	}
	_, err := toml.Decode("[server]\nport = 'eighty'\n", &cfg)
	var de *toml.DecodeError
	if errors.As(err, &de) {
		fmt.Println(de.Key, de.Line)
	}
	fmt.Println(err)
	// Output:
	// server.port 2
	// toml: line 2: key server.port: cannot decode a string into int
}

func ExampleParseError() {
	var cfg map[string]any
	_, err := toml.Decode("a = 1\na = 2\n", &cfg)
	var pe *toml.ParseError
	if errors.As(err, &pe) {
		fmt.Println(pe.Line, pe.Column)
	}
	fmt.Println(err)
	// Output:
	// 2 1
	// toml: line 2, column 1: key a is defined twice
}

// command is either an argv array or one shell string.
type command struct {
	Argv  []string
	Shell string
}

func (c *command) UnmarshalTOML(v any) error {
	switch x := v.(type) {
	case string:
		c.Shell = x
		return nil
	case []any:
		for _, e := range x {
			s, ok := e.(string)
			if !ok {
				return fmt.Errorf("command array elements must be strings, got %T", e)
			}
			c.Argv = append(c.Argv, s)
		}
		return nil
	}
	return fmt.Errorf("command must be a string or an array of strings, got %T", v)
}

func ExampleUnmarshaler() {
	var cfg struct {
		Build command `toml:"build"`
		Test  command `toml:"test"`
	}
	_, err := toml.Decode("build = ['make', 'build']\ntest = 'go test ./... | tee log'\n", &cfg)
	fmt.Println(cfg.Build.Argv, cfg.Test.Shell, err)
	// Output: [make build] go test ./... | tee log <nil>
}

// A local date-time names no instant until the caller chooses the zone.
func ExampleLocalDateTime_In() {
	var cfg struct {
		Start toml.LocalDateTime `toml:"start"`
		At    time.Time          `toml:"at"`
	}
	_, err := toml.Decode("start = 2026-11-03T09:00\nat = 2026-11-03T09:00+09:00\n", &cfg)
	if err != nil {
		fmt.Println(err)
		return
	}
	tokyo := time.FixedZone("", 9*3600)
	fmt.Println(cfg.Start)
	fmt.Println(cfg.Start.In(tokyo).Equal(cfg.At))
	// Output:
	// 2026-11-03T09:00:00
	// true
}
