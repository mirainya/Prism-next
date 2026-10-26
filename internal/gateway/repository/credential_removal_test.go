package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/mirainya/Prism/internal/gateway/credentials"
)

func credentialRemovalDB(t *testing.T) *Store {
	t.Helper()
	store := runtimeStateDB(t)
	for _, statement := range []string{
		`ALTER TABLE gw_offerings ADD COLUMN credential_pool_id INTEGER`,
		`UPDATE gw_offerings SET credential_pool_id=90 WHERE id=1201`,
		`UPDATE gw_offerings SET credential_pool_id=91 WHERE id=1202`,
		`INSERT INTO gw_offering_runtime_state(release_id,offering_id,state,state_version,reason_code,updated_at) VALUES (1,1202,'active',1,'admin_create','t')`,
		`CREATE TABLE gw_catalog_runtime_state(id INTEGER PRIMARY KEY,active_release_id INTEGER)`,
		`CREATE TABLE gateway_channels(id INTEGER PRIMARY KEY,status TEXT)`,
		`CREATE TABLE gw_credential_pools(id INTEGER PRIMARY KEY,channel_id INTEGER,status TEXT)`,
		`CREATE TABLE gw_credentials(id INTEGER PRIMARY KEY,channel_id INTEGER,credential_pool_id INTEGER,status TEXT,config_version INTEGER,secret TEXT,secret_identity_id INTEGER,current_version_id INTEGER,updated_at TEXT)`,
		`CREATE TABLE gw_credential_secret_identities(id INTEGER PRIMARY KEY,channel_id INTEGER,status TEXT)`,
		`CREATE TABLE gw_credential_versions(id INTEGER PRIMARY KEY,credential_id INTEGER,secret_identity_id INTEGER,status TEXT,valid_until TEXT,encrypted_blob_id INTEGER,retired_at TEXT)`,
		`CREATE TABLE gw_credential_purpose_grants(id INTEGER PRIMARY KEY,credential_id INTEGER,purpose TEXT,status TEXT,state_version INTEGER,revoked_at TEXT)`,
		`CREATE TABLE encrypted_blobs(id INTEGER PRIMARY KEY,purpose TEXT,keyring_id INTEGER,ciphertext BLOB,purged_at TEXT)`,
		`CREATE TABLE crypto_keyring_state(id INTEGER PRIMARY KEY,current_version INTEGER)`,
		`CREATE TABLE crypto_key_versions(keyring_id INTEGER,key_version INTEGER,status TEXT)`,
		`CREATE TABLE encrypted_blob_key_wraps(encrypted_blob_id INTEGER,keyring_id INTEGER,kek_version INTEGER)`,
		`CREATE TABLE gw_credential_slots(id INTEGER PRIMARY KEY,credential_id INTEGER,state TEXT)`,
		`CREATE TABLE gw_api_call_attempts(id INTEGER PRIMARY KEY,credential_id INTEGER,state TEXT)`,
		`CREATE TABLE gw_control_plane_runs(id INTEGER PRIMARY KEY,credential_id INTEGER,state TEXT)`,
		`CREATE TABLE gw_result_deliveries(id INTEGER PRIMARY KEY,attempt_id INTEGER,state TEXT)`,
		`CREATE TABLE gw_async_outbox(id INTEGER PRIMARY KEY,result_delivery_id INTEGER,status TEXT)`,
		`CREATE TABLE gw_availability_sources(id INTEGER PRIMARY KEY,credential_id INTEGER,status TEXT,config_version INTEGER,updated_at TEXT)`,
		`CREATE TABLE gw_catalog_models(id INTEGER PRIMARY KEY,release_id INTEGER)`,
		`CREATE TABLE gw_catalog_model_names(release_id INTEGER,catalog_model_id INTEGER,model_name_id INTEGER,is_primary INTEGER)`,
		`CREATE TABLE gw_model_names(id INTEGER PRIMARY KEY,api_name TEXT)`,
		`CREATE TABLE gw_model_operations(id INTEGER PRIMARY KEY,release_id INTEGER,catalog_model_id INTEGER)`,
		`CREATE TABLE gw_skus(id INTEGER PRIMARY KEY,release_id INTEGER,model_operation_id INTEGER)`,
		`CREATE TABLE gw_routes(release_id INTEGER,sku_id INTEGER,offering_id INTEGER)`,
		`CREATE TABLE gw_credential_state_events(credential_id INTEGER,state_version INTEGER,old_state TEXT,new_state TEXT,reason_code TEXT,created_at TEXT)`,
		`CREATE TABLE gw_credential_version_state_events(credential_version_id INTEGER,state_version INTEGER,old_state TEXT,new_state TEXT,reason_code TEXT,created_at TEXT)`,
		`CREATE TABLE gw_credential_purpose_grant_state_events(grant_id INTEGER,state_version INTEGER,old_state TEXT,new_state TEXT,reason_code TEXT,created_at TEXT)`,
		`INSERT INTO gw_catalog_runtime_state VALUES (1,1)`,
		`INSERT INTO gateway_channels VALUES (50,'active')`,
		`INSERT INTO gw_credential_pools VALUES (90,50,'active'),(91,50,'active')`,
		`INSERT INTO gw_credentials VALUES (100,50,90,'active',3,'private-key',110,120,'t'),(101,50,91,'active',3,'other-key',111,121,'t')`,
		`INSERT INTO gw_credential_secret_identities VALUES (110,50,'active'),(111,50,'active')`,
		`INSERT INTO gw_credential_versions VALUES (120,100,110,'active',NULL,130,NULL),(121,101,111,'active',NULL,NULL,NULL)`,
		`INSERT INTO encrypted_blobs VALUES (130,'credential',140,X'70726976617465',NULL)`,
		`INSERT INTO encrypted_blob_key_wraps VALUES (130,140,1)`,
		`INSERT INTO gw_credential_purpose_grants VALUES (160,100,'execution','active',1,NULL),(161,101,'execution','active',1,NULL)`,
		`INSERT INTO gw_availability_sources VALUES (170,100,'active',1,'t')`,
		`INSERT INTO gw_catalog_models VALUES (10,1),(11,1)`,
		`INSERT INTO gw_model_names VALUES (20,'model-a'),(21,'model-a-2')`,
		`INSERT INTO gw_catalog_model_names VALUES (1,10,20,1),(1,11,21,1)`,
		`INSERT INTO gw_model_operations VALUES (30,1,10),(31,1,11)`,
		`INSERT INTO gw_skus VALUES (40,1,30),(41,1,31)`,
		`INSERT INTO gw_routes VALUES (1,40,1201),(1,41,1202)`,
	} {
		if _, err := store.DB().Exec(statement); err != nil {
			t.Fatalf("fixture %s: %v", statement, err)
		}
	}
	return store
}

