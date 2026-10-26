package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/shopspring/decimal"
)

// PriceDisplaySettings belongs to presentation, not to a billing schedule.
// Model keys use stable gw_models.model_code; SKU keys use gw_skus.sku_code.
type PriceDisplaySettings struct {
	DefaultMultiplier string            `json:"default_multiplier"`
	ModelMultipliers  map[string]string `json:"model_multipliers"`
	SKUMultipliers    map[string]string `json:"sku_multipliers"`
	ConfigVersion     uint64            `json:"config_version"`
}

var displayMultiplierPattern = regexp.MustCompile(`^[0-9]{1,6}(\.[0-9]{1,12})?$`)

func (in PriceDisplaySettings) Validate() error {
	valid := func(value string) bool {
		amount, err := decimal.NewFromString(value)
		return displayMultiplierPattern.MatchString(value) && err == nil && amount.IsPositive()
	}
	if !valid(in.DefaultMultiplier) || len(in.ModelMultipliers)+len(in.SKUMultipliers) > 500 {
		return ErrInvalidInput
	}
	for _, overrides := range []map[string]string{in.ModelMultipliers, in.SKUMultipliers} {
		for code, value := range overrides {
			if len(code) > 160 || !catalogIdentityPattern.MatchString(code) || !valid(value) {
				return ErrInvalidInput
			}
		}
	}
	return nil
}

func (s *Store) ReadPriceDisplaySettings(ctx context.Context) (PriceDisplaySettings, error) {
	return readPriceDisplaySettings(ctx, s.db, "")
}

func readPriceDisplaySettings(ctx context.Context, q rowQuerier, suffix string) (PriceDisplaySettings, error) {
	var settings PriceDisplaySettings
	var models, skus []byte
	err := q.QueryRowContext(ctx, `SELECT default_multiplier,model_multipliers,sku_multipliers,config_version FROM gw_price_display_settings WHERE id=1`+suffix).
		Scan(&settings.DefaultMultiplier, &models, &skus, &settings.ConfigVersion)
	if err != nil {
		return settings, err
	}
	if err := json.Unmarshal(models, &settings.ModelMultipliers); err != nil {
		return settings, err
	}
	if err := json.Unmarshal(skus, &settings.SKUMultipliers); err != nil {
		return settings, err
	}
	if settings.ModelMultipliers == nil {
		settings.ModelMultipliers = map[string]string{}
	}
	if settings.SKUMultipliers == nil {
		settings.SKUMultipliers = map[string]string{}
	}
	return settings, settings.Validate()
}

// ValidatePriceDisplayTargets rejects misspelled identities before saving or previewing.
func (s *Store) ValidatePriceDisplayTargets(ctx context.Context, in PriceDisplaySettings) error {
	return validatePriceDisplayTargets(ctx, s.db, in)
}

func validatePriceDisplayTargets(ctx context.Context, q rowQuerier, in PriceDisplaySettings) error {
	if err := in.Validate(); err != nil {
		return err
	}
	for code := range in.ModelMultipliers {
		var id uint64
		if err := q.QueryRowContext(ctx, `SELECT id FROM gw_models WHERE model_code=?`, code).Scan(&id); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("%w: unknown model", ErrInvalidInput)
			}
			return err
		}
	}
	for code := range in.SKUMultipliers {
		var id uint64
		if err := q.QueryRowContext(ctx, `SELECT id FROM gw_skus WHERE sku_code=? LIMIT 1`, code).Scan(&id); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("%w: unknown SKU", ErrInvalidInput)
			}
			return err
		}
	}
	return nil
}

func (s *Store) SavePriceDisplaySettings(ctx context.Context, tx *sql.Tx, in PriceDisplaySettings, actor uint64) error {
	if tx == nil || actor == 0 || in.ConfigVersion == 0 {
		return ErrInvalidInput
	}
	if err := validatePriceDisplayTargets(ctx, tx, in); err != nil {
		return err
	}
	before, err := readPriceDisplaySettings(ctx, tx, s.forUpdate(""))
	if err != nil {
		return err
	}
	if before.ConfigVersion != in.ConfigVersion {
		return ErrConflict
	}
	models, _ := json.Marshal(in.ModelMultipliers)
	skus, _ := json.Marshal(in.SKUMultipliers)
	result, err := tx.ExecContext(ctx, `UPDATE gw_price_display_settings SET default_multiplier=?,model_multipliers=?,sku_multipliers=?,config_version=config_version+1 WHERE id=1 AND config_version=?`,
		in.DefaultMultiplier, string(models), string(skus), in.ConfigVersion)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrConflict
	}
	after := in
	after.ConfigVersion++
	return recordCatalogAdminChange(ctx, tx, actor, "unified.price_display.update", "price_display_settings", 1, ginSafeMetadata{"before": before, "after": after})
}
