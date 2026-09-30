package model

import (
	"testing"
	"time"
)

func TestValueEquality(t *testing.T) {
	tests := []struct {
		name  string
		v1    Value
		v2    Value
		equal bool
	}{
		{"same symbol", NewSymbol("foo"), NewSymbol("foo"), true},
		{"diff symbol", NewSymbol("foo"), NewSymbol("bar"), false},
		{"same int", NewInt(42), NewInt(42), true},
		{"diff int", NewInt(42), NewInt(43), false},
		{"int and float equal", NewInt(42), NewFloat(42.0), true},
		{"int and float diff", NewInt(42), NewFloat(42.5), false},
		{"string equality", NewString("hello"), NewString("hello"), true},
		{"variable equality", NewVariable("<x>"), NewVariable("x"), true},
		{"symbol vs string", NewSymbol("hello"), NewString("hello"), false},
		{"boolean equality", NewBoolean(true), NewBoolean(true), true},
		{"boolean diff", NewBoolean(true), NewBoolean(false), false},
		{"boolean and symbol cross-equality", NewBoolean(true), NewSymbol("true"), true},
		{"boolean and symbol false cross-equality", NewBoolean(false), NewSymbol("false"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v1.Equal(tt.v2); got != tt.equal {
				t.Errorf("Equal() = %v, want %v", got, tt.equal)
			}
		})
	}
}

func TestValueCompare(t *testing.T) {
	tests := []struct {
		name    string
		v1      Value
		v2      Value
		want    int
		wantErr bool
	}{
		{"int less", NewInt(10), NewInt(20), -1, false},
		{"int greater", NewInt(20), NewInt(10), 1, false},
		{"int equal", NewInt(10), NewInt(10), 0, false},
		{"int and float less", NewInt(10), NewFloat(10.5), -1, false},
		{"symbol less", NewSymbol("alpha"), NewSymbol("beta"), -1, false},
		{"incompatible types", NewSymbol("alpha"), NewInt(10), 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.v1.Compare(tt.v2)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Compare() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Compare() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAutoValue(t *testing.T) {
	tests := []struct {
		token   string
		wantTyp ValueType
		wantStr string
	}{
		{"active", TypeSymbol, "active"},
		{"123", TypeInteger, "123"},
		{"3.14", TypeFloat, "3.14"},
		{"<x>", TypeVariable, "<x>"},
		{`"quoted text"`, TypeString, `"quoted text"`},
		{"true", TypeBoolean, "true"},
		{"false", TypeBoolean, "false"},
	}

	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			val := AutoValue(tt.token)
			if val.Type() != tt.wantTyp {
				t.Errorf("AutoValue(%q).Type() = %v, want %v", tt.token, val.Type(), tt.wantTyp)
			}
			if val.String() != tt.wantStr {
				t.Errorf("AutoValue(%q).String() = %v, want %v", tt.token, val.String(), tt.wantStr)
			}
		})
	}
}

func TestVectorValue(t *testing.T) {
	v1 := NewVector([]Value{NewFloat(42.36), NewFloat(-71.05)})
	v2 := NewVector([]Value{NewFloat(42.36), NewFloat(-71.05)})
	v3 := NewVector([]Value{NewFloat(42.36), NewFloat(-70.00)})

	if !v1.IsVector() {
		t.Fatalf("expected v1 to be vector")
	}
	if v1.Type() != TypeVector {
		t.Fatalf("expected v1.Type() == TypeVector, got %v", v1.Type())
	}
	if !v1.Equal(v2) {
		t.Fatalf("expected v1 equal v2")
	}
	if v1.Equal(v3) {
		t.Fatalf("expected v1 not equal v3")
	}
	if v1.String() != "42.36 -71.05" {
		t.Fatalf("expected '42.36 -71.05', got %q", v1.String())
	}
	if len(v1.VectorElements()) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(v1.VectorElements()))
	}

	// Vectors of strings, integers, booleans, and heterogeneous elements
	strVec := NewVector([]Value{NewString("Beantown"), NewString("The Hub")})
	if strVec.String() != `"Beantown" "The Hub"` {
		t.Fatalf("expected '\"Beantown\" \"The Hub\"', got %q", strVec.String())
	}

	intVec := NewVector([]Value{NewInt(2101), NewInt(2108), NewInt(2115)})
	if intVec.String() != "2101 2108 2115" {
		t.Fatalf("expected '2101 2108 2115', got %q", intVec.String())
	}

	boolVec := NewVector([]Value{NewBoolean(true), NewBoolean(false), NewBoolean(true)})
	if boolVec.String() != "true false true" {
		t.Fatalf("expected 'true false true', got %q", boolVec.String())
	}

	hetVec := NewVector([]Value{NewString("alpha"), NewInt(10), NewFloat(3.14), NewBoolean(true)})
	if hetVec.String() != `"alpha" 10 3.14 true` {
		t.Fatalf("expected '\"alpha\" 10 3.14 true', got %q", hetVec.String())
	}
}

