// Package mysql connects rio to MySQL through the
// github.com/go-sql-driver/mysql driver.
//
// The package is deliberately thin: it constructs handles, keeps the DSN
// honest about parseTime, and translates the driver's error numbers into
// rio's sentinel errors. All SQL generation lives in github.com/go-rio/rio.
package mysql

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/go-rio/rio"
	"github.com/go-sql-driver/mysql"
)

// Open opens a MySQL database for rio. The DSN uses the go-sql-driver format:
// user:password@tcp(host:port)/dbname?param=value.
//
// rio scans DATETIME and TIMESTAMP columns into time.Time, which requires the
// driver option parseTime=true. Open therefore appends parseTime=true when
// the DSN does not mention the option, and returns an error when the DSN
// explicitly sets parseTime=false instead of silently overriding a choice the
// caller spelled out. Every other option — including loc — passes through
// untouched, so existing data keeps its meaning; the README explains why new
// applications should store UTC.
//
// Like database/sql, Open validates its arguments without connecting; ping
// the handle returned by Unwrap to verify the server is reachable.
func Open(dsn string, opts ...rio.Option) (*rio.DB, error) {
	dsn, err := sanitizeDSN(dsn)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("mysql: open: %w", err)
	}
	return New(db, opts...), nil
}

// New wraps an existing *sql.DB in a rio handle with the MySQL dialect and
// this package's error translator installed. The caller's options apply after
// the translator, so a rio.WithErrorTranslator among them replaces it.
// New performs no DSN hygiene — the *sql.DB is the caller's; make sure it was
// opened with parseTime=true, or time.Time columns will fail to scan.
func New(db *sql.DB, opts ...rio.Option) *rio.DB {
	return rio.New(db, rio.MySQL, append([]rio.Option{rio.WithErrorTranslator(translate)}, opts...)...)
}

// sanitizeDSN validates the DSN and guarantees the parseTime=true driver mode
// rio depends on. A DSN that never mentions parseTime gains parseTime=true; a
// DSN that already sets it to true is returned byte for byte; a DSN that
// explicitly sets it to false is rejected rather than silently rewritten.
func sanitizeDSN(dsn string) (string, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "", fmt.Errorf("mysql: parse dsn: %w", err)
	}
	if cfg.ParseTime {
		return dsn, nil
	}
	if hasExplicitParseTime(dsn) {
		return "", errors.New("mysql: dsn sets parseTime=false, but rio needs parseTime=true to scan DATETIME and TIMESTAMP columns into time.Time; drop the option or set it to true")
	}
	cfg.ParseTime = true
	return cfg.FormatDSN(), nil
}

// hasExplicitParseTime reports whether the DSN's parameter section spells out
// a parseTime option. It locates the section exactly the way the driver's
// ParseDSN does — after the first '?' following the last '/' — and, like the
// driver, considers only key=value pairs whose key is exactly "parseTime".
// The caller has already run ParseDSN successfully, so the '/' exists and the
// option's value is known to be a valid bool.
func hasExplicitParseTime(dsn string) bool {
	rest := dsn[strings.LastIndexByte(dsn, '/')+1:]
	_, params, found := strings.Cut(rest, "?")
	if !found {
		return false
	}
	for kv := range strings.SplitSeq(params, "&") {
		if key, _, ok := strings.Cut(kv, "="); ok && key == "parseTime" {
			return true
		}
	}
	return false
}

// translate maps the driver's server error numbers onto rio sentinels.
// Returning nil means "not mine"; rio keeps the driver error in the chain
// either way, so errors.As still reaches *mysql.MySQLError.
func translate(err error) error {
	var me *mysql.MySQLError
	if !errors.As(err, &me) {
		return nil
	}
	switch me.Number {
	case 1062: // ER_DUP_ENTRY: duplicate entry for a unique key.
		return rio.ErrDuplicateKey
	case 1451, 1452: // ER_ROW_IS_REFERENCED_2 and ER_NO_REFERENCED_ROW_2: a row is still referenced, or the referenced row is missing.
		return rio.ErrForeignKeyViolated
	}
	return nil
}
