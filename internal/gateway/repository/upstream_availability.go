package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mirainya/Prism/internal/gateway/billing"
)

const (
	// AvailabilityWindowMinutes matches the provider's own sliding window so the
	// locally measured figure and the upstream one mean the same thing and may
	// be shown in the same column.
	AvailabilityWindowMinutes = 120
	// availabilityMinSamples is the point below which a locally measured rate is
	// not worth showing: three calls of which one failed is not "67% reliable".
	// Below it Prism falls back to the provider's own figure, which is drawn
	// from every tenant's traffic rather than just this deployment's.
	availabilityMinSamples = 20
)

// AvailabilitySourceLocal and AvailabilitySourceUpstream name where a displayed
// rate came from. Users are told which, because the two differ in kind: one is
// what Prism itself measured, the other is what the provider claims.
const (
	AvailabilitySourceLocal    = "local"
	AvailabilitySourceUpstream = "upstream"
)

// SKUAvailability combines rate measurements with independent provider channel
// observations. A record may contain only channel status or generation pace.
type SKUAvailability struct {
	SKUID                    uint64
	SuccessRate              string
	Source                   string
	Samples                  uint32
	WindowMinutes            uint32
	ObservedAt               time.Time
	ChannelStatus            string
	AverageCompletionSeconds string
	Buckets                  json.RawMessage
	EstimatedCompletionMS    *uint64
	ChannelObservedAt        time.Time
}

// AvailabilitySource is an independent monitor configuration. Credentials use the
// availability_read purpose and share only the normal credential rotation.
type AvailabilitySource struct {
	SourceID           uint64
	ConfigVersion      uint64
	ChannelID          uint64
	ContractCode       string
	SourceCode         string
	ProtocolDefinition []byte
	CredentialID       uint64
	CredentialBlobID   uint64
	CredentialSecret   []byte
	BaseURL            string
	RequestTimeoutMS   uint32
}

// UpstreamAvailabilityRow is one stored provider observation. SuccessRate may
// be empty when the provider reports only status or generation pace.
type UpstreamAvailabilityRow struct {
	ModelCode                string
	Category                 string
	SuccessRate              string
	AverageCompletionSeconds string
	Buckets                  json.RawMessage
	ChannelStatus            string
	EstimatedCompletionMS    *uint64
	WindowMinutes            uint32
	ObservedAt               time.Time
}

// ListAvailabilitySources returns independent active monitor configurations.
// The upstream credential must be active and retain its availability_read grant.
func (s *Store) ListAvailabilitySources(ctx context.Context) ([]AvailabilitySource, error) {
	if s == nil {
		return nil, ErrInvalidInput
	}
	rows, err := s.DB().QueryContext(ctx, `SELECT src.id,src.config_version,src.channel_id,src.source_code,src.contract_code,pd.definition_json,c.id,c.secret,COALESCE(cv.encrypted_blob_id,0),src.base_url,src.request_timeout_ms
FROM gw_availability_sources src
LEFT JOIN gw_protocol_definitions pd ON pd.id=src.protocol_definition_id AND pd.status='active'
JOIN gateway_channels ch ON ch.id=src.channel_id AND ch.status='active'
JOIN gw_credentials c ON c.id=src.credential_id AND c.channel_id=src.channel_id AND c.status='active'
JOIN gw_credential_versions cv ON cv.id=c.current_version_id AND cv.credential_id=c.id AND cv.status='active'
JOIN gw_credential_purpose_grants g ON g.id=src.credential_purpose_grant_id AND g.credential_id=c.id AND g.purpose='availability_read' AND g.status='active'
WHERE src.status='active' AND (src.protocol_definition_id IS NULL OR pd.id IS NOT NULL) ORDER BY src.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sources := make([]AvailabilitySource, 0, 4)
	for rows.Next() {
		var source AvailabilitySource
		if err := rows.Scan(&source.SourceID, &source.ConfigVersion, &source.ChannelID, &source.SourceCode, &source.ContractCode, &source.ProtocolDefinition,
			&source.CredentialID, &source.CredentialSecret, &source.CredentialBlobID,
			&source.BaseURL, &source.RequestTimeoutMS); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

// SaveUpstreamAvailability replaces the stored observations for one source.
// Rows are updated in place rather than appended: this is a cache of the
// provider's current view, not an audit trail, and nothing may reconstruct a
// past charge from it.
//
// Models the provider stops reporting keep their last row. Staleness is decided
// by the reader against observed_at, so a provider outage degrades to "last
// seen at T" instead of silently deleting everything a user was relying on.
func (s *Store) SaveUpstreamAvailability(ctx context.Context, sourceID uint64, items []UpstreamAvailabilityRow) error {
	if s == nil || sourceID == 0 || len(items) > 10000 {
		return ErrInvalidInput
	}
	if len(items) == 0 {
		return nil
	}
	now := nowUTC()
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		for _, item := range items {
			if !validUpstreamAvailabilityRow(item) {
				return ErrInvalidInput
			}
			completion := item.AverageCompletionSeconds
			if completion == "" {
				completion = "0"
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO gw_upstream_availability
(availability_source_id,model_code,category,success_rate,average_completion_seconds,buckets,channel_status,estimated_completion_ms,window_minutes,observed_at,updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE category=VALUES(category),success_rate=VALUES(success_rate),
average_completion_seconds=VALUES(average_completion_seconds),buckets=VALUES(buckets),channel_status=VALUES(channel_status),
estimated_completion_ms=VALUES(estimated_completion_ms),window_minutes=VALUES(window_minutes),
observed_at=VALUES(observed_at),updated_at=VALUES(updated_at)`,
				sourceID, item.ModelCode, item.Category, nullableAvailabilityRate(item.SuccessRate), completion, nullableBuckets(item.Buckets),
				nullableAvailabilityText(item.ChannelStatus), nullableAvailabilityEstimate(item.EstimatedCompletionMS), item.WindowMinutes, item.ObservedAt.UTC(), now); err != nil {
				return err
			}
		}
		return nil
	})
}

