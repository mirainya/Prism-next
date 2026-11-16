package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
)

const modelNameOccupiedQuery = `SELECT EXISTS(SELECT 1 FROM gw_model_names WHERE api_name=?) OR EXISTS(SELECT 1 FROM gw_models WHERE model_code=?)`

func TestSuggestCatalogModelNameUsesGlobalDatabaseEquality(t *testing.T) {
	for _, test := range []struct {
		base       string
		candidates []string
	}{
		{"model-a", []string{"model-a"}},
		{"Model-A", []string{"Model-A", "Model-A-2", "Model-A-3"}},
		{"model-2", []string{"model-2", "model-2-2"}},
		{"seedance2.5-9图", []string{"seedance2.5-9图", "seedance2.5-9图-2"}},
		{strings.Repeat("a", 128), []string{strings.Repeat("a", 128), strings.Repeat("a", 126) + "-2"}},
		{"a" + strings.Repeat("图", 42) + "b", []string{"a" + strings.Repeat("图", 42) + "b", "a" + strings.Repeat("图", 41) + "-2"}},
	} {
		t.Run(test.base, func(t *testing.T) {
			store, mock := newCatalogModelOnboardTestStore(t)
			for index, candidate := range test.candidates {
				mock.ExpectQuery(regexp.QuoteMeta(modelNameOccupiedQuery)).WithArgs(candidate, candidate).
					WillReturnRows(sqlmock.NewRows([]string{"occupied"}).AddRow(index < len(test.candidates)-1))
			}
			name, err := store.SuggestCatalogModelName(context.Background(), CatalogModelNameSuggestionInput{BaseName: test.base})
			if err != nil || name != test.candidates[len(test.candidates)-1] || len(name) > 128 || !utf8.ValidString(name) {
				t.Fatalf("name=%q err=%v", name, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSuggestCatalogModelNameValidatesAndBoundsReads(t *testing.T) {
	store, mock := newCatalogModelOnboardTestStore(t)
	for _, value := range []string{"", "with space", "模型", "model?x", "model\n", strings.Repeat("a", 129)} {
		if _, err := store.SuggestCatalogModelName(context.Background(), CatalogModelNameSuggestionInput{BaseName: value}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("value=%q err=%v", value, err)
		}
	}
	for index := 1; index <= 100; index++ {
		candidate := "a"
		if index > 1 {
			candidate += "-" + strconv.Itoa(index)
		}
		mock.ExpectQuery(regexp.QuoteMeta(modelNameOccupiedQuery)).WithArgs(candidate, candidate).
			WillReturnRows(sqlmock.NewRows([]string{"occupied"}).AddRow(true))
	}
	if _, err := store.SuggestCatalogModelName(context.Background(), CatalogModelNameSuggestionInput{BaseName: "a"}); !errors.Is(err, ErrPublicModelNameConflict) {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSuggestCatalogModelNameDoesNotTreatReadFailureAsAvailable(t *testing.T) {
	store, mock := newCatalogModelOnboardTestStore(t)
	forced := errors.New("read failed")
	mock.ExpectQuery(regexp.QuoteMeta(modelNameOccupiedQuery)).WillReturnError(forced)
	if name, err := store.SuggestCatalogModelName(context.Background(), CatalogModelNameSuggestionInput{BaseName: "a"}); name != "" || !errors.Is(err, forced) {
		t.Fatalf("name=%q err=%v", name, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNewModelIdentityNeverReusesReservedIdentity(t *testing.T) {
	for _, reserved := range []string{"public name", "model code"} {
		t.Run(reserved, func(t *testing.T) {
			store, mock := newCatalogModelOnboardTestStore(t)
			mock.ExpectBegin()
			names := sqlmock.NewRows([]string{"id"})
			if reserved == "public name" {
				names.AddRow(10)
			}
			mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM gw_model_names WHERE api_name=? FOR UPDATE`)).WithArgs("a").WillReturnRows(names)
			if reserved == "model code" {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM gw_models WHERE model_code=? FOR UPDATE`)).WithArgs("internal-a").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(11))
			}
			mock.ExpectRollback()
			err := store.WithTx(context.Background(), func(tx *sql.Tx) error {
				_, _, err := createNewCatalogModelIdentity(context.Background(), tx, "internal-a", "a")
				return err
			})
			if !errors.Is(err, ErrPublicModelNameConflict) || !errors.Is(err, ErrConflict) {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNewModelIdentitySuccessAndConcurrentNameConflicts(t *testing.T) {
	for _, conflictAt := range []string{"none", "model", "name"} {
		t.Run(conflictAt, func(t *testing.T) {
			store, mock := newCatalogModelOnboardTestStore(t)
			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM gw_model_names WHERE api_name=? FOR UPDATE`)).WithArgs("a-2").WillReturnRows(sqlmock.NewRows([]string{"id"}))
			mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM gw_models WHERE model_code=? FOR UPDATE`)).WithArgs("a-2").WillReturnRows(sqlmock.NewRows([]string{"id"}))
			modelInsert := mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO gw_models(model_code,created_at) VALUES (?,?)`)).WithArgs("a-2", sqlmock.AnyArg())
			duplicate := &mysql.MySQLError{Number: 1062, Message: "duplicate"}
			if conflictAt == "model" {
				modelInsert.WillReturnError(duplicate)
			} else {
				modelInsert.WillReturnResult(sqlmock.NewResult(31, 1))
				nameInsert := mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO gw_model_names(model_id,api_name,is_primary,created_at) VALUES (?,?,TRUE,?)`)).WithArgs(31, "a-2", sqlmock.AnyArg())
				if conflictAt == "name" {
					nameInsert.WillReturnError(duplicate)
				} else {
					nameInsert.WillReturnResult(sqlmock.NewResult(32, 1))
				}
			}
			if conflictAt == "none" {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			err := store.WithTx(context.Background(), func(tx *sql.Tx) error {
				modelID, nameID, err := createNewCatalogModelIdentity(context.Background(), tx, "a-2", "a-2")
				if conflictAt == "none" && (modelID != 31 || nameID != 32) {
					t.Fatalf("model=%d name=%d", modelID, nameID)
				}
				return err
			})
			if conflictAt == "none" && err != nil || conflictAt != "none" && !errors.Is(err, ErrPublicModelNameConflict) {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOnboardCatalogModelRejectsExistingGlobalModelName(t *testing.T) {
	store, mock := newCatalogModelOnboardTestStore(t)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT active_release_id FROM gw_catalog_runtime_state WHERE id=1 FOR UPDATE`)).WillReturnRows(sqlmock.NewRows([]string{"active_release_id"}).AddRow(7))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status,config_version,content_hash,semantic_digest FROM gw_catalog_releases WHERE id=? FOR UPDATE`)).WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"status", "config_version", "content_hash", "semantic_digest"}).AddRow("published", 3, strings.Repeat("b", 64), strings.Repeat("c", 64)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM gw_model_names WHERE api_name=? FOR UPDATE`)).WithArgs("new-model").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(32))
	mock.ExpectRollback()
	err := store.WithTx(context.Background(), func(tx *sql.Tx) error {
		_, err := store.OnboardCatalogModel(context.Background(), tx, validCatalogModelOnboardInput(), func(RateEvidenceInput) (string, error) { return strings.Repeat("d", 64), nil }, 1)
		return err
	})
	if !errors.Is(err, ErrPublicModelNameConflict) {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
