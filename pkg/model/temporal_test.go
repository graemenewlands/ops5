package model_test

import (
	"testing"
	"time"

	"github.com/graemenewlands/ops5/pkg/model"
)

func TestTemporalDayAdd(t *testing.T) {
	// Date addition
	d := model.NewDate(20260929)
	dPlus5, err := d.DayAdd(5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dPlus5.DateInt() != 20261004 {
		t.Fatalf("expected 20261004, got %d", dPlus5.DateInt())
	}
	if !dPlus5.IsDate() {
		t.Fatalf("expected TypeDate, got %s", dPlus5.Type())
	}

	// Negative day addition (subtract days across month boundary)
	dMinus30, err := d.DayAdd(-30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dMinus30.DateInt() != 20260830 {
		t.Fatalf("expected 20260830, got %d", dMinus30.DateInt())
	}

	// DateTime addition
	dt := model.NewDateTime("2026-09-29T14:30:00")
	dtPlus1, err := dt.DayAdd(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dtPlus1.String() != "2026-09-30T14:30:00" {
		t.Fatalf("expected 2026-09-30T14:30:00, got %s", dtPlus1.String())
	}
	if !dtPlus1.IsDateTime() {
		t.Fatalf("expected TypeDateTime, got %s", dtPlus1.Type())
	}

	// DateUTCTime addition
	utc := model.NewDateUTCDirect("2026-09-29T21:30:00Z")
	utcPlus2, err := utc.DayAdd(2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if utcPlus2.String() != "2026-10-01T21:30:00Z" {
		t.Fatalf("expected 2026-10-01T21:30:00Z, got %s", utcPlus2.String())
	}
	if !utcPlus2.IsDateUTCTime() {
		t.Fatalf("expected TypeDateUTCTime, got %s", utcPlus2.Type())
	}

	// On integer date (20260929)
	iDate := model.NewInt(20260929)
	iPlus2, err := iDate.DayAdd(2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if iPlus2.DateInt() != 20261001 {
		t.Fatalf("expected 20261001, got %d", iPlus2.DateInt())
	}
}

func TestTemporalMonthAndYearAdd(t *testing.T) {
	d := model.NewDate(20260929)

	// MonthAdd positive
	dMonthPlus, err := d.MonthAdd(2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dMonthPlus.DateInt() != 20261129 {
		t.Fatalf("expected 20261129, got %d", dMonthPlus.DateInt())
	}

	// MonthAdd negative crossing year boundary
	dMonthMinus, err := d.MonthAdd(-10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dMonthMinus.DateInt() != 20251129 {
		t.Fatalf("expected 20251129, got %d", dMonthMinus.DateInt())
	}

	// YearAdd positive & negative
	dYearPlus, err := d.YearAdd(3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dYearPlus.DateInt() != 20290929 {
		t.Fatalf("expected 20290929, got %d", dYearPlus.DateInt())
	}
	dYearMinus, err := d.YearAdd(-1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dYearMinus.DateInt() != 20250929 {
		t.Fatalf("expected 20250929, got %d", dYearMinus.DateInt())
	}
}

func TestTemporalClamping(t *testing.T) {
	// 1. Jan 31 + 1 month in non-leap year (2026) -> Feb 28
	jan31 := model.NewDate(20260131)
	feb28, err := jan31.MonthAdd(1)
	if err != nil || feb28.DateInt() != 20260228 {
		t.Fatalf("expected 20260228, got %v (err=%v)", feb28, err)
	}

	// 2. Jan 31 + 1 month in leap year (2024) -> Feb 29
	jan31Leap := model.NewDate(20240131)
	feb29, err := jan31Leap.MonthAdd(1)
	if err != nil || feb29.DateInt() != 20240229 {
		t.Fatalf("expected 20240229, got %v (err=%v)", feb29, err)
	}

	// 3. Mar 31 - 1 month in non-leap year -> Feb 28
	mar31 := model.NewDate(20260331)
	feb28Sub, err := mar31.MonthAdd(-1)
	if err != nil || feb28Sub.DateInt() != 20260228 {
		t.Fatalf("expected 20260228, got %v (err=%v)", feb28Sub, err)
	}

	// 4. Mar 31 - 1 month in leap year -> Feb 29
	mar31Leap := model.NewDate(20240331)
	feb29Sub, err := mar31Leap.MonthAdd(-1)
	if err != nil || feb29Sub.DateInt() != 20240229 {
		t.Fatalf("expected 20240229, got %v (err=%v)", feb29Sub, err)
	}

	// 5. Aug 31 + 1 month -> Sep 30 (30-day month)
	aug31 := model.NewDate(20260831)
	sep30, err := aug31.MonthAdd(1)
	if err != nil || sep30.DateInt() != 20260930 {
		t.Fatalf("expected 20260930, got %v (err=%v)", sep30, err)
	}

	// 6. Oct 31 + 1 month -> Nov 30 (30-day month)
	oct31 := model.NewDate(20261031)
	nov30, err := oct31.MonthAdd(1)
	if err != nil || nov30.DateInt() != 20261130 {
		t.Fatalf("expected 20261130, got %v (err=%v)", nov30, err)
	}

	// 7. May 31 + 1 month -> Jun 30 (30-day month)
	may31 := model.NewDate(20260531)
	jun30, err := may31.MonthAdd(1)
	if err != nil || jun30.DateInt() != 20260630 {
		t.Fatalf("expected 20260630, got %v (err=%v)", jun30, err)
	}

	// 8. Leap day 2024-02-29 + 1 year -> 2025-02-28
	leapDay := model.NewDate(20240229)
	nonLeapNextYear, err := leapDay.YearAdd(1)
	if err != nil || nonLeapNextYear.DateInt() != 20250228 {
		t.Fatalf("expected 20250228, got %v (err=%v)", nonLeapNextYear, err)
	}

	// 9. Leap day 2024-02-29 + 4 years -> 2028-02-29 (leap year again)
	leap4Years, err := leapDay.YearAdd(4)
	if err != nil || leap4Years.DateInt() != 20280229 {
		t.Fatalf("expected 20280229, got %v (err=%v)", leap4Years, err)
	}

	// 10. Clamping on DateTime preserves time of day
	dtJan31 := model.NewDateTime("2026-01-31T15:45:00")
	dtFeb28, err := dtJan31.MonthAdd(1)
	if err != nil || dtFeb28.String() != "2026-02-28T15:45:00" {
		t.Fatalf("expected 2026-02-28T15:45:00, got %s (err=%v)", dtFeb28.String(), err)
	}
}

func TestTemporalTimeAdd(t *testing.T) {
	dt := model.NewDateTime("2026-09-29T10:00:00")

	// HourAdd
	dtHourPlus, err := dt.HourAdd(5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dtHourPlus.String() != "2026-09-29T15:00:00" {
		t.Fatalf("expected 2026-09-29T15:00:00, got %s", dtHourPlus.String())
	}
	dtHourMinus, err := dt.HourAdd(-12)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dtHourMinus.String() != "2026-09-28T22:00:00" {
		t.Fatalf("expected 2026-09-28T22:00:00, got %s", dtHourMinus.String())
	}

	// MinuteAdd
	dtMinPlus, err := dt.MinuteAdd(45)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dtMinPlus.String() != "2026-09-29T10:45:00" {
		t.Fatalf("expected 2026-09-29T10:45:00, got %s", dtMinPlus.String())
	}

	// SecondsAdd / SecondAdd
	dtSecPlus, err := dt.SecondsAdd(90)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dtSecPlus.String() != "2026-09-29T10:01:30" {
		t.Fatalf("expected 2026-09-29T10:01:30, got %s", dtSecPlus.String())
	}

	// HourAdd on TypeDate (promotes to TypeDateTime if non-midnight)
	d := model.NewDate(20260929)
	dHour, err := d.HourAdd(3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !dHour.IsDateTime() {
		t.Fatalf("expected TypeDateTime, got %s", dHour.Type())
	}
	if dHour.Time().Hour() != 3 {
		t.Fatalf("expected hour 3, got %d", dHour.Time().Hour())
	}
}

func TestDateDiff(t *testing.T) {
	// Date difference in seconds
	d1 := model.NewDate(20260930)
	d2 := model.NewDate(20260928)

	// Positive diff: d1 - d2 = 2 days = 172800 seconds
	diffPos, err := model.DateDiff(d1, d2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diffPos.Raw().(int64) != 172800 {
		t.Fatalf("expected 172800, got %d", diffPos.Raw().(int64))
	}

	// Negative diff: d2 - d1 = -172800 seconds
	diffNeg, err := model.DateDiff(d2, d1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diffNeg.Raw().(int64) != -172800 {
		t.Fatalf("expected -172800, got %d", diffNeg.Raw().(int64))
	}

	// Cross-timezone diff between UTC and local
	loc := time.FixedZone("PDT", -7*3600)
	dtLocal := model.NewDateTimeInLocation("2026-09-29T12:00:00", loc)
	dtUTC := model.NewDateUTCDirect("2026-09-29T19:00:00Z")
	// Both represent the exact same instant
	diffSame, err := model.DateDiff(dtLocal, dtUTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diffSame.Raw().(int64) != 0 {
		t.Fatalf("expected 0 seconds difference, got %d", diffSame.Raw().(int64))
	}

	// 1 hour later
	dtLater := model.NewDateUTCDirect("2026-09-29T20:00:00Z")
	diff1Hr, err := model.DateDiff(dtLater, dtLocal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff1Hr.Raw().(int64) != 3600 {
		t.Fatalf("expected 3600 seconds, got %d", diff1Hr.Raw().(int64))
	}
}

func TestMinutesHoursDaysConversions(t *testing.T) {
	// 172800 seconds = 2 days = 48 hours = 2880 minutes
	secs := model.NewInt(172800)

	days, err := model.DaysValue(secs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if days.Raw().(int64) != 2 {
		t.Fatalf("expected 2 days, got %d", days.Raw().(int64))
	}

	hours, err := model.HoursValue(secs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hours.Raw().(int64) != 48 {
		t.Fatalf("expected 48 hours, got %d", hours.Raw().(int64))
	}

	mins, err := model.MinutesValue(secs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mins.Raw().(int64) != 2880 {
		t.Fatalf("expected 2880 minutes, got %d", mins.Raw().(int64))
	}

	// Negative seconds
	negSecs := model.NewInt(-7200) // -2 hours
	negHours, _ := model.HoursValue(negSecs)
	if negHours.Raw().(int64) != -2 {
		t.Fatalf("expected -2 hours, got %d", negHours.Raw().(int64))
	}

	// Package standalone functions
	if model.Minutes(120).Raw().(int64) != 2 {
		t.Fatalf("expected 2 minutes")
	}
	if model.Hours(7200).Raw().(int64) != 2 {
		t.Fatalf("expected 2 hours")
	}
	if model.Days(86400).Raw().(int64) != 1 {
		t.Fatalf("expected 1 day")
	}
}

func TestExtractTemporalAndInt(t *testing.T) {
	d := model.NewDate(20260929)
	n := model.NewInt(5)

	// Order: date, int
	d1, n1, err := model.ExtractTemporalAndInt(d, n)
	if err != nil || d1.DateInt() != 20260929 || n1 != 5 {
		t.Fatalf("extract failed for (date, int): d=%v n=%d err=%v", d1, n1, err)
	}

	// Order: int, date (reversed)
	d2, n2, err := model.ExtractTemporalAndInt(n, d)
	if err != nil || d2.DateInt() != 20260929 || n2 != 5 {
		t.Fatalf("extract failed for (int, date): d=%v n=%d err=%v", d2, n2, err)
	}

	// Two raw integers: 20260929 and -3
	iDate := model.NewInt(20260929)
	iOffset := model.NewInt(-3)
	d3, n3, err := model.ExtractTemporalAndInt(iDate, iOffset)
	if err != nil || d3.DateInt() != 20260929 || n3 != -3 {
		t.Fatalf("extract failed for (20260929, -3): d=%v n=%d err=%v", d3, n3, err)
	}
}
