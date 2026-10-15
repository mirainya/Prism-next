package upstreamerror

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSaveAndRefreshRules(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previous := activeRules.Load()
	defer activeRules.Store(previous)
	rules := Rules{Patterns: []string{`(?i)quota`}, Message: "上游暂不可用"}
	patterns, _ := json.Marshal(rules.Patterns)
	mock.ExpectExec("INSERT INTO gw_upstream_error_rules").WithArgs(patterns, rules.Message).WillReturnResult(sqlmock.NewResult(1, 1))
	if err := Save(context.Background(), db, rules); err != nil {
		t.Fatal(err)
	}
	if got := Sanitize("QUOTA exhausted"); got != rules.Message {
		t.Fatalf("saved message = %q", got)
	}
	if got := Sanitize("max_tokens 4096"); got != "max_tokens 4096" {
		t.Fatalf("ordinary message = %q", got)
	}
	mock.ExpectQuery("SELECT patterns,message FROM gw_upstream_error_rules").WillReturnRows(sqlmock.NewRows([]string{"patterns", "message"}).AddRow(`[]`, "服务不可用"))
	if err := Refresh(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if got := Sanitize("QUOTA exhausted"); got != "QUOTA exhausted" {
		t.Fatalf("empty rules changed message: %q", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidOrFailedRulesKeepPreviousConfig(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previous := activeRules.Load()
	defer activeRules.Store(previous)
	if err := Save(context.Background(), db, Rules{Patterns: []string{"["}, Message: "unavailable"}); !errors.Is(err, ErrInvalidRules) {
		t.Fatalf("invalid regex error = %v", err)
	}
	mock.ExpectExec("INSERT INTO gw_upstream_error_rules").WillReturnError(errors.New("database unavailable"))
	if err := Save(context.Background(), db, Rules{Patterns: []string{"quota"}, Message: "unavailable"}); err == nil {
		t.Fatal("save unexpectedly succeeded")
	}
	mock.ExpectQuery("SELECT patterns,message FROM gw_upstream_error_rules").WillReturnRows(sqlmock.NewRows([]string{"patterns", "message"}).AddRow(`["["]`, "unavailable"))
	if err := Refresh(context.Background(), db); !errors.Is(err, ErrInvalidRules) {
		t.Fatalf("refresh error = %v", err)
	}
	if activeRules.Load() != previous {
		t.Fatal("failed update replaced active configuration")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
