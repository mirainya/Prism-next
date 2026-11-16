package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/mirainya/Prism/internal/gateway/billing"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/shopspring/decimal"
)

var (
	ErrInvalidFundsOperation = errors.New("invalid funds operation")
	ErrFundsRequestConflict  = errors.New("funds request ID was reused with different input")
)

type FundsAdjustmentRequest struct {
	Amount    decimal.Decimal `json:"amount" binding:"required"`
	RequestID string          `json:"request_id" binding:"required,uuid"`
	Note      string          `json:"note" binding:"required,max=500"`
}

func (r FundsAdjustmentRequest) validate() error {
	id, err := uuid.Parse(r.RequestID)
	if err != nil || id == uuid.Nil || id.String() != r.RequestID || strings.TrimSpace(r.Note) == "" ||
		!utf8.ValidString(r.Note) || utf8.RuneCountInString(r.Note) > 500 {
		return ErrInvalidFundsOperation
	}
	if _, err := billing.ParseAmount(r.Amount.String(), 18, true); err != nil || !r.Amount.IsPositive() ||
		r.Amount.GreaterThan(decimal.RequireFromString("99999999999999999999.999999999999999999")) {
		return ErrInvalidBalanceAmount
	}
	return nil
}

// ApplyUserCredit uses the request ID as the durable funding source key. The
// account row serializes retries; funding and business audit commit together.
func (s *UserService) ApplyUserCredit(ctx context.Context, actorID, userID uint, req FundsAdjustmentRequest) error {
	if err := req.validate(); err != nil {
		return err
	}
	if actorID == 0 || userID == 0 {
		return ErrInvalidFundsOperation
	}
	store, err := unifiedFundsStore()
	if err != nil {
		return err
	}
	return store.WithTx(ctx, func(tx *sql.Tx) error {
		accountID, err := store.OpenBillingAccount(ctx, tx, uint64(userID))
		if err != nil {
			return err
		}
		source := fmt.Sprintf("admin:%d:user:%d:%s", actorID, userID, req.RequestID)
		var previousEvent uint64
		var previousAmount string
		err = tx.QueryRowContext(ctx, `SELECT id,amount FROM billing_events WHERE billing_account_id=? AND event_key=? AND event_type='funding_credit'`, accountID, "funding:"+source).Scan(&previousEvent, &previousAmount)
		if err == nil {
			amount, err := decimal.NewFromString(previousAmount)
			if err != nil || !amount.Equal(req.Amount) {
				return ErrFundsRequestConflict
			}
			return verifyFundsOperationAudit(ctx, tx, "billing_event", previousEvent, req)
		} else if err != sql.ErrNoRows {
			return err
		}
		if err := store.CreditBillingAccount(ctx, tx, accountID, req.Amount.String(), source); err != nil {
			return err
		}
		var eventID uint64
		var after string
		if err := tx.QueryRowContext(ctx, `SELECT e.id,a.posted_balance FROM billing_events e
JOIN billing_accounts a ON a.id=e.billing_account_id
WHERE e.billing_account_id=? AND e.event_key=? AND e.event_type='funding_credit'`, accountID, "funding:"+source).Scan(&eventID, &after); err != nil {
			return err
		}
		return appendFundsOperationAudit(ctx, tx, actorID, userID, 0, "billing_event", eventID, "funding_credit", req, after)
	})
}

