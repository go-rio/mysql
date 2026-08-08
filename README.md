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

`Open` accepts `rio.Option` values. To wrap an existing `*sql.DB`, use `New`
with a DSN that already satisfies the requirements below:

```go
sqlDB, err := sql.Open("mysql", dsn) // must include parseTime=true
if err != nil {
	return err
}
db := mysql.New(sqlDB)
```

## parseTime

Scanning `DATETIME` or `TIMESTAMP` into `time.Time` requires `parseTime=true`.
`Open`:

- adds `parseTime=true` when omitted;
- preserves an explicit true value;
- rejects an explicit false value.

`Open` does not change `loc`. rio normalizes bound `time.Time` values to UTC at
microsecond precision.

## sql_mode

rio rewrites `?` placeholders using MySQL's default lexical rules. `Open`
rejects modes that would make the server count placeholders differently:

| Mode | Effect |
| --- | --- |
| `NO_BACKSLASH_ESCAPES` | Backslash becomes an ordinary character. |
| `ANSI_QUOTES` | Double quotes delimit identifiers instead of strings. |

Combination modes `ANSI`, `DB2`, `MAXDB`, `MSSQL`, `ORACLE`, and `POSTGRESQL`
are also rejected because they can imply `ANSI_QUOTES`. Matching is
case-insensitive and applies to comma-separated mode tokens.

Safe explicit modes pass through. When `sql_mode` is absent, `Open` does not
inject one, so the connection uses the server default. Because `Open` does not
connect, it cannot inspect that default. Override an incompatible global mode
in the DSN (`%27` is a URL-encoded single quote):

```sh
user:password@tcp(localhost:3306)/app?sql_mode=%27STRICT_TRANS_TABLES%27
```

`New` cannot perform either the `parseTime` or `sql_mode` checks; configure the
underlying driver before wrapping it.

## Connections and transactions

`Open` constructs a handle without connecting or tuning the connection pool.
Use `db.Unwrap()` to ping or configure the underlying `*sql.DB`:

```go
sqlDB := db.Unwrap()
sqlDB.SetMaxOpenConns(20)
err := sqlDB.PingContext(ctx)
```

`db.Close()` closes rio's statement cache, when enabled, and the underlying
`*sql.DB`. This also applies to a database passed to `New`.

All rio operations accept either `*rio.DB` or `*rio.Tx`. `DB.Tx` commits when
the callback returns nil and rolls back on an error or panic; nested `Tx.Tx`
calls use savepoints:

```go
err := db.Tx(ctx, func(tx *rio.Tx) error {
	return rio.Insert(ctx, tx, &user)
})
```

## Error translation

The module maps these MySQL errors to rio sentinels. Use `errors.Is` for the
sentinel; the original `*mysql.MySQLError` remains available through
`errors.As`.

| MySQL error | Sentinel |
| --- | --- |
| 1062 `ER_DUP_ENTRY` | `rio.ErrDuplicateKey` |
| 1451 / 1452 (foreign key held or missing) | `rio.ErrForeignKeyViolated` |

## Upsert semantics on MySQL

MySQL's `ON DUPLICATE KEY UPDATE` reacts to any unique index. Consequently,
`rio.OnConflict(...)` records intent but cannot select which index fires.

`DoUpdate` uses the row alias introduced in MySQL 8.0.19 and is not supported on
MariaDB. `DoNothing` supports older MySQL versions and MariaDB.

## Statement reuse

`rio.WithStmtCache()` enables a bounded DB-level prepared-statement cache:

```go
db, err := mysql.Open(dsn, rio.WithStmtCache())
```

The cache is off by default. Enabling it also creates a bounded cache local to
each transaction, which helps repeated parameterized statements avoid MySQL's
prepare/execute cycle. Do not use it with transaction- or statement-mode
connection poolers. As a client-side alternative, go-sql-driver supports the
DSN option `interpolateParams=true` and rejects character sets for which
interpolation is unsafe; rio does not enable it automatically.

## Contributing

Use Go 1.27 or newer, then run `go test ./...`, `go test -race ./...`, and
`go vet ./...` before opening a pull request.

## License

The [MIT License](LICENSE). Copyright (c) 2026-now TreeNewBee.
