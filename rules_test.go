package toml

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Each test here pins a rule that the Phase 2 review removed from the code
// without any other test failing (a mutation that survived the suite).

func TestATaggedFieldBeatsAnUntaggedOneAtTheSameDepth(t *testing.T) {
	type A struct {
		Name string `toml:"Name"`
	}
	type B struct{ Name string }
	var cfg struct {
		A
		B
	}
	mustDecode(t, "Name = 'x'\n", &cfg)
	if cfg.A.Name != "x" || cfg.B.Name != "" {
		t.Errorf("A.Name %q, B.Name %q", cfg.A.Name, cfg.B.Name)
	}
}

func TestTwoTaggedFieldsAtTheSameDepthTakeNoKey(t *testing.T) {
	type A struct {
		V string `toml:"v"`
	}
	type B struct {
		V string `toml:"v"`
	}
	var cfg struct {
		A
		B
	}
	var uk *UnknownKeysError
	if _, err := Decode("v = 'x'\n", &cfg); !errors.As(err, &uk) {
		t.Errorf("err = %v; a tie between tagged fields must take no key", err)
	}
}

type cyclic struct {
	*cyclic
	X int
}

func TestEmbeddedCyclesEnd(t *testing.T) {
	var cfg cyclic
	mustDecode(t, "X = 1\n", &cfg)
	if cfg.X != 1 {
		t.Errorf("X = %d", cfg.X)
	}
}

func TestTheFirstCaseInsensitiveMatchWins(t *testing.T) {
	var cfg struct {
		First  string `toml:"Mode"`
		Second string `toml:"MODE"`
	}
	mustDecode(t, "mode = 'x'\n", &cfg)
	if cfg.First != "x" || cfg.Second != "" {
		t.Errorf("First %q, Second %q: the first field in declaration order wins", cfg.First, cfg.Second)
	}
}

// A table reopened later in the document is walked with its first part, so
// its unknown keys are found out of document order and must be sorted back.
func TestUnknownKeysOfAReopenedTableAreInDocumentOrder(t *testing.T) {
	var cfg struct {
		A struct{} `toml:"A"`
		B struct{} `toml:"B"`
	}
	_, err := Decode("[A]\nu1 = 1\n[B]\nu2 = 1\n[A.sub]\nu3 = 1\n", &cfg)
	var uk *UnknownKeysError
	if !errors.As(err, &uk) {
		t.Fatalf("err = %v", err)
	}
	var got []string
	for _, k := range uk.Keys {
		got = append(got, k.String())
	}
	if strings.Join(got, " ") != "A.u1 B.u2 A.sub A.sub.u3" {
		t.Errorf("unknown keys %q", got)
	}
}

// Sibling keys under a parent whose Key has spare capacity must not share
// it: the first sibling's key would become the second's.
func TestSiblingKeysDoNotShareTheirParent(t *testing.T) {
	var cfg struct {
		A struct {
			B struct {
				C struct{} `toml:"c"`
			} `toml:"b"`
		} `toml:"a"`
	}
	_, err := Decode("[a.b.c]\nx = 1\ny = 2\n", &cfg)
	var uk *UnknownKeysError
	if !errors.As(err, &uk) || len(uk.Keys) != 2 || uk.Keys[0].String() != "a.b.c.x" || uk.Keys[1].String() != "a.b.c.y" {
		t.Errorf("err = %v", err)
	}
}

func TestLocalDatetimesReachATextUnmarshaler(t *testing.T) {
	var cfg struct{ D, T, DT stamp }
	mustDecode(t, "D = 1979-05-27\nT = 07:32:00.5\nDT = 1979-05-27T07:32\n", &cfg)
	if cfg.D.text != "1979-05-27" || cfg.T.text != "07:32:00.5" || cfg.DT.text != "1979-05-27T07:32:00" {
		t.Errorf("decoded %+v", cfg)
	}
}

func TestAFieldTaggedDashTakesNoKeyAtAll(t *testing.T) {
	var cfg struct {
		Skipped string `toml:"-"`
	}
	for _, src := range []string{"Skipped = 'x'\n", "\"-\" = 'x'\n"} {
		var uk *UnknownKeysError
		if _, err := Decode(src, &cfg); !errors.As(err, &uk) || cfg.Skipped != "" {
			t.Errorf("Decode(%q): %v, Skipped = %q", src, err, cfg.Skipped)
		}
	}
}

func TestAKeyNamingAnUnexportedFieldIsUnknown(t *testing.T) {
	var cfg struct {
		secret string
		Public string
	}
	var uk *UnknownKeysError
	if _, err := Decode("secret = 'x'\nPublic = 'y'\n", &cfg); !errors.As(err, &uk) || cfg.secret != "" {
		t.Errorf("err = %v, secret = %q", err, cfg.secret)
	}
}

func TestTheLargestIntegerIsNotAFloat64(t *testing.T) {
	var cfg struct{ F float64 }
	var de *DecodeError
	if _, err := Decode("F = 9223372036854775807\n", &cfg); !errors.As(err, &de) {
		t.Errorf("2^63-1 into float64: %v", err)
	}
}

func TestLocalDateTimeInUsesTheLocation(t *testing.T) {
	dt := LocalDateTime{LocalDate{1979, time.May, 27}, LocalTime{7, 32, 0, 0}}
	tokyo := time.FixedZone("", 9*3600)
	got := dt.In(tokyo)
	if _, off := got.Zone(); off != 9*3600 || !got.Equal(time.Date(1979, 5, 26, 22, 32, 0, 0, time.UTC)) {
		t.Errorf("In(+09:00) = %v", got)
	}
}

// /dev/zero never ends; DecodeFile must stop after the limit and say why,
// not read until memory runs out.
func TestDecodeFileStopsReadingAtTheLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no /dev/zero")
	}
	if _, err := os.Stat("/dev/zero"); err != nil {
		t.Skip("no /dev/zero")
	}
	var cfg struct{}
	var pe *ParseError
	if _, err := DecodeFile("/dev/zero", &cfg); !errors.As(err, &pe) || !strings.Contains(pe.Msg, "1 MiB") {
		t.Errorf("err = %v", err)
	}
}