func TestDateValue(t *testing.T) {
	d1 := NewDate(20260929)
	if !d1.IsDate() {
		t.Fatalf("expected d1 to be Date")
	}
	if d1.Type() != TypeDate {
		t.Fatalf("expected TypeDate, got %v", d1.Type())
	}
	if d1.DateInt() != 20260929 {
		t.Fatalf("expected DateInt() == 20260929, got %d", d1.DateInt())
	}
	if d1.String() != "20260929" {
		t.Fatalf("expected '20260929', got %q", d1.String())
	}

	// Equality
	d2 := NewDate(20260929)
	d3 := NewDate(20260930)
	if !d1.Equal(d2) {
		t.Fatalf("expected d1 equal d2")
	}
	if d1.Equal(d3) {
		t.Fatalf("expected d1 not equal d3")
	}

	// Cross-equality with integer
	intVal := NewInt(20260929)
	if !d1.Equal(intVal) {
		t.Fatalf("expected d1 to equal integer 20260929")
	}
	if !intVal.Equal(d1) {
		t.Fatalf("expected integer 20260929 to equal d1")
	}

	// Comparison
	cmp, err := d1.Compare(d3)
	if err != nil || cmp != -1 {
		t.Fatalf("expected d1 < d3, got %d, err=%v", cmp, err)
	}
	cmp, err = d3.Compare(d1)
	if err != nil || cmp != 1 {
		t.Fatalf("expected d3 > d1, got %d, err=%v", cmp, err)
	}
	cmp, err = d1.Compare(d2)
	if err != nil || cmp != 0 {
		t.Fatalf("expected d1 == d2, got %d, err=%v", cmp, err)
	}

	// Cross-comparison with integer
	intLater := NewInt(20260930)
	cmp, err = d1.Compare(intLater)
	if err != nil || cmp != -1 {
		t.Fatalf("expected d1 < intLater, got %d, err=%v", cmp, err)
	}

	// ParseDate
	parsed, err := ParseDate(20260929)
	if err != nil || parsed.DateInt() != 20260929 {
		t.Fatalf("ParseDate error: %v", err)
	}
	_, err = ParseDate(20260230) // Feb 30 invalid
	if err == nil {
		t.Fatalf("expected error parsing invalid calendar date 20260230")
	}
}

func TestDateTimeValue(t *testing.T) {
	dt1 := NewDateTime("2026-09-29T19:53:58")
	if !dt1.IsDateTime() {
		t.Fatalf("expected dt1 to be DateTime")
	}
	if dt1.Type() != TypeDateTime {
		t.Fatalf("expected TypeDateTime, got %v", dt1.Type())
	}
	if dt1.String() != "2026-09-29T19:53:58" {
		t.Fatalf("expected '2026-09-29T19:53:58', got %q", dt1.String())
	}

	dt2 := NewDateTime("2026-09-29T19:53:58")
	dt3 := NewDateTime("2026-09-29T20:00:00")
	if !dt1.Equal(dt2) {
		t.Fatalf("expected dt1 equal dt2")
	}
	if dt1.Equal(dt3) {
		t.Fatalf("expected dt1 not equal dt3")
	}

	cmp, err := dt1.Compare(dt3)
	if err != nil || cmp != -1 {
		t.Fatalf("expected dt1 < dt3, got %d, err=%v", cmp, err)
	}

	// AutoValue detection with ISO 8601
	autoVal := AutoValue("2026-09-29T19:53:58")
	if autoVal.Type() != TypeDateTime {
		t.Fatalf("expected AutoValue to produce TypeDateTime, got %v", autoVal.Type())
	}
}

