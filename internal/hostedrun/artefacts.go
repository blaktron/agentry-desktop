package hostedrun

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Inputs and outputs (plan §4.4). Files go up before the run is created,
// within the plan's limits, and the run's artefacts come down into the
// folder the person picked — the same folder a local run writes into.

// InputFile is one local file staged for a run.
type InputFile struct {
	// Path is the file on this machine; Name is what the run sees under
	// /inputs.
	Path string
	Name string
	Size int64
}

// StageInputs turns the panel's chosen paths into the uploads a create
// will name, refusing in the app — with the limit named — anything the
// registry would refuse with a 413 (plan §4.4: at most MaxInputFiles
// files, MaxInputBytes together).
func StageInputs(paths []string) ([]InputFile, error) {
	if len(paths) > MaxInputFiles {
		return nil, fmt.Errorf("a run In Agentry takes at most %d input files; %d were chosen", MaxInputFiles, len(paths))
	}
	var total int64
	names := map[string]string{}
	out := make([]InputFile, 0, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("%s is a folder: a run In Agentry takes files, so add the files inside it", p)
		}
		name := filepath.Base(p)
		if other, dup := names[name]; dup {
			return nil, fmt.Errorf("two input files are both named %s (%s and %s) — rename one, since the run sees them in one folder", name, other, p)
		}
		names[name] = p
		total += info.Size()
		if info.Size() > MaxInputBytes || total > MaxInputBytes {
			return nil, fmt.Errorf("input files are at most %d MB together for a run In Agentry; these come to %.1f MB",
				MaxInputBytes>>20, float64(total)/float64(1<<20))
		}
		out = append(out, InputFile{Path: p, Name: name, Size: info.Size()})
	}
	return out, nil
}

// UploadInputs stores each staged file and returns the ids a create names.
// logf, when set, gets one line per file.
func (c *Client) UploadInputs(ctx context.Context, files []InputFile, logf func(string)) ([]string, error) {
	ids := make([]string, 0, len(files))
	for _, f := range files {
		fh, err := os.Open(f.Path) //nolint:gosec // a path the person picked in the file dialog
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.Path, err)
		}
		in, err := c.UploadInput(ctx, f.Name, fh, f.Size)
		_ = fh.Close()
		if err != nil {
			return nil, err
		}
		if logf != nil {
			logf(fmt.Sprintf("uploaded %s (%d bytes)", in.Name, in.Bytes))
		}
		ids = append(ids, in.ID)
	}
	return ids, nil
}

// Downloaded is one artefact that landed on this machine.
type Downloaded struct {
	Name  string
	Path  string
	Bytes int64
}

// OutputPath places an artefact name under dir, refusing any name that
// would land outside it. The registry already keeps names relative; a
// client that writes files from a server's list checks anyway, because
// the check is what makes "into the folder you picked" true.
func OutputPath(dir, name string) (string, error) {
	clean := path.Clean(strings.ReplaceAll(name, "\\", "/"))
	if name == "" || clean == "." || clean == "/" {
		return "", fmt.Errorf("the registry named an artefact with no name")
	}
	if strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, "../") || clean == ".." || filepath.IsAbs(name) {
		return "", fmt.Errorf("refusing to write %q: an artefact name must stay inside the run folder", name)
	}
	local := filepath.FromSlash(clean)
	if !filepath.IsLocal(local) {
		return "", fmt.Errorf("refusing to write %q: an artefact name must stay inside the run folder", name)
	}
	return filepath.Join(dir, local), nil
}

// PushedOutput is the one output a pushed run keeps (agentry-enterprise#83):
// the run's out folder as a tar, sealed to the runner's key by the host
// and opened by the registry as it serves the download. A pull worker
// uploads the folder file by file and never names a file at the top level
// this way, so the name alone says which kind of run this is.
const PushedOutput = "output.tar"

// DownloadOutputs writes every output of a finished run into dir, creating
// the folders the names imply; a pushed run's bundle is unpacked there, so
// either kind of run leaves the same folder behind. It returns what
// landed, sorted by name, and stops at the first refusal so a half-written
// folder is never reported as complete.
func (c *Client) DownloadOutputs(ctx context.Context, run *Run, dir string) ([]Downloaded, error) {
	if run == nil {
		return nil, nil
	}
	got := make([]Downloaded, 0, len(run.Outputs))
	for _, a := range run.Outputs {
		if a.Name == PushedOutput {
			body, err := c.Output(ctx, run.ID, a.Name)
			if err != nil {
				return got, err
			}
			got, err = unpack(body, dir, got)
			_ = body.Close()
			if err != nil {
				return got, err
			}
			continue
		}
		dest, err := OutputPath(dir, a.Name)
		if err != nil {
			return got, err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return got, fmt.Errorf("create %s: %w", filepath.Dir(dest), err)
		}
		body, err := c.Output(ctx, run.ID, a.Name)
		if err != nil {
			return got, err
		}
		n, err := writeFile(dest, body)
		_ = body.Close()
		if err != nil {
			return got, err
		}
		got = append(got, Downloaded{Name: a.Name, Path: dest, Bytes: n})
	}
	sort.SliceStable(got, func(i, j int) bool { return got[i].Name < got[j].Name })
	return got, nil
}

// unpack writes a pushed run's bundle into dir. The host writes regular
// files only, each named relative to the run's out folder; anything else
// — a link, a device, a name that would leave dir, a name twice — is
// refused rather than skipped, because a bundle that carries one was not
// written by a host.
func unpack(body io.Reader, dir string, got []Downloaded) ([]Downloaded, error) {
	tr := tar.NewReader(body)
	seen := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return got, nil
		}
		if err != nil {
			return got, fmt.Errorf("read the run's %s: %w", PushedOutput, err)
		}
		if hdr.Typeflag != tar.TypeReg {
			return got, fmt.Errorf("refusing %q in the run's %s: only files are written, and this is not one", hdr.Name, PushedOutput)
		}
		dest, err := OutputPath(dir, hdr.Name)
		if err != nil {
			return got, err
		}
		if seen[dest] {
			return got, fmt.Errorf("refusing %q in the run's %s: it names the file twice", hdr.Name, PushedOutput)
		}
		seen[dest] = true
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return got, fmt.Errorf("create %s: %w", filepath.Dir(dest), err)
		}
		n, err := writeFile(dest, io.LimitReader(tr, hdr.Size))
		if err != nil {
			return got, err
		}
		got = append(got, Downloaded{Name: path.Clean(hdr.Name), Path: dest, Bytes: n})
	}
}

// writeFile writes a downloaded artefact and returns its size.
func writeFile(dest string, body io.Reader) (int64, error) {
	f, err := os.Create(dest) //nolint:gosec // dest is checked by OutputPath
	if err != nil {
		return 0, fmt.Errorf("write %s: %w", dest, err)
	}
	n, err := io.Copy(f, body)
	cerr := f.Close()
	if err != nil {
		return n, fmt.Errorf("write %s: %w", dest, err)
	}
	if cerr != nil {
		return n, fmt.Errorf("write %s: %w", dest, cerr)
	}
	return n, nil
}
