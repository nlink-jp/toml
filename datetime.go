package toml

import (
	"fmt"
	"strings"
	"time"
)

// An offset date-time decodes into time.Time. The three local kinds have no
// offset, so they cannot name an instant; each decodes into its own type
// below rather than into a time.Time whose zone would be a guess.

// LocalDate is a TOML local date: a day, with no time and no offset.
type LocalDate struct {
	Year  int
	Month time.Month
	Day   int
}

// LocalTime is a TOML local time: a time of day, with no date and no offset.
// Second may be 60 (a leap second).
type LocalTime struct {
	Hour, Minute, Second int
	Nanosecond           int
}

// LocalDateTime is a TOML local date-time: a date and a time, with no offset.
type LocalDateTime struct {
	Date LocalDate
	Time LocalTime
}

// String writes the date as TOML does: 1979-05-27.
func (d LocalDate) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day)
}

// String writes the time as TOML does: 07:32:00, with a fraction only when
// there is one.
func (t LocalTime) String() string {
	s := fmt.Sprintf("%02d:%02d:%02d", t.Hour, t.Minute, t.Second)
	if t.Nanosecond != 0 {
		s += strings.TrimRight(fmt.Sprintf(".%09d", t.Nanosecond), "0")
	}
	return s
}

// String writes the date-time as TOML does: 1979-05-27T07:32:00.
func (dt LocalDateTime) String() string { return dt.Date.String() + "T" + dt.Time.String() }

// In is the instant this date-time names in loc. A leap second becomes the
// first second of the next minute, as time.Date normalises it.
func (dt LocalDateTime) In(loc *time.Location) time.Time {
	return time.Date(dt.Date.Year, dt.Date.Month, dt.Date.Day,
		dt.Time.Hour, dt.Time.Minute, dt.Time.Second, dt.Time.Nanosecond, loc)
}

func (d datetime) localDate() LocalDate {
	return LocalDate{Year: d.year, Month: time.Month(d.month), Day: d.day}
}

func (d datetime) localTime() LocalTime {
	return LocalTime{Hour: d.hour, Minute: d.minute, Second: d.second, Nanosecond: d.nanos}
}

// instant is an offset date-time as a time.Time. A zero offset is UTC; any
// other keeps the offset as written, unnamed.
func (d datetime) instant() time.Time {
	return time.Date(d.year, time.Month(d.month), d.day, d.hour, d.minute, d.second, d.nanos, zoneAt(d.offset))
}

// zoneAt is the location for an offset as written: UTC for zero, otherwise an
// unnamed fixed zone. Never time.Local's name, which a document cannot mean.
func zoneAt(offset int) *time.Location {
	if offset == 0 {
		return time.UTC
	}
	return time.FixedZone("", offset)
}

// text is the value as TOML writes it, for a TextUnmarshaler target.
func (n *node) datetimeText() string {
	switch n.kind {
	case kDateLocal:
		return n.dt.localDate().String()
	case kTimeLocal:
		return n.dt.localTime().String()
	case kDatetimeLocal:
		return LocalDateTime{n.dt.localDate(), n.dt.localTime()}.String()
	}
	s := LocalDateTime{n.dt.localDate(), n.dt.localTime()}.String()
	if n.dt.offset == 0 {
		return s + "Z"
	}
	off, sign := n.dt.offset, '+'
	if off < 0 {
		off, sign = -off, '-'
	}
	return fmt.Sprintf("%s%c%02d:%02d", s, sign, off/3600, off/60%60)
}
