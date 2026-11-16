package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mirainya/Prism/internal/gateway/billing"
)

type BillingAccountSummary struct {
	ID             uint64     `json:"id"`
	UserID         uint64     `json:"user_id"`
	Status         string     `json:"status"`
	Currency       string     `json:"currency"`
	PostedBalance  string     `json:"posted_balance"`
	HeldAmount     string     `json:"held_amount"`
	CreditLimit    string     `json:"credit_limit"`
	Available      string     `json:"available"`
	StateEventID   uint64     `json:"state_event_id"`
	StateReason    string     `json:"state_reason"`
	StateChangedAt *time.Time `json:"state_changed_at"`
}

type BillingSettlementAnomaly struct {
	CallID              uint64    `json:"call_id"`
	PublicCallID        string    `json:"public_call_id"`
	TokenID             uint64    `json:"token_id"`
	AuthorizedAmount    string    `json:"authorized_amount"`
	ActualAmount        string    `json:"actual_amount"`
	AuthorizationExcess string    `json:"authorization_excess"`
	DebtAfter           string    `json:"debt_after"`
	BudgetExcess        string    `json:"budget_excess"`
	CreatedAt           time.Time `json:"created_at"`
}

type BillingBudgetIssue struct {
	TokenID uint64 `json:"token_id"`
	Limit   string `json:"limit"`
	Used    string `json:"used"`
	Held    string `json:"held"`
}

type BillingAccountDetail struct {
	Account             BillingAccountSummary      `json:"account"`
	RecentAnomalies     []BillingSettlementAnomaly `json:"recent_anomalies"`
	BudgetIssues        []BillingBudgetIssue       `json:"budget_issues"`
	CanUnfreeze         bool                       `json:"can_unfreeze"`
	UnfreezeBlockReason string                     `json:"unfreeze_block_reason"`
}

const billingAccountSummarySQL = `SELECT a.id,a.user_id,a.status,a.currency_code,
a.posted_balance,a.held_amount,a.credit_limit,
COALESCE(e.id,0),COALESCE(e.reason_code,''),e.created_at
FROM billing_accounts a
LEFT JOIN billing_account_state_events e ON e.id=(
 SELECT latest.id FROM billing_account_state_events latest
 WHERE latest.billing_account_id=a.id ORDER BY latest.state_version DESC LIMIT 1
)`