// AdjustTokenBudget changes the Key limit, never the owner's account balance.
func (s *TokenService) AdjustTokenBudget(ctx context.Context, userID, tokenID uint, req FundsAdjustmentRequest) (*TokenFunds, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	store, err := unifiedFundsStore()
	if err != nil {
		return nil, err
	}
	err = store.WithTx(ctx, func(tx *sql.Tx) error {
		// Serialize operations for this owner, including retries across budget
		// policy changes. This also shares the funding operation's lock order.
		if _, err := store.OpenBillingAccount(ctx, tx, uint64(userID)); err != nil {
			return err
		}
		windowID, err := currentTokenBudgetWindow(ctx, tx, uint64(userID), uint64(tokenID))
		if err != nil {
			return err
		}
		source := fmt.Sprintf("user:%d:token:%d:%s", userID, tokenID, req.RequestID)
		var existingID uint64
		var previousAmount string
		err = tx.QueryRowContext(ctx, `SELECT id,amount_delta FROM token_budget_adjustment_events WHERE source_type='console_budget' AND source_key=?`, source).
			Scan(&existingID, &previousAmount)
		if err == nil {
			amount, parseErr := decimal.NewFromString(previousAmount)
			if parseErr != nil || !amount.Equal(req.Amount) {
				return ErrFundsRequestConflict
			}
			// Do not apply an old request a second time to a new budget window.
			return verifyFundsOperationAudit(ctx, tx, "budget_adjustment", existingID, req)
		} else if err != sql.ErrNoRows {
			return err
		}
		eventID, _, err := store.ApplyBudgetAdjustment(ctx, tx, repository.BudgetAdjustmentInput{
			WindowID: windowID, AmountDelta: req.Amount.String(), SourceType: "console_budget", SourceKey: source,
		})
		if err != nil {
			return err
		}
		var limit string
		if err := tx.QueryRowContext(ctx, `SELECT limit_amount FROM token_budget_windows WHERE id=?`, windowID).Scan(&limit); err != nil {
			return err
		}
		return appendFundsOperationAudit(ctx, tx, userID, userID, tokenID, "budget_adjustment", eventID, "budget_increase", req, limit)
	})
	if err != nil {
		return nil, err
	}
	// Read current values even when the request was a replay of an older event.
	current, err := loadTokenFunds(ctx, []uint{tokenID})
	if err != nil {
		return nil, err
	}
	snapshot := current[tokenID]
	return &TokenFunds{ID: tokenID, Balance: snapshot.Available, TotalUsed: snapshot.Used}, nil
}

func verifyFundsOperationAudit(ctx context.Context, tx *sql.Tx, resource string, eventID uint64, req FundsAdjustmentRequest) error {
	var metadata []byte
	if err := tx.QueryRowContext(ctx, `SELECT metadata FROM audit_events WHERE resource_type=? AND resource_id=? AND outcome='success' ORDER BY id LIMIT 1`, resource, fmt.Sprint(eventID)).Scan(&metadata); err != nil {
		return err
	}
	var previous struct {
		RequestID string `json:"request_id"`
		Note      string `json:"note"`
	}
	if err := json.Unmarshal(metadata, &previous); err != nil || previous.RequestID != req.RequestID || previous.Note != strings.TrimSpace(req.Note) {
		return ErrFundsRequestConflict
	}
	return nil
}

func appendFundsOperationAudit(ctx context.Context, tx *sql.Tx, actorID, userID, tokenID uint, resource string, eventID uint64, action string, req FundsAdjustmentRequest, after string) error {
	var existing uint64
	err := tx.QueryRowContext(ctx, `SELECT id FROM audit_events WHERE resource_type=? AND resource_id=? AND outcome='success' ORDER BY id LIMIT 1`, resource, fmt.Sprint(eventID)).Scan(&existing)
	if err == nil {
		return verifyFundsOperationAudit(ctx, tx, resource, eventID, req)
	} else if err != sql.ErrNoRows {
		return err
	}
	value, err := decimal.NewFromString(after)
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(map[string]any{
		"user_id": userID, "token_id": tokenID, "request_id": req.RequestID, "note": strings.TrimSpace(req.Note),
		"amount": req.Amount.String(), "before": value.Sub(req.Amount).String(), "after": value.String(),
	})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(request_id,actor_type,actor_user_id,actor_token_id,action,resource_type,resource_id,outcome,http_status,ip,metadata,created_at) VALUES (?,'user',?,0,?,?,?,'success',200,'',?,?)`,
		req.RequestID, actorID, action, resource, fmt.Sprint(eventID), string(metadata), time.Now().UTC())
	return err
}
