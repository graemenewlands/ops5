package model

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	// Matches numbers followed by optional whitespace and unit name: e.g. "10ms", "10 ms", "10 minutes"
	durationRegex = regexp.MustCompile(`^(?i)^\s*([0-9]+(?:\.[0-9]+)?)\s*([a-z]+)\s*$`)

	// Matches 12-hour wall-clock times: e.g. "10:10am", "10:10 am", "6:00PM", "06:30am"
	wallClock12Regex = regexp.MustCompile(`^(?i)^\s*([0-1]?[0-9]):([0-5][0-9])(?::([0-5][0-9]))?\s*([ap]m)\s*$`)

	// Matches 24-hour wall-clock times: e.g. "14:30", "06:00", "23:59:59"
	wallClock24Regex = regexp.MustCompile(`^\s*([0-2]?[0-9]):([0-5][0-9])(?::([0-5][0-9]))?\s*$`)
)

// ParseDuration parses human-readable duration strings with extended unit support.
// Allowed units: ms, millis, milliseconds, s, sec, secs, seconds, m, min, mins, minutes, h, hr, hrs, hours, d, day, days.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration string")
	}

	m := durationRegex.FindStringSubmatch(s)
	if len(m) != 3 {
		return 0, fmt.Errorf("invalid duration format %q", s)
	}

	numStr, unit := m[1], strings.ToLower(m[2])
	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric value %q: %w", numStr, err)
	}

	switch unit {
	case "ms", "milli", "millis", "millisecond", "milliseconds":
		return time.Duration(val * float64(time.Millisecond)), nil
	case "s", "sec", "secs", "second", "seconds":
		return time.Duration(val * float64(time.Second)), nil
	case "m", "min", "mins", "minute", "minutes":
		return time.Duration(val * float64(time.Minute)), nil
	case "h", "hr", "hrs", "hour", "hours":
		return time.Duration(val * float64(time.Hour)), nil
	case "d", "day", "days":
		return time.Duration(val * 24 * float64(time.Hour)), nil
	default:
		return 0, fmt.Errorf("unsupported duration unit %q", unit)
	}
}

// ParseWallClock parses a 12-hour or 24-hour wall-clock string and computes its next occurrence
// relative to refTime in the specified location.
func ParseWallClock(s string, refTime time.Time, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = refTime.Location()
	}
	refInLoc := refTime.In(loc)
	s = strings.TrimSpace(s)

	var hour, minute, second int

	if m := wallClock12Regex.FindStringSubmatch(s); len(m) == 5 {
		h, _ := strconv.Atoi(m[1])
		min, _ := strconv.Atoi(m[2])
		sec := 0
		if m[3] != "" {
			sec, _ = strconv.Atoi(m[3])
		}
		meridiem := strings.ToLower(m[4])

		if h < 1 || h > 12 {
			return time.Time{}, fmt.Errorf("hour out of range for 12-hour clock: %d", h)
		}

		if meridiem == "pm" {
			if h < 12 {
				h += 12
			}
		} else { // "am"
			if h == 12 {
				h = 0
			}
		}

		hour, minute, second = h, min, sec
	} else if m := wallClock24Regex.FindStringSubmatch(s); len(m) == 4 {
		h, _ := strconv.Atoi(m[1])
		min, _ := strconv.Atoi(m[2])
		sec := 0
		if m[3] != "" {
			sec, _ = strconv.Atoi(m[3])
		}

		if h < 0 || h > 23 {
			return time.Time{}, fmt.Errorf("hour out of range for 24-hour clock: %d", h)
		}

		hour, minute, second = h, min, sec
	} else {
		return time.Time{}, fmt.Errorf("invalid wall-clock format %q", s)
	}

	target := time.Date(
		refInLoc.Year(), refInLoc.Month(), refInLoc.Day(),
		hour, minute, second, 0, loc,
	)

	// If the target wall-clock time has already passed today, advance to tomorrow
	if target.Before(refInLoc) {
		target = target.AddDate(0, 0, 1)
	}

	return target, nil
}

// ParseTTL parses a raw Value (integer, string, or symbol) into an absolute Unix timestamp in milliseconds.
// If raw is 0, returns 0 (immortal).
// If raw is a duration string (e.g. "10ms", "30m"), returns nowMs + duration.
// If raw is a wall-clock string (e.g. "10:10am"), returns the next occurrence in unix milliseconds.
func ParseTTL(raw Value, nowMs int64, loc *time.Location) (int64, error) {
	if loc == nil {
		loc = time.Local
	}
	refTime := time.UnixMilli(nowMs).In(loc)

	switch raw.Type() {
	case TypeInteger:
		v := raw.Raw().(int64)
		if v <= 0 {
			return 0, nil
		}
		// If already an absolute Unix millisecond timestamp (> year 2000 in ms: ~946684800000)
		if v > 946684800000 {
			return v, nil
		}
		// Otherwise treated as a raw millisecond relative duration
		return nowMs + v, nil

	case TypeString, TypeSymbol:
		str, _ := raw.Text()
		str = strings.TrimSpace(str)
		if str == "" || str == "0" {
			return 0, nil
		}

		// Try duration first
		if d, err := ParseDuration(str); err == nil {
			return nowMs + d.Milliseconds(), nil
		}

		// Try wall-clock time
		if t, err := ParseWallClock(str, refTime, loc); err == nil {
			return t.UnixMilli(), nil
		}

		// Try parsing as integer literal
		if v, err := strconv.ParseInt(str, 10, 64); err == nil {
			if v <= 0 {
				return 0, nil
			}
			if v > 946684800000 {
				return v, nil
			}
			return nowMs + v, nil
		}

		return 0, fmt.Errorf("cannot parse %q as duration, wall-clock time, or millisecond timestamp", str)

	default:
		return 0, fmt.Errorf("invalid type %s for TTL attribute", raw.Type())
	}
}
