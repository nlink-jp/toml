package toml

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// These tests restate how the organization's tools use BurntSushi/toml today,
// so that moving them to this library is a change of import, not of meaning.
// Each is modelled on a real consumer; the shapes are theirs, the code is not.

// pcap-analyzer-mcp: defaults first, the file decoded on top, a Duration that
// wraps time.Duration through UnmarshalText, and unknown keys turned into a
// message that names keys the server has removed.
type wrappedDuration struct{ time.Duration }

func (d *wrappedDuration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", b, err)
	}
	d.Duration = v
	return nil
}

func TestCompatDefaultsThenFileThenRemovedKeys(t *testing.T) {
	type config struct {
		Output struct {
			MaxBytes int `toml:"max_bytes"`
		} `toml:"output"`
		Limits struct {
			Timeout wrappedDuration `toml:"timeout"`
		} `toml:"limits"`
		Log struct {
			Level string `toml:"level"`
			File  string `toml:"file"`
		} `toml:"log"`
	}
	cfg := config{}
	cfg.Output.MaxBytes = 65536
	cfg.Limits.Timeout.Duration = 30 * time.Second
	cfg.Log.Level = "info"

	removed := map[string]string{"output.inline_max_bytes": "removed in ADR-0009"}
	src := "[output]\ninline_max_bytes = 1\n[limits]\ntimeout = '2m'\n[log]\nfile = '/tmp/x.log'\n"
	_, err := Decode(src, &cfg)

	var uk *UnknownKeysError
	if !errors.As(err, &uk) || len(uk.Keys) != 1 {
		t.Fatalf("err = %v", err)
	}
	if why, ok := removed[uk.Keys[0].String()]; !ok || why == "" {
		t.Errorf("the removed key is not found by its String(): %q", uk.Keys[0])
	}
	if cfg.Output.MaxBytes != 65536 || cfg.Log.Level != "info" {
		t.Errorf("defaults lost: %+v", cfg)
	}
	if cfg.Limits.Timeout.Duration != 2*time.Minute || cfg.Log.File != "/tmp/x.log" {
		t.Errorf("file values lost: %+v", cfg)
	}

	_, err = Decode("[limits]\ntimeout = 'soon'\n", &cfg)
	var de *DecodeError
	if !errors.As(err, &de) || de.Key.String() != "limits.timeout" || !strings.Contains(err.Error(), `invalid duration "soon"`) {
		t.Errorf("a bad duration: %v", err)
	}
}

// task-clock: a named time.Duration with UnmarshalText on its pointer, which
// refuses negative values.
type namedDuration time.Duration

func (d *namedDuration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	if v < 0 {
		return fmt.Errorf("negative duration %q", b)
	}
	*d = namedDuration(v)
	return nil
}

func TestCompatNamedDuration(t *testing.T) {
	var cfg struct {
		Interval namedDuration  `toml:"interval"`
		Grace    *namedDuration `toml:"grace"`
	}
	mustDecode(t, "interval = '90s'\ngrace = '5s'\n", &cfg)
	if time.Duration(cfg.Interval) != 90*time.Second || cfg.Grace == nil || time.Duration(*cfg.Grace) != 5*time.Second {
		t.Errorf("decoded %+v", cfg)
	}
	decodeErr(t, "interval = '-1s'\n", &cfg)
}

// task-clock: a command that is either an argv array or one shell string,
// through UnmarshalTOML.
type commandValue struct {
	Argv  []string
	Str   string
	IsStr bool
}

