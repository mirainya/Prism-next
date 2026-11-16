package repository
import (
	"context"
	"database/sql"
	"strconv"
	"strings"
)

// TransportBaseURLChange moves one catalog transport to a new upstream
// origin.  The allowed-host entry is rebuilt from the origin so a transport
// cannot accidentally keep authorizing the previous provider after a move.
// This is intentionally a catalog change: the active release version and
// digest advance together with the endpoint update.
type TransportBaseURLChange struct {
	catalogChangeGuard
	TransportCode string `json:"transport_code"`
	BaseURL       string `json:"base_url"`
}

func (in *TransportBaseURLChange) Normalize() {
	in.TransportCode = strings.ToLower(strings.TrimSpace(in.TransportCode))
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	in.SemanticVersion = strings.TrimSpace(in.SemanticVersion)
}

func (in TransportBaseURLChange) Validate() error {
	if err := in.catalogChangeGuard.validate(); err != nil {
		return err
	}
	if !catalogIdentityPattern.MatchString(in.TransportCode) || in.BaseURL == "" {
		return ErrInvalidInput
	}
	if _, err := validateCatalogBaseURL(in.BaseURL); err != nil {
		return ErrInvalidInput
	}
	return nil
}

func transportBaseURLHost(baseURL string) (CatalogAllowedHostInput, error) {
	parsed, err := validateCatalogBaseURL(baseURL)
	if err != nil {
		return CatalogAllowedHostInput{}, err
	}
	port := uint16(443)
	if parsed.Scheme == "http" {
		port = 80
	}
	if parsed.Port() != "" {
		value, parseErr := strconv.ParseUint(parsed.Port(), 10, 16)
		if parseErr != nil || value == 0 {
			return CatalogAllowedHostInput{}, ErrInvalidInput
		}
		port = uint16(value)
	}
	return CatalogAllowedHostInput{
		Protocol: parsed.Scheme,
		Host:     strings.ToLower(strings.TrimSuffix(parsed.Hostname(), ".")),
		Port:     port,
	}, nil
}

func (s *Store) ChangeTransportBaseURL(ctx context.Context, tx *sql.Tx, in TransportBaseURLChange, actorID uint64) (CatalogChangeResult, error) {
	if tx == nil || actorID == 0 {
		return CatalogChangeResult{}, ErrInvalidInput
	}
	in.Normalize()
	if err := in.Validate(); err != nil {
		return CatalogChangeResult{}, err
	}
	host, err := transportBaseURLHost(in.BaseURL)
	if err != nil {
		return CatalogChangeResult{}, err
	}
	active, err := lockActiveCatalogRelease(ctx, tx, in.ExpectedActiveReleaseID, in.ExpectedConfigVersion)
	if err != nil {
		return CatalogChangeResult{}, err
	}
	var transportID uint64
	var before string
	if err := tx.QueryRowContext(ctx, `SELECT id,base_url FROM gw_channel_transports WHERE release_id=? AND transport_code=? FOR UPDATE`, active.ID, in.TransportCode).
		Scan(&transportID, &before); err == sql.ErrNoRows {
		return CatalogChangeResult{}, ErrNotFound
	} else if err != nil {
		return CatalogChangeResult{}, err
	}
	if strings.TrimRight(before, "/") == in.BaseURL {
		return CatalogChangeResult{ReleaseID: active.ID, ConfigVersion: active.Version, Activated: true, SourceReleaseID: active.ID}, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE gw_channel_transports SET base_url=? WHERE release_id=? AND id=?`, in.BaseURL, active.ID, transportID); err != nil {
		return CatalogChangeResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM gw_transport_allowed_hosts WHERE release_id=? AND channel_transport_id=?`, active.ID, transportID); err != nil {
		return CatalogChangeResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO gw_transport_allowed_hosts(release_id,channel_transport_id,protocol,host_pattern,port,created_at) VALUES (?,?,?,?,?,?)`, active.ID, transportID, host.Protocol, host.Host, host.Port, nowUTC()); err != nil {
		return CatalogChangeResult{}, err
	}
	version, err := advanceActiveCatalog(ctx, tx, active, "catalog_change.transport_base_url", actorID, ginSafeMetadata{
		"source_release_id":    active.ID,
		"channel_transport_id": transportID,
		"transport_code":       in.TransportCode,
		"before_base_url":      before,
		"after_base_url":       in.BaseURL,
		"allowed_host":         host,
	})
	if err != nil {
		return CatalogChangeResult{}, err
	}
	return CatalogChangeResult{ReleaseID: active.ID, ConfigVersion: version, Activated: true, SourceReleaseID: active.ID}, nil
}
