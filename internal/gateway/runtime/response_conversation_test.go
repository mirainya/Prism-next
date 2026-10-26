package runtime

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/gateway/billing"
	"github.com/mirainya/Prism/internal/gateway/canonical"
	"github.com/mirainya/Prism/internal/gateway/execution"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/internal/service"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type responseConversationItemsArg struct {
	want []canonical.Item
}

func (arg responseConversationItemsArg) Match(value driver.Value) bool {
	var raw []byte
	switch value := value.(type) {
	case []byte:
		raw = value
	case string:
		raw = []byte(value)
	default:
		return false
	}
	var got []canonical.Item
	return json.Unmarshal(raw, &got) == nil && reflect.DeepEqual(got, arg.want)
}

func responseConversationTestService(t *testing.T) (*Service, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	previous := (*gorm.DB)(nil)
	if model.HasDB() {
		previous = model.DB()
	}
	gormDB, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	model.SetDB(gormDB)
	t.Cleanup(func() { model.SetDB(previous) })
	store, err := repository.New(db)
	if err != nil {
		t.Fatal(err)
	}
	runtimeService, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	return runtimeService, mock
}

func TestBackgroundResponseStagesConversationOutputBeforeTerminalCommit(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     string
		output     *service.ConversationProjectionOutputRequest
		wantItems  []canonical.Item
		wantUpdate string
	}{
		{
			name: "completed response preserves canonical output", status: "completed",
			output: &service.ConversationProjectionOutputRequest{
				CallID: "ignored", RequestLogID: 27, ProviderResponseID: "upstream-1", FinishReason: "stop",
				OutputItems: []canonical.Item{{Type: "message", Role: canonical.RoleAssistant}},
			},
			wantItems:  []canonical.Item{{Type: "message", Role: canonical.RoleAssistant}},
			wantUpdate: "AND input_ready = ?",
		},
		{
			name: "failed response stages empty output only if missing", status: "failed",
			wantItems: []canonical.Item{}, wantUpdate: "AND input_ready = ? AND output_ready = ?",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtimeService, mock := responseConversationTestService(t)
			expires := time.Now().Add(time.Minute).UTC()
			item := repository.OutboxItem{
				ID: 31, AttemptID: 12, Action: "submit", ActionSeq: 1, StateVersion: 1,
				Attempts: 1, LeaseOwner: "worker", LeaseExpiresAt: expires,
				CallLease: repository.CallLease{CallID: 9, AttemptID: 12, Owner: "worker", ExpiresAt: expires},
			}
			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta("SELECT a.call_id,a.state,a.state_version,c.status,c.state_version,r.id,c.public_id")).
				WithArgs(uint64(12)).WillReturnRows(sqlmock.NewRows([]string{
				"call_id", "state", "attempt_version", "status", "call_version", "resource_id", "public_id",
			}).AddRow(9, "started", 1, "in_progress", 1, 15, "resp_1"))
			mock.ExpectQuery(regexp.QuoteMeta("SELECT resource_kind FROM gw_api_resources WHERE id=? FOR UPDATE")).
				WithArgs(uint64(15)).WillReturnRows(sqlmock.NewRows([]string{"resource_kind"}).AddRow("response"))
			mock.ExpectQuery(regexp.QuoteMeta("SELECT status FROM gw_ai_responses WHERE resource_id=? FOR UPDATE")).
				WithArgs(uint64(15)).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("queued"))
			mock.ExpectExec(regexp.QuoteMeta("UPDATE gw_ai_responses SET status=?")).
				WithArgs(test.status, sqlmock.AnyArg(), sqlmock.AnyArg(), uint64(15)).WillReturnResult(sqlmock.NewResult(0, 1))
			projectionUpdate := regexp.QuoteMeta("UPDATE `conversation_projection_outbox` SET ") + ".*" +
				regexp.QuoteMeta(" WHERE call_id = ? "+test.wantUpdate)
			projection := mock.ExpectExec(projectionUpdate)
			if test.output != nil {
				projection.WithArgs("stop", nil, responseConversationItemsArg{want: test.wantItems}, true,
					"upstream-1", uint(27), sqlmock.AnyArg(), "resp_1", true)
			} else {
				projection.WithArgs("", nil, responseConversationItemsArg{want: test.wantItems}, true,
					"", uint(0), sqlmock.AnyArg(), "resp_1", true, false)
			}
			projection.WillReturnResult(sqlmock.NewResult(0, 1))
			leaseFailure := errors.New("later lease verification failed")
			mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM gw_api_calls")).WillReturnError(leaseFailure)
			mock.ExpectRollback()
			err := runtimeService.Store.WithTx(context.Background(), func(tx *sql.Tx) error {
				return runtimeService.finishBackgroundResponseTx(context.Background(), tx, item, 1, test.status,
					execution.AttemptCompleted, execution.CallCompleted, "test", billing.Facts{}, nil, nil, test.output)
			})
			if !errors.Is(err, leaseFailure) {
				t.Fatalf("terminal error = %v, want lease rejection after projection update", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCancelQueuedResponseStagesEmptyConversationOutput(t *testing.T) {
	runtimeService, mock := responseConversationTestService(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT c.id,a.id")).
		WithArgs("resp_1", uint64(6), uint64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"call_id", "attempt_id"}).AddRow(9, 12))
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM billing_reservations WHERE call_id=?")).
		WithArgs(uint64(9)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(20))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT call_id,billing_account_id,budget_window_id FROM billing_reservations WHERE id=?")).
		WithArgs(uint64(20)).WillReturnRows(sqlmock.NewRows([]string{"call_id", "billing_account_id", "budget_window_id"}).AddRow(9, 21, 22))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id,currency_code,currency_version,posted_balance,held_amount,credit_limit,status,state_version FROM billing_accounts WHERE id=? FOR UPDATE")).
		WithArgs(uint64(21)).WillReturnRows(sqlmock.NewRows([]string{"user_id", "currency_code", "currency_version", "posted_balance", "held_amount", "credit_limit", "status", "state_version"}).
		AddRow(6, "USD", 1, "100", "0", "0", "active", 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT token_id,limit_amount,used_amount,held_amount,window_start,window_end FROM token_budget_windows WHERE id=? FOR UPDATE")).
		WithArgs(uint64(22)).WillReturnRows(sqlmock.NewRows([]string{"token_id", "limit_amount", "used_amount", "held_amount", "window_start", "window_end"}).
		AddRow(7, nil, "0", "0", time.Now(), nil))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id,token_id FROM gw_api_calls WHERE id=? FOR UPDATE")).
		WithArgs(uint64(9)).WillReturnRows(sqlmock.NewRows([]string{"user_id", "token_id"}).AddRow(6, 7))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id FROM tokens WHERE id=?")).
		WithArgs(uint64(7)).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(6))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT call_id,billing_account_id,budget_window_id,amount,state,state_version FROM billing_reservations WHERE id=? FOR UPDATE")).
		WithArgs(uint64(20)).WillReturnRows(sqlmock.NewRows([]string{"call_id", "billing_account_id", "budget_window_id", "amount", "state", "state_version"}).
		AddRow(9, 21, 22, "0", "held", 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status,state_version,current_attempt_id FROM gw_api_calls WHERE id=? FOR UPDATE")).
		WithArgs(uint64(9)).WillReturnRows(sqlmock.NewRows([]string{"status", "state_version", "current_attempt_id"}).AddRow("in_progress", 1, 12))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT state,state_version FROM gw_api_call_attempts WHERE id=? AND call_id=? FOR UPDATE")).
		WithArgs(uint64(12), uint64(9)).WillReturnRows(sqlmock.NewRows([]string{"state", "state_version"}).AddRow("started", 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT r.id,response.status FROM gw_api_resources r JOIN gw_ai_responses response ON response.resource_id=r.id WHERE r.call_id=? AND r.public_id=? AND r.user_id=? AND r.token_id=? AND r.deleted_at IS NULL FOR UPDATE")).
		WithArgs(uint64(9), "resp_1", uint64(6), uint64(7)).WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(15, "queued"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id,status,attempt_count,lease_owner FROM gw_async_outbox WHERE attempt_id=? AND action='submit' ORDER BY action_seq DESC LIMIT 1 FOR UPDATE")).
		WithArgs(uint64(12)).WillReturnRows(sqlmock.NewRows([]string{"id", "status", "attempt_count", "lease_owner"}).AddRow(31, "pending", 0, ""))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM gw_channel_request_logs WHERE attempt_id=? ORDER BY request_seq DESC LIMIT 1 FOR UPDATE")).
		WithArgs(uint64(12)).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE gw_async_outbox SET status='failed',last_error_code='cancelled_before_send',updated_at=? WHERE id=? AND status='pending' AND attempt_count=0 AND lease_owner=''")).
		WithArgs(sqlmock.AnyArg(), uint64(31)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT resource_kind FROM gw_api_resources WHERE id=? FOR UPDATE")).
		WithArgs(uint64(15)).WillReturnRows(sqlmock.NewRows([]string{"resource_kind"}).AddRow("response"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status FROM gw_ai_responses WHERE resource_id=? FOR UPDATE")).
		WithArgs(uint64(15)).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("queued"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE gw_ai_responses SET status=?")).
		WithArgs("cancelled", sqlmock.AnyArg(), sqlmock.AnyArg(), uint64(15)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `conversation_projection_outbox` SET ")+".*"+
		regexp.QuoteMeta(" WHERE call_id = ? AND input_ready = ? AND output_ready = ?")).
		WithArgs("", nil, responseConversationItemsArg{want: []canonical.Item{}}, true,
			"", uint(0), sqlmock.AnyArg(), "resp_1", true, false).
		WillReturnResult(sqlmock.NewResult(0, 1))
	transitionFailure := errors.New("later attempt transition failed")
	mock.ExpectExec(regexp.QuoteMeta("UPDATE gw_api_call_attempts SET state=?")).WillReturnError(transitionFailure)
	mock.ExpectRollback()
	err := runtimeService.CancelQueuedResponse(context.Background(), 6, 7, "resp_1")
	if !errors.Is(err, transitionFailure) {
		t.Fatalf("cancel error = %v, want transition failure after projection update", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