func removeCredentialForTest(t *testing.T, store *Store, version uint64) (CredentialRemovalImpact, error) {
	t.Helper()
	var result CredentialRemovalImpact
	err := store.WithTx(context.Background(), func(tx *sql.Tx) error {
		var err error
		result, err = store.RemoveManagedCredential(context.Background(), tx, 100, version, 1)
		return err
	})
	return result, err
}

func TestCredentialRemovalErasesOnlySecretsAndPreservesHistory(t *testing.T) {
	store := credentialRemovalDB(t)
	result, err := removeCredentialForTest(t, store, 3)
	if err != nil || !result.SecretRemoved || result.Status != "disabled" || result.ConfigVersion != 6 {
		t.Fatalf("remove=%#v err=%v", result, err)
	}
	if len(result.Models) != 1 || result.Models[0].APIName != "model-a" || !result.Models[0].Unavailable {
		t.Fatalf("wrong impact: %#v", result.Models)
	}
	var secret sql.NullString
	var state string
	var ciphertext []byte
	if err := store.DB().QueryRow(`SELECT secret,status FROM gw_credentials WHERE id=100`).Scan(&secret, &state); err != nil || secret.Valid || state != "disabled" {
		t.Fatalf("secret retained: %#v %s %v", secret, state, err)
	}
	if err := store.DB().QueryRow(`SELECT ciphertext FROM encrypted_blobs WHERE id=130 AND purged_at IS NOT NULL`).Scan(&ciphertext); err != nil || len(ciphertext) != 0 {
		t.Fatalf("legacy secret retained: %x %v", ciphertext, err)
	}
	for query, want := range map[string]int{
		`SELECT COUNT(*) FROM encrypted_blob_key_wraps`:                                                  0,
		`SELECT COUNT(*) FROM gw_credentials`:                                                            2,
		`SELECT COUNT(*) FROM gw_catalog_models`:                                                         2,
		`SELECT COUNT(*) FROM gw_credential_versions WHERE id=120 AND status='retired'`:                  1,
		`SELECT COUNT(*) FROM gw_credential_purpose_grants WHERE credential_id=100 AND status='revoked'`: 1,
		`SELECT COUNT(*) FROM gw_offering_runtime_state WHERE offering_id=1201 AND state='disabled'`:     1,
		`SELECT COUNT(*) FROM gw_offering_runtime_state WHERE offering_id=1202 AND state='active'`:       1,
	} {
		var got int
		if err := store.DB().QueryRow(query).Scan(&got); err != nil || got != want {
			t.Fatalf("%s got=%d want=%d err=%v", query, got, want, err)
		}
	}
	var metadata string
	rows, err := store.DB().Query(`SELECT metadata FROM audit_events`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		if err := rows.Scan(&metadata); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(metadata, "private-key") || strings.Contains(metadata, "other-key") {
			t.Fatal("secret leaked to audit")
		}
	}
	rows.Close()
	result, err = removeCredentialForTest(t, store, 6)
	if err != nil || !result.SecretRemoved || result.ConfigVersion != 6 {
		t.Fatalf("repeat removal not idempotent: %#v %v", result, err)
	}
}

