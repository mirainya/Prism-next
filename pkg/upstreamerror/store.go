package upstreamerror

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"sync"
	"time"
)

// Serializes refresh and save so an older refresh cannot replace a newly saved rule.
var storeMu sync.Mutex

func read(ctx context.Context, db *sql.DB) (*compiledRules, error) {
	var rules Rules
	var patterns []byte
	if err := db.QueryRowContext(ctx, `SELECT patterns,message FROM gw_upstream_error_rules WHERE id=1`).Scan(&patterns, &rules.Message); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(patterns, &rules.Patterns); err != nil {
		return nil, err
	}
	return compile(rules)
}

func Read(ctx context.Context, db *sql.DB) (Rules, error) {
	compiled, err := read(ctx, db)
	if err != nil {
		return Rules{}, err
	}
	return compiled.rules, nil
}

func Refresh(ctx context.Context, db *sql.DB) error {
	storeMu.Lock()
	defer storeMu.Unlock()
	compiled, err := read(ctx, db)
	if err != nil {
		return err
	}
	activeRules.Store(compiled)
	return nil
}

func Save(ctx context.Context, db *sql.DB, rules Rules) error {
	compiled, err := compile(rules)
	if err != nil {
		return err
	}
	patterns, err := json.Marshal(compiled.rules.Patterns)
	if err != nil {
		return err
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	_, err = db.ExecContext(ctx, `INSERT INTO gw_upstream_error_rules(id,patterns,message) VALUES(1,?,?) ON DUPLICATE KEY UPDATE patterns=VALUES(patterns),message=VALUES(message)`, patterns, rules.Message)
	if err != nil {
		return err
	}
	activeRules.Store(compiled)
	return nil
}

// Start loads the database rules before serving traffic and refreshes other instances every 30 seconds.
func Start(db *sql.DB) (func(), error) {
	ctx, cancel := context.WithCancel(context.Background())
	initialCtx, initialCancel := context.WithTimeout(ctx, 5*time.Second)
	err := Refresh(initialCtx, db)
	initialCancel()
	if err != nil {
		cancel()
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refreshCtx, refreshCancel := context.WithTimeout(ctx, 5*time.Second)
				err := Refresh(refreshCtx, db)
				refreshCancel()
				if err != nil && ctx.Err() == nil {
					log.Printf("failed to refresh upstream error rules: %v", err)
				}
			}
		}
	}()
	return func() { cancel(); <-done }, nil
}
