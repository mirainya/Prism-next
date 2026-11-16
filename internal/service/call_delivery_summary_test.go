package service

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCallDeliverySummaryClassifiesFiles(t *testing.T) {
	for _, test := range []struct {
		name string
		row  callDeliverySummaryRow
		want model.APICallDeliverySummary
	}{
		{"stored", callDeliverySummaryRow{DeliveryMode: "managed_copy", State: "ready", Available: true}, model.APICallDeliverySummary{Total: 1, Ready: 1}},
		{"missing asset", callDeliverySummaryRow{DeliveryMode: "managed_copy", State: "ready"}, model.APICallDeliverySummary{Total: 1, Unavailable: 1}},
		{"pending", callDeliverySummaryRow{DeliveryMode: "managed_copy", State: "pending"}, model.APICallDeliverySummary{Total: 1, Processing: 1}},
		{"checking", callDeliverySummaryRow{DeliveryMode: "managed_copy", State: "delivery_failed", ReasonCode: "managed_copy_import_retry_uncertain", RetryStatus: "pending"}, model.APICallDeliverySummary{Total: 1, Processing: 1, Checking: 1}},
		{"retrying", callDeliverySummaryRow{DeliveryMode: "managed_copy", State: "delivery_failed", ReasonCode: "managed_copy_upload_retry_timeout", RetryStatus: "dispatching"}, model.APICallDeliverySummary{Total: 1, Processing: 1}},
		{"stopped", callDeliverySummaryRow{DeliveryMode: "managed_copy", State: "delivery_failed", ReasonCode: "managed_copy_import_retry_uncertain", RetryStatus: "dead_letter"}, model.APICallDeliverySummary{Total: 1, Failed: 1}},
		{"expired", callDeliverySummaryRow{DeliveryMode: "managed_copy", State: "expired"}, model.APICallDeliverySummary{Total: 1, Expired: 1}},
		{"reference", callDeliverySummaryRow{DeliveryMode: "reference", State: "ready"}, model.APICallDeliverySummary{Total: 1, Reference: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var got model.APICallDeliverySummary
			test.row.addTo(&got)
			if got != test.want {
				t.Fatalf("summary=%+v want=%+v", got, test.want)
			}
		})
	}
}

func TestCallDeliverySummaryLoadsOnlyAuthorizedPageInOneQuery(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}),
		&gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := appendCallDeliverySummaries(db, nil); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`(?s)SELECT c.public_id AS call_id,.*JOIN gw_api_calls AS c ON c.id=d.call_id AND c.user_id=d.user_id AND c.token_id=d.token_id.*c.public_id IN \(\?,\?\)`).
		WithArgs(sqlmock.AnyArg(), "allowed-1", "allowed-2").
		WillReturnRows(sqlmock.NewRows([]string{"call_id", "delivery_mode", "state", "reason_code", "retry_status", "available"}).
			AddRow("allowed-1", "managed_copy", "ready", "", "", true).
			AddRow("allowed-1", "managed_copy", "delivery_failed", "managed_copy_size_exceeded", "dead_letter", false))
	calls := []model.APICall{{ID: "allowed-1"}, {ID: "allowed-2"}}
	if err := appendCallDeliverySummaries(db, calls); err != nil {
		t.Fatal(err)
	}
	if got := calls[0].DeliverySummary; got == nil || got.Total != 2 || got.Ready != 1 || got.Failed != 1 {
		t.Fatalf("partial failure missing: %+v", got)
	}
	if got := calls[1].DeliverySummary; got == nil || got.Total != 0 {
		t.Fatalf("call without files was not represented: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
