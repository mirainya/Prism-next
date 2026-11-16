package repository

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/mirainya/Prism/internal/gateway/billing"
)

func (s *Store) StoreCallRateSnapshot(ctx context.Context, tx *sql.Tx, reservationID uint64, schedule billing.RateSchedule, requestFacts billing.Facts) error {
	if s == nil || tx == nil || reservationID == 0 || schedule.Validate() != nil {
		return ErrInvalidInput
	}
	body, err := json.Marshal(schedule)
	if err != nil {
		return err
	}
	factsBody, err := json.Marshal(requestFacts)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO billing_call_rate_snapshots(reservation_id,sell_schedule,request_facts,created_at) VALUES (?,?,?,?)`, reservationID, body, factsBody, nowUTC())
	return err
}

func (s *Store) loadCallRateSnapshot(ctx context.Context, tx *sql.Tx, reservationID uint64) (billing.RateSchedule, billing.Facts, bool, error) {
	var body, factsBody []byte
	err := tx.QueryRowContext(ctx, `SELECT sell_schedule,request_facts FROM billing_call_rate_snapshots WHERE reservation_id=?`, reservationID).Scan(&body, &factsBody)
	if err == sql.ErrNoRows {
		return billing.RateSchedule{}, billing.Facts{}, false, nil
	}
	if err != nil {
		return billing.RateSchedule{}, billing.Facts{}, false, err
	}
	var schedule billing.RateSchedule
	if err := json.Unmarshal(body, &schedule); err != nil || len(schedule.Components) == 0 {
		// Accept an early development snapshot that wrapped schedule and facts.
		var wrapped struct {
			Schedule billing.RateSchedule `json:"schedule"`
		}
		if legacyErr := json.Unmarshal(body, &wrapped); legacyErr != nil || len(wrapped.Schedule.Components) == 0 {
			return billing.RateSchedule{}, billing.Facts{}, false, ErrConflict
		}
		schedule = wrapped.Schedule
	}
	var requestFacts billing.Facts
	if len(factsBody) > 0 {
		if err := json.Unmarshal(factsBody, &requestFacts); err != nil {
			return billing.RateSchedule{}, billing.Facts{}, false, ErrConflict
		}
	}
	for index := range schedule.Components {
		component := &schedule.Components[index]
		if component.PricingMode != billing.PricingModeExpression {
			continue
		}
		expression, err := billing.ParseExpression(component.PricingExpr, component.ExpressionDeclared)
		if err != nil {
			return billing.RateSchedule{}, billing.Facts{}, false, ErrConflict
		}
		component.Expr = expression
	}
	if err := schedule.Validate(); err != nil {
		return billing.RateSchedule{}, billing.Facts{}, false, ErrConflict
	}
	return schedule, requestFacts, true, nil
}
