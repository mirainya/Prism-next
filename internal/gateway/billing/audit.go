package billing

import "encoding/json"

type ChargeAudit struct {
	Currency Currency    `json:"currency"`
	Amount   string      `json:"amount"`
	Lines    []ChargeLine `json:"lines"`
	Facts    Facts        `json:"facts"`
}

func MarshalChargeAudit(schedule RateSchedule, charge Charge, facts Facts) ([]byte, error) {
	return json.Marshal(ChargeAudit{
		Currency: schedule.Currency,
		Amount:   charge.Amount.String(),
		Lines:    charge.Lines,
		Facts:    facts,
	})
}