func TestDateUTCTimeValue(t *testing.T) {
	// NewDateUTCTime converts local time to UTC
	utcVal := NewDateUTCTime("2026-09-29T19:53:58")
	if !utcVal.IsDateUTCTime() {
		t.Fatalf("expected utcVal to be DateUTCTime")
	}
	if utcVal.Type() != TypeDateUTCTime {
		t.Fatalf("expected TypeDateUTCTime, got %v", utcVal.Type())
	}

	// Cross-equality: NewDateTime in local time and NewDateUTCTime converted from local time represent the same moment
	localVal := NewDateTime("2026-09-29T19:53:58")
	if !localVal.Equal(utcVal) {
		t.Fatalf("expected localVal and utcVal to represent same instant")
	}
	if !utcVal.Equal(localVal) {
		t.Fatalf("expected utcVal and localVal to represent same instant")
	}

	// Location conversion: supplier in JST (UTC+9)
	jst := time.FixedZone("JST", 9*3600)
	supplierVal := NewDateUTCTimeInLocation("2026-09-30T04:53:58", jst)
	// 2026-09-30T04:53:58 JST == 2026-09-29T19:53:58 UTC
	directUTC := NewDateUTCDirect("2026-09-29T19:53:58Z")
	if !supplierVal.Equal(directUTC) {
		t.Fatalf("expected supplier JST time converted to UTC to equal direct UTC time")
	}

	// AutoValue UTC detection with 'Z' suffix
	autoUTC := AutoValue("2026-09-29T19:53:58Z")
	if autoUTC.Type() != TypeDateUTCTime {
		t.Fatalf("expected AutoValue('2026-09-29T19:53:58Z') to be TypeDateUTCTime, got %v", autoUTC.Type())
	}

	// Conversions
	c1 := ConvertToDateUTCTime(localVal)
	if c1.Type() != TypeDateUTCTime || !c1.Equal(localVal) {
		t.Fatalf("ConvertToDateUTCTime failed")
	}
	c2 := ConvertToDateTime(directUTC)
	if c2.Type() != TypeDateTime || !c2.Equal(directUTC) {
		t.Fatalf("ConvertToDateTime failed")
	}
	c3 := ConvertToDate(directUTC)
	if c3.Type() != TypeDate || c3.DateInt() != 20260929 {
		t.Fatalf("ConvertToDate failed, got %v", c3)
	}
}

func TestTimeAttributesInWME(t *testing.T) {
	jst := time.FixedZone("JST", 9*3600)
	currentTime := NewDateTime("2026-09-29T19:53:58")
	supplierTime := NewDateUTCTimeInLocation("2026-09-30T05:00:00", jst)
	orderDate := NewDate(20260929)

	wme := NewWME(10, "order", map[string]Value{
		"current_time":          currentTime,
		"supplier_current_time": supplierTime,
		"order_date":            orderDate,
	})

	if !wme.Has("current_time") || !wme.Has("supplier_current_time") || !wme.Has("order_date") {
		t.Fatalf("missing attributes on WME")
	}

	ct, _ := wme.Get("current_time")
	sct, _ := wme.Get("supplier_current_time")
	od, _ := wme.Get("order_date")

	if ct.Type() != TypeDateTime {
		t.Fatalf("expected current_time to be DateTime, got %v", ct.Type())
	}
	if sct.Type() != TypeDateUTCTime {
		t.Fatalf("expected supplier_current_time to be DateUTCTime, got %v", sct.Type())
	}
	if od.Type() != TypeDate {
		t.Fatalf("expected order_date to be Date, got %v", od.Type())
	}

	// 2026-09-30T05:00:00 JST is 2026-09-29T20:00:00 UTC
	// currentTime is 2026-09-29T19:53:58 Local.
	_, err := ct.Compare(sct)
	if err != nil {
		t.Fatalf("Compare current_time with supplier_current_time error: %v", err)
	}

	wmeStr := wme.String()
	if wmeStr == "" {
		t.Fatalf("WME string representation should not be empty")
	}
}

func TestISODateStringParsing(t *testing.T) {
	if !IsDateString("2026-09-29") {
		t.Fatalf("expected 2026-09-29 to be recognized as Date string")
	}
	if IsDateString("20260929") {
		t.Fatalf("unhyphenated number should not be recognized as Date string")
	}

	d, err := ParseDateString("2026-09-29")
	if err != nil {
		t.Fatalf("ParseDateString error: %v", err)
	}
	if d.Type() != TypeDate {
		t.Fatalf("expected TypeDate, got %v", d.Type())
	}
	if d.DateInt() != 20260929 {
		t.Fatalf("expected DateInt() 20260929, got %d", d.DateInt())
	}

	autoD := AutoValue("2026-09-29")
	if autoD.Type() != TypeDate {
		t.Fatalf("expected AutoValue('2026-09-29') to be TypeDate, got %v", autoD.Type())
	}
}
