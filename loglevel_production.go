//go:build production

package main

import "log/slog"

// logLevel is the level of the runtime's log in a production build: errors
// only.
const logLevel = slog.LevelError
