package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestModelSummariesDoNotDependOnBillingOrObservations(t *testing.T) {
	db := setupPublicCatalogQueryDB(t)
	for _, statement := range []string{
		`DROP TABLE gw_sell_rates`,
		`DROP TABLE gw_upstream_availability`,
		`DROP TABLE gw_api_calls`,
		`UPDATE gw_products SET capability_constraints='{"llm":{"context_window":128000,"supports_tools":true}}'`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	items, err := newQueryServiceWithDB(db).ListModelSummaries(context.Background())
	if err != nil || len(items) != 1 {
		t.Fatalf("summaries=%#v err=%v", items, err)
	}
	if items[0].Pricing != nil || items[0].Availability != nil || items[0].LLMOptions != nil || items[0].VideoOptions != nil {
		t.Fatalf("summary contains detailed data: %#v", items[0])
	}
}

func TestModelDetailPricesAllModelTypesAndOperations(t *testing.T) {
	for _, modelType := range []string{"chat", "image", "video"} {
		t.Run(modelType, func(t *testing.T) {
			db := setupPublicCatalogQueryDB(t)
			if modelType != "chat" {
				operation, path := "videos.generate", "/v1/videos/generations"
				if modelType == "image" {
					operation, path = "images.generate", "/v1/images/generations"
				}
				if _, err := db.Exec(`UPDATE gw_operation_contracts SET operation_code=? WHERE id=100`, operation); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`UPDATE gw_operation_routes SET route_template=? WHERE operation_contract_id=100`, path); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`UPDATE gw_sku_downstream_paths SET path=? WHERE sku_id=30`, path); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec(`DROP TABLE gw_upstream_availability`); err != nil {
				t.Fatal(err)
			}
			item, err := newQueryServiceWithDB(db).GetModelDetail(context.Background(), "gpt-4.1")
			if err != nil || item == nil || item.Pricing == nil {
				t.Fatalf("detail=%#v err=%v", item, err)
			}
			if item.Availability != nil || len(item.Pricing.Variants) != 2 {
				t.Fatalf("unexpected detail projection: %#v", item)
			}
			variants := make(map[string]AvailableModelPricingVariant)
			for _, variant := range item.Pricing.Variants {
				variants[variant.Operation] = variant
			}
			operation := "chat.completions"
			if modelType == "image" {
				operation = "images.generate"
			} else if modelType == "video" {
				operation = "videos.generate"
			}
			if len(variants[operation].Components) != 2 || variants[operation].Components[0].UnitPrice != "0.0000054" ||
				variants["responses.create"].Components[0].UnitPrice != "0.036" {
				t.Fatalf("incorrect prices or duplicate routes: %#v", item.Pricing)
			}
			if modelType != "video" && len(variants[operation].Constraints) != 0 {
				t.Fatal("non-video price advertises video constraints")
			}
		})
	}
}

func TestModelDetailMissingAndInvalidPrices(t *testing.T) {
	db := setupPublicCatalogQueryDB(t)
	service := newQueryServiceWithDB(db)
	if _, err := db.Exec(`DELETE FROM gw_sell_rates WHERE sku_id=31`); err != nil {
		t.Fatal(err)
	}
	if item, err := service.GetModelDetail(context.Background(), "unknown-model"); err != nil || item != nil {
		t.Fatalf("unknown model detail=%#v err=%v", item, err)
	}
	if _, err := service.GetModelDetail(context.Background(), "gpt-4.1"); err == nil {
		t.Fatal("missing prices must not publish a partial quotation")
	}
}

func TestModelDetailKeepsPricesForEachPublicOperation(t *testing.T) {
	db := setupPublicCatalogQueryDB(t)
	for _, statement := range []string{
		`UPDATE gw_operation_contracts SET operation_code='images.generate' WHERE id=100`,
		`UPDATE gw_operation_routes SET route_template='/v1/images/generations' WHERE operation_contract_id=100`,
		`UPDATE gw_sku_downstream_paths SET path='/v1/images/generations' WHERE sku_id=30`,
		`INSERT INTO gw_operation_contracts VALUES (102,'images.edit','active')`,
		`INSERT INTO gw_operation_routes VALUES (102,'POST','/v1/images/edits')`,
		`INSERT INTO gw_sku_downstream_paths VALUES (1,30,'/v1/images/edits')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	item, err := newQueryServiceWithDB(db).GetModelDetail(context.Background(), "gpt-4.1")
	if err != nil || item == nil || item.Pricing == nil || len(item.Pricing.Variants) != 3 {
		t.Fatalf("detail=%#v err=%v", item, err)
	}
	operations := make(map[string]int)
	for _, variant := range item.Pricing.Variants {
		operations[variant.Operation]++
	}
	if operations["images.generate"] != 1 || operations["images.edit"] != 1 || operations["responses.create"] != 1 {
		t.Fatalf("shared SKU lost its public operation: %#v", operations)
	}
}

func TestModelAvailabilityQueryFailureIsNotUnknownObservation(t *testing.T) {
	db := setupPublicCatalogQueryDB(t)
	if _, err := db.Exec(`DROP TABLE gw_api_calls`); err != nil {
		t.Fatal(err)
	}
	items, err := newQueryServiceWithDB(db).ListModelAvailability(context.Background())
	if err == nil || items != nil {
		t.Fatalf("query failure was reported as unknown data: %#v err=%v", items, err)
	}
}

func TestModelAvailabilityIsIndependentAndOmitsUnknownObservations(t *testing.T) {
	db := setupPublicCatalogQueryDB(t)
	for _, statement := range []string{
		`DROP TABLE gw_sell_rates`,
		`DROP TABLE gw_catalog_discovery_items`,
		`UPDATE gw_products SET capability_constraints='invalid JSON'`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	service := newQueryServiceWithDB(db)
	items, err := service.ListModelAvailability(context.Background())
	if err != nil || len(items) != 1 || items[0].ID != "gpt-4.1" || items[0].Availability == nil ||
		items[0].Availability.SuccessRate != "98.7" || !json.Valid(items[0].Availability.Buckets) {
		t.Fatalf("availability=%#v err=%v", items, err)
	}
	if _, err := db.Exec(`UPDATE gw_availability_sources SET status='disabled'`); err != nil {
		t.Fatal(err)
	}
	items, err = service.ListModelAvailability(context.Background())
	if err != nil || len(items) != 1 || items[0].Availability != nil {
		t.Fatalf("unknown observations=%#v err=%v", items, err)
	}
	encoded, err := json.Marshal(items)
	if err != nil || strings.Contains(string(encoded), "availability") {
		t.Fatalf("unknown observation became a value: %s err=%v", encoded, err)
	}
	if _, err := db.Exec(`UPDATE gateway_channels SET status='disabled'`); err != nil {
		t.Fatal(err)
	}
	items, err = service.ListModelAvailability(context.Background())
	if err != nil || len(items) != 0 || items == nil {
		t.Fatalf("inactive models should return an empty array: %#v err=%v", items, err)
	}
}
