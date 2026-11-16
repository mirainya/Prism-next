package repository

import "context"

type BackgroundCapabilityDispatch struct {
	CapabilityDispatch
	SKUID, ProductTransportID uint64
	Operation                 string
}

func (s *Store) ReadBackgroundCapabilityDispatch(ctx context.Context, attemptID uint64) (BackgroundCapabilityDispatch, error) {
	fixed, err := s.ReadCapabilityDispatch(ctx, attemptID)
	if err != nil {
		return BackgroundCapabilityDispatch{}, err
	}
	out := BackgroundCapabilityDispatch{CapabilityDispatch: fixed}
	err = s.db.QueryRowContext(ctx, `SELECT c.sku_id,a.product_transport_id,JSON_UNQUOTE(JSON_EXTRACT(task.parameter_summary,'$.operation'))
FROM gw_api_call_attempts a JOIN gw_api_calls c ON c.id=a.call_id
JOIN gw_api_resources r ON r.call_id=c.id AND r.resource_kind='capability_task'
JOIN gw_capability_tasks task ON task.resource_id=r.id WHERE a.id=?`, attemptID).
		Scan(&out.SKUID, &out.ProductTransportID, &out.Operation)
	return out, err
}

func (s *Store) ReadAttemptResourceKind(ctx context.Context, attemptID uint64) (string, error) {
	var kind string
	err := s.db.QueryRowContext(ctx, `SELECT r.resource_kind FROM gw_api_call_attempts a JOIN gw_api_resources r ON r.call_id=a.call_id WHERE a.id=?`, attemptID).Scan(&kind)
	return kind, err
}
