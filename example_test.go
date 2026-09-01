package mysql_test

import (
	"context"
	"database/sql"
	"log"

	"github.com/go-rio/mysql"
	"github.com/go-rio/rio"
)

type User struct {
	ID    int64
	Email string
	Age   int
}

func ExampleOpen() {
	db, err := mysql.Open("user:password@tcp(localhost:3306)/app")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	users, err := rio.From[User]().Where("age > ?", 18).All(context.Background(), db)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%d adults", len(users))
}

// A transaction-mode proxy cannot hold prepared statements across requests,
// so the statement cache is switched off there.
func ExampleOpen_withoutStmtCache() {
	db, err := mysql.Open("user:password@tcp(proxy:6033)/app", rio.WithoutStmtCache())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
}

// New wraps a pool the caller already configured; the DSN checks are the
// caller's responsibility.
func ExampleNew() {
	raw, err := sql.Open("mysql", "user:password@tcp(localhost:3306)/app?parseTime=true")
	if err != nil {
		log.Fatal(err)
	}
	raw.SetMaxOpenConns(32)

	db := mysql.New(raw)
	defer db.Close()
}
