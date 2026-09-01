# mysql

[![Doc](https://pkg.go.dev/badge/github.com/go-rio/mysql.svg)](https://pkg.go.dev/github.com/go-rio/mysql)
[![Go](https://img.shields.io/github/go-mod/go-version/go-rio/mysql)](https://go.dev/)
[![Release](https://img.shields.io/github/release/go-rio/mysql.svg)](https://github.com/go-rio/mysql/releases)
[![Test](https://github.com/go-rio/mysql/actions/workflows/test.yml/badge.svg)](https://github.com/go-rio/mysql/actions/workflows/test.yml)
[![License](https://img.shields.io/github/license/go-rio/mysql)](https://opensource.org/license/MIT)

MySQL driver module for [rio](https://github.com/go-rio/rio), backed by
[go-sql-driver/mysql](https://github.com/go-sql-driver/mysql): DSN hygiene,
error translation, and a prepared-statement cache that is on by default. rio
renders the SQL.

```go
db, err := mysql.Open("user:password@tcp(localhost:3306)/app")
if err != nil {
	return err
}
defer db.Close()

users, err := rio.From[User]().Where("age > ?", 18).All(ctx, db)
err = rio.Upsert(ctx, db, &user, rio.DoUpdate("age"))
```

## Getting started

```sh
go get github.com/go-rio/mysql
```

```go
package main

import (
	"context"
	"log"

	"github.com/go-rio/mysql"
	"github.com/go-rio/rio"
)

type User struct {
	ID    int64
	Email string
	Age   int
}

func main() {
	ctx := context.Background()
	db, err := mysql.Open("user:password@tcp(localhost:3306)/app")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	users, err := rio.From[User]().Where("age > ?", 18).All(ctx, db)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%d adults", len(users))
}
```

Requires Go 1.27 and MySQL 8.0 or later; `DoUpdate` upserts need 8.0.19
(see [Upserts](#upserts)).

## Features

### Constructors

`Open` accepts `rio.Option` values, validates the DSN, and does not connect;
use `db.Unwrap()` to ping or tune the underlying `*sql.DB`. `db.Close()`
closes the statement cache and the `*sql.DB`. `New` wraps an existing
`*sql.DB` and performs none of the DSN checks below.

### parseTime

Scanning `DATETIME` or `TIMESTAMP` into `time.Time` requires
`parseTime=true`. `Open` adds it when omitted, keeps an explicit true, and
rejects an explicit false.

### Time values

rio normalizes bound `time.Time` values to UTC at microsecond precision. The
driver converts them into the DSN's `loc` (UTC by default) and never sets the
session `time_zone`, so a `TIMESTAMP` column is interpreted in the server's
zone: keep `loc` at UTC, or set `time_zone` in the DSN to match it.

### sql_mode

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

### clientFoundRows

`Open` rejects `clientFoundRows=true`. rio's upsert backfill, optimistic
locking, and idempotent zero-affected updates are built on MySQL's default
changed-rows counting.

### Statement reuse

`Open` and `New` enable rio's bounded prepared-statement cache (one per
`*rio.DB` plus one per transaction), so a parameterized query costs one
round trip after its first use instead of a prepare plus an execute. Pass
`rio.WithoutStmtCache()` to opt out; do so behind transaction- or
statement-mode proxies that cannot hold prepared statements across
requests:

```go
db, err := mysql.Open(dsn, rio.WithoutStmtCache())
```

Without the cache, go-sql-driver's `interpolateParams=true` skips the
prepare round trip client-side and rejects character sets where
interpolation is unsafe; rio never enables it automatically.

### Error translation

Use `errors.Is` for the sentinel; the original `*mysql.MySQLError` stays
available through `errors.As`.

| MySQL error | Sentinel |
| --- | --- |
| 1022 `ER_DUP_KEY`, 1062 `ER_DUP_ENTRY`, 1169 `ER_DUP_UNIQUE`, 1586 `ER_DUP_ENTRY_WITH_KEY_NAME` | `rio.ErrDuplicateKey` |
| 1216 `ER_NO_REFERENCED_ROW`, 1217 `ER_ROW_IS_REFERENCED`, 1451 `ER_ROW_IS_REFERENCED_2`, 1452 `ER_NO_REFERENCED_ROW_2` | `rio.ErrForeignKeyViolated` |

### Upserts

MySQL's `ON DUPLICATE KEY UPDATE` reacts to any unique index, so
`rio.OnConflict(...)` cannot select which index fires. `DoUpdate` and
`DoUpdateSet` use the row alias from MySQL 8.0.19 (the incoming row is
`_rio_new` inside a `rio.Expr`) and are not supported on MariaDB;
`DoNothing` works on older MySQL and MariaDB. MySQL has no `RETURNING`, so
`InsertAll` does not backfill keys and `UpdateAllReturning` and
`DeleteAllReturning` are rejected.

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md): a clone, `go test ./...`, and the
one-line Docker command for the gated integration test.

## Contributors

Thanks to everyone who has filed issues and opened pull requests on
[go-rio/mysql](https://github.com/go-rio/mysql/graphs/contributors).

## License

The [MIT License](LICENSE). Copyright (c) 2026-now TreeNewBee.
