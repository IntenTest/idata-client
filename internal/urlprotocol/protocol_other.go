//go:build !windows

package urlprotocol

import "errors"

func Register() error {
	return errors.New("idata URL protocol registration is only available on Windows")
}

func Unregister() error {
	return errors.New("idata URL protocol registration is only available on Windows")
}
