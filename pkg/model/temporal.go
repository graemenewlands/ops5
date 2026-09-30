package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// NormalizeTemporalOp normalizes a temporal operation name to canonical lowercase without hyphens/underscores.
func NormalizeTemporalOp(op string) string {
	op = strings.ToLower(op)
	op = strings.ReplaceAll(op, "-", "")
	op = strings.ReplaceAll(op, "_", "")
	switch op {
	case "dayadd":
		return "dayadd"
	case "monthadd":
		return "monthadd"
	case "yearadd":
		return "yearadd"
	case "houradd":
		return "houradd"
	case "minuteadd":
		return "minuteadd"
	case "secondsadd", "secondadd":
		return "secondsadd"
	case "datediff":
		return "datediff"
	case "minutes", "minute":
		return "minutes"
	case "hours", "hour":
		return "hours"
	case "days", "day":
		return "days"
	case "utc":
		return "utc"
	case "datetime":
		return "datetime"
	case "date":
		return "date"
	}
	return op
}

// IsTemporalOp returns true if the operation is a known temporal function.
func IsTemporalOp(op string) bool {
	norm := NormalizeTemporalOp(op)
	switch norm {
	case "dayadd", "monthadd", "yearadd", "houradd", "minuteadd", "secondsadd",
		"datediff", "minutes", "hours", "days", "utc", "datetime", "date":
		return true
	default:
		return false
	}
}

func isDateInt(n int64) bool {
	if n < 10000101 || n > 99991231 {
		return false
	}
	month := int((n % 10000) / 100)
	day := int(n % 100)
	return month >= 1 && month <= 12 && day >= 1 && day <= 31
}

func toInt64(v Value) (int64, bool) {
	if v.Type() == TypeInteger {
		return v.Raw().(int64), true
	}
	if v.Type() == TypeFloat {
		return int64(v.Raw().(float64)), true
	}
	if v.Type() == TypeString {
		if n, err := strconv.ParseInt(strings.TrimSpace(v.Raw().(string)), 10, 64); err == nil {
			return n, true
		}
	}
	return 0, false
}

func isTemporalOrDate(v Value) bool {
	if v.IsDate() || v.IsDateTime() || v.IsDateUTCTime() {
		return true
	}
	if v.Type() == TypeInteger && isDateInt(v.Raw().(int64)) {
		return true
	}
	if v.Type() == TypeString {
		s := v.Raw().(string)
		isDT, _ := IsDateTimeString(s)
		return isDT || IsDateString(s)
	}
	return false
}

func toTime(v Value) (time.Time, ValueType, error) {
	if v.IsDate() {
		return v.Time(), TypeDate, nil
	}
	if v.IsDateTime() {
		return v.Time(), TypeDateTime, nil
	}
	if v.IsDateUTCTime() {
		return v.Time(), TypeDateUTCTime, nil
	}
	if v.Type() == TypeInteger {
		n := v.Raw().(int64)
		if isDateInt(n) {
			d, err := ParseDate(n)
			if err == nil {
				return d.Time(), TypeDate, nil
			}
		}
		return time.Time{}, 0, fmt.Errorf("integer %d is not a valid YYYYMMDD date", n)
	}
	if v.Type() == TypeString {
		s := strings.TrimSpace(v.Raw().(string))
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t, TypeDateUTCTime, nil
		}
		if t, err := parseDateTimeString(s, time.Local); err == nil {
			return t, TypeDateTime, nil
		}
		if d, err := ParseDateString(s); err == nil {
			return d.Time(), TypeDate, nil
		}
		return time.Time{}, 0, fmt.Errorf("string %q is not a valid date or datetime", s)
	}
	return time.Time{}, 0, fmt.Errorf("cannot convert value of type %s to time", v.Type())
}

// ExtractTemporalAndInt extracts a base date/time value and an integer offset from two values in either order.
func ExtractTemporalAndInt(a, b Value) (Value, int64, error) {
	if isTemporalOrDate(a) && !isTemporalOrDate(b) {
		if n, ok := toInt64(b); ok {
			return a, n, nil
		}
	}
	if isTemporalOrDate(b) && !isTemporalOrDate(a) {
		if n, ok := toInt64(a); ok {
			return b, n, nil
		}
	}
	if a.Type() == TypeInteger && b.Type() == TypeInteger {
		i1 := a.Raw().(int64)
		i2 := b.Raw().(int64)
		if isDateInt(i1) && !isDateInt(i2) {
			d, err := ParseDate(i1)
			if err == nil {
				return d, i2, nil
			}
		}
		if isDateInt(i2) && !isDateInt(i1) {
			d, err := ParseDate(i2)
			if err == nil {
				return d, i1, nil
			}
		}
		// If both or neither match strict isDateInt, treat first as date, second as offset
		d, err := ParseDate(i1)
		if err == nil {
			return d, i2, nil
		}
	}
	// Also attempt if a is temporal and b has int conversion
	if isTemporalOrDate(a) {
		if n, ok := toInt64(b); ok {
			return a, n, nil
		}
	}
	if isTemporalOrDate(b) {
		if n, ok := toInt64(a); ok {
			return b, n, nil
		}
	}
	return Value{}, 0, fmt.Errorf("expected date/datetime and integer offset, got %v and %v", a, b)
}

