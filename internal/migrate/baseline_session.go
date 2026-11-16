package migrate

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"
)

// A dump contains session settings needed for FK creation and trigger parsing.
// Restore them even if a multi-statement installation fails partway through;
// sql.Conn.Close alone would return a modified connection to the pool.
func executeCleanBaseline(ctx context.Context, connection *sql.Conn, content string) (resultErr error) {
	var foreignKeys, uniqueChecks, notes int
	var mode, zone, charsetClient, collation string
	var charsetResults sql.NullString
	if err := connection.QueryRowContext(ctx,
		"SELECT @@SESSION.foreign_key_checks, @@SESSION.unique_checks, @@SESSION.sql_notes, @@SESSION.sql_mode, @@SESSION.time_zone, @@SESSION.character_set_client, @@SESSION.character_set_results, @@SESSION.collation_connection",
	).Scan(&foreignKeys, &uniqueChecks, &notes, &mode, &zone, &charsetClient, &charsetResults, &collation); err != nil {
		return fmt.Errorf("read baseline session settings: %w", err)
	}
	defer func() {
		restoreContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := connection.ExecContext(restoreContext,
			"SET SESSION foreign_key_checks = ?, SESSION unique_checks = ?, SESSION sql_notes = ?, SESSION sql_mode = ?, SESSION time_zone = ?, SESSION character_set_client = ?, SESSION character_set_results = ?, SESSION collation_connection = ?",
			foreignKeys, uniqueChecks, notes, mode, zone, charsetClient, charsetResults, collation,
		)
		if err != nil {
			// Do not return an unsafe session to the connection pool.
			_ = connection.Raw(func(any) error { return driver.ErrBadConn })
			resultErr = errors.Join(resultErr, fmt.Errorf("restore baseline session settings: %w", err))
		}
	}()
	_, resultErr = connection.ExecContext(ctx, content)
	return resultErr
}
