package install

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/klauspost/compress/zstd"

	"github.com/blaktron/agentry/pkg/manifest"
)

// ShippedPack is one knowledge pack a release ships, as the consent shows
// it: its name and, when the archive's headers gave it, its size.
type ShippedPack struct {
	Name  string `json:"name"`
	Bytes *int64 `json:"bytes,omitempty"`
}

// shippedPackSizes reads a release archive's tar headers (zstd, as the
// registry builds it) and sums, for each shipped pack, the sizes of the
// files of its tree — what the consent shows before anything is unpacked,
// the CLI's rule (agentry-cli#389), replicated because it lives in the
// CLI's package main (D-006). It reads within the install's own bounds,
// since it runs before the consent.
func shippedPackSizes(archive []byte, ships []string) (map[string]int64, error) {
	if len(ships) == 0 {
		return nil, nil
	}
	zr, err := zstd.NewReader(bytes.NewReader(archive), zstd.WithDecoderMaxMemory(64<<20))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	lim := extractLimits()
	// As the extractor bounds it: the total, and the compression ratio.
	limit := lim.MaxTotalBytes + (64 << 20)
	if r := int64(len(archive)) * lim.MaxRatio; lim.MaxRatio > 0 && r < limit {
		limit = r
	}
	tr := tar.NewReader(io.LimitReader(zr, limit))
	out := map[string]int64{}
	var total int64
	for files := 0; ; files++ {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read the archive's entries: %w", err)
		}
		if files >= lim.MaxFiles || hdr.Size < 0 || hdr.Size > lim.MaxTotalBytes-total {
			return nil, errors.New("the archive is past the install's bounds")
		}
		total += hdr.Size
		if name, ok := manifest.ShippedPackFile(ships, strings.TrimPrefix(hdr.Name, "./")); ok && hdr.Typeflag == tar.TypeReg {
			out[name] += hdr.Size
		}
	}
}
