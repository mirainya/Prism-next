package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

var ErrPublicModelNameConflict = fmt.Errorf("%w: public model identity is already reserved", ErrConflict)

type CatalogModelNameSuggestionInput struct {
	BaseName string
}

func (in CatalogModelNameSuggestionInput) Validate() error {
	if !validPublicModelIdentity(in.BaseName) {
		return ErrInvalidInput
	}
	return nil
}

// Suggestions read global identities, including hidden models and aliases in
// previous releases. SQL equality uses the same collation as the unique keys.
// They reserve nothing; the onboarding transaction remains the final authority.
func (s *Store) SuggestCatalogModelName(ctx context.Context, in CatalogModelNameSuggestionInput) (string, error) {
	if err := in.Validate(); err != nil {
		return "", err
	}
	for index := 1; index <= 100; index++ {
		candidate := in.BaseName
		if index > 1 {
			suffix := "-" + strconv.Itoa(index)
			stem := in.BaseName
			for len(stem)+len(suffix) > 128 {
				_, size := utf8.DecodeLastRuneInString(stem)
				stem = stem[:len(stem)-size]
			}
			candidate = stem + suffix
		}
		var occupied bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM gw_model_names WHERE api_name=?) OR EXISTS(SELECT 1 FROM gw_models WHERE model_code=?)`, candidate, candidate).Scan(&occupied); err != nil {
			return "", err
		}
		if !occupied {
			return candidate, nil
		}
	}
	return "", ErrPublicModelNameConflict
}

func createNewCatalogModelIdentity(ctx context.Context, tx *sql.Tx, modelCode, apiName string) (uint64, uint64, error) {
	if tx == nil || !validPublicModelIdentity(modelCode) || !validPublicModelIdentity(apiName) {
		return 0, 0, ErrInvalidInput
	}
	for _, lookup := range []struct{ query, value string }{
		{`SELECT id FROM gw_model_names WHERE api_name=? FOR UPDATE`, apiName},
		{`SELECT id FROM gw_models WHERE model_code=? FOR UPDATE`, modelCode},
	} {
		var existingID uint64
		err := tx.QueryRowContext(ctx, lookup.query, lookup.value).Scan(&existingID)
		if err == nil {
			return 0, 0, ErrPublicModelNameConflict
		}
		if err != sql.ErrNoRows {
			return 0, 0, err
		}
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO gw_models(model_code,created_at) VALUES (?,?)`, modelCode, nowUTC())
	if err != nil {
		return 0, 0, publicModelIdentityInsertError(err)
	}
	modelID, err := lastID(result)
	if err != nil {
		return 0, 0, err
	}
	result, err = tx.ExecContext(ctx, `INSERT INTO gw_model_names(model_id,api_name,is_primary,created_at) VALUES (?,?,TRUE,?)`, modelID, apiName, nowUTC())
	if err != nil {
		return 0, 0, publicModelIdentityInsertError(err)
	}
	nameID, err := lastID(result)
	return modelID, nameID, err
}

func publicModelIdentityInsertError(err error) error {
	if isMySQLDuplicateKey(err) {
		return ErrPublicModelNameConflict
	}
	return err
}

// Kept at the API boundary: whitespace is not part of a public identity.
func (in *CatalogModelNameSuggestionInput) Normalize() { in.BaseName = strings.TrimSpace(in.BaseName) }
