package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestUpstreamAvailabilityAcceptsStatusOnlyRows(t *testing.T) {
	estimated := uint64(285000)
	item := UpstreamAvailabilityRow{
		ModelCode: "doubao-seedance-2-5-260628", Category: "video",
		ChannelStatus: "正常", EstimatedCompletionMS: &estimated,
		ObservedAt: time.Unix(1789380174, 0).UTC(),
	}
	if !validUpstreamAvailabilityRow(item) {
		t.Fatal("status-only observation should be valid without a success rate or window")
	}
	item.ChannelStatus = ""
	if !validUpstreamAvailabilityRow(item) {
		t.Fatal("generation pace should be valid without a success rate or status")
	}
}

func TestUpstreamAvailabilityRequiresAValidAvailableFact(t *testing.T) {
	observed := time.Unix(1789380174, 0).UTC()
	for name, item := range map[string]UpstreamAvailabilityRow{
		"empty observation":         {ModelCode: "model-a", Category: "video", ObservedAt: observed},
		"rate without window":       {ModelCode: "model-a", Category: "video", SuccessRate: "90", ObservedAt: observed},
		"negative rate":             {ModelCode: "model-a", Category: "video", SuccessRate: "-1", WindowMinutes: 120, ObservedAt: observed},
		"rate above 100":            {ModelCode: "model-a", Category: "video", SuccessRate: "100.1", WindowMinutes: 120, ObservedAt: observed},
		"excessive generation time": {ModelCode: "model-a", Category: "video", EstimatedCompletionMS: ptrUint64(86400001), ObservedAt: observed},
	} {
		t.Run(name, func(t *testing.T) {
			if validUpstreamAvailabilityRow(item) {
				t.Fatal("unexpected valid observation")
			}
		})
	}
}

func ptrUint64(value uint64) *uint64 { return &value }

func TestUpstreamAvailabilityDiscardsResponsesAfterSourceChanges(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  string
		version uint64
		allowed bool
	}{
		{"current source", "active", 3, true},
		{"edited source", "active", 4, false},
		{"disabled source", "disabled", 3, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store, _ := New(db)
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT status,config_version FROM gw_availability_sources").WithArgs(14).
				WillReturnRows(sqlmock.NewRows([]string{"status", "version"}).AddRow(test.status, test.version))
			if test.allowed {
				mock.ExpectExec("DELETE FROM gw_upstream_availability WHERE availability_source_id").WithArgs(14).WillReturnResult(sqlmock.NewResult(0, 2))
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			err = store.ReplaceUpstreamAvailability(context.Background(), 14, 3, nil)
			if test.allowed && err != nil || !test.allowed && !errors.Is(err, ErrConflict) {
				t.Fatalf("allowed=%v, error=%v", test.allowed, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
