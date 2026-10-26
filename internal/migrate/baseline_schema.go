package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

//go:embed clean_baseline_schema.json
var cleanBaselineSchemaJSON []byte

//go:embed retired_schema_extensions.json
var retiredSchemaExtensionsJSON []byte

type baselineSchema struct {
	Schema   int               `json:"schema"`
	Tables   map[string]string `json:"tables"`
	Triggers []baselineTrigger `json:"triggers"`
}

type baselineTrigger struct {
	Name         string `json:"name"`
	Table        string `json:"table"`
	Timing       string `json:"timing"`
	Event        string `json:"event"`
	BodyChecksum string `json:"body_checksum"`
}

var schemaIdentifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
var schemaColumnCharsetPattern = regexp.MustCompile(`^CHARACTER\s+SET\s+([a-zA-Z0-9_]+)\s+COLLATE\s+([a-zA-Z0-9_]+)`)

// canonicalSchemaDDL ignores formatting and data-dependent allocation counters,
// while preserving quoted values, constraints, types, defaults, and comments.
func canonicalSchemaDDL(ddl string) (string, error) {
	ddl = strings.TrimRight(strings.TrimSpace(ddl), ";")
	if strings.HasPrefix(ddl, "CREATE TABLE IF NOT EXISTS ") {
		ddl = "CREATE TABLE " + strings.TrimPrefix(ddl, "CREATE TABLE IF NOT EXISTS ")
	}
	var out strings.Builder
	runes := []rune(ddl)
	var quote rune
	pending := false
	for i := 0; i < len(runes); i++ {
		char := runes[i]
		if quote != 0 {
			out.WriteRune(char)
			if char == '\\' && i+1 < len(runes) {
				i++
				out.WriteRune(runes[i])
			} else if char == quote {
				if i+1 < len(runes) && runes[i+1] == quote {
					i++
					out.WriteRune(runes[i])
				} else {
					quote = 0
				}
			}
		} else if unicode.IsSpace(char) {
			pending = out.Len() > 0
		} else {
			// mysqldump omits a column charset implied by its explicit collation;
			// SHOW CREATE TABLE may print it. These describe the same schema.
			if char == 'C' {
				match := schemaColumnCharsetPattern.FindStringSubmatch(string(runes[i:]))
				if len(match) == 3 && strings.HasPrefix(match[2], match[1]+"_") {
					i += strings.Index(match[0], "COLLATE") - 1
					continue
				}
			}
			if char == 'A' && i > 0 && unicode.IsSpace(runes[i-1]) && strings.HasPrefix(string(runes[i:]), "AUTO_INCREMENT=") {
				end := i + len("AUTO_INCREMENT=")
				start := end
				for end < len(runes) && runes[end] >= '0' && runes[end] <= '9' {
					end++
				}
				if end > start && (end == len(runes) || unicode.IsSpace(runes[end])) {
					i = end - 1
					continue
				}
			}
			if pending {
				out.WriteByte(' ')
				pending = false
			}
			out.WriteRune(char)
			if char == '\'' || char == '"' || char == '`' {
				quote = char
			}
		}
	}
	if quote != 0 {
		return "", fmt.Errorf("unclosed schema SQL quote")
	}
	return out.String(), nil
}

func schemaDDLChecksum(ddl string) (string, error) {
	canonical, err := canonicalSchemaDDL(ddl)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(hash[:]), nil
}

func loadBaselineSchema() (baselineSchema, error) {
	var schema baselineSchema
	if err := json.Unmarshal(cleanBaselineSchemaJSON, &schema); err != nil {
		return schema, fmt.Errorf("decode baseline schema: %w", err)
	}
	if schema.Schema != 1 || len(schema.Tables) == 0 {
		return schema, fmt.Errorf("invalid baseline schema manifest")
	}
	for name, checksum := range schema.Tables {
		if !schemaIdentifierPattern.MatchString(name) || !historyChecksumPattern.MatchString(checksum) {
			return schema, fmt.Errorf("invalid baseline table manifest")
		}
	}
	seen := make(map[string]bool)
	for _, trigger := range schema.Triggers {
		if !schemaIdentifierPattern.MatchString(trigger.Name) || seen[trigger.Name] ||
			schema.Tables[trigger.Table] == "" || !historyChecksumPattern.MatchString(trigger.BodyChecksum) ||
			(trigger.Timing != "BEFORE" && trigger.Timing != "AFTER") ||
			(trigger.Event != "INSERT" && trigger.Event != "UPDATE" && trigger.Event != "DELETE") {
			return schema, fmt.Errorf("invalid baseline trigger manifest")
		}
		seen[trigger.Name] = true
	}
	return schema, nil
}

