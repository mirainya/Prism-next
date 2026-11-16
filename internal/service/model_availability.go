package service

import (
	"context"
	"sort"
)

type ModelAvailability struct {
	ID           string               `json:"id"`
	Availability *PricingAvailability `json:"availability,omitempty"`
}

// ListModelAvailability refreshes observations without loading prices or capability metadata.
// Models without observations retain their ID but omit availability.
func (s *QueryService) ListModelAvailability(ctx context.Context) ([]ModelAvailability, error) {
	rows, store, err := s.activeRoutes(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]ModelAvailability, 0)
	if len(rows) == 0 {
		return result, nil
	}
	observations, err := store.LoadSKUAvailability(ctx, rows[0].ReleaseID)
	if err != nil {
		return nil, err
	}
	items := make(map[string]*AvailableModelCapability)
	for _, row := range rows {
		item := items[row.APIName]
		if item == nil {
			item = &AvailableModelCapability{ID: row.APIName, Sort: row.SortOrder}
			items[row.APIName] = item
		}
		item.applyAvailability(observations[row.SKUID])
	}
	ordered := make([]*AvailableModelCapability, 0, len(items))
	for _, item := range items {
		ordered = append(ordered, item)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Sort != ordered[j].Sort {
			return ordered[i].Sort < ordered[j].Sort
		}
		return ordered[i].ID < ordered[j].ID
	})
	for _, item := range ordered {
		result = append(result, ModelAvailability{ID: item.ID, Availability: item.Availability})
	}
	return result, nil
}
