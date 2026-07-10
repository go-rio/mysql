# mysql

[![Doc](https://pkg.go.dev/badge/github.com/go-rio/mysql)](https://pkg.go.dev/github.com/go-rio/mysql)
[![Go](https://img.shields.io/github/go-mod/go-version/go-rio/mysql)](https://go.dev/)
[![Release](https://img.shields.io/github/release/go-rio/mysql.svg)](https://github.com/go-rio/mysql/releases)
[![Test](https://github.com/go-rio/mysql/actions/workflows/test.yml/badge.svg)](https://github.com/go-rio/mysql/actions)
[![Report Card](https://goreportcard.com/badge/github.com/go-rio/mysql)](https://goreportcard.com/report/github.com/go-rio/mysql)
[![Stars](https://img.shields.io/github/stars/go-rio/mysql?style=flat)](https://github.com/go-rio/mysql)
[![License](https://img.shields.io/github/license/go-rio/mysql)](https://opensource.org/license/MIT)

MySQL driver module for [rio](https://github.com/go-rio/rio), the
zero-surprise Go ORM, backed by
[go-sql-driver/mysql](https://github.com/go-sql-driver/mysql).

The module is deliberately thin: constructors, DSN hygiene, and precise error
translation. All SQL generation lives in rio itself — see the
[rio documentation](https://github.com/go-rio/rio) for queries, writes,
relations and everything else.

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

`Open` accepts every `rio.Option`. If you already manage your own `*sql.DB`,
wrap it instead — but then parseTime and sql_mode (see below) are on you:

```go
sqlDB, err := sql.Open("mysql", dsn) // must include parseTime=true
db := mysql.New(sqlDB)
```

## parseTime

rio scans `DATETIME` and `TIMESTAMP` columns into `time.Time`, which requires
the driver option `parseTime=true`. `Open` keeps that invariant without
surprising you:

- DSN does not mention `parseTime` → `Open` adds `parseTime=true`.
- DSN sets `parseTime=true` → passed through byte for byte.
- DSN sets `parseTime=false` → `Open` returns an error instead of silently
  overriding an option you spelled out.

`Open` never touches `loc`, so existing data keeps its meaning. For new
applications we recommend storing UTC — the driver's default `loc` — and
converting to local time at the edges; rio already writes `time.Time` values
as UTC truncated to microseconds.

## sql_mode

rio rewrites `?` placeholders by lexing your SQL with MySQL's **default**
lexical rules: backslashes escape characters inside string literals, and
`"double quoted"` text is a string. Two `sql_mode` flags change that lexing
on the server side, so they are not supported:

- `NO_BACKSLASH_ESCAPES` — backslash becomes an ordinary character;
- `ANSI_QUOTES` — double quotes delimit identifiers instead of strings
  (also implied by the combination modes `ANSI` and, on MariaDB and
  MySQL ≤ 5.7, `DB2`/`MAXDB`/`MSSQL`/`ORACLE`/`POSTGRESQL`).

Under either mode a literal in your SQL could hide or expose a `?`
differently on each side. rio fails loudly — a placeholder/argument arity
error, never a misbound query — but the fix belongs in the DSN. `Open`
keeps the invariant the same way it keeps parseTime:

- DSN sets a `sql_mode` without those modes → passed through untouched (the
  driver runs `SET sql_mode='...'` on every new connection).
- DSN sets a `sql_mode` containing one of them → `Open` returns an error
  naming the offending mode.
- DSN does not mention `sql_mode` → nothing is injected; the session keeps
  the server's default. `Open` never connects, so it cannot inspect that
  default — if your server enables `NO_BACKSLASH_ESCAPES` or `ANSI_QUOTES`
  globally, override the mode for rio's connections in the DSN (`%27` is a
  URL-encoded `'`), e.g. MySQL 8's factory default list:

  ```text
  user:password@tcp(localhost:3306)/app?sql_mode=%27ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION%27
  ```

## Error translation

Constraint violations come back as rio sentinels with the driver error still
in the chain, so both checks work:

| MySQL error | Sentinel |
|---|---|
| 1062 `ER_DUP_ENTRY` | `rio.ErrDuplicateKey` |
| 1451 / 1452 (foreign key held or missing) | `rio.ErrForeignKeyViolated` |

```go
err := rio.Insert(ctx, db, &user)
if errors.Is(err, rio.ErrDuplicateKey) { ... }

var me *mysql.MySQLError // errors.As still reaches the driver error
```

## Upsert semantics on MySQL

MySQL has no conflict target: `rio.Upsert` renders `ON DUPLICATE KEY UPDATE`,
which reacts to *any* unique index on the table, so `rio.OnConflict(...)`
documents intent rather than constraining which index fires — a documented
semantic difference rio does not paper over.

The `DoUpdate` branch names the would-be inserted row with the 8.0.19+ row
alias (`VALUES()` is deprecated there): it needs MySQL 8.0.19 or newer, and
MariaDB — which implements neither — supports `DoNothing` only.

## Performance: the prepare round-trip

go-sql-driver runs every parameterized query as prepare + execute — two
blocking round-trips per statement. On a local MySQL 8.4 that doubles
single-statement latency (~210µs vs ~105µs in rio's bench suite). Two ways
to get the round-trip back, measured on rio's three-leg benchmarks:

- **`interpolateParams=true` in the DSN** interpolates arguments client-side
  and sends one plain query: ReadOne −45%, Insert −48%, Update −46%,
  InsertBatch100 −24%. The driver refuses the option under charsets where
  escaping would be unsafe (big5, gbk, sjis, …); with the default utf8mb4 it
  is safe. rio never injects this option — the DSN stays yours:

  ```go
  db, err := mysql.Open("user:pass@tcp(127.0.0.1:3306)/app?interpolateParams=true")
  ```

- **`rio.WithStmtCache()`** keeps server-side prepared statements and reuses
  them per SQL text: ReadOne −35%, Insert −46%. Prefer it when you want the
  binary protocol or per-column type fidelity; avoid it behind
  transaction-mode connection poolers.

  ```go
  db, err := mysql.Open(dsn, rio.WithStmtCache())
  ```

Both level off multi-row reads less (the per-row transfer dominates), and
they compose with everything else in rio unchanged.

## License

The [MIT License](LICENSE). Copyright (c) 2026-now TreeNewBee.
