package calendar

import (
	"testing"
	"time"
)

func seoul(t *testing.T) Zone {
	t.Helper()
	z, err := Named("Asia/Seoul")
	if err != nil {
		t.Fatal(err)
	}
	return z
}

func la(t *testing.T) Zone {
	t.Helper()
	z, err := Named("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	return z
}

func TestDayInstantKST(t *testing.T) {
	// 2026-08-18 01:00 KST is stored as 2026-08-17T16:00:00.000Z.
	day, ok := Day("2026-08-17T16:00:00.000Z", Instant, seoul(t))
	if !ok || day != "2026-08-18" {
		t.Fatalf("got %q ok=%v", day, ok)
	}
	utc, ok := Day("2026-08-17T16:00:00.000Z", Instant, UTC())
	if !ok || utc != "2026-08-17" {
		t.Fatalf("utc day %q ok=%v", utc, ok)
	}
}

func TestInRangeKSTCreatedFrom(t *testing.T) {
	if !InRange("2026-08-17T16:00:00.000Z", Instant, "2026-08-18", "", seoul(t)) {
		t.Fatal("KST 01:00 must match created_from=2026-08-18")
	}
	if InRange("2026-08-17T16:00:00.000Z", Instant, "2026-08-18", "", UTC()) {
		t.Fatal("UTC calendar day is the 17th; must not match from=18")
	}
}

func TestDateOnlyNotUTCShifted(t *testing.T) {
	day, ok := Day("2026-08-20", Date, la(t))
	if !ok || day != "2026-08-20" {
		t.Fatalf("date-only must stay 2026-08-20, got %q ok=%v", day, ok)
	}
	// Same string passed as Instant must not go through UTC midnight either.
	day, ok = Day("2026-08-20", Instant, la(t))
	if !ok || day != "2026-08-20" {
		t.Fatalf("date-only instant must stay 2026-08-20, got %q ok=%v", day, ok)
	}
}

func TestInRangeEmptyRaw(t *testing.T) {
	if InRange("", Instant, "2026-08-18", "", seoul(t)) {
		t.Fatal("empty timestamp with a lower bound must fail")
	}
	if !InRange("", Instant, "", "", seoul(t)) {
		t.Fatal("empty timestamp with no lower bound must pass")
	}
}

func TestExplainReportsDecision(t *testing.T) {
	d := Explain("2026-08-17T16:00:00.000Z", Instant, seoul(t))
	if !d.OK || d.CalendarDay != "2026-08-18" || d.Kind != "instant" || d.Zone != "Asia/Seoul" {
		t.Fatalf("%+v", d)
	}
	d = Explain("2026-08-20", Date, la(t))
	if !d.OK || d.CalendarDay != "2026-08-20" || d.Kind != "date" {
		t.Fatalf("%+v", d)
	}
}

func TestStartOfWeekMonday(t *testing.T) {
	// Wednesday 2026-08-19 01:00 KST → Monday 2026-08-17.
	now := time.Date(2026, 8, 18, 16, 0, 0, 0, time.UTC)
	got := StartOfWeekMonday(now, seoul(t))
	if got != "2026-08-17" {
		t.Fatalf("got %s", got)
	}
}

func TestFormatDayUsesZone(t *testing.T) {
	ts := time.Date(2026, 8, 17, 16, 0, 0, 0, time.UTC)
	if got := FormatDay(ts, seoul(t)); got != "2026-08-18" {
		t.Fatalf("got %s", got)
	}
}
