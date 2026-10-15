package update

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestLoadSourcesIncludesOnlyGitHubByDefaultWithRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT revision FROM prism_update_sources_meta").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(7))
	mock.ExpectQuery("SELECT id,source_name,base_url").WillReturnRows(sqlmock.NewRows([]string{"id", "source_name", "base_url", "enabled", "priority", "config_version"}))
	mock.ExpectCommit()
	config, err := LoadSources(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if config.Revision != 7 || len(config.Sources) != 1 || !config.Sources[0].BuiltIn || config.Sources[0].BaseURL != DefaultGitHubBaseURL {
		t.Fatalf("unexpected config %+v", config)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSaveSourcesRejectsStaleWholeListEvenWhenEmpty(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT revision FROM prism_update_sources_meta.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(9))
	mock.ExpectRollback()
	if err := SaveSources(context.Background(), db, []SourceConfig{}, 1, 8); !errors.Is(err, ErrSourceConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSaveEmptySourceListIsSerializedAndAdvancesRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT revision FROM prism_update_sources_meta.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(1))
	mock.ExpectQuery("SELECT id,config_version FROM prism_update_sources.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"id", "config_version"}))
	mock.ExpectExec("UPDATE prism_update_sources_meta SET revision=revision\\+1").WithArgs(sqlmock.AnyArg(), uint64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := SaveSources(context.Background(), db, []SourceConfig{}, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSourceConfigurationRejectsUnsafeOrChangedBuiltInSources(t *testing.T) {
	for _, source := range []SourceConfig{
		{Name: "unsafe", BaseURL: "http://8.8.8.8", Enabled: true},
		{Name: "unsafe", BaseURL: "https://127.0.0.1", Enabled: true},
		{Name: "unsafe", BaseURL: "https://user:password@8.8.8.8", Enabled: true},
		{Name: "unsafe", BaseURL: "https://8.8.8.8/?token=secret", Enabled: true},
		{Name: GitHubSourceName, BaseURL: DefaultGitHubBaseURL, Enabled: false, BuiltIn: true},
	} {
		if _, err := validateSourceList(context.Background(), []SourceConfig{source}); !errors.Is(err, ErrInvalidSource) {
			t.Errorf("unexpected validation result for %s: %v", source.Name, err)
		}
	}
}