// DayAdd adds intN days (positive or negative) to a date or datetime value.
func DayAdd(v Value, intN int64) (Value, error) {
	t, typ, err := toTime(v)
	if err != nil {
		return Value{}, err
	}
	newT := t.AddDate(0, 0, int(intN))
	switch typ {
	case TypeDate:
		return NewDateFromTime(newT), nil
	case TypeDateUTCTime:
		return NewDateUTCTimeFromTime(newT), nil
	default:
		return NewDateTimeFromTime(newT), nil
	}
}

func addMonthsClamped(t time.Time, months int64) time.Time {
	y := t.Year()
	m := int(t.Month()) // 1..12
	d := t.Day()

	totalM := int64(y)*12 + int64(m-1) + months
	targetYear := int(totalM / 12)
	targetMonth0 := int(totalM % 12)
	if targetMonth0 < 0 {
		targetMonth0 += 12
		targetYear -= 1
	}
	targetMonth := time.Month(targetMonth0 + 1)

	maxDays := time.Date(targetYear, targetMonth+1, 0, 0, 0, 0, 0, time.UTC).Day()
	targetDay := d
	if targetDay > maxDays {
		targetDay = maxDays
	}

	return time.Date(targetYear, targetMonth, targetDay, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
}

func addYearsClamped(t time.Time, years int64) time.Time {
	targetYear := t.Year() + int(years)
	targetMonth := t.Month()
	d := t.Day()

	maxDays := time.Date(targetYear, targetMonth+1, 0, 0, 0, 0, 0, time.UTC).Day()
	targetDay := d
	if targetDay > maxDays {
		targetDay = maxDays
	}

	return time.Date(targetYear, targetMonth, targetDay, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
}

// MonthAdd adds intN months (positive or negative) to a date or datetime value with end-of-month clamping.
func MonthAdd(v Value, intN int64) (Value, error) {
	t, typ, err := toTime(v)
	if err != nil {
		return Value{}, err
	}
	newT := addMonthsClamped(t, intN)
	switch typ {
	case TypeDate:
		return NewDateFromTime(newT), nil
	case TypeDateUTCTime:
		return NewDateUTCTimeFromTime(newT), nil
	default:
		return NewDateTimeFromTime(newT), nil
	}
}

// YearAdd adds intN years (positive or negative) to a date or datetime value with leap-year clamping.
func YearAdd(v Value, intN int64) (Value, error) {
	t, typ, err := toTime(v)
	if err != nil {
		return Value{}, err
	}
	newT := addYearsClamped(t, intN)
	switch typ {
	case TypeDate:
		return NewDateFromTime(newT), nil
	case TypeDateUTCTime:
		return NewDateUTCTimeFromTime(newT), nil
	default:
		return NewDateTimeFromTime(newT), nil
	}
}

// HourAdd adds intN hours (positive or negative) to a date or datetime value.
func HourAdd(v Value, intN int64) (Value, error) {
	t, typ, err := toTime(v)
	if err != nil {
		return Value{}, err
	}
	newT := t.Add(time.Duration(intN) * time.Hour)
	switch typ {
	case TypeDate:
		if newT.Hour() == 0 && newT.Minute() == 0 && newT.Second() == 0 {
			return NewDateFromTime(newT), nil
		}
		return NewDateTimeFromTime(newT), nil
	case TypeDateUTCTime:
		return NewDateUTCTimeFromTime(newT), nil
	default:
		return NewDateTimeFromTime(newT), nil
	}
}

// MinuteAdd adds intN minutes (positive or negative) to a date or datetime value.
func MinuteAdd(v Value, intN int64) (Value, error) {
	t, typ, err := toTime(v)
	if err != nil {
		return Value{}, err
	}
	newT := t.Add(time.Duration(intN) * time.Minute)
	switch typ {
	case TypeDate:
		if newT.Hour() == 0 && newT.Minute() == 0 && newT.Second() == 0 {
			return NewDateFromTime(newT), nil
		}
		return NewDateTimeFromTime(newT), nil
	case TypeDateUTCTime:
		return NewDateUTCTimeFromTime(newT), nil
	default:
		return NewDateTimeFromTime(newT), nil
	}
}

// SecondsAdd adds intN seconds (positive or negative) to a date or datetime value.
func SecondsAdd(v Value, intN int64) (Value, error) {
	t, typ, err := toTime(v)
	if err != nil {
		return Value{}, err
	}
	newT := t.Add(time.Duration(intN) * time.Second)
	switch typ {
	case TypeDate:
		if newT.Hour() == 0 && newT.Minute() == 0 && newT.Second() == 0 {
			return NewDateFromTime(newT), nil
		}
		return NewDateTimeFromTime(newT), nil
	case TypeDateUTCTime:
		return NewDateUTCTimeFromTime(newT), nil
	default:
		return NewDateTimeFromTime(newT), nil
	}
}

// SecondAdd is an alias for SecondsAdd.
func SecondAdd(v Value, intN int64) (Value, error) {
	return SecondsAdd(v, intN)
}

// DateDiff calculates the difference in seconds between v1 and v2 (v1 - v2).
// A positive result indicates that v1 is later than v2.
func DateDiff(v1, v2 Value) (Value, error) {
	t1, _, err1 := toTime(v1)
	if err1 != nil {
		return Value{}, fmt.Errorf("DateDiff: invalid first argument %v: %w", v1, err1)
	}
	t2, _, err2 := toTime(v2)
	if err2 != nil {
		return Value{}, fmt.Errorf("DateDiff: invalid second argument %v: %w", v2, err2)
	}
	diff := t1.Sub(t2)
	secs := int64(diff.Seconds())
	return NewInt(secs), nil
}

// Minutes converts a quantity of seconds to whole minutes.
func Minutes(seconds int64) Value {
	return NewInt(seconds / 60)
}

// MinutesValue converts a seconds Value (integer, float, or string) to minutes.
func MinutesValue(v Value) (Value, error) {
	if v.Type() == TypeInteger {
		return NewInt(v.Raw().(int64) / 60), nil
	}
	if v.Type() == TypeFloat {
		return NewFloat(v.Raw().(float64) / 60.0), nil
	}
	if n, ok := toInt64(v); ok {
		return NewInt(n / 60), nil
	}
	return Value{}, fmt.Errorf("cannot convert %v to minutes", v)
}

// Hours converts a quantity of seconds to whole hours.
func Hours(seconds int64) Value {
	return NewInt(seconds / 3600)
}

// HoursValue converts a seconds Value (integer, float, or string) to hours.
func HoursValue(v Value) (Value, error) {
	if v.Type() == TypeInteger {
		return NewInt(v.Raw().(int64) / 3600), nil
	}
	if v.Type() == TypeFloat {
		return NewFloat(v.Raw().(float64) / 3600.0), nil
	}
	if n, ok := toInt64(v); ok {
		return NewInt(n / 3600), nil
	}
	return Value{}, fmt.Errorf("cannot convert %v to hours", v)
}

// Days converts a quantity of seconds to whole days.
func Days(seconds int64) Value {
	return NewInt(seconds / 86400)
}

// DaysValue converts a seconds Value (integer, float, or string) to days.
func DaysValue(v Value) (Value, error) {
	if v.Type() == TypeInteger {
		return NewInt(v.Raw().(int64) / 86400), nil
	}
	if v.Type() == TypeFloat {
		return NewFloat(v.Raw().(float64) / 86400.0), nil
	}
	if n, ok := toInt64(v); ok {
		return NewInt(n / 86400), nil
	}
	return Value{}, fmt.Errorf("cannot convert %v to days", v)
}

// DayAdd method on Value.
func (v Value) DayAdd(intN int64) (Value, error) {
	return DayAdd(v, intN)
}

// MonthAdd method on Value.
func (v Value) MonthAdd(intN int64) (Value, error) {
	return MonthAdd(v, intN)
}

// YearAdd method on Value.
func (v Value) YearAdd(intN int64) (Value, error) {
	return YearAdd(v, intN)
}

// HourAdd method on Value.
func (v Value) HourAdd(intN int64) (Value, error) {
	return HourAdd(v, intN)
}

// MinuteAdd method on Value.
func (v Value) MinuteAdd(intN int64) (Value, error) {
	return MinuteAdd(v, intN)
}

// SecondsAdd method on Value.
func (v Value) SecondsAdd(intN int64) (Value, error) {
	return SecondsAdd(v, intN)
}

// SecondAdd method on Value.
func (v Value) SecondAdd(intN int64) (Value, error) {
	return SecondsAdd(v, intN)
}

// DateDiff method on Value.
func (v Value) DateDiff(v2 Value) (Value, error) {
	return DateDiff(v, v2)
}

// Minutes method on Value.
func (v Value) Minutes() (Value, error) {
	return MinutesValue(v)
}

// Hours method on Value.
func (v Value) Hours() (Value, error) {
	return HoursValue(v)
}

// Days method on Value.
func (v Value) Days() (Value, error) {
	return DaysValue(v)
}

// EvaluateTemporalExpr evaluates a TemporalExpr against variable bindings.
func EvaluateTemporalExpr(te *TemporalExpr, bindings map[string]Value) (Value, error) {
	if te == nil {
		return NewSymbol("nil"), nil
	}
	op := NormalizeTemporalOp(te.Op)
	resolvedArgs := make([]Value, len(te.Args))
	for i, a := range te.Args {
		resolvedArgs[i] = ResolveValue(a, bindings)
		if resolvedArgs[i].IsVariable() {
			return Value{}, fmt.Errorf("unbound variable in temporal function %s: %s", op, resolvedArgs[i].VariableName())
		}
	}

	switch op {
	case "utc":
		if len(resolvedArgs) == 0 {
			return NewSymbol("nil"), fmt.Errorf("utc expects 1 argument")
		}
		return ConvertToDateUTCTime(resolvedArgs[0]), nil
	case "datetime":
		if len(resolvedArgs) == 0 {
			return NewSymbol("nil"), fmt.Errorf("datetime expects 1 argument")
		}
		return ConvertToDateTime(resolvedArgs[0]), nil
	case "date":
		if len(resolvedArgs) == 0 {
			return NewSymbol("nil"), fmt.Errorf("date expects 1 argument")
		}
		return ConvertToDate(resolvedArgs[0]), nil
	case "dayadd":
		if len(resolvedArgs) < 2 {
			return NewSymbol("nil"), fmt.Errorf("dayadd expects 2 arguments")
		}
		d, n, err := ExtractTemporalAndInt(resolvedArgs[0], resolvedArgs[1])
		if err != nil {
			return Value{}, err
		}
		return DayAdd(d, n)
	case "monthadd":
		if len(resolvedArgs) < 2 {
			return NewSymbol("nil"), fmt.Errorf("monthadd expects 2 arguments")
		}
		d, n, err := ExtractTemporalAndInt(resolvedArgs[0], resolvedArgs[1])
		if err != nil {
			return Value{}, err
		}
		return MonthAdd(d, n)
	case "yearadd":
		if len(resolvedArgs) < 2 {
			return NewSymbol("nil"), fmt.Errorf("yearadd expects 2 arguments")
		}
		d, n, err := ExtractTemporalAndInt(resolvedArgs[0], resolvedArgs[1])
		if err != nil {
			return Value{}, err
		}
		return YearAdd(d, n)
	case "houradd":
		if len(resolvedArgs) < 2 {
			return NewSymbol("nil"), fmt.Errorf("houradd expects 2 arguments")
		}
		d, n, err := ExtractTemporalAndInt(resolvedArgs[0], resolvedArgs[1])
		if err != nil {
			return Value{}, err
		}
		return HourAdd(d, n)
	case "minuteadd":
		if len(resolvedArgs) < 2 {
			return NewSymbol("nil"), fmt.Errorf("minuteadd expects 2 arguments")
		}
		d, n, err := ExtractTemporalAndInt(resolvedArgs[0], resolvedArgs[1])
		if err != nil {
			return Value{}, err
		}
		return MinuteAdd(d, n)
	case "secondsadd":
		if len(resolvedArgs) < 2 {
			return NewSymbol("nil"), fmt.Errorf("secondsadd expects 2 arguments")
		}
		d, n, err := ExtractTemporalAndInt(resolvedArgs[0], resolvedArgs[1])
		if err != nil {
			return Value{}, err
		}
		return SecondsAdd(d, n)
	case "datediff":
		if len(resolvedArgs) < 2 {
			return NewSymbol("nil"), fmt.Errorf("datediff expects 2 arguments")
		}
		return DateDiff(resolvedArgs[0], resolvedArgs[1])
	case "minutes":
		if len(resolvedArgs) == 0 {
			return NewSymbol("nil"), fmt.Errorf("minutes expects 1 argument")
		}
		return MinutesValue(resolvedArgs[0])
	case "hours":
		if len(resolvedArgs) == 0 {
			return NewSymbol("nil"), fmt.Errorf("hours expects 1 argument")
		}
		return HoursValue(resolvedArgs[0])
	case "days":
		if len(resolvedArgs) == 0 {
			return NewSymbol("nil"), fmt.Errorf("days expects 1 argument")
		}
		return DaysValue(resolvedArgs[0])
	default:
		return NewSymbol("nil"), fmt.Errorf("unknown temporal operator: %s", op)
	}
}
