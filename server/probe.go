package main

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func (t *Target) Open() (*sql.DB, error) {
	var db *sql.DB
	if t.mysql != nil {
		connector, err := mysql.NewConnector(t.mysql.Clone())
		if err != nil {
			return nil, err
		}
		db = sql.OpenDB(connector)
	} else {
		cfg, err := pgx.ParseConfig(t.postgres)
		if err != nil {
			return nil, err
		}
		db = stdlib.OpenDB(*cfg)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(time.Minute)
	return db, nil
}

type sampleResult struct {
	Online bool
	Bytes  int64
	Error  string
}

func (t *Target) Sample(ctx context.Context) sampleResult {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	db, err := t.Open()
	if err != nil {
		return sampleResult{Error: safeErr(err, t.Secret)}
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return sampleResult{Error: safeErr(err, t.Secret)}
	}

	warn := applyExtras(ctx, db, t.Kind, t.Extras)
	query, args := t.usageQuery()
	var bytes int64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&bytes); err != nil {
		msg := safeErr(err, t.Secret)
		if warn != "" {
			msg = warn + "；" + msg
		}
		return sampleResult{Online: true, Error: msg}
	}
	if bytes < 0 {
		bytes = 0
	}
	return sampleResult{Online: true, Bytes: bytes, Error: warn}
}

func (t *Target) usageQuery() (string, []any) {
	if t.Kind == kindPostgres {
		return `SELECT COALESCE(pg_database_size(current_database()), 0)`, nil
	}
	if t.Database == "" {
		return `SELECT CAST(COALESCE(SUM(DATA_LENGTH + INDEX_LENGTH), 0) AS SIGNED)
FROM information_schema.TABLES
WHERE TABLE_SCHEMA NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys')`, nil
	}
	return `SELECT CAST(COALESCE(SUM(DATA_LENGTH + INDEX_LENGTH), 0) AS SIGNED)
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = ?`, []any{t.Database}
}

func applyExtras(ctx context.Context, db *sql.DB, kind int16, extras map[string]string) string {
	if len(extras) == 0 {
		return ""
	}
	keys := make([]string, 0, len(extras))
	for k := range extras {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var warns []string
	for _, key := range keys {
		if !identRe.MatchString(key) {
			warns = append(warns, key+" 已忽略")
			continue
		}
		var err error
		if kind == kindPostgres {
			_, err = db.ExecContext(ctx, `SELECT set_config($1, $2, false)`, key, extras[key])
		} else {
			_, err = db.ExecContext(ctx, "SET SESSION `"+key+"` = ?", extras[key])
		}
		if err != nil {
			warns = append(warns, key+" 未生效")
		}
	}
	return strings.Join(warns, "；")
}

func safeErr(err error, secret string) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if secret != "" {
		msg = strings.ReplaceAll(msg, secret, "***")
	}
	if len(msg) > 400 {
		msg = msg[:400]
	}
	return msg
}
