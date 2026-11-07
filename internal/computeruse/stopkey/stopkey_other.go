//go:build !linux && !darwin && !windows

package stopkey

import (
	"context"
	"errors"
)

func bind(context.Context, func()) (Binding, error) {
	return nil, errors.New("this version of Agentry Desktop binds the stop key on Linux, macOS and Windows only")
}
