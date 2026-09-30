package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ValueType represents the type of an OPS5 value.
type ValueType int

const (
	TypeSymbol ValueType = iota
	TypeInteger
	TypeFloat
	TypeString
	TypeBoolean
	TypeVariable
	TypeVector
	TypeCompute
	TypeAccept
	TypeGenatom
	TypeLitval
	TypeSubstr
	TypeDate
	TypeDateTime
	TypeDateUTCTime
	TypeTemporalExpr
)

func (t ValueType) String() string {
	switch t {
	case TypeSymbol:
		return "symbol"
	case TypeInteger:
		return "integer"
	case TypeFloat:
		return "float"
	case TypeString:
		return "string"
	case TypeBoolean:
		return "boolean"
	case TypeVariable:
		return "variable"
	case TypeVector:
		return "vector"
	case TypeCompute:
		return "compute"
	case TypeAccept:
		return "accept"
	case TypeGenatom:
		return "genatom"
	case TypeLitval:
		return "litval"
	case TypeSubstr:
		return "substr"
	case TypeDate:
		return "date"
	case TypeDateTime:
		return "datetime"
	case TypeDateUTCTime:
		return "dateutctime"
	case TypeTemporalExpr:
		return "temporal"
	default:
		return "unknown"
	}
}

// Value is an immutable representation of an OPS5 datum.
type Value struct {
	typ ValueType
	val any
}

// NewSymbol creates a new symbol value.
func NewSymbol(s string) Value {
	return Value{typ: TypeSymbol, val: s}
}

// NewInt creates a new integer value.
func NewInt(n int64) Value {
	return Value{typ: TypeInteger, val: n}
}

// NewFloat creates a new floating-point value.
func NewFloat(f float64) Value {
	return Value{typ: TypeFloat, val: f}
}

// NewString creates a new string value.
func NewString(s string) Value {
	return Value{typ: TypeString, val: s}
}

// NewBoolean creates a new boolean value.
func NewBoolean(b bool) Value {
	return Value{typ: TypeBoolean, val: b}
}

// NewVariable creates a new variable placeholder (e.g. <x>).
func NewVariable(name string) Value {
	// Strip enclosing angle brackets if present
	trimmed := strings.TrimPrefix(strings.TrimSuffix(name, ">"), "<")
	return Value{typ: TypeVariable, val: trimmed}
}

// NewVector creates a new vector value representing a sequence of Values.
func NewVector(elements []Value) Value {
	copied := make([]Value, len(elements))
	copy(copied, elements)
	return Value{typ: TypeVector, val: copied}
}

// NewDate creates a new Date value from an integer in YYYYMMDD format.
// Dates are normalized to midnight UTC.
func NewDate(yyyymmdd int64) Value {
	year := int(yyyymmdd / 10000)
	month := int((yyyymmdd % 10000) / 100)
	day := int(yyyymmdd % 100)
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return Value{typ: TypeDate, val: t}
}

// NewDateFromTime creates a Date value from a time.Time, normalized to UTC midnight.
func NewDateFromTime(t time.Time) Value {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return Value{typ: TypeDate, val: d}
}

// NewDateTime creates a new DateTime value from a string in "YYYYMMDD:HhMmSs" format,
// parsed in the local timezone (time.Local).
func NewDateTime(s string) Value {
	return NewDateTimeInLocation(s, time.Local)
}

// NewDateTimeInLocation creates a new DateTime value from a string in the specified timezone/location.
func NewDateTimeInLocation(s string, loc *time.Location) Value {
	t, err := parseDateTimeString(s, loc)
	if err != nil {
		return Value{typ: TypeDateTime, val: time.Time{}}
	}
	return Value{typ: TypeDateTime, val: t}
}

// NewDateTimeFromTime creates a DateTime value from an existing time.Time.
func NewDateTimeFromTime(t time.Time) Value {
	return Value{typ: TypeDateTime, val: t}
}

