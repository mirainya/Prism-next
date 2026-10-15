package migrate

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSchemaCanonicalizationPreservesMeaning(t *testing.T) {
	pairs := [][2]string{
		{"`name` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", "`name` varchar(20) COLLATE utf8mb4_unicode_ci"},
		{"CREATE TABLE `t` ( `id` int ) ENGINE=InnoDB AUTO_INCREMENT=42 DEFAULT CHARSET=utf8mb4;", "CREATE TABLE `t` ( `id` int ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"},
		{"CREATE TABLE IF NOT EXISTS `t` (\n `id` int\n) ENGINE=InnoDB", "CREATE TABLE `t` ( `id` int ) ENGINE=InnoDB"},
	}
	for _, pair := range pairs {
		a, err := schemaDDLChecksum(pair[0])
		if err != nil {
			t.Fatal(err)
		}
		b, err := schemaDDLChecksum(pair[1])
		if err != nil || a != b {
			t.Fatalf("formatting changed checksum: %v", err)
		}
	}
	different := [][2]string{
		{"CHECK (`id` >= 0)", "CHECK (`id` > 0)"},
		{"DEFAULT 'two  spaces'", "DEFAULT 'two spaces'"},
		{"DEFAULT ' AUTO_INCREMENT=42 '", "DEFAULT ' '"},
		{"DEFAULT 'CREATE TABLE IF NOT EXISTS t'", "DEFAULT 'CREATE TABLE t'"},
		{"DEFAULT 'a''  b'", "DEFAULT 'a'' b'"},
	}
	for _, pair := range different {
		a, _ := schemaDDLChecksum(pair[0])
		b, _ := schemaDDLChecksum(pair[1])
		if a == b {
			t.Fatal("structurally different SQL has the same canonical checksum")
		}
	}
	if _, err := canonicalSchemaDDL("DEFAULT 'unclosed"); err == nil {
		t.Fatal("accepted unclosed quote")
	}
}

func TestBaselineTriggersRejectMissingAndChangedProtection(t *testing.T) {
	checksum, _ := schemaDDLChecksum("SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'immutable'")
	wanted := []baselineTrigger{{Name: "protect_rows", Table: "billing_entries", Timing: "BEFORE", Event: "DELETE", BodyChecksum: checksum}}
	for _, kind := range []string{"valid", "missing", "changed", "extra"} {
		t.Run(kind, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			connection, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			rows := sqlmock.NewRows([]string{"name", "table", "timing", "event", "body"})
			if kind != "missing" {
				body := "SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'immutable'"
				if kind == "changed" {
					body = "SET @ignored = 1"
				}
				rows.AddRow("protect_rows", "billing_entries", "BEFORE", "DELETE", body)
				if kind == "extra" {
					rows.AddRow("extra_trigger", "billing_entries", "BEFORE", "DELETE", body)
				}
			}
			mock.ExpectQuery("SELECT TRIGGER_NAME").WillReturnRows(rows)
			err = validateBaselineTriggers(context.Background(), connection, wanted)
			if (err == nil) != (kind == "valid") {
				t.Fatalf("kind=%s error=%v", kind, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFailedBaselineRestoresSessionSettings(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	connection, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	mock.ExpectQuery("SELECT @@SESSION.foreign_key_checks").WillReturnRows(sqlmock.NewRows([]string{"fk", "unique", "notes", "mode", "zone", "client", "results", "collation"}).AddRow(1, 1, 1, "STRICT_TRANS_TABLES", "+00:00", "utf8mb4", nil, "utf8mb4_unicode_ci"))
	wanted := errors.New("DDL rejected")
	mock.ExpectExec(regexp.QuoteMeta("broken baseline SQL")).WillReturnError(wanted)
	mock.ExpectExec("SET SESSION foreign_key_checks").WithArgs(1, 1, 1, "STRICT_TRANS_TABLES", "+00:00", "utf8mb4", nil, "utf8mb4_unicode_ci").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := executeCleanBaseline(context.Background(), connection, "broken baseline SQL"); !errors.Is(err, wanted) {
		t.Fatalf("error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
