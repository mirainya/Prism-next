package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mirainya/Prism/internal/model"
	"github.com/shopspring/decimal"
)

type ListFundsEventsRequest struct {
	ObservabilityListFilter
	AccountType string `form:"account_type"`
	Direction   string `form:"direction"`
	Category    string `form:"category"`
	CallID      string `form:"call_id"`
}

type FundsEvent struct {
	ID          uint64    `json:"id"`
	AccountType string    `json:"account_type"`
	AccountID   uint64    `json:"account_id"`
	UserID      uint      `json:"user_id"`
	Username    string    `json:"username"`
	TokenID     uint      `json:"token_id"`
	TokenName   string    `json:"token_name"`
	Direction   string    `json:"direction"`
	Category    string    `json:"category"`
	Amount      string    `json:"amount"`
	Currency    string    `json:"currency"`
	CallID      string    `json:"call_id"`
	CreatedAt   time.Time `json:"created_at"`
	ActorUserID uint      `json:"actor_user_id" gorm:"-"`
	RequestID   string    `json:"request_id" gorm:"-"`
	Note        string    `json:"note" gorm:"-"`
	Before      *string   `json:"before" gorm:"-"`
	After       *string   `json:"after" gorm:"-"`
}

func (s *ObservabilityService) ListFundsEvents(req *ListFundsEventsRequest, scope ObservabilityScope) (*ObservabilityListResponse[FundsEvent], error) {
	if req == nil {
		return nil, ErrObservabilityInvalidInput
	}
	kind := strings.TrimSpace(req.AccountType)
	if kind == "" {
		kind = "user"
	}
	if kind != "user" && kind != "token" {
		return nil, ErrObservabilityInvalidInput
	}
	db := model.DB()
	base := db.Table("billing_events AS e").
		Select(`e.id, 'user' AS account_type, a.id AS account_id, a.user_id, COALESCE(u.username,'') AS username,
COALESCE(c.token_id,0) AS token_id, COALESCE(t.name,'') AS token_name,
CASE e.event_type WHEN 'reservation_created' THEN 'hold' WHEN 'reservation_released' THEN 'release'
ELSE COALESCE(NULLIF(p.user_direction,''),'info') END AS direction,
e.event_type AS category, e.amount, e.currency_code AS currency, COALESCE(c.public_id,'') AS call_id, e.created_at`).
		Joins("JOIN billing_accounts a ON a.id=e.billing_account_id").
		Joins("LEFT JOIN users u ON u.id=a.user_id").
		Joins("LEFT JOIN gw_api_calls c ON c.id=e.call_id").
		Joins("LEFT JOIN tokens t ON t.id=c.token_id AND t.user_id=a.user_id").
		Joins("LEFT JOIN billing_posting_rules p ON p.id=e.posting_rule_id")
	resource := "billing_event"
	if kind == "token" {
		base = db.Table("token_budget_adjustment_events AS e").
			Select(`e.id, 'token' AS account_type, w.token_id AS account_id, t.user_id, COALESCE(u.username,'') AS username,
w.token_id AS token_id, t.name AS token_name, CASE WHEN e.amount_delta>=0 THEN 'credit' ELSE 'debit' END AS direction,
'budget_adjustment' AS category, e.amount_delta AS amount, a.currency_code AS currency, '' AS call_id, e.created_at`).
			Joins("JOIN token_budget_windows w ON w.id=e.budget_window_id").
			Joins("JOIN tokens t ON t.id=w.token_id").
			Joins("JOIN billing_accounts a ON a.user_id=t.user_id").
			Joins("LEFT JOIN users u ON u.id=t.user_id")
		resource = "budget_adjustment"
	}
	query, page, size, err := prepareObservabilityQuery(db.Table("(?) AS funds", base), req.ObservabilityListFilter, scope, "user_id", "token_id")
	if err != nil {
		return nil, err
	}
	if value := strings.TrimSpace(req.Direction); value != "" {
		if value != "credit" && value != "debit" && value != "hold" && value != "release" && value != "info" {
			return nil, ErrObservabilityInvalidInput
		}
		query = query.Where("direction = ?", value)
	}
	if value := strings.TrimSpace(req.Category); value != "" {
		query = query.Where("category = ?", value)
	}
	if value := strings.TrimSpace(req.CallID); value != "" {
		query = query.Where("call_id = ?", value)
	}
	items := make([]FundsEvent, 0)
	total, snapshot, err := findObservabilityPage(query, page, size, req.SnapshotID, &items)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = strconv.FormatUint(items[i].ID, 10)
		amount, err := decimal.NewFromString(items[i].Amount)
		if err != nil {
			return nil, fmt.Errorf("invalid stored funds event amount: %w", err)
		}
		items[i].Amount = amount.Abs().String()
	}
	if len(ids) > 0 {
		var audits []model.AuditEvent
		if err := db.Select("resource_id", "actor_user_id", "metadata").Where("resource_type=? AND resource_id IN ? AND outcome='success'", resource, ids).Order("id DESC").Find(&audits).Error; err != nil {
			return nil, err
		}
		for _, audit := range audits {
			var metadata struct {
				RequestID string  `json:"request_id"`
				Note      string  `json:"note"`
				Before    *string `json:"before"`
				After     *string `json:"after"`
			}
			if json.Unmarshal(audit.Metadata, &metadata) != nil {
				continue
			}
			for i := range items {
				if ids[i] == audit.ResourceID {
					items[i].ActorUserID, items[i].RequestID, items[i].Note = audit.ActorUserID, metadata.RequestID, metadata.Note
					items[i].Before, items[i].After = metadata.Before, metadata.After
				}
			}
		}
	}
	return &ObservabilityListResponse[FundsEvent]{Items: items, Total: total, Page: page, PageSize: size, SnapshotID: snapshot}, nil
}
