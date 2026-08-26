//go:build !windows

package pairingprompt

import (
	"context"
	"errors"
)

func Confirm(context.Context, Request) (bool, error) {
	return false, errors.New("browser pairing confirmation is only available on Windows")
}