func (s *Store) LoadBillingAccountSummaries(ctx context.Context, userIDs []uint64) (map[uint64]BillingAccountSummary, error) {
	result := make(map[uint64]BillingAccountSummary, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	args := make([]any, len(userIDs))
	for i, id := range userIDs {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, billingAccountSummarySQL+` WHERE a.user_id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var account BillingAccountSummary
		var changed sql.NullTime
		if err := rows.Scan(&account.ID, &account.UserID, &account.Status, &account.Currency,
			&account.PostedBalance, &account.HeldAmount, &account.CreditLimit,
			&account.StateEventID, &account.StateReason, &changed); err != nil {
			return nil, err
		}
		posted, err := billing.ParseAmount(account.PostedBalance, 18, false)
		if err != nil {
			return nil, err
		}
		held, err := billing.ParseAmount(account.HeldAmount, 18, true)
		if err != nil {
			return nil, err
		}
		credit, err := billing.ParseAmount(account.CreditLimit, 18, true)
		if err != nil {
			return nil, err
		}
		account.Available = posted.Add(credit).Sub(held).String()
		if changed.Valid {
			at := changed.Time.UTC()
			account.StateChangedAt = &at
		}
		result[account.UserID] = account
	}
	return result, rows.Err()
}

func (s *Store) GetBillingAccountDetail(ctx context.Context, userID uint64) (BillingAccountDetail, error) {
	detail := BillingAccountDetail{RecentAnomalies: []BillingSettlementAnomaly{}, BudgetIssues: []BillingBudgetIssue{}}
	accounts, err := s.LoadBillingAccountSummaries(ctx, []uint64{userID})
	if err != nil {
		return detail, err
	}
	account, found := accounts[userID]
	if !found {
		return detail, ErrNotFound
	}
	detail.Account = account
	var userStatus int
	if err := s.db.QueryRowContext(ctx, `SELECT status FROM users WHERE id=? AND deleted_at IS NULL`, userID).Scan(&userStatus); err != nil {
		return detail, err
	}
	detail.BudgetIssues, err = s.billingBudgetIssues(ctx, nil, userID)
	if err != nil {
		return detail, err
	}
	detail.UnfreezeBlockReason = billingUnfreezeBlock(account, userStatus, detail.BudgetIssues)
	detail.CanUnfreeze = detail.UnfreezeBlockReason == ""
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,c.public_id,c.token_id,
settled.authorized_amount,settled.actual_amount,settled.authorization_excess,
settled.debt_after,settled.budget_excess,settled.created_at
FROM billing_reservations r
JOIN billing_settlements settled ON settled.reservation_id=r.id
JOIN gw_api_calls c ON c.id=r.call_id
WHERE r.billing_account_id=?
 AND (settled.authorization_excess>0 OR settled.budget_excess>0 OR settled.debt_after>0)
ORDER BY r.id DESC LIMIT 10`, account.ID)
	if err != nil {
		return detail, err
	}
	defer rows.Close()
	for rows.Next() {
		var anomaly BillingSettlementAnomaly
		if err := rows.Scan(&anomaly.CallID, &anomaly.PublicCallID, &anomaly.TokenID,
			&anomaly.AuthorizedAmount, &anomaly.ActualAmount, &anomaly.AuthorizationExcess,
			&anomaly.DebtAfter, &anomaly.BudgetExcess, &anomaly.CreatedAt); err != nil {
			return detail, err
		}
		anomaly.CreatedAt = anomaly.CreatedAt.UTC()
		detail.RecentAnomalies = append(detail.RecentAnomalies, anomaly)
	}
	return detail, rows.Err()
}

// Read window identities first, then lock only budget rows in ID order. Joining
// tokens in a locking query would invert the identity -> account -> budget order.
func (s *Store) billingBudgetIssues(ctx context.Context, tx *sql.Tx, userID uint64) ([]BillingBudgetIssue, error) {
	var db DB = s.db
	if tx != nil {
		db = tx
	}
	now := nowUTC()
	rows, err := db.QueryContext(ctx, `SELECT w.id,w.token_id,w.limit_amount,w.used_amount,w.held_amount
FROM token_budget_windows w
JOIN tokens t ON t.id=w.token_id
JOIN token_budget_policy_activations a ON a.id=w.activation_id AND a.token_id=w.token_id
WHERE t.user_id=? AND t.status=1 AND t.deleted_at IS NULL AND t.revoked_at IS NULL
 AND a.effective_at<=? AND w.window_start<=? AND (w.window_end IS NULL OR w.window_end>?)
 AND w.limit_amount IS NOT NULL
 AND NOT EXISTS (SELECT 1 FROM token_budget_policy_activations newer
  WHERE newer.token_id=a.token_id AND newer.activation_seq>a.activation_seq AND newer.effective_at<=?)
ORDER BY w.id`, userID, now, now, now, now)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		id uint64
		BillingBudgetIssue
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.TokenID, &item.Limit, &item.Used, &item.Held); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	issues := []BillingBudgetIssue{}
	for _, item := range candidates {
		if tx != nil {
			window, err := s.lockBillingWindow(ctx, tx, item.id)
			if err != nil {
				return nil, err
			}
			if window.tokenID != item.TokenID || window.limit == nil {
				return nil, ErrConflict
			}
			item.Limit, item.Used, item.Held = window.limit.String(), window.used.String(), window.held.String()
		}
		limit, err := billing.ParseAmount(item.Limit, 18, true)
		if err != nil {
			return nil, err
		}
		used, err := billing.ParseAmount(item.Used, 18, true)
		if err != nil {
			return nil, err
		}
		held, err := billing.ParseAmount(item.Held, 18, true)
		if err != nil {
			return nil, err
		}
		if used.Add(held).Cmp(limit) > 0 {
			issues = append(issues, item.BillingBudgetIssue)
		}
	}
	return issues, nil
}

