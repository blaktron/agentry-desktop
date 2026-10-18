//go:build windows

package runfile

// noFollowOpenFlags: Windows has no O_NOFOLLOW; Open's per-part Lstat and
// its same-file check after the open refuse a link or a junction.
const noFollowOpenFlags = 0
