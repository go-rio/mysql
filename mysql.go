// Package mysql connects rio to MySQL through go-sql-driver/mysql.
package mysql

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/go-rio/rio"
	"github.com/go-sql-driver/mysql"
)

// Open creates a rio database from a go-sql-driver DSN without connecting.
// It enables parseTime when omitted and rejects parseTime=false,
// clientFoundRows=true, and sql_mode values that change placeholder lexing
// (NO_BACKSLASH_ESCAPES, ANSI_QUOTES). Other DSN options pass through.
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

// New wraps db with the MySQL dialect and error translator. It does not
// validate the DSN; the caller must satisfy Open's parseTime, sql_mode, and
// clientFoundRows contract. Options may replace the default translator.
func New(db *sql.DB, opts ...rio.Option) *rio.DB {
	return rio.New(db, rio.MySQL, append([]rio.Option{rio.WithErrorTranslator(translate)}, opts...)...)
}

// sanitizeDSN enables parseTime when omitted and rejects incompatible options.
func sanitizeDSN(dsn string) (string, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "", fmt.Errorf("mysql: parse dsn: %w", err)
	}
	if bad := lexBreakingSQLMode(cfg.Params); bad != "" {
		return "", fmt.Errorf(
			"mysql: dsn sets a sql_mode containing %s, but rio rewrites ? placeholders "+
				"assuming MySQL's default lexing — backslash escapes inside strings, "+
				"double quotes delimiting strings — which that mode changes, "+
				"so the two would count placeholders differently; "+
				"use a sql_mode without NO_BACKSLASH_ESCAPES and ANSI_QUOTES",
			bad,
		)
	}
	if cfg.ClientFoundRows {
		return "", errors.New(
			"mysql: dsn sets clientFoundRows=true, but rio's write semantics are built on " +
				"MySQL's default changed-rows counting (upsert backfill, optimistic locking, " +
				"idempotent updates); drop the option",
		)
	}
	if cfg.ParseTime {
		return dsn, nil
	}
	if hasExplicitParseTime(dsn) {
		return "", errors.New(
			"mysql: dsn sets parseTime=false, but rio needs parseTime=true to scan DATETIME and TIMESTAMP " +
				"columns into time.Time; drop the option or set it to true",
		)
	}
	cfg.ParseTime = true
	return cfg.FormatDSN(), nil
}

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

// lexBreakingSQLMode returns the first sql_mode token that changes rio's
// assumed placeholder lexing. Matching is case-insensitive and token-based.
func lexBreakingSQLMode(params map[string]string) string {
	for key, val := range params {
		if !strings.EqualFold(key, "sql_mode") {
			continue
		}
		val = strings.TrimSpace(val)
		isSingleQuoted := len(val) >= 2 && val[0] == '\'' && val[len(val)-1] == '\''
		isDoubleQuoted := len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"'
		if isSingleQuoted || isDoubleQuoted {
			val = val[1 : len(val)-1]
		}
		for token := range strings.SplitSeq(val, ",") {
			switch token = strings.ToUpper(strings.TrimSpace(token)); token {
			case "NO_BACKSLASH_ESCAPES", "ANSI_QUOTES":
				return token
			case "ANSI", "DB2", "MAXDB", "MSSQL", "ORACLE", "POSTGRESQL":
				return token + " (which implies ANSI_QUOTES)"
			}
		}
	}
	return ""
}

// translate maps supported MySQL errors onto rio sentinels.
func translate(err error) error {
	var me *mysql.MySQLError
	if !errors.As(err, &me) {
		return nil
	}
	switch me.Number {
	case 1062: // ER_DUP_ENTRY
		return rio.ErrDuplicateKey
	case 1451, 1452: // ER_ROW_IS_REFERENCED_2, ER_NO_REFERENCED_ROW_2
		return rio.ErrForeignKeyViolated
	}
	return nil
}
