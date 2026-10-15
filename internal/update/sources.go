package update

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mirainya/Prism/pkg/safeurl"
)

const (
	// GitHubSourceName and GitHubPriority describe the immutable source that is
	// always available even when no operator-managed mirror has been configured.
	GitHubSourceName = "github"
	GitHubPriority   = 100000
	MaxCustomSources = 8
)

var (
	ErrInvalidSource        = errors.New("invalid update source")
	ErrSourceConflict       = errors.New("update source configuration conflict")
	ErrSourceVersionMissing = errors.New("update source version is required")
)

// SourceConfig is the operator-facing representation of a database source.
// ID=0 and BuiltIn=true identify the immutable GitHub source.
type SourceConfig struct {
	ID            uint64 `json:"id"`
	Name          string `json:"name"`
	BaseURL       string `json:"base_url"`
	Enabled       bool   `json:"enabled"`
	Priority      int    `json:"priority"`
	ConfigVersion uint64 `json:"config_version"`
	BuiltIn       bool   `json:"built_in"`
}

// SourcesConfig is the atomically read configuration used by the update
// console. Revision changes after every successful whole-list replacement.
type SourcesConfig struct {
	Sources  []SourceConfig `json:"sources"`
	Revision uint64         `json:"revision"`
}

// LoadSources returns custom database sources followed by the immutable
// GitHub fallback, sorted by priority and then name. The fallback is never
// read from or written to the database.
func LoadSources(ctx context.Context, db *sql.DB) (SourcesConfig, error) {
	if db == nil {
		return SourcesConfig{}, ErrInvalidSource
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return SourcesConfig{}, fmt.Errorf("begin load update sources transaction: %w", err)
	}
	defer tx.Rollback()
	var revision uint64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM prism_update_sources_meta WHERE id=1`).Scan(&revision); err != nil {
		return SourcesConfig{}, fmt.Errorf("load update sources revision: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,source_name,base_url,enabled,priority,config_version FROM prism_update_sources ORDER BY priority,source_name,id`)
	if err != nil {
		return SourcesConfig{}, fmt.Errorf("load update sources: %w", err)
	}
	defer rows.Close()
	items := make([]SourceConfig, 0)
	for rows.Next() {
		var item SourceConfig
		var enabled bool
		if err := rows.Scan(&item.ID, &item.Name, &item.BaseURL, &enabled, &item.Priority, &item.ConfigVersion); err != nil {
			return SourcesConfig{}, fmt.Errorf("scan update source: %w", err)
		}
		item.Enabled = enabled
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return SourcesConfig{}, fmt.Errorf("iterate update sources: %w", err)
	}
	items = append(items, SourceConfig{
		Name: GitHubSourceName, BaseURL: DefaultGitHubBaseURL, Enabled: true,
		Priority: GitHubPriority, ConfigVersion: 1, BuiltIn: true,
	})
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority < items[j].Priority
		}
		return items[i].Name < items[j].Name
	})
	if err := tx.Commit(); err != nil {
		return SourcesConfig{}, fmt.Errorf("commit load update sources transaction: %w", err)
	}
	return SourcesConfig{Sources: items, Revision: revision}, nil
}

// Sources converts administrator configuration to the downloader's source
// type, omitting disabled entries. It is intentionally separate from loading
// so the updater remains usable without a database connection.
func Sources(items []SourceConfig) []Source {
	result := make([]Source, 0, len(items))
	for _, item := range items {
		if item.Enabled && strings.TrimSpace(item.BaseURL) != "" {
			result = append(result, Source{Name: item.Name, BaseURL: item.BaseURL})
		}
	}
	return result
}

