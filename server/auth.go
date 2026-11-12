package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	totpDigits    = 6
	totpPeriod    = 30
	sessionTTL    = 12 * time.Hour
	maxLoginFails = 5
	loginLockout  = time.Minute
)

var errAuthFailed = errors.New("auth failed")

func hotpCode(secret []byte, counter uint64, digits int) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, secret)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, bin%mod)
}

func totpCode(secret []byte, at time.Time) (string, uint64) {
	step := uint64(at.Unix()) / totpPeriod
	return hotpCode(secret, step, totpDigits), step
}

func decodeBase32(s string) ([]byte, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.TrimRight(s, "=")
	return base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
}

func randomBase32(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf), nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (s *StatsStore) EnsureAdmin(ctx context.Context) (created bool, accessKey, otpauth string, err error) {
	var n int
	if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_auth`).Scan(&n); err != nil {
		return false, "", "", err
	}
	if n > 0 {
		return false, "", "", nil
	}
	keyMaterial, err := randomBase32(20)
	if err != nil {
		return false, "", "", err
	}
	accessKey = "dba_" + keyMaterial
	secret, err := randomBase32(20)
	if err != nil {
		return false, "", "", err
	}
	if _, err = s.db.ExecContext(ctx,
		`INSERT INTO admin_auth (id, access_key_hash, totp_secret, last_step) VALUES (1, ?, ?, 0)`,
		sha256Hex(accessKey), secret,
	); err != nil {
		return false, "", "", err
	}
	return true, accessKey, otpauthURL(secret), nil
}

func otpauthURL(secret string) string {
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", "DBA")
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprintf("%d", totpDigits))
	q.Set("period", fmt.Sprintf("%d", totpPeriod))
	return "otpauth://totp/" + url.PathEscape("DBA:admin") + "?" + q.Encode()
}

func (s *StatsStore) Login(ctx context.Context, accessKey, code string, now time.Time) (token string, expiresAt int64, err error) {
	accessKey = strings.TrimSpace(accessKey)
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	var hash, secret string
	var lastStep int64
	err = s.db.QueryRowContext(ctx, `SELECT access_key_hash, totp_secret, last_step FROM admin_auth WHERE id = 1`).
		Scan(&hash, &secret, &lastStep)
	if err != nil {
		return "", 0, err
	}
	keyOK := subtle.ConstantTimeCompare([]byte(hash), []byte(sha256Hex(accessKey))) == 1
	secretBytes, decErr := decodeBase32(secret)
	step, codeOK := int64(0), false
	if decErr == nil && len(code) == totpDigits {
		step, codeOK = matchTOTP(secretBytes, code, lastStep, now)
	}
	if !keyOK || !codeOK {
		return "", 0, errAuthFailed
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", 0, err
	}
	token = hex.EncodeToString(raw)
	expiresAt = now.Add(sessionTTL).Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE admin_auth SET last_step = ? WHERE id = 1 AND last_step = ?`, step, lastStep)
	if err != nil {
		return "", 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", 0, err
	}
	if n != 1 {
		return "", 0, errAuthFailed
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now.Unix()); err != nil {
		return "", 0, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO sessions (token_hash, expires_at) VALUES (?, ?)`, sha256Hex(token), expiresAt); err != nil {
		return "", 0, err
	}
	if err = tx.Commit(); err != nil {
		return "", 0, err
	}
	return token, expiresAt, nil
}

func matchTOTP(secret []byte, code string, lastStep int64, now time.Time) (int64, bool) {
	base := now.Unix() / totpPeriod
	for _, delta := range []int64{-1, 0, 1} {
		step := base + delta
		if step < 0 || step <= lastStep {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(hotpCode(secret, uint64(step), totpDigits)), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

func (s *StatsStore) ValidSession(ctx context.Context, token string, now time.Time) (bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return false, nil
	}
	var expires int64
	err := s.db.QueryRowContext(ctx, `SELECT expires_at FROM sessions WHERE token_hash = ?`, sha256Hex(token)).Scan(&expires)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if expires <= now.Unix() {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, sha256Hex(token))
		return false, nil
	}
	return true, nil
}

func (s *StatsStore) Logout(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, sha256Hex(token))
	return err
}

type attemptLimiter struct {
	mu    sync.Mutex
	state map[string]attemptState
}

type attemptState struct {
	fails int
	until time.Time
}

func newAttemptLimiter() *attemptLimiter {
	return &attemptLimiter{state: map[string]attemptState{}}
}

func (l *attemptLimiter) blocked(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	st := l.state[key]
	return st.fails >= maxLoginFails && now.Before(st.until)
}

func (l *attemptLimiter) fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	st := l.state[key]
	if !st.until.IsZero() && !now.Before(st.until) {
		st = attemptState{}
	}
	st.fails++
	if st.fails >= maxLoginFails {
		st.until = now.Add(loginLockout)
	}
	l.state[key] = st
}

func (l *attemptLimiter) success(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.state, key)
}
