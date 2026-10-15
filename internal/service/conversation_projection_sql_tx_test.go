package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mirainya/Prism/internal/gateway/canonical"
	"github.com/mirainya/Prism/internal/model"
)

func TestConversationProjectionSQLTxUsesCallerTransaction(t *testing.T) {
	db := setupConversationProjectionOutboxTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	input := []canonical.Item{{
		Type: "message", Role: canonical.RoleUser,
		Content: []canonical.Content{{Type: "input_text", Text: "question"}},
	}}
	output := []canonical.Item{{
		Type: "message", Role: canonical.RoleAssistant,
		Content: []canonical.Content{{Type: "output_text", Text: "answer"}},
	}}

	for _, test := range []struct {
		name   string
		callID string
		commit bool
	}{
		{name: "rollback", callID: "call_sql_tx_rollback"},
		{name: "commit", callID: "call_sql_tx_commit", commit: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx, err := sqlDB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()

			now := time.Now()
			_, err = tx.ExecContext(ctx, `INSERT INTO gw_api_calls
				(public_id,user_id,token_id,catalog_release_id,model_operation_id,status,created_at,updated_at)
				VALUES (?,?,?,?,?,?,?,?)`, test.callID, 201, 202, 1, 1,
				string(model.APICallStatusInProgress), now, now)
			if err != nil {
				t.Fatal(err)
			}
			if err := StageAPIConversationProjectionInputSQLTx(ctx, tx, ConversationProjectionInputRequest{
				CallID: test.callID, InputItems: input,
			}); err != nil {
				t.Fatalf("stage input in caller transaction: %v", err)
			}
			if test.commit {
				updated, err := StageAPIConversationProjectionOutputIfMissingSQLTx(ctx, tx, ConversationProjectionOutputRequest{
					CallID: test.callID,
				})
				if err != nil || !updated {
					t.Fatalf("stage fallback output: updated=%v err=%v", updated, err)
				}
			}
			updated, err := StageAPIConversationProjectionOutputIfPresentSQLTx(ctx, tx, ConversationProjectionOutputRequest{
				CallID: test.callID, OutputItems: output,
			})
			if err != nil || !updated {
				t.Fatalf("stage output in caller transaction: updated=%v err=%v", updated, err)
			}
			if test.commit {
				updated, err = StageAPIConversationProjectionOutputIfMissingSQLTx(ctx, tx, ConversationProjectionOutputRequest{
					CallID: test.callID,
				})
				if err != nil || updated {
					t.Fatalf("fallback replaced real output: updated=%v err=%v", updated, err)
				}
			}
			var count int
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM conversation_projection_outbox WHERE call_id = ? AND input_ready = 1 AND output_ready = 1", test.callID).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("outbox rows visible in caller transaction = %d, want 1", count)
			}
			if test.commit {
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
			} else if err := tx.Rollback(); err != nil {
				t.Fatal(err)
			}

			var entries []model.ConversationProjectionOutbox
			if err := db.Where("call_id = ?", test.callID).Find(&entries).Error; err != nil {
				t.Fatal(err)
			}
			if !test.commit {
				if len(entries) != 0 {
					t.Fatalf("rolled-back outbox rows = %d, want 0", len(entries))
				}
				return
			}
			if len(entries) != 1 || !entries[0].InputReady || !entries[0].OutputReady {
				t.Fatalf("committed outbox entries = %#v", entries)
			}
			items, err := unmarshalConversationProjectionItems(entries[0].CanonicalOutput, "output")
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 1 || items[0].Content[0].Text != "answer" {
				t.Fatalf("committed output = %#v", items)
			}
		})
	}
}

func TestConversationProjectionSQLTxRejectsNilTransaction(t *testing.T) {
	if err := StageAPIConversationProjectionInputSQLTx(context.Background(), nil, ConversationProjectionInputRequest{}); !errors.Is(err, ErrAPICallInvalidInput) {
		t.Fatalf("nil input transaction error = %v", err)
	}
	if _, err := StageAPIConversationProjectionOutputIfPresentSQLTx(context.Background(), nil, ConversationProjectionOutputRequest{}); !errors.Is(err, ErrAPICallInvalidInput) {
		t.Fatalf("nil present-output transaction error = %v", err)
	}
	if _, err := StageAPIConversationProjectionOutputIfMissingSQLTx(context.Background(), nil, ConversationProjectionOutputRequest{}); !errors.Is(err, ErrAPICallInvalidInput) {
		t.Fatalf("nil missing-output transaction error = %v", err)
	}
}
