package model

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"10ms", 10 * time.Millisecond, false},
		{"10 ms", 10 * time.Millisecond, false},
		{"500millis", 500 * time.Millisecond, false},
		{"1000milliseconds", 1000 * time.Millisecond, false},
		{"5s", 5 * time.Second, false},
		{"5 sec", 5 * time.Second, false},
		{"30secs", 30 * time.Second, false},
		{"45 seconds", 45 * time.Second, false},
		{"10m", 10 * time.Minute, false},
		{"10 min", 10 * time.Minute, false},
		{"30mins", 30 * time.Minute, false},
		{"15 minutes", 15 * time.Minute, false},
		{"2h", 2 * time.Hour, false},
		{"1 hr", 1 * time.Hour, false},
		{"4hrs", 4 * time.Hour, false},
		{"3 hours", 3 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"1 day", 24 * time.Hour, false},
		{"7 days", 7 * 24 * time.Hour, false},
		{"invalid", 0, true},
		{"10lightyears", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		got, err := ParseDuration(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseDuration(%q) err = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.expected {
			t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestParseWallClock(t *testing.T) {
	loc := time.UTC
	// Reference: 2026-10-28 08:00:00 UTC
	refTime := time.Date(2026, 10, 28, 8, 0, 0, 0, loc)

	// Test 1: Future time today: 10:10am -> 2026-10-28 10:10:00
	t1, err := ParseWallClock("10:10am", refTime, loc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected1 := time.Date(2026, 10, 28, 10, 10, 0, 0, loc)
	if !t1.Equal(expected1) {
		t.Errorf("got %v, want %v", t1, expected1)
	}

	// Test 2: Past time today: 06:00am -> 2026-10-29 06:00:00 (tomorrow)
	t2, err := ParseWallClock("06:00am", refTime, loc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected2 := time.Date(2026, 10, 29, 6, 0, 0, 0, loc)
	if !t2.Equal(expected2) {
		t.Errorf("got %v, want %v", t2, expected2)
	}

	// Test 3: 24-hour time: 14:30 -> 2026-10-28 14:30:00
	t3, err := ParseWallClock("14:30", refTime, loc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected3 := time.Date(2026, 10, 28, 14, 30, 0, 0, loc)
	if !t3.Equal(expected3) {
		t.Errorf("got %v, want %v", t3, expected3)
	}

	// Test 4: 12-hour PM: 10:10pm -> 2026-10-28 22:10:00
	t4, err := ParseWallClock("10:10pm", refTime, loc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected4 := time.Date(2026, 10, 28, 22, 10, 0, 0, loc)
	if !t4.Equal(expected4) {
		t.Errorf("got %v, want %v", t4, expected4)
	}

	// Test 5: 12:00am (midnight) -> 00:00 tomorrow
	t5, err := ParseWallClock("12:00am", refTime, loc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected5 := time.Date(2026, 10, 29, 0, 0, 0, 0, loc)
	if !t5.Equal(expected5) {
		t.Errorf("got %v, want %v", t5, expected5)
	}

	// Test 6: 12:00pm (noon) -> 12:00 today
	t6, err := ParseWallClock("12:00pm", refTime, loc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected6 := time.Date(2026, 10, 28, 12, 0, 0, 0, loc)
	if !t6.Equal(expected6) {
		t.Errorf("got %v, want %v", t6, expected6)
	}
}

func TestParseTTL(t *testing.T) {
	loc := time.UTC
	nowTime := time.Date(2026, 10, 28, 8, 0, 0, 0, loc)
	nowMs := nowTime.UnixMilli()

	// 1. Integer zero -> 0
	val0, err := ParseTTL(NewInt(0), nowMs, loc)
	if err != nil || val0 != 0 {
		t.Errorf("expected 0, got %d, err %v", val0, err)
	}

	// 2. Relative string duration "10ms"
	val1, err := ParseTTL(NewString("10ms"), nowMs, loc)
	if err != nil || val1 != nowMs+10 {
		t.Errorf("expected %d, got %d, err %v", nowMs+10, val1, err)
	}

	// 3. Symbol duration 30mins
	val2, err := ParseTTL(NewSymbol("30mins"), nowMs, loc)
	if err != nil || val2 != nowMs+30*60*1000 {
		t.Errorf("expected %d, got %d, err %v", nowMs+30*60*1000, val2, err)
	}

	// 4. Wall-clock string "10:00am" (2 hours in future)
	val3, err := ParseTTL(NewString("10:00am"), nowMs, loc)
	expected3 := time.Date(2026, 10, 28, 10, 0, 0, 0, loc).UnixMilli()
	if err != nil || val3 != expected3 {
		t.Errorf("expected %d, got %d, err %v", expected3, val3, err)
	}

	// 5. Raw epoch timestamp
	epoch := time.Date(2026, 12, 25, 0, 0, 0, 0, loc).UnixMilli()
	val4, err := ParseTTL(NewInt(epoch), nowMs, loc)
	if err != nil || val4 != epoch {
		t.Errorf("expected %d, got %d, err %v", epoch, val4, err)
	}
}