func TestCredentialRemovalWaitsForEveryPinnedOperation(t *testing.T) {
	for _, test := range []struct{ name, insert, finish string }{
		{"slot", `INSERT INTO gw_credential_slots VALUES (1,100,'recovery_required')`, `UPDATE gw_credential_slots SET state='released'`},
		{"attempt", `INSERT INTO gw_api_call_attempts VALUES (1,100,'started')`, `UPDATE gw_api_call_attempts SET state='completed'`},
		{"unknown", `INSERT INTO gw_api_call_attempts VALUES (1,100,'terminated_unknown')`, `UPDATE gw_api_call_attempts SET state='failed'`},
		{"control", `INSERT INTO gw_control_plane_runs VALUES (1,100,'running')`, `UPDATE gw_control_plane_runs SET state='failed'`},
		{"delivery", `INSERT INTO gw_api_call_attempts VALUES (1,100,'completed'); INSERT INTO gw_result_deliveries VALUES (1,1,'pending')`, `UPDATE gw_result_deliveries SET state='ready'`},
		{"delivery_retry", `INSERT INTO gw_api_call_attempts VALUES (1,100,'completed'); INSERT INTO gw_result_deliveries VALUES (1,1,'ready'); INSERT INTO gw_async_outbox VALUES (1,1,'pending')`, `UPDATE gw_async_outbox SET status='succeeded'`},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := credentialRemovalDB(t)
			for _, statement := range strings.Split(test.insert, ";") {
				if _, err := store.DB().Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			result, err := removeCredentialForTest(t, store, 3)
			if err != nil || result.SecretRemoved || result.Status != "draining" || !result.Pending() {
				t.Fatalf("secret erased before completion: %#v %v", result, err)
			}
			var secret string
			if err := store.DB().QueryRow(`SELECT secret FROM gw_credentials WHERE id=100`).Scan(&secret); err != nil || secret != "private-key" {
				t.Fatal("draining lost secret")
			}
			if _, err := store.DB().Exec(test.finish); err != nil {
				t.Fatal(err)
			}
			result, err = removeCredentialForTest(t, store, result.ConfigVersion)
			if err != nil || !result.SecretRemoved {
				t.Fatalf("completed removal=%#v err=%v", result, err)
			}
		})
	}
}

