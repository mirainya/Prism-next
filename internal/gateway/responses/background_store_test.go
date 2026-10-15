package responses

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/mirainya/Prism/internal/gateway/canonical"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
	protocol "github.com/mirainya/Prism/internal/provider/responses"
)

func TestPrepareBackgroundCanonicalContinuesSynchronousHistory(t *testing.T) {
	store, db, kek, hmacKey := setupBackgroundStoreTest(t)
	addStoredResponse(t, store, db, kek, hmacKey, 1, "resp_sync_first", protocol.Request{
		Model: "public", Input: json.RawMessage(`"first"`),
	}, canonical.Response{ID: "provider_first", Model: "public", Status: "completed", Output: []canonical.Item{backgroundTextItem("assistant", "first answer")}})
	addStoredResponse(t, store, db, kek, hmacKey, 2, "resp_sync_second", protocol.Request{
		Model: "public", Input: json.RawMessage(`"second"`), PreviousResponseID: "resp_sync_first",
	}, canonical.Response{ID: "provider_second", Model: "public", Status: "completed", Output: []canonical.Item{backgroundTextItem("assistant", "second answer")}})

	request := canonical.Request{Model: "public", Items: []canonical.Item{backgroundTextItem("user", "third")}, PreviousResponseID: "resp_sync_second"}
	got, assets, err := prepareBackgroundCanonical(context.Background(), 7, 9, "resp_sync_second", request)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 0 || got.PreviousResponseID != "" || len(got.Items) != 5 {
		t.Fatalf("assets=%v previous=%q items=%#v", assets, got.PreviousResponseID, got.Items)
	}
	for index, want := range []string{"first", "first answer", "second", "second answer", "third"} {
		if len(got.Items[index].Content) != 1 || got.Items[index].Content[0].Text != want {
			t.Fatalf("item %d = %#v, want %q", index, got.Items[index], want)
		}
	}
}

