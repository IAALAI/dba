package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestTOTPRFC6238(t *testing.T) {
	secret := []byte("12345678901234567890")
	cases := []struct {
		sec  uint64
		code string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	}
	for _, tc := range cases {
		if got := hotpCode(secret, tc.sec/30, 8); got != tc.code {
			t.Fatalf("t=%d got %s want %s", tc.sec, got, tc.code)
		}
	}
	if got := hotpCode(secret, 59/30, 6); got != "287082" {
		t.Fatalf("6 digits got %s", got)
	}
}

func TestLoginSessionAndReplay(t *testing.T) {
	store, err := OpenStats(filepath.Join(t.TempDir(), "stats.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	created, key, uri, err := store.EnsureAdmin(ctx)
	if err != nil || !created || key == "" || uri == "" {
		t.Fatalf("created=%v key=%q uri=%q err=%v", created, key, uri, err)
	}
	again, key2, _, err := store.EnsureAdmin(ctx)
	if err != nil || again || key2 != "" {
		t.Fatalf("second ensure created=%v key=%q err=%v", again, key2, err)
	}

	var secret string
	if err := store.db.QueryRowContext(ctx, `SELECT totp_secret FROM admin_auth WHERE id = 1`).Scan(&secret); err != nil {
		t.Fatal(err)
	}
	raw, err := decodeBase32(secret)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0).UTC()
	code, _ := totpCode(raw, now)

	if _, _, err := store.Login(ctx, "wrong-key", code, now); !errors.Is(err, errAuthFailed) {
		t.Fatalf("wrong key err=%v", err)
	}
	token, exp, err := store.Login(ctx, key, code, now)
	if err != nil {
		t.Fatal(err)
	}
	if exp <= now.Unix() || token == "" {
		t.Fatalf("token=%q exp=%d", token, exp)
	}
	ok, err := store.ValidSession(ctx, token, now)
	if err != nil || !ok {
		t.Fatalf("session ok=%v err=%v", ok, err)
	}
	if _, _, err := store.Login(ctx, key, code, now); !errors.Is(err, errAuthFailed) {
		t.Fatalf("replay err=%v", err)
	}
	if err := store.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
	ok, err = store.ValidSession(ctx, token, now)
	if err != nil || ok {
		t.Fatalf("after logout ok=%v err=%v", ok, err)
	}

	var stored string
	if err := store.db.QueryRowContext(ctx, `SELECT access_key_hash FROM admin_auth`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == key || stored == sha256Hex("wrong-key") {
		t.Fatal("access key must be stored only as a hash")
	}
}
