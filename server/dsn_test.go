package main

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestParseMySQLURL(t *testing.T) {
	raw := "mysql://avnadmin:s3cret@db.example.com:14838/defaultdb?ssl-mode=REQUIRED"
	tgt, err := ParseTarget(raw)
	if err != nil {
		t.Fatal(err)
	}
	if tgt.Kind != kindMySQL || tgt.KindName != "mysql" {
		t.Fatalf("kind = %d %s", tgt.Kind, tgt.KindName)
	}
	if tgt.Endpoint != "avnadmin@db.example.com:14838/defaultdb" {
		t.Fatalf("endpoint = %s", tgt.Endpoint)
	}
	if tgt.Database != "defaultdb" {
		t.Fatalf("database = %s", tgt.Database)
	}
	if tgt.mysql.User != "avnadmin" || tgt.mysql.Passwd != "s3cret" {
		t.Fatalf("user/pass = %s / %s", tgt.mysql.User, tgt.mysql.Passwd)
	}
	if tgt.mysql.Addr != "db.example.com:14838" || tgt.mysql.DBName != "defaultdb" {
		t.Fatalf("addr/db = %s / %s", tgt.mysql.Addr, tgt.mysql.DBName)
	}
	if tgt.mysql.TLS == nil || !tgt.mysql.TLS.InsecureSkipVerify {
		t.Fatalf("ssl-mode=REQUIRED should encrypt without verifying the certificate, tls=%v", tgt.mysql.TLS)
	}
	if len(tgt.Extras) != 0 {
		t.Fatalf("ssl-mode must not be sent as a session variable: %#v", tgt.Extras)
	}
	if strings.Join(tgt.Options, ",") != "ssl-mode=REQUIRED" {
		t.Fatalf("options = %#v", tgt.Options)
	}
}

func TestParseMySQLMessyParams(t *testing.T) {
	raw := "mysql://u:p%40ss@10.0.0.8/app?ssl-mode=REQUIRED&charset=utf8mb4&timeout=5s&parseTime=true&time_zone=%27UTC%27&connection_limit=10&some-flag=1&allowPublicKeyRetrieval=true"
	tgt, err := ParseTarget(raw)
	if err != nil {
		t.Fatal(err)
	}
	if tgt.mysql.Passwd != "p@ss" {
		t.Fatalf("password = %q", tgt.mysql.Passwd)
	}
	if tgt.Endpoint != "u@10.0.0.8:3306/app" {
		t.Fatalf("endpoint = %s", tgt.Endpoint)
	}
	dsn := tgt.mysql.FormatDSN()
	for _, want := range []string{"charset=utf8mb4", "timeout=5s", "parseTime=true", "tls=skip-verify"} {
		if !strings.Contains(dsn, want) {
			t.Fatalf("dsn %s missing %s", dsn, want)
		}
	}
	if tgt.Extras["time_zone"] != "'UTC'" || tgt.Extras["connection_limit"] != "10" {
		t.Fatalf("extras = %#v", tgt.Extras)
	}
	if _, ok := tgt.Extras["some-flag"]; ok {
		t.Fatal("hyphenated junk should not be a session variable")
	}
	joined := strings.Join(tgt.Options, " ")
	if !strings.Contains(joined, "some-flag=1") || !strings.Contains(joined, "allowPublicKeyRetrieval=true") {
		t.Fatalf("options = %#v", tgt.Options)
	}
}

func TestParseMySQLPasswordWithAt(t *testing.T) {
	tgt, err := ParseTarget("mysql://user:p@ss@10.0.0.8:3307/db?ssl-mode=DISABLED")
	if err != nil {
		t.Fatal(err)
	}
	if tgt.mysql.Passwd != "p@ss" || tgt.mysql.Addr != "10.0.0.8:3307" {
		t.Fatalf("user=%s pass=%s addr=%s", tgt.mysql.User, tgt.mysql.Passwd, tgt.mysql.Addr)
	}
	if tgt.mysql.TLS != nil {
		t.Fatal("DISABLED should not enable TLS")
	}
}

func TestParseMySQLVerifyIdentity(t *testing.T) {
	tgt, err := ParseTarget("mysql://u:p@db.example.com/db?ssl-mode=VERIFY_IDENTITY")
	if err != nil {
		t.Fatal(err)
	}
	if tgt.mysql.TLS == nil || tgt.mysql.TLS.InsecureSkipVerify {
		t.Fatalf("verify identity should check the certificate: %#v", tgt.mysql.TLS)
	}
	if tgt.mysql.TLS.ServerName != "db.example.com" {
		t.Fatalf("server name = %q", tgt.mysql.TLS.ServerName)
	}
}

func TestParsePostgresURL(t *testing.T) {
	raw := "postgres://u:p%2Fword@pg.example.com:5432/app?ssl-mode=REQUIRED&application_name=dba&connect_timeout=5s&weird=1"
	tgt, err := ParseTarget(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pgx.ParseConfig(tgt.postgres)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "pg.example.com" || cfg.Port != 5432 || cfg.Database != "app" || cfg.User != "u" || cfg.Password != "p/word" {
		t.Fatalf("cfg host=%s port=%d db=%s user=%s pass=%s", cfg.Host, cfg.Port, cfg.Database, cfg.User, cfg.Password)
	}
	if cfg.TLSConfig == nil || !cfg.TLSConfig.InsecureSkipVerify {
		t.Fatalf("sslmode=require tls = %#v", cfg.TLSConfig)
	}
	if !strings.Contains(tgt.postgres, "sslmode=require") || !strings.Contains(tgt.postgres, "connect_timeout=5") {
		t.Fatalf("dsn = %s", tgt.postgres)
	}
	if tgt.Extras["application_name"] != "dba" || tgt.Extras["weird"] != "1" {
		t.Fatalf("extras = %#v", tgt.Extras)
	}
	if tgt.Endpoint != "u@pg.example.com:5432/app" {
		t.Fatalf("endpoint = %s", tgt.Endpoint)
	}
}

func TestParseRejectsIncompleteURL(t *testing.T) {
	for _, raw := range []string{"", "localhost:3306", "redis://localhost/db", "mysql://"} {
		if _, err := ParseTarget(raw); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}