// NewDateUTCTime creates a new DateUTCTime value from a string in "YYYYMMDD:HhMmSs" format,
// interpreting the input string in the local timezone and converting it to UTC.
func NewDateUTCTime(s string) Value {
	return NewDateUTCTimeFromLocal(s)
}

// NewDateUTCTimeFromLocal parses s in time.Local and converts it to UTC.
func NewDateUTCTimeFromLocal(s string) Value {
	t, err := parseDateTimeString(s, time.Local)
	if err != nil {
		return Value{typ: TypeDateUTCTime, val: time.Time{}.UTC()}
	}
	return Value{typ: TypeDateUTCTime, val: t.UTC()}
}

// NewDateUTCTimeInLocation parses s in the specified location (e.g. supplier's local timezone)
// and converts it to UTC.
func NewDateUTCTimeInLocation(s string, loc *time.Location) Value {
	t, err := parseDateTimeString(s, loc)
	if err != nil {
		return Value{typ: TypeDateUTCTime, val: time.Time{}.UTC()}
	}
	return Value{typ: TypeDateUTCTime, val: t.UTC()}
}

// NewDateUTCDirect parses s directly as UTC time (assuming input string is already UTC).
func NewDateUTCDirect(s string) Value {
	t, err := parseDateTimeString(s, time.UTC)
	if err != nil {
		return Value{typ: TypeDateUTCTime, val: time.Time{}.UTC()}
	}
	return Value{typ: TypeDateUTCTime, val: t.UTC()}
}

// NewDateUTCTimeFromTime creates a DateUTCTime value from a time.Time, converting to UTC.
func NewDateUTCTimeFromTime(t time.Time) Value {
	return Value{typ: TypeDateUTCTime, val: t.UTC()}
}

// ParseDate parses a YYYYMMDD integer into a Date Value with validation error.
func ParseDate(yyyymmdd int64) (Value, error) {
	year := int(yyyymmdd / 10000)
	month := int((yyyymmdd % 10000) / 100)
	day := int(yyyymmdd % 100)
	if year < 1 || month < 1 || month > 12 || day < 1 || day > 31 {
		return Value{}, fmt.Errorf("invalid date %d: year=%d month=%d day=%d", yyyymmdd, year, month, day)
	}
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if t.Year() != year || int(t.Month()) != month || t.Day() != day {
		return Value{}, fmt.Errorf("invalid calendar date: %d", yyyymmdd)
	}
	return Value{typ: TypeDate, val: t}, nil
}

// ParseDateTime parses a string into a DateTime Value in time.Local with error return.
func ParseDateTime(s string) (Value, error) {
	t, err := parseDateTimeString(s, time.Local)
	if err != nil {
		return Value{}, err
	}
	return Value{typ: TypeDateTime, val: t}, nil
}

// ParseDateUTCTime parses a string in time.Local and converts to UTC with error return.
func ParseDateUTCTime(s string) (Value, error) {
	t, err := parseDateTimeString(s, time.Local)
	if err != nil {
		return Value{}, err
	}
	return Value{typ: TypeDateUTCTime, val: t.UTC()}, nil
}

