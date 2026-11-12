package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
)

const (
	kindMySQL    int16 = 1
	kindPostgres int16 = 2
)

// Target is a monitored database parsed from a URL such as
// mysql://user:secret@host:3306/dbname?ssl-mode=REQUIRED&charset=utf8mb4.
type Target struct {
	Kind     int16
	KindName string
	Endpoint string
	Database string
	Options  []string
	Secret   string
	Extras   map[string]string

	mysql    *mysql.Config
	postgres string
}

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func ParseTarget(raw string) (*Target, error) {
	scheme, user, pass, hostport, database, query, err := splitConnURL(raw)
	if err != nil {
		return nil, err
	}
	opts := displayOptions(query)
	switch strings.ToLower(scheme) {
	case "mysql":
		return buildMySQL(user, pass, hostport, database, query, opts)
	case "postgres", "postgresql":
		return buildPostgres(user, pass, hostport, database, query, opts)
	default:
		return nil, fmt.Errorf("不支持的协议 %q，请使用 mysql:// 或 postgres://", scheme)
	}
}

func splitConnURL(raw string) (scheme, user, pass, hostport, database string, query url.Values, err error) {
	raw = strings.TrimSpace(raw)
	scheme, rest, ok := strings.Cut(raw, "://")
	if !ok || scheme == "" || strings.Contains(scheme, " ") {
		return "", "", "", "", "", nil, errors.New("连接串需要 mysql:// 或 postgres:// 开头")
	}
	var rawQuery string
	rest, rawQuery, _ = strings.Cut(rest, "?")
	rest, _, _ = strings.Cut(rest, "#")
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		user, pass, err = splitUserinfo(rest[:at])
		if err != nil {
			return "", "", "", "", "", nil, err
		}
		rest = rest[at+1:]
	}
	if slash := strings.Index(rest, "/"); slash >= 0 {
		hostport = rest[:slash]
		database = strings.TrimPrefix(rest[slash:], "/")
	} else {
		hostport = rest
	}
	if database, err = url.PathUnescape(database); err != nil {
		return "", "", "", "", "", nil, fmt.Errorf("数据库名无效: %w", err)
	}
	if i := strings.Index(database, "/"); i >= 0 {
		database = database[:i]
	}
	query, err = url.ParseQuery(rawQuery)
	if err != nil {
		return "", "", "", "", "", nil, fmt.Errorf("查询参数无效: %w", err)
	}
	if hostport == "" {
		return "", "", "", "", "", nil, errors.New("连接串缺少主机")
	}
	return scheme, user, pass, hostport, database, query, nil
}

func splitUserinfo(info string) (string, string, error) {
	name, secret, hasSecret := strings.Cut(info, ":")
	user, err := url.PathUnescape(name)
	if err != nil {
		return "", "", fmt.Errorf("用户名无效: %w", err)
	}
	if !hasSecret {
		return user, "", nil
	}
	pass, err := url.PathUnescape(secret)
	if err != nil {
		return "", "", fmt.Errorf("密码无效: %w", err)
	}
	return user, pass, nil
}

func displayOptions(q url.Values) []string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		v := q.Get(k)
		if secretKey(k) {
			v = "***"
		}
		out = append(out, k+"="+v)
	}
	return out
}

func secretKey(k string) bool {
	f := foldKey(k)
	return strings.Contains(f, "pass") || strings.Contains(f, "pwd") || strings.Contains(f, "secret") || strings.Contains(f, "token")
}

func foldKey(k string) string {
	k = strings.ToLower(strings.TrimSpace(k))
	k = strings.ReplaceAll(k, "-", "")
	k = strings.ReplaceAll(k, "_", "")
	return k
}

func endpoint(user, host, port, database string) string {
	var b strings.Builder
	if user != "" {
		b.WriteString(user)
		b.WriteByte('@')
	}
	b.WriteString(host)
	if port != "" {
		b.WriteByte(':')
		b.WriteString(port)
	}
	if database != "" {
		b.WriteByte('/')
		b.WriteString(database)
	}
	return b.String()
}