func (c *commandValue) UnmarshalTOML(v any) error {
	switch x := v.(type) {
	case string:
		c.Str, c.IsStr = x, true
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
	return fmt.Errorf("command must be a string array or a string, got %T", v)
}

func TestCompatUnmarshalTOML(t *testing.T) {
	type task struct {
		Name    string       `toml:"name"`
		Command commandValue `toml:"command"`
	}
	var cfg struct {
		Tasks []task `toml:"task"`
	}
	mustDecode(t, "[[task]]\nname = 'a'\ncommand = ['echo', 'hi']\n[[task]]\nname = 'b'\ncommand = 'echo hi | wc'\n", &cfg)
	if len(cfg.Tasks) != 2 || len(cfg.Tasks[0].Command.Argv) != 2 || !cfg.Tasks[1].Command.IsStr {
		t.Errorf("decoded %+v", cfg)
	}
	_, err := Decode("[[task]]\ncommand = [1]\n", &cfg)
	var de *DecodeError
	if !errors.As(err, &de) || de.Key.String() != "task.command" || !strings.Contains(err.Error(), "must be strings, got int64") {
		t.Errorf("err = %v", err)
	}
}

// gem-agent and lagent: pointer fields tell "the file set it" from "absent",
// and IsDefined, fed dotted names split on ".", records where a setting came
// from.
func TestCompatProvenanceThroughIsDefined(t *testing.T) {
	var cfg struct {
		Model struct {
			Name      string `toml:"name"`
			MaxTokens *int   `toml:"max_tokens"`
		} `toml:"model"`
		Sandbox struct {
			Enabled *bool `toml:"enabled"`
		} `toml:"sandbox"`
	}
	md := mustDecode(t, "[model]\nname = 'm'\n[sandbox]\nenabled = false\n", &cfg)
	from := map[string]bool{}
	for _, k := range []string{"model.name", "model.max_tokens", "sandbox.enabled"} {
		from[k] = md.IsDefined(strings.Split(k, ".")...)
	}
	if !from["model.name"] || from["model.max_tokens"] || !from["sandbox.enabled"] {
		t.Errorf("IsDefined: %v", from)
	}
	if cfg.Model.MaxTokens != nil || cfg.Sandbox.Enabled == nil || *cfg.Sandbox.Enabled {
		t.Errorf("pointers: %+v", cfg)
	}
}

// voice-studio-mcp: a table of entries keyed by name, each with a nested
// map, and unknown keys reported by printing the list with %v.
func TestCompatTableOfEntries(t *testing.T) {
	type castEntry struct {
		SpeakerUUID string         `toml:"speaker_uuid"`
		StyleID     int            `toml:"style_id"`
		Styles      map[string]int `toml:"styles"`
	}
	var c struct {
		Characters map[string]castEntry `toml:"characters"`
	}
	src := `
[characters.narrator]
speaker_uuid = "u-1"
style_id = 3
styles = { calm = 3, loud = 4 }

[characters."少女"]
speaker_uuid = "u-2"
style_id = 7
pitch = 1.2
`
	_, err := Decode(src, &c)
	var uk *UnknownKeysError
	if !errors.As(err, &uk) {
		t.Fatalf("err = %v", err)
	}
	if got := fmt.Sprintf("%v", uk.Keys); got != `[characters."少女".pitch]` {
		t.Errorf("unknown keys printed as %s", got)
	}
	n := c.Characters["narrator"]
	if n.SpeakerUUID != "u-1" || n.Styles["loud"] != 4 || c.Characters["少女"].StyleID != 7 {
		t.Errorf("decoded %+v", c)
	}
}

// Every consumer wraps the error with %v or %w and shows it to an operator;
// what they see must say where and what.
func TestCompatErrorsReadWellWhenWrapped(t *testing.T) {
	var cfg struct {
		Port int `toml:"port"`
	}
	_, err := Decode("port = 80\nport = 81\n", &cfg)
	wrapped := fmt.Errorf("parse %s: %w", "/etc/x/config.toml", err)
	if wrapped.Error() != "parse /etc/x/config.toml: toml: line 2, column 1: key port is defined twice" {
		t.Errorf("wrapped: %q", wrapped.Error())
	}
	var pe *ParseError
	if !errors.As(wrapped, &pe) {
		t.Error("the ParseError is lost through %w")
	}
}
