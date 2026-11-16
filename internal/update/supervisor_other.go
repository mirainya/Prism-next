//go:build !linux

package update

import (
	"context"
	"errors"
)

func RunSupervisor(context.Context) error {
	return errors.New("online installation requires Linux systemd")
}