func billingUnfreezeBlock(account BillingAccountSummary, userStatus int, issues []BillingBudgetIssue) string {
	switch {
	case account.Status != "frozen":
		return "账户当前未冻结，或已关闭"
	case userStatus != 1:
		return "用户已停用，不能恢复计费账户"
	case account.StateEventID == 0 || account.StateReason != "settlement_overrun":
		return "冻结原因需要人工核查，暂不支持从此处解冻"
	}
	available, err := billing.ParseAmount(account.Available, 18, false)
	if err != nil || available.Sign() < 0 {
		return "可用资金不足，请先补充余额或调整授信"
	}
	if len(issues) != 0 {
		return "仍有 Key 的当前预算超限，请先处理预算"
	}
	return ""
}

type BillingUnfreezeInput struct {
	ExpectedFreezeEventID uint64 `json:"expected_freeze_event_id"`
	Note                  string `json:"note"`
}

func (in BillingUnfreezeInput) Validate() error {
	note := strings.TrimSpace(in.Note)
	if in.ExpectedFreezeEventID == 0 || note == "" || !utf8.ValidString(note) || utf8.RuneCountInString(note) > 500 || strings.ContainsRune(note, '\x00') {
		return ErrInvalidInput
	}
	return nil
}

type BillingUnfreezeBlockedError struct{ Reason string }

func (e *BillingUnfreezeBlockedError) Error() string { return e.Reason }
func (e *BillingUnfreezeBlockedError) Unwrap() error { return ErrConflict }

func (s *Store) UnfreezeBillingAccount(ctx context.Context, tx *sql.Tx, userID uint64, in BillingUnfreezeInput, actorID uint64) error {
	in.Note = strings.TrimSpace(in.Note)
	if tx == nil || userID == 0 || actorID == 0 {
		return ErrInvalidInput
	}
	if err := in.Validate(); err != nil {
		return err
	}
	var userStatus int
	if err := tx.QueryRowContext(ctx, s.forShare(`SELECT status FROM users WHERE id=? AND deleted_at IS NULL`), userID).Scan(&userStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	var accountID uint64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM billing_accounts WHERE user_id=?`, userID).Scan(&accountID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	account, err := s.lockBillingAccount(ctx, tx, accountID)
	if err != nil {
		return err
	}
	var eventID uint64
	var reason string
	if err := tx.QueryRowContext(ctx, `SELECT id,reason_code FROM billing_account_state_events WHERE billing_account_id=? ORDER BY state_version DESC LIMIT 1`, accountID).Scan(&eventID, &reason); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConflict
		}
		return err
	}
	// Account versions also advance when existing calls settle while frozen.
	// The freeze event identifies the operator's reviewed incident instead.
	if eventID != in.ExpectedFreezeEventID {
		return ErrConflict
	}
	summary := BillingAccountSummary{Status: account.status, StateEventID: eventID, StateReason: reason, Available: account.available().String()}
	if blocked := billingUnfreezeBlock(summary, userStatus, nil); blocked != "" {
		return &BillingUnfreezeBlockedError{Reason: blocked}
	}
	issues, err := s.billingBudgetIssues(ctx, tx, userID)
	if err != nil {
		return err
	}
	if blocked := billingUnfreezeBlock(summary, userStatus, issues); blocked != "" {
		return &BillingUnfreezeBlockedError{Reason: blocked}
	}
	now := nowUTC()
	if err := requireOneRow(tx.ExecContext(ctx, `UPDATE billing_accounts SET status='open',state_version=state_version+1,updated_at=? WHERE id=? AND status='frozen' AND state_version=?`, now, accountID, account.version)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO billing_account_state_events(billing_account_id,state_version,old_state,new_state,reason_code,created_at) VALUES (?,?,'frozen','open','manual_overrun_recovery',?)`, accountID, account.version+1, now); err != nil {
		return err
	}
	return recordCatalogAdminChange(ctx, tx, actorID, "billing.account.unfreeze", "billing_account", accountID, map[string]any{
		"user_id": userID, "freeze_event_id": eventID, "state_version": account.version + 1,
		"old_state": "frozen", "new_state": "open", "note": in.Note,
	})
}
