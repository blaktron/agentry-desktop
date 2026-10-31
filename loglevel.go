//go:build !production

package main

import "log/slog"

// logLevel is the level of the runtime's log: everything from info up while
// developing.
const logLevel = slog.LevelInfo
