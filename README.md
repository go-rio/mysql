# mysql

[![Doc](https://pkg.go.dev/badge/github.com/go-rio/mysql.svg)](https://pkg.go.dev/github.com/go-rio/mysql)
[![Go](https://img.shields.io/github/go-mod/go-version/go-rio/mysql)](https://go.dev/)
[![Release](https://img.shields.io/github/release/go-rio/mysql.svg)](https://github.com/go-rio/mysql/releases)
[![Test](https://github.com/go-rio/mysql/actions/workflows/test.yml/badge.svg)](https://github.com/go-rio/mysql/actions/workflows/test.yml)
[![License](https://img.shields.io/github/license/go-rio/mysql)](https://opensource.org/license/MIT)

MySQL driver module for [rio](https://github.com/go-rio/rio), backed by
[go-sql-driver/mysql](https://github.com/go-sql-driver/mysql). It handles
constructors, DSN hygiene, and error translation; SQL generation lives in rio.

## Install

```sh
go get github.com/go-rio/mysql
```

## Usage

```go
import (
	"github.com/go-rio/rio"
	"github.com/go-rio/mysql"
)

db, err := mysql.Open("user:password@tcp(localhost:3306)/app")
if err != nil {
	log.Fatal(err)
}
defer db.Close()

users, err := rio.From[User]().Where("age > ?", 18).All(ctx, db)
```

`Open` accepts every `rio.Option` and applies parseTime/sql_mode hygiene. `New`
wraps an existing `*sql.DB` without hygiene; parseTime and sql_mode are yours:

```go
sqlDB, err := sql.Open("mysql", dsn) // must include parseTime=true
db := mysql.New(sqlDB)
```

## parseTime

rio scans `DATETIME`/`TIMESTAMP` into `time.Time`, requiring `parseTime=true`.
`Open`:

- no `parseTime` → adds `parseTime=true`;
- `parseTime=true` → kept verbatim;
- `parseTime=false` → error, not a silent override.

`Open` never touches `loc` (default UTC). rio writes `time.Time` as UTC
truncated to microseconds.

## sql_mode

rio rewrites `?` placeholders under MySQL's default lexing: backslash escapes
inside string literals, `"double quoted"` text is a string. Two `sql_mode` flags
change it, rejected:

| Mode | Effect |
|---|---|
| `NO_BACKSLASH_ESCAPES` | Backslash becomes an ordinary character. |
| `ANSI_QUOTES` | Double quotes delimit identifiers, not strings. Also implied by `ANSI` and, on MariaDB and MySQL ≤ 5.7, `DB2`/`MAXDB`/`MSSQL`/`ORACLE`/`POSTGRESQL`. |

Under either, a literal could hide or expose a `?` per side; rio fails with a
placeholder/argument arity error, never a misbound query. `Open`:

- `sql_mode` without them → kept; the driver runs `SET sql_mode='...'` per new
  connection;
- `sql_mode` with one → error naming the mode;
- no `sql_mode` → nothing injected; the session keeps the server's default.

`Open` never connects, so it cannot inspect that default. If either is enabled
globally, set it in the DSN (`%27` is URL-encoded `'`). MySQL 8's factory
default:

```text
user:password@tcp(localhost:3306)/app?sql_mode=%27ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION%27
```

## Error translation

Constraint violations return rio sentinels wrapping the driver error:

| MySQL error | Sentinel |
|---|---|
| 1062 `ER_DUP_ENTRY` | `rio.ErrDuplicateKey` |
| 1451 / 1452 (foreign key held or missing) | `rio.ErrForeignKeyViolated` |

```go
err := rio.Insert(ctx, db, &user)
if errors.Is(err, rio.ErrDuplicateKey) { ... }

var me *mysql.MySQLError // errors.As reaches the driver error
```

## Upsert semantics on MySQL

MySQL has no conflict target. `rio.Upsert` renders `ON DUPLICATE KEY UPDATE`,
which reacts to any unique index, so `rio.OnConflict(...)` documents intent, not
which index fires.

`DoUpdate` uses the 8.0.19+ row alias (`VALUES()` is deprecated there), needing
MySQL 8.0.19+. MariaDB implements neither and supports `DoNothing` only.

## Performance: the prepare round-trip

go-sql-driver runs every parameterized query as prepare + execute — two
blocking round-trips. On a local MySQL 8.4 this doubles single-statement
latency (~210µs vs ~105µs in rio's bench suite). Two ways to recover it, on
rio's three-leg benchmarks:

- **`interpolateParams=true` (DSN)** — client-side interpolation, one plain
  query. ReadOne −45%, Insert −48%, Update −46%, InsertBatch100 −24%. Refused
  where escaping is unsafe (big5, gbk, sjis, …); the default utf8mb4 is safe.
  rio never injects it.

  ```go
  db, err := mysql.Open("user:pass@tcp(127.0.0.1:3306)/app?interpolateParams=true")
  ```

- **`rio.WithStmtCache()`** reuses server-side prepared statements per SQL text.
  ReadOne −35%, Insert −46%. Prefer for the binary protocol or per-column type
  fidelity; avoid behind transaction-mode connection poolers.

  ```go
  db, err := mysql.Open(dsn, rio.WithStmtCache())
  ```

Both help multi-row reads less; per-row transfer dominates.

## License

The [MIT License](LICENSE). Copyright (c) 2026-now TreeNewBee.
