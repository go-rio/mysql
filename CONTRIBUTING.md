# Contributing to go-rio/mysql

## Prerequisites

- Go 1.27 or newer
- Docker, for the gated integration test

## Setup

```sh
git clone https://github.com/go-rio/mysql
cd mysql
go build ./...
```

## Tests

```sh
go vet ./...
go test ./...
go test -race ./...
```

The integration test needs a server and skips without `RIO_MYSQL_DSN`:

```sh
docker run -d --name rio-mysql -e MYSQL_ROOT_PASSWORD=bench -e MYSQL_DATABASE=bench -p 127.0.0.1:13306:3306 mysql:8.4
RIO_MYSQL_DSN='root:bench@tcp(127.0.0.1:13306)/bench?parseTime=true' go test -race ./...
docker rm -f rio-mysql
```

## Pull requests

- Every change ships with a test; one test file per source file
  (`mysql.go` ↔ `mysql_test.go`).
- Comments state contracts. Exported identifiers get a doc comment naming
  purpose, constraints, and error cases; internal comments are one line, two
  at most; no history or narrative.
- Commit subjects carry a conventional prefix (`feat:`, `fix:`, `docs:`,
  `test:`, `chore:`).
- Keep `gofmt` and `go vet` clean.

## Releases

Maintainers tag signed releases (`git tag -s vX.Y.Z`) after the rio core
version they depend on is tagged, and record every user-visible change in
[CHANGELOG.md](CHANGELOG.md).