func splitHost(hostport, defaultPort string) (string, string, error) {
	if strings.HasPrefix(hostport, "[") && !strings.Contains(hostport, "]") {
		return "", "", fmt.Errorf("主机无效: %s", hostport)
	}
	if h, p, err := net.SplitHostPort(hostport); err == nil {
		if p == "" {
			p = defaultPort
		}
		return h, p, nil
	}
	if strings.HasPrefix(hostport, "[") && strings.HasSuffix(hostport, "]") {
		return strings.Trim(hostport, "[]"), defaultPort, nil
	}
	if strings.Contains(hostport, ":") {
		return "", "", fmt.Errorf("主机或端口无效: %s", hostport)
	}
	return hostport, defaultPort, nil
}

func canonicalSSLMode(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "false", "off", "disable", "disabled":
		return "disable", nil
	case "allow":
		return "allow", nil
	case "1", "true", "on", "prefer", "preferred":
		return "prefer", nil
	case "require", "required":
		return "require", nil
	case "verify_ca", "verify-ca":
		return "verify-ca", nil
	case "verify_identity", "verify-identity", "verify_full", "verify-full":
		return "verify-full", nil
	default:
		return "", fmt.Errorf("无法识别 ssl-mode=%s", v)
	}
}

func normalizeDuration(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", errors.New("时长为空")
	}
	if _, err := time.ParseDuration(v); err == nil {
		return v, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return "", fmt.Errorf("无效的时长 %q", v)
	}
	return strconv.Itoa(n) + "s", nil
}