func validateBaselineSchema(ctx context.Context, connection *sql.Conn) error {
	return validateBaselineSchemaWithExtras(ctx, connection, nil)
}

func validateForwardBaselineSchema(ctx context.Context, connection *sql.Conn, version string) error {
	history, err := parseRetiredHistory(retiredHistoryJSON)
	if err != nil { return err }
	for _, mapping := range history.ForwardMigrations {
		if mapping.ForwardVersion != version { continue }
		var extensions map[string]map[string]string
		if err := json.Unmarshal(retiredSchemaExtensionsJSON, &extensions); err != nil { return err }
		extras, exists := extensions[mapping.RetiredVersion]
		if !exists { return fmt.Errorf("missing forward schema extension") }
		return validateBaselineSchemaWithExtras(ctx, connection, extras)
	}
	return nil
}

func validateBaselineSchemaWithExtras(ctx context.Context, connection *sql.Conn, extras map[string]string) error {
	schema, err := loadBaselineSchema()
	if err != nil {
		return err
	}
	for name, checksum := range extras {
		if !schemaIdentifierPattern.MatchString(name) || !historyChecksumPattern.MatchString(checksum) || schema.Tables[name] != "" {
			return fmt.Errorf("invalid retired schema extension")
		}
		schema.Tables[name] = checksum
	}
	rows, err := connection.QueryContext(ctx, "SELECT TABLE_NAME, TABLE_TYPE FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name <> ? ORDER BY TABLE_NAME", migrationTableName)
	if err != nil {
		return fmt.Errorf("list baseline tables: %w", err)
	}
	names := make([]string, 0, len(schema.Tables))
	for rows.Next() {
		var name, kind string
		if err := rows.Scan(&name, &kind); err != nil {
			rows.Close()
			return err
		}
		if kind != "BASE TABLE" || schema.Tables[name] == "" {
			rows.Close()
			return fmt.Errorf("unexpected schema object %s; baseline adoption refused", name)
		}
		names = append(names, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(names) != len(schema.Tables) {
		return fmt.Errorf("baseline table set mismatch: found %d, expected %d", len(names), len(schema.Tables))
	}
	sort.Strings(names)
	for _, name := range names {
		var actualName, ddl string
		// The identifier is checked against the embedded manifest above.
		if err := connection.QueryRowContext(ctx, "SHOW CREATE TABLE `"+name+"`").Scan(&actualName, &ddl); err != nil {
			return fmt.Errorf("inspect baseline table %s: %w", name, err)
		}
		checksum, err := schemaDDLChecksum(ddl)
		if err != nil || actualName != name || checksum != schema.Tables[name] {
			return fmt.Errorf("baseline structure mismatch for table %s", name)
		}
	}
	return validateBaselineTriggers(ctx, connection, schema.Triggers)
}

func validateBaselineTriggers(ctx context.Context, connection *sql.Conn, expected []baselineTrigger) error {
	rows, err := connection.QueryContext(ctx, "SELECT TRIGGER_NAME, EVENT_OBJECT_TABLE, ACTION_TIMING, EVENT_MANIPULATION, ACTION_STATEMENT FROM information_schema.triggers WHERE trigger_schema = DATABASE() ORDER BY TRIGGER_NAME")
	if err != nil {
		return err
	}
	defer rows.Close()
	byName := make(map[string]baselineTrigger, len(expected))
	for _, trigger := range expected {
		byName[trigger.Name] = trigger
	}
	for rows.Next() {
		var trigger baselineTrigger
		var body string
		if err := rows.Scan(&trigger.Name, &trigger.Table, &trigger.Timing, &trigger.Event, &body); err != nil {
			return err
		}
		checksum, err := schemaDDLChecksum(body)
		trigger.BodyChecksum = checksum
		if wanted, exists := byName[trigger.Name]; err != nil || !exists || trigger != wanted {
			return fmt.Errorf("baseline structure mismatch for trigger %s", trigger.Name)
		}
		delete(byName, trigger.Name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(byName) != 0 {
		return fmt.Errorf("baseline triggers are missing")
	}
	return nil
}