func TestCredentialRemovalPreservesAlternativeKeysAndRoutes(t *testing.T) {
	for _, variant := range []string{"shared_key", "other_route", "expired_key"} {
		t.Run(variant, func(t *testing.T) {
			store := credentialRemovalDB(t)
			switch variant {
			case "shared_key", "expired_key":
				if _, err := store.DB().Exec(`UPDATE gw_credentials SET credential_pool_id=90 WHERE id=101`); err != nil {
					t.Fatal(err)
				}
				if variant == "expired_key" {
					if _, err := store.DB().Exec(`UPDATE gw_credential_versions SET valid_until='2000-01-01' WHERE id=121`); err != nil {
						t.Fatal(err)
					}
				}
			case "other_route":
				if _, err := store.DB().Exec(`INSERT INTO gw_routes VALUES (1,40,1202)`); err != nil {
					t.Fatal(err)
				}
			}
			result, err := removeCredentialForTest(t, store, 3)
			if err != nil {
				t.Fatal(err)
			}
			if result.Models[0].Unavailable != (variant == "expired_key") {
				t.Fatalf("wrong availability: %#v", result)
			}
			var state string
			if err := store.DB().QueryRow(`SELECT state FROM gw_offering_runtime_state WHERE offering_id=1201`).Scan(&state); err != nil {
				t.Fatal(err)
			}
			if (state == "active") != (variant == "shared_key") {
				t.Fatalf("wrong pool line state: %s", state)
			}
		})
	}
}

func TestCredentialRemovalConflictAndFailureRollBackAllChanges(t *testing.T) {
	store := credentialRemovalDB(t)
	if _, err := removeCredentialForTest(t, store, 2); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale write accepted: %v", err)
	}
	if _, err := store.DB().Exec(`CREATE TRIGGER fail_removal BEFORE UPDATE OF secret ON gw_credentials BEGIN SELECT RAISE(ABORT,'test erase failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := removeCredentialForTest(t, store, 3); err == nil {
		t.Fatal("erase failure ignored")
	}
	for query, want := range map[string]string{
		`SELECT status FROM gw_credentials WHERE id=100`:                     "active",
		`SELECT state FROM gw_offering_runtime_state WHERE offering_id=1201`: "active",
		`SELECT status FROM gw_credential_versions WHERE id=120`:             "active",
	} {
		var got string
		if err := store.DB().QueryRow(query).Scan(&got); err != nil || got != want {
			t.Fatalf("rollback failed %s: %s %v", query, got, err)
		}
	}
}

func TestExistingDisabledKeyCanEraseRetainedSecret(t *testing.T) {
	store := credentialRemovalDB(t)
	if _, err := store.DB().Exec(`UPDATE gw_credentials SET status='disabled' WHERE id=100`); err != nil {
		t.Fatal(err)
	}
	result, err := removeCredentialForTest(t, store, 3)
	if err != nil || !result.SecretRemoved || result.ConfigVersion != 4 {
		t.Fatalf("disabled key removal=%#v err=%v", result, err)
	}
}

func TestCredentialDisableCannotBypassUnresolvedAttempts(t *testing.T) {
	store := credentialRemovalDB(t)
	if _, err := store.DB().Exec(`UPDATE gw_credentials SET status='draining' WHERE id=100; INSERT INTO gw_api_call_attempts VALUES (1,100,'started')`); err != nil {
		t.Fatal(err)
	}
	err := store.WithTx(context.Background(), func(tx *sql.Tx) error {
		return store.TransitionCredential(context.Background(), tx, 100, credentials.CredentialDraining, credentials.CredentialDisabled, "admin:1")
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("disabled a pinned request without a slot: %v", err)
	}
}
