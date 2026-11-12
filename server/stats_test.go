package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestStatsHistoryAndLatest(t *testing.T) {
	store, err := OpenStats(filepath.Join(t.TempDir(), "stats.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	rows := []Stat{
		{ItemID: "a", Online: true, Bytes: 10, At: 100},
		{ItemID: "a", Online: false, Bytes: 0, Error: "down", At: 200},
		{ItemID: "b", Online: true, Bytes: 50, At: 150},
	}
	for _, row := range rows {
		if err := store.Insert(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := store.Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if latest["a"].At != 200 || latest["a"].Online || latest["a"].Error != "down" {
		t.Fatalf("latest a = %#v", latest["a"])
	}
	if latest["b"].Bytes != 50 {
		t.Fatalf("latest b = %#v", latest["b"])
	}
	hist, err := store.History(ctx, "a", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 || hist[0].At != 100 || hist[1].At != 200 {
		t.Fatalf("history = %#v", hist)
	}
	if err := store.DeleteItem(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	hist, err = store.History(ctx, "a", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 0 {
		t.Fatalf("history after delete = %#v", hist)
	}
}

func TestTargetRoundTrip(t *testing.T) {
	store, err := OpenStats(filepath.Join(t.TempDir(), "stats.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	item, err := store.InsertTarget(ctx, itemDoc{Name: "main", Type: kindMySQL, DSN: "mysql://u:p@127.0.0.1:3306/app?ssl-mode=REQUIRED"})
	if err != nil {
		t.Fatal(err)
	}
	if len(item.ID) != 32 {
		t.Fatalf("id = %s", item.ID)
	}
	got, err := store.GetTarget(ctx, item.ID)
	if err != nil || got.Name != "main" || got.Type != kindMySQL {
		t.Fatalf("got %#v err %v", got, err)
	}
	got.Name = "renamed"
	if err := store.UpdateTarget(ctx, got); err != nil {
		t.Fatal(err)
	}
	list, err := store.ListTargets(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "renamed" {
		t.Fatalf("list %#v err %v", list, err)
	}
	if err := store.DeleteTarget(ctx, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetTarget(ctx, item.ID); !errors.Is(err, errNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}
