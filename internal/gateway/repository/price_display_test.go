package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
)

func priceDisplayStore(t *testing.T) *Store {
	t.Helper()
	store := modelMetaDB(t)
	for _, statement := range []string{
		`CREATE TABLE gw_price_display_settings(id INTEGER PRIMARY KEY,default_multiplier TEXT,model_multipliers TEXT,sku_multipliers TEXT,config_version INTEGER)`,
		`INSERT INTO gw_price_display_settings VALUES(1,'1.8','{}','{}',1)`,
		`CREATE TABLE gw_models(id INTEGER PRIMARY KEY, model_code TEXT)`,
		`INSERT INTO gw_models VALUES(1,'image-model')`,
		`CREATE TABLE gw_skus(id INTEGER PRIMARY KEY, sku_code TEXT)`,
		`INSERT INTO gw_skus VALUES(1,'image-standard')`,
	} {
		if _, err := store.DB().Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func TestPriceDisplaySaveAuditsBothVersionsAndRejectsStaleWrite(t *testing.T) {
	store := priceDisplayStore(t)
	ctx := context.Background()
	in := PriceDisplaySettings{DefaultMultiplier: "2", ModelMultipliers: map[string]string{"image-model": "1.5"}, SKUMultipliers: map[string]string{"image-standard": "1.2"}, ConfigVersion: 1}
	if err := store.WithTx(ctx, func(tx *sql.Tx) error { return store.SavePriceDisplaySettings(ctx, tx, in, 7) }); err != nil {
		t.Fatal(err)
	}
	got, err := store.ReadPriceDisplaySettings(ctx)
	if err != nil || got.ConfigVersion != 2 || got.SKUMultipliers["image-standard"] != "1.2" {
		t.Fatalf("settings=%v err=%v", got, err)
	}
	var metadata string
	if err := store.DB().QueryRow(`SELECT metadata FROM audit_events WHERE action='unified.price_display.update'`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	var audit struct {
		Before PriceDisplaySettings
		After  PriceDisplaySettings
	}
	if err := json.Unmarshal([]byte(metadata), &audit); err != nil {
		t.Fatal(err)
	}
	if audit.Before.DefaultMultiplier != "1.8" || audit.Before.ConfigVersion != 1 || audit.After.DefaultMultiplier != "2" || audit.After.ConfigVersion != 2 {
		t.Fatalf("audit=%s", metadata)
	}
	err = store.WithTx(ctx, func(tx *sql.Tx) error { return store.SavePriceDisplaySettings(ctx, tx, in, 7) })
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("stale save err=%v", err)
	}
	// Removing overrides restores inheritance and does not retain stale JSON keys.
	got.ModelMultipliers, got.SKUMultipliers = map[string]string{}, map[string]string{}
	if err := store.WithTx(ctx, func(tx *sql.Tx) error { return store.SavePriceDisplaySettings(ctx, tx, got, 7) }); err != nil {
		t.Fatal(err)
	}
	cleared, err := store.ReadPriceDisplaySettings(ctx)
	if err != nil || len(cleared.ModelMultipliers)+len(cleared.SKUMultipliers) != 0 || cleared.ConfigVersion != 3 {
		t.Fatalf("clear=%v err=%v", cleared, err)
	}
}

func TestPriceDisplayRejectedWriteAndAuditFailureLeaveSettingsUnchanged(t *testing.T) {
	for _, failure := range []string{"audit", "unknown-model", "unknown-sku", "actor", "zero-version"} {
		t.Run(failure, func(t *testing.T) {
			store := priceDisplayStore(t)
			in := PriceDisplaySettings{DefaultMultiplier: "2", ConfigVersion: 1}
			actor := uint64(7)
			switch failure {
			case "audit":
				if _, err := store.DB().Exec(`DROP TABLE audit_events`); err != nil {
					t.Fatal(err)
				}
			case "unknown-model":
				in.ModelMultipliers = map[string]string{"missing": "2"}
			case "unknown-sku":
				in.SKUMultipliers = map[string]string{"missing": "2"}
			case "actor":
				actor = 0
			case "zero-version":
				in.ConfigVersion = 0
			}
			err := store.WithTx(context.Background(), func(tx *sql.Tx) error { return store.SavePriceDisplaySettings(context.Background(), tx, in, actor) })
			if err == nil {
				t.Fatal("invalid write succeeded")
			}
			got, err := store.ReadPriceDisplaySettings(context.Background())
			if err != nil || got.DefaultMultiplier != "1.8" || got.ConfigVersion != 1 {
				t.Fatalf("failed write changed settings: %v %v", got, err)
			}
		})
	}
}

func TestPriceDisplayMultiplierValidation(t *testing.T) {
	for _, value := range []string{"", "0", "-1", "NaN", "1e3", " 1.8 ", "1000000", "1.0000000000001"} {
		if err := (PriceDisplaySettings{DefaultMultiplier: value}).Validate(); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("accepted %q", value)
		}
	}
	for _, value := range []string{"1.8", "1", "0.5", "999999.123456789012"} {
		if err := (PriceDisplaySettings{DefaultMultiplier: value}).Validate(); err != nil {
			t.Errorf("rejected %q: %v", value, err)
		}
	}
}
