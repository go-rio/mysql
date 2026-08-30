# mysql

[![Doc](https://pkg.go.dev/badge/github.com/go-rio/mysql.svg)](https://pkg.go.dev/github.com/go-rio/mysql)
[![Go](https://img.shields.io/github/go-mod/go-version/go-rio/mysql)](https://go.dev/)
[![Release](https://img.shields.io/github/release/go-rio/mysql.svg)](https://github.com/go-rio/mysql/releases)
[![Test](https://github.com/go-rio/mysql/actions/workflows/test.yml/badge.svg)](https://github.com/go-rio/mysql/actions/workflows/test.yml)
[![License](https://img.shields.io/github/license/go-rio/mysql)](https://opensource.org/license/MIT)

MySQL driver module for [rio](https://github.com/go-rio/rio), backed by
[go-sql-driver/mysql](https://github.com/go-sql-driver/mysql).

## Getting started

```sh
go get github.com/go-rio/mysql
```

```go
db, err := mysql.Open("user:password@tcp(localhost:3306)/app")
if err != nil {
	return err
}
defer db.Close()

users, err := rio.From[User]().Where("age > ?", 18).All(ctx, db)
```

`Open` accepts `rio.Option` values, validates the DSN, and does not connect;
use `db.Unwrap()` to ping or tune the underlying `*sql.DB`. `db.Close()`
closes the statement cache (when enabled) and the `*sql.DB`. `New` wraps an
existing `*sql.DB` and performs none of the DSN checks below.

## parseTime

Scanning `DATETIME` or `TIMESTAMP` into `time.Time` requires
`parseTime=true`. `Open` adds it when omitted, keeps an explicit true, and
rejects an explicit false. It does not change `loc`; rio normalizes bound
`time.Time` values to UTC at microsecond precision.

## sql_mode

rio rewrites `?` placeholders using MySQL's default lexical rules. `Open`
rejects modes that make the server count placeholders differently:

| Mode | Effect |
| --- | --- |
| `NO_BACKSLASH_ESCAPES` | Backslash becomes an ordinary character. |
| `ANSI_QUOTES` | Double quotes delimit identifiers instead of strings. |

The combination modes `ANSI`, `DB2`, `MAXDB`, `MSSQL`, `ORACLE`, and
`POSTGRESQL` are also rejected because they can imply `ANSI_QUOTES`.
Matching is case-insensitive per comma-separated token. When the DSN sets no
`sql_mode`, `Open` injects none — it does not connect, so it cannot inspect
the server default. Override an incompatible global mode in the DSN (`%27`
is a URL-encoded single quote):

```sh
user:password@tcp(localhost:3306)/app?sql_mode=%27STRICT_TRANS_TABLES%27
```

## clientFoundRows

`Open` rejects `clientFoundRows=true`. rio's upsert backfill, optimistic
locking, and idempotent zero-affected updates are built on MySQL's default
changed-rows counting.

## Error translation

Use `errors.Is` for the sentinel; the original `*mysql.MySQLError` stays
available through `errors.As`.

| MySQL error | Sentinel |
| --- | --- |
| 1062 `ER_DUP_ENTRY` | `rio.ErrDuplicateKey` |
| 1451 / 1452 (foreign key held or missing) | `rio.ErrForeignKeyViolated` |

## Upserts

MySQL's `ON DUPLICATE KEY UPDATE` reacts to any unique index, so
`rio.OnConflict(...)` cannot select which index fires. `DoUpdate` uses the
row alias from MySQL 8.0.19 and is not supported on MariaDB; `DoNothing`
works on older MySQL and MariaDB.

## Statement reuse

`mysql.Open(dsn, rio.WithStmtCache())` adds a bounded DB-level
prepared-statement cache plus a cache local to each transaction. It is off
by default; do not use it with transaction- or statement-mode connection
poolers. As a client-side alternative, go-sql-driver supports
`interpolateParams=true` and rejects character sets where interpolation is
unsafe; rio never enables it automatically.

## Contributing

Use Go 1.27 or newer, then run `go test ./...`, `go test -race ./...`, and
`go vet ./...` before opening a pull request.

## License

The [MIT License](LICENSE). Copyright (c) 2026-now TreeNewBee.
