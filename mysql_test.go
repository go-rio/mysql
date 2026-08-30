package mysql

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-rio/rio"
	"github.com/go-sql-driver/mysql"
)

func TestSanitizeDSNAddsParseTime(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		{"bare", "root:secret@tcp(localhost:3306)/app"},
		{"with other params", "root:secret@tcp(localhost:3306)/app?loc=Local&timeout=5s"},
		{"unix socket", "root@unix(/tmp/mysql.sock)/app"},
		{"password with slash and question mark", "root:pa/ss?word@tcp(localhost:3306)/app"},
		{"password spelling parseTime=false", "root:pa?parseTime=false@tcp(localhost:3306)/app"},
		{"bare parseTime key without value", "root@tcp(localhost:3306)/app?parseTime"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizeDSN(tt.dsn)
			if err != nil {
				t.Fatalf("sanitizeDSN(%q) returned error: %v", tt.dsn, err)
			}
			gotCfg, err := mysql.ParseDSN(got)
			if err != nil {
				t.Fatalf("sanitized DSN %q does not parse: %v", got, err)
			}
			if !gotCfg.ParseTime {
				t.Fatalf("sanitized DSN %q does not enable parseTime", got)
			}
			want, err := mysql.ParseDSN(tt.dsn)
			if err != nil {
				t.Fatalf("ParseDSN(%q): %v", tt.dsn, err)
			}
			want.ParseTime = true
			if !reflect.DeepEqual(gotCfg, want) {
				t.Errorf("sanitizeDSN(%q) changed more than parseTime:\n got %+v\nwant %+v", tt.dsn, gotCfg, want)
			}
		})
	}
}

func TestSanitizeDSNKeepsExplicitTrue(t *testing.T) {
	dsns := []string{
		"root:secret@tcp(localhost:3306)/app?parseTime=true",
		"root:secret@tcp(localhost:3306)/app?loc=UTC&parseTime=1&timeout=5s",
		"root@tcp(localhost:3306)/app?parseTime=True",
		"root@tcp(localhost:3306)/app?parseTime=false&parseTime=true",
		"root@tcp(localhost:3306)/app?parseTime=true&sql_mode=%27STRICT_TRANS_TABLES%27",
	}
	for _, dsn := range dsns {
		got, err := sanitizeDSN(dsn)
		if err != nil {
			t.Fatalf("sanitizeDSN(%q) returned error: %v", dsn, err)
		}
		if got != dsn {
			t.Errorf("sanitizeDSN(%q) rewrote an already-correct DSN to %q", dsn, got)
		}
	}
}

func TestSanitizeDSNRejectsExplicitFalse(t *testing.T) {
	dsns := []string{
		"root:secret@tcp(localhost:3306)/app?parseTime=false",
		"root:secret@tcp(localhost:3306)/app?parseTime=0",
		"root@tcp(localhost:3306)/app?parseTime=FALSE",
		"root@tcp(localhost:3306)/app?loc=UTC&parseTime=False&timeout=5s",
		"root@tcp(localhost:3306)/app?parseTime=true&parseTime=false",
		"root:se/cret@tcp(localhost:3306)/app?parseTime=false",
	}
	for _, dsn := range dsns {
		got, err := sanitizeDSN(dsn)
		if err == nil {
			t.Errorf("sanitizeDSN(%q) = %q, want an error for explicit parseTime=false", dsn, got)
			continue
		}
		if !strings.Contains(err.Error(), "parseTime") {
			t.Errorf("sanitizeDSN(%q) error %q does not mention parseTime", dsn, err)
		}
	}
}

// CLIENT_FOUND_ROWS reports matched rows, and rio's write semantics — upsert
// backfill (1 insert / 2 update / 0 no-change), optimistic locking, and the
// idempotent zero-affected probes — are built on changed-rows counting: a
// no-change conflict would be mislabeled a fresh insert and backfill a stale
// LastInsertId into the primary key.
func TestSanitizeDSNRejectsClientFoundRows(t *testing.T) {
	dsns := []string{
		"root:secret@tcp(localhost:3306)/app?clientFoundRows=true",
		"root@tcp(localhost:3306)/app?loc=UTC&clientFoundRows=1&timeout=5s",
	}
	for _, dsn := range dsns {
		got, err := sanitizeDSN(dsn)
		if err == nil {
			t.Errorf("sanitizeDSN(%q) = %q, want an error for clientFoundRows", dsn, got)
			continue
		}
		if !strings.Contains(err.Error(), "clientFoundRows") {
			t.Errorf("sanitizeDSN(%q) error should name the option: %v", dsn, err)
		}
	}
	if _, err := sanitizeDSN("root@tcp(localhost:3306)/app?clientFoundRows=false"); err != nil {
		t.Errorf("an explicit clientFoundRows=false is the default and must pass: %v", err)
	}
}