// SaveSources replaces all operator-managed sources in one transaction.
// Built-in GitHub may be included in the request for round-tripping, but it
// cannot be changed, disabled, or persisted.
func SaveSources(ctx context.Context, db *sql.DB, items []SourceConfig, actorID, expectedRevision uint64) error {
	if db == nil || actorID == 0 {
		return ErrInvalidSource
	}
	if expectedRevision == 0 {
		return ErrSourceVersionMissing
	}
	custom, err := validateSourceList(ctx, items)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin update source transaction: %w", err)
	}
	defer tx.Rollback()
	var revision uint64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM prism_update_sources_meta WHERE id=1 FOR UPDATE`).Scan(&revision); err != nil {
		return fmt.Errorf("lock update sources revision: %w", err)
	}
	if revision != expectedRevision {
		return ErrSourceConflict
	}
	// Lock the complete source set before changing it. This keeps replacements
	// and removals serialized and checks every submitted version before writes.
	rows, err := tx.QueryContext(ctx, `SELECT id,config_version FROM prism_update_sources ORDER BY id FOR UPDATE`)
	if err != nil {
		return fmt.Errorf("lock update sources: %w", err)
	}
	existing := make(map[uint64]uint64)
	for rows.Next() {
		var id, version uint64
		if err := rows.Scan(&id, &version); err != nil {
			rows.Close()
			return fmt.Errorf("scan locked update source: %w", err)
		}
		existing[id] = version
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate locked update sources: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close locked update sources: %w", err)
	}
	for _, item := range custom {
		if item.ID != 0 {
			if item.ConfigVersion == 0 {
				return ErrSourceVersionMissing
			}
			if version, exists := existing[item.ID]; !exists || version != item.ConfigVersion {
				return ErrSourceConflict
			}
		}
	}
	now := time.Now().UTC()
	keep := make(map[uint64]struct{}, len(custom))
	for _, item := range custom {
		if item.ID == 0 {
			result, err := tx.ExecContext(ctx, `INSERT INTO prism_update_sources(source_name,base_url,enabled,priority,config_version,created_by,updated_by,created_at,updated_at) VALUES (?,?,?,?,1,?,?,?,?)`, item.Name, item.BaseURL, item.Enabled, item.Priority, actorID, actorID, now, now)
			if err != nil {
				return fmt.Errorf("insert update source: %w", err)
			}
			id, err := result.LastInsertId()
			if err != nil {
				return fmt.Errorf("insert update source id: %w", err)
			}
			if id <= 0 {
				return ErrSourceConflict
			}
			keep[uint64(id)] = struct{}{}
			continue
		}
		keep[item.ID] = struct{}{}
		result, err := tx.ExecContext(ctx, `UPDATE prism_update_sources SET source_name=?,base_url=?,enabled=?,priority=?,config_version=config_version+1,updated_by=?,updated_at=? WHERE id=? AND config_version=?`, item.Name, item.BaseURL, item.Enabled, item.Priority, actorID, now, item.ID, item.ConfigVersion)
		if err != nil {
			return fmt.Errorf("update update source: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("read update source result: %w", err)
		}
		if affected != 1 {
			return ErrSourceConflict
		}
	}
	for id := range existing {
		if _, exists := keep[id]; exists {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM prism_update_sources WHERE id=?`, id); err != nil {
			return fmt.Errorf("remove update source: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE prism_update_sources_meta SET revision=revision+1,updated_at=? WHERE id=1 AND revision=?`, now, expectedRevision); err != nil {
		return fmt.Errorf("advance update sources revision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit update source transaction: %w", err)
	}
	return nil
}

func validateSourceList(ctx context.Context, items []SourceConfig) ([]SourceConfig, error) {
	if len(items) > MaxCustomSources+1 {
		return nil, ErrInvalidSource
	}
	custom := make([]SourceConfig, 0, len(items))
	names := make(map[string]struct{}, len(items))
	urls := make(map[string]struct{}, len(items))
	ids := make(map[uint64]struct{}, len(items))
	builtInSeen := false
	for _, raw := range items {
		if raw.BuiltIn || strings.EqualFold(strings.TrimSpace(raw.Name), GitHubSourceName) {
			if builtInSeen || raw.ID != 0 || raw.Name != GitHubSourceName || raw.BaseURL != DefaultGitHubBaseURL || !raw.Enabled || raw.Priority != 0 && raw.Priority != GitHubPriority {
				return nil, ErrInvalidSource
			}
			builtInSeen = true
			continue
		}
		if len(custom) >= MaxCustomSources {
			return nil, ErrInvalidSource
		}
		item := raw
		item.Name = strings.ToLower(strings.TrimSpace(item.Name))
		baseURL, err := normalizeBaseURL(item.BaseURL)
		if err != nil {
			return nil, ErrInvalidSource
		}
		item.BaseURL = baseURL
		if !validSourceName(item.Name) || item.Name == GitHubSourceName || item.Priority < 0 || item.Priority >= GitHubPriority {
			return nil, ErrInvalidSource
		}
		if item.BaseURL == DefaultGitHubBaseURL || item.ID == 0 && item.ConfigVersion != 0 {
			return nil, ErrInvalidSource
		}
		if item.ID != 0 {
			if _, exists := ids[item.ID]; exists {
				return nil, ErrInvalidSource
			}
			ids[item.ID] = struct{}{}
		}
		if _, exists := names[item.Name]; exists {
			return nil, ErrInvalidSource
		}
		if _, exists := urls[item.BaseURL]; exists {
			return nil, ErrInvalidSource
		}
		if err := validateMirrorURL(ctx, item.BaseURL); err != nil {
			return nil, err
		}
		names[item.Name] = struct{}{}
		urls[item.BaseURL] = struct{}{}
		custom = append(custom, item)
	}
	return custom, nil
}

func validSourceName(value string) bool {
	if value == "" || utf8.RuneCountInString(value) > 64 || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	return strings.IndexFunc(value, unicode.IsControl) < 0
}

func validateMirrorURL(ctx context.Context, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || len(raw) > 512 || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || strings.Contains(parsed.Path, `\`) || strings.Contains(parsed.Path, "..") {
		return ErrInvalidSource
	}
	if err := safeurl.Validate(ctx, parsed.String()); err != nil {
		return fmt.Errorf("%w: mirror must resolve to a public HTTPS host", ErrInvalidSource)
	}
	return nil
}