// ReplaceUpstreamAvailability stores one complete provider observation. An
// empty set clears the source's cached observations instead of serving stale data.
func (s *Store) ReplaceUpstreamAvailability(ctx context.Context, sourceID, expectedVersion uint64, items []UpstreamAvailabilityRow) error {
	if s == nil || sourceID == 0 || expectedVersion == 0 || len(items) > 10000 {
		return ErrInvalidInput
	}
	for _, item := range items {
		if !validUpstreamAvailabilityRow(item) {
			return ErrInvalidInput
		}
	}
	now := nowUTC()
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		// An edit or disable may finish while the upstream request is in flight.
		// Lock the source so obsolete results cannot repopulate its cache.
		var status string
		var version uint64
		if err := tx.QueryRowContext(ctx, `SELECT status,config_version FROM gw_availability_sources WHERE id=? FOR SHARE`, sourceID).Scan(&status, &version); err != nil {
			return err
		}
		if status != "active" || version != expectedVersion {
			return ErrConflict
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM gw_upstream_availability WHERE availability_source_id=?`, sourceID); err != nil {
			return err
		}
		for _, item := range items {
			completion := item.AverageCompletionSeconds
			if completion == "" {
				completion = "0"
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO gw_upstream_availability
(availability_source_id,model_code,category,success_rate,average_completion_seconds,buckets,channel_status,estimated_completion_ms,window_minutes,observed_at,updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)`, sourceID, item.ModelCode, item.Category, nullableAvailabilityRate(item.SuccessRate), completion, nullableBuckets(item.Buckets),
				nullableAvailabilityText(item.ChannelStatus), nullableAvailabilityEstimate(item.EstimatedCompletionMS), item.WindowMinutes, item.ObservedAt.UTC(), now); err != nil {
				return err
			}
		}
		return nil
	})
}

func validUpstreamAvailabilityRow(item UpstreamAvailabilityRow) bool {
	if len(item.Buckets) > 0 && (!json.Valid(item.Buckets) || len(item.Buckets) > 131072) {
		return false
	}
	if !validCatalogSourceText(item.ModelCode, 128) || !validOptionalCatalogSourceText(item.Category, 32) || item.ObservedAt.IsZero() ||
		!validOptionalCatalogSourceText(item.ChannelStatus, 64) || item.SuccessRate == "" && item.ChannelStatus == "" && item.EstimatedCompletionMS == nil && len(item.Buckets) == 0 ||
		item.EstimatedCompletionMS != nil && *item.EstimatedCompletionMS > 86400000 {
		return false
	}
	if item.SuccessRate == "" {
		return item.WindowMinutes == 0
	}
	if item.WindowMinutes == 0 {
		return false
	}
	rate, err := billing.ParseAmount(item.SuccessRate, 18, true)
	if err != nil {
		return false
	}
	maximum, _ := billing.ParseAmount("100", 18, true)
	return rate.Cmp(maximum) <= 0
}