func TestSanitizeDSNRejectsLexBreakingSQLMode(t *testing.T) {
	tests := []struct {
		name    string
		dsn     string
		mention string // the offending token the error must name
	}{
		{
			name:    "NO_BACKSLASH_ESCAPES alone",
			dsn:     "root:secret@tcp(localhost:3306)/app?sql_mode=%27NO_BACKSLASH_ESCAPES%27",
			mention: "NO_BACKSLASH_ESCAPES",
		},
		{
			name:    "ANSI_QUOTES alone",
			dsn:     "root:secret@tcp(localhost:3306)/app?sql_mode=%27ANSI_QUOTES%27",
			mention: "ANSI_QUOTES",
		},
		{
			name: "mixed into a longer list",
			dsn: "root:secret@tcp(localhost:3306)/app?" +
				"sql_mode=%27STRICT_TRANS_TABLES,NO_BACKSLASH_ESCAPES,NO_ENGINE_SUBSTITUTION%27",
			mention: "NO_BACKSLASH_ESCAPES",
		},
		{
			name:    "combination mode ANSI",
			dsn:     "root:secret@tcp(localhost:3306)/app?sql_mode=%27ANSI%27",
			mention: "ANSI",
		},
		{
			name:    "vendor combination mode",
			dsn:     "root@tcp(localhost:3306)/app?sql_mode=%27ORACLE%27",
			mention: "ORACLE",
		},
		{
			name:    "lower-case spelling",
			dsn:     "root@tcp(localhost:3306)/app?sql_mode=%27ansi_quotes%27",
			mention: "ANSI_QUOTES",
		},
		{
			name:    "upper-case variable name",
			dsn:     "root@tcp(localhost:3306)/app?SQL_MODE=%27ANSI%27",
			mention: "ANSI",
		},
		{
			name:    "unquoted value",
			dsn:     "root@tcp(localhost:3306)/app?sql_mode=NO_BACKSLASH_ESCAPES",
			mention: "NO_BACKSLASH_ESCAPES",
		},
		{
			name:    "space after the comma",
			dsn:     "root@tcp(localhost:3306)/app?sql_mode=%27TRADITIONAL,+ANSI_QUOTES%27",
			mention: "ANSI_QUOTES",
		},
		{
			name:    "raw unencoded quotes",
			dsn:     "root@tcp(localhost:3306)/app?sql_mode='ANSI'",
			mention: "ANSI",
		},
		{
			name:    "last duplicate wins in the driver; it is the bad one",
			dsn:     "root@tcp(localhost:3306)/app?sql_mode=%27TRADITIONAL%27&sql_mode=%27ANSI_QUOTES%27",
			mention: "ANSI_QUOTES",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizeDSN(tt.dsn)
			if err == nil {
				t.Fatalf("sanitizeDSN(%q) = %q, want an error for a lexing-breaking sql_mode", tt.dsn, got)
			}
			if !strings.Contains(err.Error(), "sql_mode") {
				t.Errorf("sanitizeDSN(%q) error %q does not mention sql_mode", tt.dsn, err)
			}
			if !strings.Contains(err.Error(), tt.mention) {
				t.Errorf("sanitizeDSN(%q) error %q does not name the offending mode %q", tt.dsn, err, tt.mention)
			}
		})
	}
}