func toSeconds(v string) (string, error) {
	v = strings.TrimSpace(v)
	if d, err := time.ParseDuration(v); err == nil {
		sec := int(d.Round(time.Second) / time.Second)
		if d > 0 && sec < 1 {
			sec = 1
		}
		return strconv.Itoa(sec), nil
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 0 {
		return strconv.Itoa(n), nil
	}
	return "", fmt.Errorf("无效的时长 %q", v)
}

var mysqlDriverAlias = map[string]string{
	"allowallfiles":            "allowAllFiles",
	"allowcleartextpasswords":  "allowCleartextPasswords",
	"allowfallbacktoplaintext": "allowFallbackToPlaintext",
	"allownativepasswords":     "allowNativePasswords",
	"allowoldpasswords":        "allowOldPasswords",
	"charset":                  "charset",
	"characterencoding":        "charset",
	"characterset":             "charset",
	"checkconnliveness":        "checkConnLiveness",
	"collation":                "collation",
	"clientfoundrows":          "clientFoundRows",
	"columnswithalias":         "columnsWithAlias",
	"compress":                 "compress",
	"interpolateparams":        "interpolateParams",
	"loc":                      "loc",
	"servertimezone":           "loc",
	"maxallowedpacket":         "maxAllowedPacket",
	"multistatements":          "multiStatements",
	"parsetime":                "parseTime",
	"readtimeout":              "readTimeout",
	"rejectreadonly":           "rejectReadOnly",
	"serverpubkey":             "serverPubKey",
	"timeout":                  "timeout",
	"connecttimeout":           "timeout",
	"connectiontimeout":        "timeout",
	"writetimeout":             "writeTimeout",
	"connectionattributes":     "connectionAttributes",
	"timetruncate":             "timeTruncate",
}

func buildMySQL(user, pass, hostport, database string, query url.Values, opts []string) (*Target, error) {
	host, port, err := splitHost(hostport, "3306")
	if err != nil {
		return nil, err
	}
	driverParams := url.Values{}
	extras := map[string]string{}
	var mode string
	var hasMode bool
	var tlsName string
	var caFile, certFile, keyFile string

	for key, values := range query {
		if len(values) == 0 {
			continue
		}
		val := values[0]
		fold := foldKey(key)
		if identityParam(fold) {
			continue
		}
		switch fold {
		case "sslmode":
			mode, err = canonicalSSLMode(val)
			if err != nil {
				return nil, err
			}
			hasMode = true
		case "tls":
			tlsName = val
		case "usessl", "requiressl":
			if !hasMode && tlsName == "" {
				on, err := strconv.ParseBool(val)
				if err != nil {
					return nil, fmt.Errorf("%s=%s 不是布尔值", key, val)
				}
				hasMode = true
				if on {
					mode = "require"
				} else {
					mode = "disable"
				}
			}
		case "sslca", "sslrootcert":
			caFile = val
		case "sslcert":
			certFile = val
		case "sslkey":
			keyFile = val
		case "sslpassword", "allowpublickeyretrieval", "useunicode", "autoreconnect", "enabledtlsprotocols", "tlsversions", "serversslcert", "verifyservercertificate":
			// 这些是 JDBC / 客户端参数，解析时收下，不当成 MySQL 会话变量下发。
		case "sockettimeout":
			d, err := normalizeDuration(val)
			if err != nil {
				return nil, err
			}
			if driverParams.Get("readTimeout") == "" {
				driverParams.Set("readTimeout", d)
			}
			if driverParams.Get("writeTimeout") == "" {
				driverParams.Set("writeTimeout", d)
			}
		default:
			if canon, ok := mysqlDriverAlias[fold]; ok {
				out := val
				switch canon {
				case "timeout", "readTimeout", "writeTimeout", "timeTruncate":
					out, err = normalizeDuration(val)
					if err != nil {
						return nil, err
					}
				}
				driverParams.Set(canon, out)
				continue
			}
			if identRe.MatchString(key) {
				extras[key] = val
			}
		}
	}

	switch {
	case tlsName != "":
	case hasMode:
		switch mode {
		case "disable":
			tlsName = "false"
		case "allow", "prefer":
			tlsName = "preferred"
		case "require":
			tlsName = "skip-verify"
		case "verify-ca", "verify-full":
			tlsName = "true"
		}
	}

	base := mysql.NewConfig()
	base.User = "user"
	base.Passwd = "pass"
	base.Net = "tcp"
	base.Addr = net.JoinHostPort(host, port)
	base.DBName = "db"
	base.ParseTime = true
	base.Timeout = 10 * time.Second
	base.ReadTimeout = 20 * time.Second
	base.WriteTimeout = 20 * time.Second
	if tlsName != "" {
		base.TLSConfig = tlsName
	}
	dsn := base.FormatDSN()
	if enc := driverParams.Encode(); enc != "" {
		dsn += "&" + enc
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("MySQL 参数无效: %w", err)
	}
	cfg.User = user
	cfg.Passwd = pass
	cfg.Addr = net.JoinHostPort(host, port)
	cfg.DBName = database
	if caFile != "" || certFile != "" || keyFile != "" {
		custom, err := mysqlTLS(host, mode, caFile, certFile, keyFile)
		if err != nil {
			return nil, err
		}
		cfg.TLS = custom
	}
	if opts == nil {
		opts = []string{}
	}
	return &Target{
		Kind:     kindMySQL,
		KindName: "mysql",
		Endpoint: endpoint(user, host, port, database),
		Database: database,
		Options:  opts,
		Secret:   pass,
		Extras:   extras,
		mysql:    cfg,
	}, nil
}

func mysqlTLS(host, mode, caFile, certFile, keyFile string) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile != "" {
		pemBytes, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("读取 ssl-ca: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, errors.New("ssl-ca 不是有效的 PEM 证书")
		}
		cfg.RootCAs = pool
	}
	if certFile != "" || keyFile != "" {
		if certFile == "" || keyFile == "" {
			return nil, errors.New("ssl-cert 和 ssl-key 需要同时提供")
		}
		pair, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("读取客户端证书: %w", err)
		}
		cfg.Certificates = []tls.Certificate{pair}
	}
	switch mode {
	case "", "require", "prefer", "allow":
		cfg.InsecureSkipVerify = true
	case "verify-ca":
		cfg.InsecureSkipVerify = true
		cfg.VerifyPeerCertificate = verifyCAOnly(cfg.RootCAs)
	default:
		cfg.ServerName = host
	}
	return cfg, nil
}