func nullableAvailabilityRate(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableAvailabilityText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableAvailabilityEstimate(value *uint64) any {
	if value == nil {
		return nil
	}
	return int64(*value)
}

// LoadSKUAvailability combines success-rate and provider-status observations
// for each SKU. Local measurements replace only the rate fields; provider
// status and generation pace remain available alongside them.
func (s *Store) LoadSKUAvailability(ctx context.Context, releaseID uint64) (map[uint64]SKUAvailability, error) {
	if s == nil || releaseID == 0 {
		return nil, ErrInvalidInput
	}
	local, err := s.loadLocalAvailability(ctx, releaseID)
	if err != nil {
		return nil, err
	}
	upstream, err := s.loadUpstreamAvailability(ctx, releaseID)
	if err != nil {
		return nil, err
	}
	result := make(map[uint64]SKUAvailability, len(local)+len(upstream))
	for skuID, item := range upstream {
		result[skuID] = item
	}
	for skuID, item := range local {
		if upstreamItem, exists := result[skuID]; exists {
			item.ChannelStatus = upstreamItem.ChannelStatus
			item.AverageCompletionSeconds = upstreamItem.AverageCompletionSeconds
			item.Buckets = upstreamItem.Buckets
			item.EstimatedCompletionMS = upstreamItem.EstimatedCompletionMS
			item.ChannelObservedAt = upstreamItem.ChannelObservedAt
		}
		result[skuID] = item
	}
	return result, nil
}

func (s *Store) loadLocalAvailability(ctx context.Context, releaseID uint64) (map[uint64]SKUAvailability, error) {
	// The window boundary is computed here rather than with UTC_TIMESTAMP(3) in
	// SQL. gw_api_calls.created_at is written from Go (see CreateCall), so the
	// driver serialises it through cfg.Loc; a boundary taken from the database
	// clock would be offset by the connection's zone and silently widen the
	// window to the whole history. Passing a time.Time makes both sides travel
	// through the same conversion.
	since := nowUTC().Add(-AvailabilityWindowMinutes * time.Minute)
	// Only business terminal states count. A cancelled call was the caller's own
	// decision and an indeterminate one is exactly the case where Prism does not
	// know whether it succeeded, so neither may be scored against the model.
	rows, err := s.DB().QueryContext(ctx, `SELECT sku_id,
	SUM(status='completed'),SUM(status IN ('completed','failed')),MAX(created_at)
FROM gw_api_calls
WHERE catalog_release_id=? AND created_at>=?
GROUP BY sku_id`, releaseID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make(map[uint64]SKUAvailability, 16)
	for rows.Next() {
		var skuID uint64
		var completed, total int64
		var rawObserved any
		if err := rows.Scan(&skuID, &completed, &total, &rawObserved); err != nil {
			return nil, err
		}
		if total < availabilityMinSamples || completed > total {
			continue
		}
		observed, err := availabilityTime(rawObserved)
		if err != nil {
			return nil, err
		}
		items[skuID] = SKUAvailability{
			SKUID: skuID, SuccessRate: percentOf(completed, total),
			Source: AvailabilitySourceLocal, Samples: uint32(total),
			WindowMinutes: AvailabilityWindowMinutes, ObservedAt: observed.UTC(),
		}
	}
	return items, rows.Err()
}

// loadUpstreamAvailability maps the provider's per-model figures onto SKUs
// through the vendor model each route actually calls. Where several routes back
// one SKU the lowest rate and the oldest observation win: a rate shown next to a
// price should not read better than the worst path a request may take.
func (s *Store) loadUpstreamAvailability(ctx context.Context, releaseID uint64) (map[uint64]SKUAvailability, error) {
	rows, err := s.DB().QueryContext(ctx, `SELECT route.sku_id,
	avail.success_rate,avail.average_completion_seconds,avail.buckets,avail.window_minutes,avail.channel_status,avail.estimated_completion_ms,avail.observed_at
FROM gw_routes route
JOIN gw_offerings offering ON offering.release_id=route.release_id AND offering.id=route.offering_id
JOIN gw_product_transports pt ON pt.release_id=offering.release_id AND pt.id=offering.product_transport_id
JOIN gw_products p ON p.release_id=pt.release_id AND p.id=pt.product_id
JOIN gw_upstream_availability avail ON LOWER(TRIM(avail.model_code))=LOWER(TRIM(p.vendor_model))
JOIN gw_availability_sources source ON source.id=avail.availability_source_id AND source.channel_id=p.channel_id AND source.status='active'
LEFT JOIN gw_protocol_definitions pd ON pd.id=source.protocol_definition_id AND pd.status='active'
WHERE route.release_id=? AND (source.protocol_definition_id IS NULL OR pd.id IS NOT NULL)`, releaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make(map[uint64]SKUAvailability, 16)
	for rows.Next() {
		var skuID uint64
		var successRate, completion, status sql.NullString
		var buckets []byte
		var window uint32
		var estimated sql.NullInt64
		var rawObserved any
		if err := rows.Scan(&skuID, &successRate, &completion, &buckets, &window, &status, &estimated, &rawObserved); err != nil {
			return nil, err
		}
		observed, err := availabilityTime(rawObserved)
		if err != nil {
			return nil, err
		}
		item := items[skuID]
		item.SKUID = skuID
		if completion.Valid && completion.String != "0" && completion.String != "0.0" {
			if item.AverageCompletionSeconds == "" || lessAvailabilityRate(item.AverageCompletionSeconds, completion.String) {
				item.AverageCompletionSeconds = completion.String
			}
			item.ChannelObservedAt = oldestAvailabilityTime(item.ChannelObservedAt, observed)
		}
		if len(buckets) > 0 {
			if len(item.Buckets) == 0 {
				item.Buckets = json.RawMessage(buckets)
			}
			item.ChannelObservedAt = oldestAvailabilityTime(item.ChannelObservedAt, observed)
		}
		if successRate.Valid && (item.SuccessRate == "" || lessAvailabilityRate(successRate.String, item.SuccessRate)) {
			item.SuccessRate = successRate.String
			item.Source = AvailabilitySourceUpstream
			item.WindowMinutes = window
			item.ObservedAt = observed
		} else if successRate.Valid && observed.Before(item.ObservedAt) {
			item.ObservedAt = observed
		}
		if status.Valid {
			item.ChannelStatus = worseStoredChannelStatus(item.ChannelStatus, status.String)
			item.ChannelObservedAt = oldestAvailabilityTime(item.ChannelObservedAt, observed)
		}
		if estimated.Valid && estimated.Int64 >= 0 {
			value := uint64(estimated.Int64)
			if item.EstimatedCompletionMS == nil || value > *item.EstimatedCompletionMS {
				item.EstimatedCompletionMS = &value
			}
			item.ChannelObservedAt = oldestAvailabilityTime(item.ChannelObservedAt, observed)
		}
		if item.ObservedAt.IsZero() {
			item.ObservedAt = observed
		}
		items[skuID] = item
	}
	return items, rows.Err()
}

func lessAvailabilityRate(left, right string) bool {
	leftAmount, leftErr := billing.ParseAmount(left, 18, true)
	rightAmount, rightErr := billing.ParseAmount(right, 18, true)
	return leftErr == nil && rightErr == nil && leftAmount.Cmp(rightAmount) < 0
}

func worseStoredChannelStatus(left, right string) string {
	if right == "" {
		return left
	}
	if left == "" || storedChannelStatusRank(right) > storedChannelStatusRank(left) {
		return right
	}
	return left
}

func storedChannelStatusRank(value string) int {
	status := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(status, "不可用"), strings.Contains(status, "异常"), strings.Contains(status, "unavailable"), strings.Contains(status, "failed"), strings.Contains(status, "error"), status == "down":
		return 3
	case strings.Contains(status, "波动"), strings.Contains(status, "不稳定"), strings.Contains(status, "degrad"), strings.Contains(status, "fluctuat"), strings.Contains(status, "unstable"):
		return 2
	default:
		return 1
	}
}

func oldestAvailabilityTime(current, next time.Time) time.Time {
	if current.IsZero() || next.Before(current) {
		return next
	}
	return current
}

func availabilityTime(value any) (time.Time, error) {
	switch typed := value.(type) {
	case time.Time:
		return typed.UTC(), nil
	case string:
		return parseAvailabilityTime(typed)
	case []byte:
		return parseAvailabilityTime(string(typed))
	default:
		return time.Time{}, fmt.Errorf("invalid availability observation time %T", value)
	}
}

func parseAvailabilityTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid availability observation time")
}

// percentOf renders a ratio on a 0..100 scale with two decimals using integer
// arithmetic only. Floating point is not permitted on this path, and a rate
// shown beside a price must be reproducible exactly.
func percentOf(part, whole int64) string {
	if whole <= 0 {
		return "0.00"
	}
	scaled := part * 10000 / whole
	return fmt.Sprintf("%d.%02d", scaled/100, scaled%100)
}

func nullableBuckets(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}