func TestPrepareBackgroundCanonicalKeepsBackgroundAssetSnapshot(t *testing.T) {
	store, db, kek, hmacKey := setupBackgroundStoreTest(t)
	previous := canonical.Request{Model: "public", Items: []canonical.Item{{
		Type: "message", Role: canonical.RoleUser, Content: []canonical.Content{{Type: "input_file", FileID: "file_previous"}},
	}}}
	addStoredResponse(t, store, db, kek, hmacKey, 3, "resp_background", backgroundRequestEnvelope{
		Version: 1, Request: protocol.Request{Model: "public", Input: json.RawMessage(`"ignored"`), Background: true}, Canonical: previous,
	}, protocol.Response{ID: "resp_background", Object: "response", Model: "public", Status: "completed",
		Output: json.RawMessage(`[ {"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]} ]`)})
	for _, statement := range []string{
		`INSERT INTO gw_media_assets (id,user_id,token_id,xfs_object_locator,content_type,content_length,sha256,state) VALUES (31,7,9,'asset/object','text/plain',1,'` + fmt.Sprintf("%064x", 1) + `','active')`,
		`INSERT INTO gw_media_asset_refs (call_id,user_id,token_id,media_asset_id,role,ordinal) VALUES (3,7,9,31,'input',0)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	current := canonical.Request{Model: "public", Items: []canonical.Item{backgroundTextItem("user", "next")}}
	got, assets, err := prepareBackgroundCanonical(context.Background(), 7, 9, "resp_background", current)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0] != 31 || len(got.Items) != 3 || got.Items[0].Content[0].FileID != "file_previous" {
		t.Fatalf("assets=%v items=%#v", assets, got.Items)
	}
}

func backgroundTextItem(role canonical.Role, value string) canonical.Item {
	contentType := "input_text"
	if role == canonical.RoleAssistant {
		contentType = "output_text"
	}
	return canonical.Item{Type: "message", Role: role, Content: []canonical.Content{{Type: contentType, Text: value}}}
}

func setupBackgroundStoreTest(t *testing.T) (*repository.Store, *sql.DB, []byte, []byte) {
	t.Helper()
	database := openResponsesTestDB(t)
	model.SetDB(database)
	db, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE gw_api_calls (id INTEGER PRIMARY KEY, public_id TEXT, user_id INTEGER, token_id INTEGER, request_payload_id INTEGER, result_payload_id INTEGER, status TEXT, created_at DATETIME, final_attempt_id INTEGER, current_attempt_id INTEGER)`,
		`CREATE TABLE gw_api_resources (id INTEGER PRIMARY KEY, public_id TEXT, resource_kind TEXT, call_id INTEGER, user_id INTEGER, token_id INTEGER, deleted_at DATETIME)`,
		`CREATE TABLE gw_ai_responses (resource_id INTEGER, status TEXT, result_summary BLOB)`,
		`CREATE TABLE gw_api_call_attempts (id INTEGER, credential_id INTEGER, product_transport_id INTEGER, catalog_release_id INTEGER)`,
		`CREATE TABLE gw_product_transports (id INTEGER, release_id INTEGER, product_id INTEGER, channel_transport_id INTEGER)`,
		`CREATE TABLE gw_products (id INTEGER, release_id INTEGER, channel_id INTEGER)`,
		`CREATE TABLE gw_channel_transports (id INTEGER, release_id INTEGER, protocol TEXT)`,
		`CREATE TABLE gw_api_call_payloads (id INTEGER PRIMARY KEY, call_id INTEGER, kind TEXT, encrypted_blob_id INTEGER)`,
		`CREATE TABLE encrypted_blobs (id INTEGER PRIMARY KEY, keyring_id INTEGER, purpose TEXT, schema_version INTEGER, aad_hash TEXT, nonce BLOB, ciphertext BLOB, content_hmac TEXT, content_length INTEGER, created_at DATETIME, purged_at DATETIME)`,
		`CREATE TABLE encrypted_blob_key_wraps (encrypted_blob_id INTEGER, keyring_id INTEGER, kek_version INTEGER, wrap_nonce BLOB, wrapped_dek BLOB, created_at DATETIME)`,
		`CREATE TABLE gw_media_assets (id INTEGER PRIMARY KEY, user_id INTEGER, token_id INTEGER, xfs_object_locator TEXT, storage_locator TEXT, object_key TEXT, xfs_base_url TEXT, xfs_api_key TEXT, content_type TEXT, content_length INTEGER, sha256 TEXT, state TEXT)`,
		`CREATE TABLE gw_media_asset_refs (call_id INTEGER, user_id INTEGER, token_id INTEGER, media_asset_id INTEGER, role TEXT, ordinal INTEGER)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	kek, hmacKey := bytes.Repeat([]byte{7}, 32), bytes.Repeat([]byte{9}, 32)
	t.Setenv("PRISM_GATEWAY_PAYLOAD_KEK_B64", base64.StdEncoding.EncodeToString(kek))
	t.Setenv("PRISM_GATEWAY_PAYLOAD_HMAC_B64", base64.StdEncoding.EncodeToString(hmacKey))
	store, err := repository.New(db)
	if err != nil {
		t.Fatal(err)
	}
	return store, db, kek, hmacKey
}

func addStoredResponse(t *testing.T, store *repository.Store, db *sql.DB, kek, hmacKey []byte, callID int, publicID string, request, result any) {
	t.Helper()
	requestBody, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	resultBody, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for index, payload := range [][]byte{requestBody, resultBody} {
		kind := "request"
		if index == 1 {
			kind = "result"
		}
		blobID, err := store.PutEncryptedBlob(context.Background(), tx, repository.BlobInput{
			KeyringID: 1, KEKVersion: 1, Purpose: "gateway-payload", SchemaVersion: 1,
			Owner: []byte(fmt.Sprintf("call:%d:%s", callID, kind)), Plaintext: payload, KEK: kek, HMACKey: hmacKey,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO gw_api_call_payloads (id,call_id,kind,encrypted_blob_id) VALUES (?,?,?,?)`, callID*2-1+index, callID, kind, blobID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO gw_api_calls (id,public_id,user_id,token_id,request_payload_id,result_payload_id,status,created_at) VALUES (?,?,?,?,?,?,?,?)`, callID, fmt.Sprintf("call_%d", callID), 7, 9, callID*2-1, callID*2, "completed", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO gw_api_resources (id,public_id,resource_kind,call_id,user_id,token_id) VALUES (?,?,?,?,?,?)`, callID, publicID, "response", callID, 7, 9); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO gw_ai_responses (resource_id,status,result_summary) VALUES (?,?,?)`, callID, "completed", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