func verifyCAOnly(roots *x509.CertPool) func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return errors.New("服务器没有提供证书")
		}
		certs := make([]*x509.Certificate, len(rawCerts))
		for i, asn1Data := range rawCerts {
			cert, err := x509.ParseCertificate(asn1Data)
			if err != nil {
				return err
			}
			certs[i] = cert
		}
		opts := x509.VerifyOptions{Roots: roots, Intermediates: x509.NewCertPool()}
		for _, cert := range certs[1:] {
			opts.Intermediates.AddCert(cert)
		}
		_, err := certs[0].Verify(opts)
		return err
	}
}

func buildPostgres(user, pass, hostport, database string, query url.Values, opts []string) (*Target, error) {
	host, port, err := splitHost(hostport, "5432")
	if err != nil {
		return nil, err
	}
	kept := url.Values{}
	extras := map[string]string{}
	var mode string
	var hasMode bool

	for key, values := range query {
		if len(values) == 0 {
			continue
		}
		val := values[0]
		fold := foldKey(key)
		if identityParam(fold) {
			continue
		}
		switch fold {
		case "sslmode":
			mode, err = canonicalSSLMode(val)
			if err != nil {
				return nil, err
			}
			hasMode = true
		case "usessl", "requiressl":
			if !hasMode {
				on, err := strconv.ParseBool(val)
				if err != nil {
					return nil, fmt.Errorf("%s=%s 不是布尔值", key, val)
				}
				hasMode = true
				if on {
					mode = "require"
				} else {
					mode = "disable"
				}
			}
		case "sslca", "sslrootcert":
			kept.Set("sslrootcert", val)
		case "sslcert":
			kept.Set("sslcert", val)
		case "sslkey":
			kept.Set("sslkey", val)
		case "sslpassword":
			kept.Set("sslpassword", val)
		case "sslsni":
			kept.Set("sslsni", val)
		case "sslnegotiation":
			kept.Set("sslnegotiation", val)
		case "connecttimeout", "connectiontimeout":
			sec, err := toSeconds(val)
			if err != nil {
				return nil, err
			}
			kept.Set("connect_timeout", sec)
		case "targetsessionattrs":
			kept.Set("target_session_attrs", val)
		case "krbspn":
			kept.Set("krbspn", val)
		case "krbsrvname":
			kept.Set("krbsrvname", val)
		default:
			if identRe.MatchString(key) {
				extras[key] = val
			}
		}
	}
	if hasMode {
		kept.Set("sslmode", mode)
	}

	u := url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + database,
	}
	if pass != "" {
		u.User = url.UserPassword(user, pass)
	} else if user != "" {
		u.User = url.User(user)
	}
	u.RawQuery = kept.Encode()
	if _, err := pgx.ParseConfig(u.String()); err != nil {
		return nil, fmt.Errorf("Postgres 参数无效: %w", err)
	}
	if opts == nil {
		opts = []string{}
	}
	return &Target{
		Kind:     kindPostgres,
		KindName: "postgres",
		Endpoint: endpoint(user, host, port, database),
		Database: database,
		Options:  opts,
		Secret:   pass,
		Extras:   extras,
		postgres: u.String(),
	}, nil
}

func identityParam(fold string) bool {
	switch fold {
	case "user", "username", "password", "passwd", "pwd", "host", "port", "database", "dbname", "db":
		return true
	default:
		return false
	}
}
