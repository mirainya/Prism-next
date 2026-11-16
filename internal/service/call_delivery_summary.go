package service

import (
	"time"

	"github.com/mirainya/Prism/internal/gateway/delivery"
	"github.com/mirainya/Prism/internal/model"
	"gorm.io/gorm"
)

type callDeliverySummaryRow struct {
	CallID       string
	DeliveryMode string
	State        string
	ReasonCode   string
	RetryStatus  string
	Available    bool
}

func (row callDeliverySummaryRow) addTo(summary *model.APICallDeliverySummary) {
	summary.Total++
	retrying := row.RetryStatus == "pending" || row.RetryStatus == "dispatching"
	switch {
	case row.State == "delivery_failed" && row.DeliveryMode == "managed_copy" && retrying:
		summary.Processing++
		if delivery.ManagedCopyWaiting(row.ReasonCode) {
			summary.Checking++
		}
	case row.State == "delivery_failed":
		summary.Failed++
	case row.State == "expired":
		summary.Expired++
	case row.DeliveryMode == "reference":
		summary.Reference++
	case row.Available:
		summary.Ready++
	case row.State == "pending":
		summary.Processing++
	default:
		summary.Unavailable++
	}
}

// The IDs come from the already authorized page. One bounded query loads only
// status fields, without signing URLs or reading storage credentials.
func appendCallDeliverySummaries(db *gorm.DB, calls []model.APICall) error {
	if len(calls) == 0 {
		return nil
	}
	ids := make([]string, 0, len(calls))
	summaries := make(map[string]*model.APICallDeliverySummary, len(calls))
	for index := range calls {
		calls[index].DeliverySummary = &model.APICallDeliverySummary{}
		ids = append(ids, calls[index].ID)
		summaries[calls[index].ID] = calls[index].DeliverySummary
	}
	var rows []callDeliverySummaryRow
	if err := callResultDeliveriesQuery(db).Select(`c.public_id AS call_id,d.delivery_mode,d.state,
COALESCE(d.reason_code,'') AS reason_code,
COALESCE((SELECT o.status FROM gw_async_outbox o
 WHERE o.result_delivery_id=d.id AND o.state_version=d.state_version
 AND o.action='reconcile_delivery' ORDER BY o.id DESC LIMIT 1),'') AS retry_status,
CASE WHEN d.delivery_mode='managed_copy' AND d.state='ready' AND ma.state='active'
 AND COALESCE(NULLIF(ma.storage_locator,''),ma.object_key,'')<>''
 AND (ma.retention_until IS NULL OR ma.retention_until>?) THEN 1 ELSE 0 END AS available`, time.Now()).
		Where("c.public_id IN ?", ids).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if summary := summaries[row.CallID]; summary != nil {
			row.addTo(summary)
		}
	}
	return nil
}
