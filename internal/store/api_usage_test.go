package store

import (
	"testing"
)

func TestAPIUsageAccumulatesAndOrders(t *testing.T) {
	db := openTemp(t)
	if db.SchemaVersion() != len(migrations) {
		t.Fatalf("schema version %d, want %d", db.SchemaVersion(), len(migrations))
	}

	if err := db.AddAPIUsage("2026-08-04", APIUsageDelta{
		Requests: 10, Throttled: 1, ServerErrors: 2, Retries: 3, WaitMS: 100,
		LastThrottledAt: "2026-08-04T10:00:00.000Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.AddAPIUsage("2026-08-04", APIUsageDelta{
		Requests: 5, Throttled: 1, ServerErrors: 0, Retries: 1, WaitMS: 50,
		LastThrottledAt: "2026-08-04T14:00:00.000Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.AddAPIUsage("2026-08-05", APIUsageDelta{
		Requests: 7, Retries: 0,
	}); err != nil {
		t.Fatal(err)
	}
	// Empty last_throttled_at on a later flush must not wipe the stored value.
	if err := db.AddAPIUsage("2026-08-04", APIUsageDelta{Requests: 1}); err != nil {
		t.Fatal(err)
	}

	days, err := db.APIUsage(7)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 {
		t.Fatalf("len = %d, want 2", len(days))
	}
	if days[0].Day != "2026-08-05" || days[1].Day != "2026-08-04" {
		t.Fatalf("order = %s, %s; want newest first", days[0].Day, days[1].Day)
	}
	d4 := days[1]
	if d4.Requests != 16 || d4.Throttled != 2 || d4.ServerErrors != 2 || d4.Retries != 4 || d4.WaitMS != 150 {
		t.Errorf("2026-08-04 accum = %+v", d4)
	}
	if d4.LastThrottledAt == nil || *d4.LastThrottledAt != "2026-08-04T14:00:00.000Z" {
		t.Errorf("last_throttled_at = %v, want 14:00Z", d4.LastThrottledAt)
	}
	if days[0].Requests != 7 {
		t.Errorf("2026-08-05 requests = %d", days[0].Requests)
	}
}

func TestNewDBSchemaVersionMatchesMigrations(t *testing.T) {
	db := openTemp(t)
	want := len(migrations)
	if got := db.SchemaVersion(); got != want {
		t.Fatalf("SchemaVersion = %d, want %d", got, want)
	}
	var uv int
	if err := db.sql.QueryRow("PRAGMA user_version").Scan(&uv); err != nil {
		t.Fatal(err)
	}
	if uv != want {
		t.Fatalf("PRAGMA user_version = %d, want %d", uv, want)
	}
}