func TestSanitizeDSNAllowsSafeSQLMode(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		{
			name: "MySQL 8 factory default list",
			dsn: "root:secret@tcp(localhost:3306)/app?" +
				"sql_mode=%27ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE," +
				"ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION%27",
		},
		{
			name: "TRADITIONAL implies no lexing change",
			dsn:  "root:secret@tcp(localhost:3306)/app?sql_mode=%27TRADITIONAL%27",
		},
		{
			name: "empty mode disables everything",
			dsn:  "root:secret@tcp(localhost:3306)/app?sql_mode=%27%27",
		},
		{
			name: "unknown token passes through",
			dsn:  "root:secret@tcp(localhost:3306)/app?sql_mode=%27ANSII%27",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizeDSN(tt.dsn)
			if err != nil {
				t.Fatalf("sanitizeDSN(%q) returned error: %v", tt.dsn, err)
			}
			cfg, err := mysql.ParseDSN(got)
			if err != nil {
				t.Fatalf("sanitized DSN %q does not parse: %v", got, err)
			}
			wantCfg, err := mysql.ParseDSN(tt.dsn)
			if err != nil {
				t.Fatalf("ParseDSN(%q): %v", tt.dsn, err)
			}
			if cfg.Params["sql_mode"] != wantCfg.Params["sql_mode"] {
				t.Errorf("sanitizeDSN(%q) changed sql_mode from %q to %q",
					tt.dsn, wantCfg.Params["sql_mode"], cfg.Params["sql_mode"])
			}
			if !cfg.ParseTime {
				t.Errorf("sanitized DSN %q does not enable parseTime", got)
			}
		})
	}
}

func TestSanitizeDSNDoesNotInjectSQLMode(t *testing.T) {
	got, err := sanitizeDSN("root:secret@tcp(localhost:3306)/app")
	if err != nil {
		t.Fatalf("sanitizeDSN: %v", err)
	}
	cfg, err := mysql.ParseDSN(got)
	if err != nil {
		t.Fatalf("ParseDSN(%q): %v", got, err)
	}
	for key := range cfg.Params {
		if strings.EqualFold(key, "sql_mode") {
			t.Fatalf("sanitizeDSN injected sql_mode: %q", got)
		}
	}
}

func TestSanitizeDSNInvalid(t *testing.T) {
	if _, err := sanitizeDSN("this is not a dsn"); err == nil {
		t.Fatal("sanitizeDSN accepted a DSN without a slash")
	}
}

func TestOpenRejectsExplicitParseTimeFalse(t *testing.T) {
	db, err := Open("root:secret@tcp(localhost:3306)/app?parseTime=false")
	if err == nil {
		_ = db.Close()
		t.Fatal("Open accepted parseTime=false")
	}
	if !strings.Contains(err.Error(), "parseTime") {
		t.Errorf("Open error %q does not mention parseTime", err)
	}
}

func TestOpenRejectsLexBreakingSQLMode(t *testing.T) {
	db, err := Open("root:secret@tcp(localhost:3306)/app?sql_mode=%27NO_BACKSLASH_ESCAPES%27")
	if err == nil {
		_ = db.Close()
		t.Fatal("Open accepted a sql_mode with NO_BACKSLASH_ESCAPES")
	}
	if !strings.Contains(err.Error(), "sql_mode") {
		t.Errorf("Open error %q does not mention sql_mode", err)
	}
}