// parseDateTimeString parses a date-time string against supported formats, prioritizing ISO 8601 / RFC 3339.
func parseDateTimeString(s string, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.Local
	}
	s = strings.TrimSpace(s)
	// 1. Try RFC 3339 (e.g. 2026-09-29T19:53:58Z or 2026-09-29T19:53:58-07:00)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	// 2. Try RFC 3339 without timezone in the specified loc
	layoutsInLoc := []string{
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"20060102T150405",
		"20060102:150405",
		"20060102:15:04:05",
	}
	for _, layout := range layoutsInLoc {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	// 3. Try with Z suffix in UTC
	if strings.HasSuffix(s, "Z") || strings.HasSuffix(s, "z") {
		trimmed := s[:len(s)-1]
		for _, layout := range layoutsInLoc {
			if t, err := time.ParseInLocation(layout, trimmed, time.UTC); err == nil {
				return t, nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse datetime string: %q", s)
}

// IsDateTimeString reports whether a string matches ISO 8601 / RFC 3339 or compact datetime formats,
// and whether it designates UTC (e.g. ending in 'Z').
func IsDateTimeString(s string) (bool, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return false, false
	}
	isUTC := strings.HasSuffix(s, "Z") || strings.HasSuffix(s, "z")

	// Standard ISO 8601 / RFC 3339 extended: YYYY-MM-DDTHH:MM:SS...
	if len(s) >= 19 && s[4] == '-' && s[7] == '-' && (s[10] == 'T' || s[10] == 't' || s[10] == ' ') && s[13] == ':' && s[16] == ':' {
		if isAllDigits(s[0:4]) && isAllDigits(s[5:7]) && isAllDigits(s[8:10]) &&
			isAllDigits(s[11:13]) && isAllDigits(s[14:16]) && isAllDigits(s[17:19]) {
			return true, isUTC
		}
	}

	// Compact ISO 8601 (YYYYMMDDTHHMMSS) or hybrid (YYYYMMDD:HHMMSS)
	trimmed := s
	if isUTC {
		trimmed = s[:len(s)-1]
	}
	if len(trimmed) == 15 && (trimmed[8] == 'T' || trimmed[8] == 't' || trimmed[8] == ':') {
		if isAllDigits(trimmed[0:8]) && isAllDigits(trimmed[9:15]) {
			return true, isUTC
		}
	}
	if len(trimmed) == 17 && trimmed[8] == ':' && trimmed[11] == ':' && trimmed[14] == ':' {
		if isAllDigits(trimmed[0:8]) && isAllDigits(trimmed[9:11]) && isAllDigits(trimmed[12:14]) && isAllDigits(trimmed[15:17]) {
			return true, isUTC
		}
	}
	return false, false
}

func isAllDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// IsDateString reports whether a string matches ISO 8601 date format YYYY-MM-DD.
func IsDateString(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) == 10 && s[4] == '-' && s[7] == '-' {
		return isAllDigits(s[0:4]) && isAllDigits(s[5:7]) && isAllDigits(s[8:10])
	}
	return false
}

// ParseDateString parses a YYYY-MM-DD string into a TypeDate Value.
func ParseDateString(s string) (Value, error) {
	s = strings.TrimSpace(s)
	if !IsDateString(s) {
		return Value{}, fmt.Errorf("invalid date string format (expected YYYY-MM-DD): %q", s)
	}
	year, err1 := strconv.Atoi(s[0:4])
	month, err2 := strconv.Atoi(s[5:7])
	day, err3 := strconv.Atoi(s[8:10])
	if err1 != nil || err2 != nil || err3 != nil {
		return Value{}, fmt.Errorf("invalid date digits in: %q", s)
	}
	return ParseDate(int64(year*10000 + month*100 + day))
}

// TemporalExpr represents a dynamic (date ...), (datetime ...), or (utc ...) function call.
type TemporalExpr struct {
	Op  string // "date", "datetime", "utc"
	Arg Value
}

// NewTemporalExpr creates a new TemporalExpr Value.
func NewTemporalExpr(op string, arg Value) Value {
	return Value{
		typ: TypeTemporalExpr,
		val: &TemporalExpr{Op: strings.ToLower(op), Arg: arg},
	}
}

// IsTemporalExpr returns true if this value is a TemporalExpr.
func (v Value) IsTemporalExpr() bool {
	return v.typ == TypeTemporalExpr
}

// TemporalExpr returns the underlying TemporalExpr pointer if this value is a TemporalExpr.
func (v Value) TemporalExpr() *TemporalExpr {
	if v.typ == TypeTemporalExpr {
		return v.val.(*TemporalExpr)
	}
	return nil
}

// ConvertToDate converts any Value to TypeDate.
func ConvertToDate(v Value) Value {
	if v.typ == TypeDate {
		return v
	}
	if v.typ == TypeInteger {
		return NewDate(v.DateInt())
	}
	if v.IsTemporal() {
		return NewDateFromTime(v.Time())
	}
	str := v.String()
	if v.typ == TypeString {
		str = v.Raw().(string)
	}
	if n, err := strconv.ParseInt(str, 10, 64); err == nil && len(str) == 8 {
		return NewDate(n)
	}
	if t, err := parseDateTimeString(str, time.Local); err == nil {
		return NewDateFromTime(t)
	}
	return NewDate(0)
}

// ConvertToDateTime converts any Value to TypeDateTime (in local timezone).
func ConvertToDateTime(v Value) Value {
	if v.typ == TypeDateTime {
		return v
	}
	if v.typ == TypeDateUTCTime || v.typ == TypeDate {
		return NewDateTimeFromTime(v.Time().In(time.Local))
	}
	if v.typ == TypeInteger {
		d := NewDate(v.DateInt())
		return NewDateTimeFromTime(d.Time().In(time.Local))
	}
	str := v.String()
	if v.typ == TypeString {
		str = v.Raw().(string)
	}
	return NewDateTime(str)
}

// ConvertToDateUTCTime converts any Value to TypeDateUTCTime (in UTC).
func ConvertToDateUTCTime(v Value) Value {
	if v.typ == TypeDateUTCTime {
		return v
	}
	if v.typ == TypeDateTime || v.typ == TypeDate {
		return NewDateUTCTimeFromTime(v.Time())
	}
	if v.typ == TypeInteger {
		d := NewDate(v.DateInt())
		return NewDateUTCTimeFromTime(d.Time())
	}
	str := v.String()
	if v.typ == TypeString {
		str = v.Raw().(string)
	}
	return NewDateUTCTime(str)
}

// ComputeOp represents an arithmetic operator in a compute expression.
type ComputeOp int

const (
	ComputeOpAdd ComputeOp = iota
	ComputeOpSub
	ComputeOpMul
	ComputeOpDiv
	ComputeOpMod
)

func (op ComputeOp) String() string {
	switch op {
	case ComputeOpAdd:
		return "+"
	case ComputeOpSub:
		return "-"
	case ComputeOpMul:
		return "*"
	case ComputeOpDiv:
		return "/"
	case ComputeOpMod:
		return "//"
	default:
		return "+"
	}
}

// ComputeExpr represents a (compute ...) arithmetic expression.
type ComputeExpr struct {
	Operands  []Value
	Operators []ComputeOp
}

// NewCompute creates a new compute expression value.
func NewCompute(operands []Value, operators []ComputeOp) Value {
	return Value{
		typ: TypeCompute,
		val: &ComputeExpr{
			Operands:  operands,
			Operators: operators,
		},
	}
}

// AcceptExpr represents an (accept) or (acceptline) function invocation.
type AcceptExpr struct {
	LogicalFile string // optional logical file, or empty for default input
	IsLine      bool   // true for acceptline, false for accept
}

// NewAccept creates an accept or acceptline expression value.
func NewAccept(logicalFile string, isLine bool) Value {
	return Value{
		typ: TypeAccept,
		val: &AcceptExpr{
			LogicalFile: logicalFile,
			IsLine:      isLine,
		},
	}
}

// IsAccept returns true if this value is an accept or acceptline function call.
func (v Value) IsAccept() bool {
	return v.typ == TypeAccept
}

// AcceptExpr returns the underlying AcceptExpr.
func (v Value) AcceptExpr() *AcceptExpr {
	if v.typ == TypeAccept {
		return v.val.(*AcceptExpr)
	}
	return nil
}

// NewGenatom creates a new genatom function value.
func NewGenatom() Value {
	return Value{
		typ: TypeGenatom,
		val: "genatom",
	}
}

// IsGenatom returns true if this value is a genatom function call.
func (v Value) IsGenatom() bool {
	return v.typ == TypeGenatom
}

// LitvalExpr represents a (litval [class] attr) function invocation.
type LitvalExpr struct {
	Class     string // optional class name, or empty string
	Attribute Value  // attribute name (symbol, string, or variable)
}

// NewLitval creates a new litval expression value.
func NewLitval(class string, attribute Value) Value {
	return Value{
		typ: TypeLitval,
		val: &LitvalExpr{
			Class:     class,
			Attribute: attribute,
		},
	}
}

// IsLitval returns true if this value is a litval function call.
func (v Value) IsLitval() bool {
	return v.typ == TypeLitval
}

// LitvalExpr returns the underlying LitvalExpr.
func (v Value) LitvalExpr() *LitvalExpr {
	if v.typ == TypeLitval {
		return v.val.(*LitvalExpr)
	}
	return nil
}

// SubstrExpr represents a (substr elemRef start end) function invocation.
type SubstrExpr struct {
	ElementRef Value // variable (e.g. <str>) or integer condition element index (e.g. 1)
	Start      Value // attribute symbol, integer index, variable, or nested expr
	End        Value // attribute symbol, integer index, variable, "inf", or nested expr
}

// NewSubstr creates a new substr expression value.
func NewSubstr(elemRef, start, end Value) Value {
	return Value{
		typ: TypeSubstr,
		val: &SubstrExpr{
			ElementRef: elemRef,
			Start:      start,
			End:        end,
		},
	}
}

// IsSubstr returns true if this value is a substr function call.
func (v Value) IsSubstr() bool {
	return v.typ == TypeSubstr
}

// SubstrExpr returns the underlying SubstrExpr.
func (v Value) SubstrExpr() *SubstrExpr {
	if v.typ == TypeSubstr {
		return v.val.(*SubstrExpr)
	}
	return nil
}

// Type returns the ValueType.
func (v Value) Type() ValueType {
	return v.typ
}

// Raw returns the underlying primitive value.
func (v Value) Raw() any {
	return v.val
}

// IsVariable returns true if this value is a variable reference.
func (v Value) IsVariable() bool {
	return v.typ == TypeVariable
}

// IsBoolean returns true if this value is a boolean.
func (v Value) IsBoolean() bool {
	return v.typ == TypeBoolean
}

// Boolean returns the underlying boolean if this value is a boolean, otherwise false.
func (v Value) Boolean() bool {
	if v.typ == TypeBoolean {
		return v.val.(bool)
	}
	return false
}

// IsVector returns true if this value is a vector of values.
func (v Value) IsVector() bool {
	return v.typ == TypeVector
}

// VectorElements returns the underlying elements if this value is a vector.
func (v Value) VectorElements() []Value {
	if v.typ == TypeVector {
		return v.val.([]Value)
	}
	return nil
}

// IsCompute returns true if this value is a compute expression.
func (v Value) IsCompute() bool {
	return v.typ == TypeCompute
}

// ComputeExpr returns the underlying ComputeExpr pointer if this value is a compute expression.
func (v Value) ComputeExpr() *ComputeExpr {
	if v.typ == TypeCompute {
		return v.val.(*ComputeExpr)
	}
	return nil
}

// VariableName returns the variable identifier without enclosing brackets.
func (v Value) VariableName() string {
	if v.typ == TypeVariable {
		return v.val.(string)
	}
	return ""
}

// IsDate returns true if this value is a Date.
func (v Value) IsDate() bool {
	return v.typ == TypeDate
}

// IsDateTime returns true if this value is a DateTime.
func (v Value) IsDateTime() bool {
	return v.typ == TypeDateTime
}

// IsDateUTCTime returns true if this value is a DateUTCTime.
func (v Value) IsDateUTCTime() bool {
	return v.typ == TypeDateUTCTime
}

// IsTemporal returns true if this value is Date, DateTime, or DateUTCTime.
func (v Value) IsTemporal() bool {
	return v.typ == TypeDate || v.typ == TypeDateTime || v.typ == TypeDateUTCTime
}

// DateInt returns the YYYYMMDD integer representation if this value is a Date.
// If it is an Integer, it returns the integer value directly. Otherwise returns 0.
func (v Value) DateInt() int64 {
	if v.typ == TypeDate {
		if t, ok := v.val.(time.Time); ok {
			return int64(t.Year())*10000 + int64(t.Month())*100 + int64(t.Day())
		}
	}
	if v.typ == TypeInteger {
		if n, ok := v.val.(int64); ok {
			return n
		}
	}
	return 0
}

// Time returns the underlying time.Time representation for Date, DateTime, and DateUTCTime.
func (v Value) Time() time.Time {
	switch v.typ {
	case TypeDate, TypeDateTime, TypeDateUTCTime:
		if t, ok := v.val.(time.Time); ok {
			return t
		}
	}
	return time.Time{}
}

// String returns the string representation.
func (v Value) String() string {
	if v.val == nil {
		return ""
	}
	switch v.typ {
	case TypeSymbol:
		return v.val.(string)
	case TypeInteger:
		return strconv.FormatInt(v.val.(int64), 10)
	case TypeFloat:
		return strconv.FormatFloat(v.val.(float64), 'g', -1, 64)
	case TypeString:
		return strconv.Quote(v.val.(string))
	case TypeBoolean:
		if v.val.(bool) {
			return "true"
		}
		return "false"
	case TypeVariable:
		return "<" + v.val.(string) + ">"
	case TypeVector:
		elems := v.val.([]Value)
		var parts []string
		for _, el := range elems {
			parts = append(parts, el.String())
		}
		return strings.Join(parts, " ")
	case TypeCompute:
		ce := v.val.(*ComputeExpr)
		var parts []string
		parts = append(parts, "compute")
		for i, op := range ce.Operands {
			parts = append(parts, op.String())
			if i < len(ce.Operators) {
				parts = append(parts, ce.Operators[i].String())
			}
		}
		return "(" + strings.Join(parts, " ") + ")"
	case TypeAccept:
		ae := v.val.(*AcceptExpr)
		name := "accept"
		if ae.IsLine {
			name = "acceptline"
		}
		if ae.LogicalFile != "" {
			return "(" + name + " " + ae.LogicalFile + ")"
		}
		return "(" + name + ")"
	case TypeGenatom:
		return "(genatom)"
	case TypeLitval:
		le := v.val.(*LitvalExpr)
		if le.Class != "" {
			return "(litval " + le.Class + " " + le.Attribute.String() + ")"
		}
		return "(litval " + le.Attribute.String() + ")"
	case TypeSubstr:
		se := v.val.(*SubstrExpr)
		return fmt.Sprintf("(substr %s %s %s)", se.ElementRef.String(), se.Start.String(), se.End.String())
	case TypeDate:
		t := v.val.(time.Time)
		return fmt.Sprintf("%04d%02d%02d", t.Year(), t.Month(), t.Day())
	case TypeDateTime:
		t := v.val.(time.Time)
		return t.Format("2006-01-02T15:04:05")
	case TypeDateUTCTime:
		t := v.val.(time.Time)
		return t.UTC().Format("2006-01-02T15:04:05Z")
	case TypeTemporalExpr:
		te := v.val.(*TemporalExpr)
		return fmt.Sprintf("(%s %s)", te.Op, te.Arg.String())
	default:
		return fmt.Sprintf("%v", v.val)
	}
}

// Equal checks equality between two values.
// Supports numeric cross-equality between integer and float if values match.
func (v Value) Equal(o Value) bool {
	if v.typ == o.typ {
		if v.typ == TypeDate {
			return v.DateInt() == o.DateInt()
		}
		if v.typ == TypeDateTime || v.typ == TypeDateUTCTime {
			return v.val.(time.Time).Equal(o.val.(time.Time))
		}
		if v.typ == TypeTemporalExpr {
			t1 := v.val.(*TemporalExpr)
			t2 := o.val.(*TemporalExpr)
			return t1.Op == t2.Op && t1.Arg.Equal(t2.Arg)
		}
		if v.typ == TypeGenatom {
			return true
		}
		if v.typ == TypeLitval {
			l1 := v.val.(*LitvalExpr)
			l2 := o.val.(*LitvalExpr)
			return strings.EqualFold(l1.Class, l2.Class) && l1.Attribute.Equal(l2.Attribute)
		}
		if v.typ == TypeSubstr {
			s1 := v.val.(*SubstrExpr)
			s2 := o.val.(*SubstrExpr)
			return s1.ElementRef.Equal(s2.ElementRef) && s1.Start.Equal(s2.Start) && s1.End.Equal(s2.End)
		}
		if v.typ == TypeVector {
			v1 := v.val.([]Value)
			v2 := o.val.([]Value)
			if len(v1) != len(v2) {
				return false
			}
			for i := range v1 {
				if !v1[i].Equal(v2[i]) {
					return false
				}
			}
			return true
		}
		if v.typ == TypeCompute {
			c1 := v.val.(*ComputeExpr)
			c2 := o.val.(*ComputeExpr)
			if len(c1.Operands) != len(c2.Operands) || len(c1.Operators) != len(c2.Operators) {
				return false
			}
			for i := range c1.Operators {
				if c1.Operators[i] != c2.Operators[i] {
					return false
				}
			}
			for i := range c1.Operands {
				if !c1.Operands[i].Equal(c2.Operands[i]) {
					return false
				}
			}
			return true
		}
		if v.typ == TypeAccept {
			a1 := v.val.(*AcceptExpr)
			a2 := o.val.(*AcceptExpr)
			return a1.LogicalFile == a2.LogicalFile && a1.IsLine == a2.IsLine
		}
		return v.val == o.val
	}
	// Numeric cross-comparison
	if v.typ == TypeInteger && o.typ == TypeFloat {
		return float64(v.val.(int64)) == o.val.(float64)
	}
	if v.typ == TypeFloat && o.typ == TypeInteger {
		return v.val.(float64) == float64(o.val.(int64))
	}
	// Date and Integer cross-comparison (e.g. TypeDate(20260929) == 20260929)
	if v.typ == TypeDate && o.typ == TypeInteger {
		return v.DateInt() == o.val.(int64)
	}
	if v.typ == TypeInteger && o.typ == TypeDate {
		return v.val.(int64) == o.DateInt()
	}
	// DateTime and DateUTCTime cross-equality (same instant across timezones)
	if (v.typ == TypeDateTime && o.typ == TypeDateUTCTime) || (v.typ == TypeDateUTCTime && o.typ == TypeDateTime) {
		return v.val.(time.Time).Equal(o.val.(time.Time))
	}
	// Boolean and Symbol cross-comparison (e.g. true vs "true", false vs "false")
	if v.typ == TypeBoolean && o.typ == TypeSymbol {
		return strings.EqualFold(strconv.FormatBool(v.val.(bool)), o.val.(string))
	}
	if v.typ == TypeSymbol && o.typ == TypeBoolean {
		return strings.EqualFold(v.val.(string), strconv.FormatBool(o.val.(bool)))
	}
	return false
}

// Compare returns:
// -1 if v < o
//  0 if v == o
//  1 if v > o
// Returns an error if types cannot be ordered (e.g., symbols vs numbers).
func (v Value) Compare(o Value) (int, error) {
	// Numeric comparisons
	if (v.typ == TypeInteger || v.typ == TypeFloat) && (o.typ == TypeInteger || o.typ == TypeFloat) {
		var f1, f2 float64
		if v.typ == TypeInteger {
			f1 = float64(v.val.(int64))
		} else {
			f1 = v.val.(float64)
		}

		if o.typ == TypeInteger {
			f2 = float64(o.val.(int64))
		} else {
			f2 = o.val.(float64)
		}

		if f1 < f2 {
			return -1, nil
		} else if f1 > f2 {
			return 1, nil
		}
		return 0, nil
	}

	if v.typ == TypeString && o.typ == TypeString {
		s1 := v.val.(string)
		s2 := o.val.(string)
		if s1 < s2 {
			return -1, nil
		} else if s1 > s2 {
			return 1, nil
		}
		return 0, nil
	}

	if v.typ == TypeBoolean && o.typ == TypeBoolean {
		b1 := v.val.(bool)
		b2 := o.val.(bool)
		if !b1 && b2 {
			return -1, nil
		} else if b1 && !b2 {
			return 1, nil
		}
		return 0, nil
	}

	if v.typ == TypeSymbol && o.typ == TypeSymbol {
		s1 := v.val.(string)
		s2 := o.val.(string)
		if s1 < s2 {
			return -1, nil
		} else if s1 > s2 {
			return 1, nil
		}
		return 0, nil
	}

	if (v.typ == TypeBoolean && o.typ == TypeSymbol) || (v.typ == TypeSymbol && o.typ == TypeBoolean) {
		s1 := v.String()
		s2 := o.String()
		if s1 < s2 {
			return -1, nil
		} else if s1 > s2 {
			return 1, nil
		}
		return 0, nil
	}

	// Date and Integer cross-comparison
	if (v.typ == TypeDate || v.typ == TypeInteger) && (o.typ == TypeDate || o.typ == TypeInteger) {
		if v.typ == TypeDate || o.typ == TypeDate {
			d1 := v.DateInt()
			d2 := o.DateInt()
			if d1 < d2 {
				return -1, nil
			} else if d1 > d2 {
				return 1, nil
			}
			return 0, nil
		}
	}

	// Temporal comparisons (Date, DateTime, DateUTCTime)
	if v.IsTemporal() && o.IsTemporal() {
		t1 := v.val.(time.Time)
		t2 := o.val.(time.Time)
		if t1.Before(t2) {
			return -1, nil
		} else if t1.After(t2) {
			return 1, nil
		}
		return 0, nil
	}

	return 0, fmt.Errorf("cannot compare incompatible types: %s and %s", v.typ, o.typ)
}

// AutoValue creates an appropriate Value from a string token.
// - If wrapped in quotes, creates a String.
// - If starts with '<' and ends with '>', creates a Variable.
// - If "true" or "false" (case-insensitive), creates a Boolean.
// - If matches datetime format (YYYYMMDD:HhMmSs), creates a DateTime or DateUTCTime.
// - If parses as integer, creates an Int.
// - If parses as float, creates a Float.
// - Otherwise, creates a Symbol.
func AutoValue(token string) Value {
	if strings.HasPrefix(token, "\"") && strings.HasSuffix(token, "\"") && len(token) >= 2 {
		unquoted, err := strconv.Unquote(token)
		if err == nil {
			return NewString(unquoted)
		}
		return NewString(token[1 : len(token)-1])
	}
	if strings.HasPrefix(token, "<") && strings.HasSuffix(token, ">") && len(token) > 2 {
		return NewVariable(token)
	}
	lower := strings.ToLower(token)
	if lower == "true" {
		return NewBoolean(true)
	}
	if lower == "false" {
		return NewBoolean(false)
	}
	if isDT, isUTC := IsDateTimeString(token); isDT {
		if isUTC {
			return NewDateUTCDirect(token)
		}
		return NewDateTime(token)
	}
	if IsDateString(token) {
		if d, err := ParseDateString(token); err == nil {
			return d
		}
	}
	if n, err := strconv.ParseInt(token, 10, 64); err == nil {
		return NewInt(n)
	}
	if f, err := strconv.ParseFloat(token, 64); err == nil && strings.ContainsAny(token, ".eE") {
		return NewFloat(f)
	}
	return NewSymbol(token)
}
