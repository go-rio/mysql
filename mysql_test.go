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

// --- DSN hygiene ---

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
		// The driver ignores a parameter without '='; so do we.
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
			// Everything except ParseTime must survive the round trip.
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
		// The last occurrence wins in the driver; true wins here.
		"root@tcp(localhost:3306)/app?parseTime=false&parseTime=true",
		// A safe explicit sql_mode is the caller's choice and survives
		// byte for byte like every other option.
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
		// The last occurrence wins in the driver; false wins here.
		"root@tcp(localhost:3306)/app?parseTime=true&parseTime=false",
		// The '/' in the password must not confuse the parameter search.
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

func TestSanitizeDSNAllowsClientFoundRows(t *testing.T) {
	dsn := "root:secret@tcp(localhost:3306)/app?clientFoundRows=true"
	got, err := sanitizeDSN(dsn)
	if err != nil {
		t.Fatalf("sanitizeDSN(%q): %v", dsn, err)
	}
	cfg, err := mysql.ParseDSN(got)
	if err != nil {
		t.Fatalf("ParseDSN(%q): %v", got, err)
	}
	if !cfg.ClientFoundRows {
		t.Fatalf("sanitizeDSN(%q) did not preserve clientFoundRows=true: %q", dsn, got)
	}
	if !cfg.ParseTime {
		t.Fatalf("sanitizeDSN(%q) did not still add parseTime=true: %q", dsn, got)
	}
}

func TestSanitizeDSNRejectsLexBreakingSQLMode(t *testing.T) {
	tests := []struct {
		name    string
		dsn     string
		mention string // the offending token the error must name
	}{
		{"NO_BACKSLASH_ESCAPES alone",
			"root:secret@tcp(localhost:3306)/app?sql_mode=%27NO_BACKSLASH_ESCAPES%27", "NO_BACKSLASH_ESCAPES"},
		{"ANSI_QUOTES alone",
			"root:secret@tcp(localhost:3306)/app?sql_mode=%27ANSI_QUOTES%27", "ANSI_QUOTES"},
		{"mixed into a longer list",
			"root:secret@tcp(localhost:3306)/app?sql_mode=%27STRICT_TRANS_TABLES,NO_BACKSLASH_ESCAPES,NO_ENGINE_SUBSTITUTION%27", "NO_BACKSLASH_ESCAPES"},
		{"combination mode ANSI",
			"root:secret@tcp(localhost:3306)/app?sql_mode=%27ANSI%27", "ANSI"},
		{"vendor combination mode",
			"root@tcp(localhost:3306)/app?sql_mode=%27ORACLE%27", "ORACLE"},
		{"lower-case spelling",
			"root@tcp(localhost:3306)/app?sql_mode=%27ansi_quotes%27", "ANSI_QUOTES"},
		{"upper-case variable name",
			"root@tcp(localhost:3306)/app?SQL_MODE=%27ANSI%27", "ANSI"},
		{"unquoted value",
			"root@tcp(localhost:3306)/app?sql_mode=NO_BACKSLASH_ESCAPES", "NO_BACKSLASH_ESCAPES"},
		{"space after the comma",
			"root@tcp(localhost:3306)/app?sql_mode=%27TRADITIONAL,+ANSI_QUOTES%27", "ANSI_QUOTES"},
		{"raw unencoded quotes",
			"root@tcp(localhost:3306)/app?sql_mode='ANSI'", "ANSI"},
		{"last duplicate wins in the driver; it is the bad one",
			"root@tcp(localhost:3306)/app?sql_mode=%27TRADITIONAL%27&sql_mode=%27ANSI_QUOTES%27", "ANSI_QUOTES"},
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
		{"MySQL 8 factory default list",
			"root:secret@tcp(localhost:3306)/app?sql_mode=%27ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION%27"},
		{"TRADITIONAL implies no lexing change",
			"root:secret@tcp(localhost:3306)/app?sql_mode=%27TRADITIONAL%27"},
		{"empty mode disables everything",
			"root:secret@tcp(localhost:3306)/app?sql_mode=%27%27"},
		// Matching is per comma-separated token, never substring; unknown
		// tokens are the server's to reject, not ours.
		{"unknown token passes through",
			"root:secret@tcp(localhost:3306)/app?sql_mode=%27ANSII%27"},
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
	// Open cannot see the server's sql_mode (it never connects), so it must
	// not guess one either: a DSN without sql_mode stays without sql_mode.
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
	// Open must succeed without a reachable server, like database/sql.
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

// --- error translation ---

func TestTranslate(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{"duplicate entry", &mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'a' for key 'users.email'"}, rio.ErrDuplicateKey},
		{"row is referenced", &mysql.MySQLError{Number: 1451, Message: "Cannot delete or update a parent row"}, rio.ErrForeignKeyViolated},
		{"no referenced row", &mysql.MySQLError{Number: 1452, Message: "Cannot add or update a child row"}, rio.ErrForeignKeyViolated},
		{"wrapped driver error", fmt.Errorf("insert users: %w", &mysql.MySQLError{Number: 1062}), rio.ErrDuplicateKey},
		{"unrelated mysql error", &mysql.MySQLError{Number: 1213, Message: "Deadlock found"}, nil},
		{"not a mysql error", errors.New("plain"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := translate(tt.err); got != tt.want {
				t.Errorf("translate(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// --- integration against a real server, gated by RIO_MYSQL_DSN ---

// Author and Book drive the real-database tests. TableName keeps the tables
// namespaced so the suite can share a database with other tools.
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

// openTestDB connects to RIO_MYSQL_DSN or skips the test, e.g.
// RIO_MYSQL_DSN="root:secret@tcp(localhost:53306)/app".
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

	// Insert must backfill the auto-increment ID via LastInsertId.
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

	// Reading the row back proves parseTime survived the DSN hygiene:
	// without it, scanning DATETIME into time.Time fails.
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

	// A second author with the same email must translate 1062 into
	// ErrDuplicateKey while errors.As still reaches the driver error.
	dup := Author{Name: "Imposter", Email: "ada@example.com"}
	err = rio.Insert(ctx, db, &dup)
	if !errors.Is(err, rio.ErrDuplicateKey) {
		t.Fatalf("duplicate insert error = %v, want rio.ErrDuplicateKey", err)
	}
	var me *mysql.MySQLError
	if !errors.As(err, &me) || me.Number != 1062 {
		t.Fatalf("driver error lost from the chain: %v", err)
	}

	// A book pointing at a missing author must translate 1452.
	orphan := Book{AuthorID: ada.ID + 1000, Title: "Orphan"}
	if err := rio.Insert(ctx, db, &orphan); !errors.Is(err, rio.ErrForeignKeyViolated) {
		t.Fatalf("orphan insert error = %v, want rio.ErrForeignKeyViolated", err)
	}

	// Deleting an author who still has books must translate 1451.
	book := Book{AuthorID: ada.ID, Title: "Notes on the Analytical Engine"}
	if err := rio.Insert(ctx, db, &book); err != nil {
		t.Fatalf("insert book: %v", err)
	}
	if err := rio.Delete(ctx, db, &ada); !errors.Is(err, rio.ErrForeignKeyViolated) {
		t.Fatalf("referenced delete error = %v, want rio.ErrForeignKeyViolated", err)
	}
}