func TestOpenDoesNotConnect(t *testing.T) {
	db, err := Open("root:secret@tcp(localhost:1)/nowhere")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if db.Unwrap() == nil {
		t.Fatal("Unwrap returned nil")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestTranslate(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "duplicate entry",
			err: &mysql.MySQLError{
				Number:  1062,
				Message: "Duplicate entry 'a' for key 'users.email'",
			},
			want: rio.ErrDuplicateKey,
		},
		{
			name: "row is referenced",
			err: &mysql.MySQLError{
				Number:  1451,
				Message: "Cannot delete or update a parent row",
			},
			want: rio.ErrForeignKeyViolated,
		},
		{
			name: "no referenced row",
			err: &mysql.MySQLError{
				Number:  1452,
				Message: "Cannot add or update a child row",
			},
			want: rio.ErrForeignKeyViolated,
		},
		{
			name: "wrapped driver error",
			err:  fmt.Errorf("insert users: %w", &mysql.MySQLError{Number: 1062}),
			want: rio.ErrDuplicateKey,
		},
		{
			name: "unrelated mysql error",
			err:  &mysql.MySQLError{Number: 1213, Message: "Deadlock found"},
		},
		{name: "not a mysql error", err: errors.New("plain")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := translate(tt.err); got != tt.want {
				t.Errorf("translate(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

type Author struct {
	ID        int64
	Name      string
	Email     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Author) TableName() string { return "rio_mysql_authors" }

type Book struct {
	ID       int64
	AuthorID int64
	Title    string
}

func (Book) TableName() string { return "rio_mysql_books" }

// openTestDB skips integration tests when RIO_MYSQL_DSN is unset.
func openTestDB(t *testing.T) *rio.DB {
	t.Helper()
	dsn := os.Getenv("RIO_MYSQL_DSN")
	if dsn == "" {
		t.Skip("RIO_MYSQL_DSN not set")
	}
	db, err := Open(dsn)
	if err != nil {
		t.Fatalf("Open(%q): %v", dsn, err)
	}
	if err := db.Unwrap().Ping(); err != nil {
		t.Fatalf("ping mysql: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func mustExec(t *testing.T, ctx context.Context, db *rio.DB, sqlText string) {
	t.Helper()
	if _, err := rio.Exec(ctx, db, sqlText); err != nil {
		t.Fatalf("exec %q: %v", sqlText, err)
	}
}

func createSchema(t *testing.T, ctx context.Context, db *rio.DB) {
	t.Helper()
	drop := func() {
		mustExec(t, ctx, db, "DROP TABLE IF EXISTS rio_mysql_books")
		mustExec(t, ctx, db, "DROP TABLE IF EXISTS rio_mysql_authors")
	}
	drop()
	mustExec(t, ctx, db, `CREATE TABLE rio_mysql_authors (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		email VARCHAR(100) NOT NULL,
		created_at DATETIME(6) NOT NULL,
		updated_at DATETIME(6) NOT NULL,
		UNIQUE KEY uniq_rio_mysql_authors_email (email)
	)`)
	mustExec(t, ctx, db, `CREATE TABLE rio_mysql_books (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		author_id BIGINT NOT NULL,
		title VARCHAR(200) NOT NULL,
		CONSTRAINT fk_rio_mysql_books_author
			FOREIGN KEY (author_id) REFERENCES rio_mysql_authors (id)
	)`)
	t.Cleanup(drop)
}

func TestIntegration(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	createSchema(t, ctx, db)

	ada := Author{Name: "Ada", Email: "ada@example.com"}
	if err := rio.Insert(ctx, db, &ada); err != nil {
		t.Fatalf("insert author: %v", err)
	}
	if ada.ID == 0 {
		t.Fatal("insert did not backfill the auto-increment ID")
	}
	if ada.CreatedAt.IsZero() || ada.UpdatedAt.IsZero() {
		t.Fatal("insert did not stamp timestamps")
	}

	got, err := rio.Find[Author](ctx, db, ada.ID)
	if err != nil {
		t.Fatalf("find author: %v", err)
	}
	if got.Email != ada.Email {
		t.Errorf("reloaded email = %q, want %q", got.Email, ada.Email)
	}
	if !got.CreatedAt.Equal(ada.CreatedAt) {
		t.Errorf("reloaded created_at = %v, want %v", got.CreatedAt, ada.CreatedAt)
	}

	dup := Author{Name: "Imposter", Email: "ada@example.com"}
	err = rio.Insert(ctx, db, &dup)
	if !errors.Is(err, rio.ErrDuplicateKey) {
		t.Fatalf("duplicate insert error = %v, want rio.ErrDuplicateKey", err)
	}
	var me *mysql.MySQLError
	if !errors.As(err, &me) || me.Number != 1062 {
		t.Fatalf("driver error lost from the chain: %v", err)
	}

	orphan := Book{AuthorID: ada.ID + 1000, Title: "Orphan"}
	if err := rio.Insert(ctx, db, &orphan); !errors.Is(err, rio.ErrForeignKeyViolated) {
		t.Fatalf("orphan insert error = %v, want rio.ErrForeignKeyViolated", err)
	}

	book := Book{AuthorID: ada.ID, Title: "Notes on the Analytical Engine"}
	if err := rio.Insert(ctx, db, &book); err != nil {
		t.Fatalf("insert book: %v", err)
	}
	if err := rio.Delete(ctx, db, &ada); !errors.Is(err, rio.ErrForeignKeyViolated) {
		t.Fatalf("referenced delete error = %v, want rio.ErrForeignKeyViolated", err)
	}
}
