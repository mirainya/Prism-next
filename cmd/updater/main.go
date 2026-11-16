// The updater runs as a separate, deployment-installed systemd oneshot service.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mirainya/Prism/internal/update"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err := update.RunSupervisor(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
