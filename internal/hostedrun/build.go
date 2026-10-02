package hostedrun

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A knowledge build In Agentry (plan knowledge.md §4.5, §4.8; Knowledge M6,
// agentry-desktop#274): the built-in Knowledge Creator run on a sandbox
// host against the platform's model, for credits, its pack kept in Agentry
// and scanned there. It is a run of kind knowledge_build in the same runs
// extension, so it shares a run's one active row per account, its input
// upload and its caps; what differs is what a token needs, what it holds,
// and that its result is a pack rather than outputs.

// The PAT scopes a build needs beside run:read and run:write: it reads and
// writes a knowledge pack, and its output holds the pack in the clear,
// sealed to its runner (agentry-enterprise#171).
const (
	ScopeKnowledgeRead  = "knowledge:read"
	ScopeKnowledgeWrite = "knowledge:write"
)

// BuildFacts are what the app knows about a registry before it offers a
// build In Agentry.
type BuildFacts struct {
	// SignedIn is true when this registry has a stored credential.
	SignedIn bool
	// Knowledge and Runs are true when /meta lists the `knowledge` and
	// the `runs` extension.
	Knowledge bool
	Runs      bool
	// Scopes are the token's scopes; ScopesKnown is false when they could
	// not be read.
	Scopes      []string
	ScopesKnown bool
	// Price is the status route's knowledgeBuild block, nil when the route
	// did not answer it; StatusKnown is false when the route did not answer.
	Price       *BuildPrice
	StatusKnown bool
	// Active is the account's run or build in flight, nil for none.
	Active *Run
	// Allowed is the registry's caller.build.allowed: whether this
	// account's payer may create knowledge in Agentry, Pro and above
	// (DEV-162, agentry-desktop#328). nil from a registry that predates it,
	// which offered every tier a build.
	Allowed *bool
}

// BuildIsProSentence is the site's sentence for a payer the registry does
// not let build (agentry-web#587), so every client says the same thing.
const BuildIsProSentence = "Creating knowledge in Agentry is part of Pro, Business and Enterprise."

// DecideBuild applies the rule. Signed out, or a registry without the
// knowledge extension, means the choice is absent and Create knowledge is
// the local build it always was. Otherwise the choice appears, and says in
// a sentence why it cannot be chosen when it cannot: the words are a run's
// wherever the reason is the same (Decide).
func DecideBuild(f BuildFacts) Offer {
	if !f.SignedIn || !f.Knowledge {
		return Offer{}
	}
	o := Offer{Offered: true}
	switch {
	case !f.Runs:
		o.Reason = "This registry does not run anything In Agentry, so a pack is made on this computer here."
	case !f.ScopesKnown:
		o.Reason = "This registry did not say what the stored token may do, so the app cannot tell whether it may start a build."
		o.Remedy = "Sign in again from Settings."
	case !has(f.Scopes, ScopeRunWrite) || !has(f.Scopes, ScopeRunRead) || !has(f.Scopes, ScopeKnowledgeRead) || !has(f.Scopes, ScopeKnowledgeWrite):
		o.Reason = "The token stored for this registry does not carry " + ScopeRunRead + ", " + ScopeRunWrite + ", " +
			ScopeKnowledgeRead + " and " + ScopeKnowledgeWrite + ", which a build In Agentry needs. It was minted before this app asked for them."
		o.Remedy = "Sign in again from Settings to mint a token that carries them."
	case !f.StatusKnown:
		o.Reason = "The registry did not answer whether it is taking hosted runs."
		o.Remedy = "Try again, or build it on this computer."
	case f.Price == nil || !f.Price.Enabled:
		o.Reason = "This registry is not taking knowledge builds at the moment."
		o.Remedy = "Build it on this computer, or try again later."
	case f.Allowed != nil && !*f.Allowed:
		o.Reason = BuildIsProSentence
		o.Remedy = "Create it on this computer with your own model provider, for no credits, then Upload to Agentry; or upgrade to Pro."
		o.Upgrade = true
	case f.Active != nil:
		o.Reason = "This account already has " + ActiveWhat(f.Active) + " In Agentry in flight. Agentry runs one at a time."
		o.Remedy = "Wait for it to finish, or cancel it."
	default:
		o.Available = true
	}
	return o
}

// CanContinue reports whether a build stopped short of its work — at its
// turn, cost or time limit, interrupted, cancelled, or finished over a pack that
// took other pages while it ran — so a new build may continue it from the
// pack as it is now, with its documents (the registry copies them).
func CanContinue(r *Run) bool {
	if !r.IsBuild() {
		return false
	}
	if r.Status == StatusCancelled {
		return true
	}
	if r.Status != StatusFinished {
		return false
	}
	switch r.Outcome {
	case "budget", "timeout", "interrupted":
		return true
	}
	return r.Build != nil && r.Build.Note == "changed"
}

// NoteSentence says why a finished build's pages are not the pack's
// (x-runs.yaml KnowledgeBuild.note), "" for a note this app does not know.
// The settlement is said apart, from the run's own numbers.
func NoteSentence(note string) string {
	switch note {
	case "nothing":
		return "The build wrote nothing new, so the pack is as it was and was not scanned again."
	case "changed":
		return "The pack took other pages while this build ran, so the build's pages were not put over them. Continue starts a new build from the pack as it is now, with these documents."
	case "allocation":
		return "The pages this build wrote would pass your knowledge allocation, so they were not kept."
	case "deleted":
		return "The pack was deleted while this build ran, so its pages had nowhere to go."
	case "invalid":
		return "The build's output held no pages Agentry could keep. That is the platform's failure, so the hold was refunded in full."
	}
	return ""
}

// StageDocuments turns what the person picked — files and folders — into
// the files a build uploads: a folder is every regular file under it,
// hidden ones and links aside, as the local build reads a folder. A build
// sees its documents in one folder by name, so two with one name are
// refused, and the caps are a run's (§4.4), named in a build's words.
func StageDocuments(paths []string) ([]InputFile, error) {
	var files []string
	for _, p := range paths {
		info, err := os.Lstat(p)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		switch {
		case info.IsDir():
			var found []string
			err := filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if path != p && strings.HasPrefix(d.Name(), ".") {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				if d.Type().IsRegular() {
					found = append(found, path)
				}
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("read the folder %s: %w", p, err)
			}
			sort.Strings(found)
			files = append(files, found...)
		case info.Mode().IsRegular():
			files = append(files, p)
		default:
			return nil, fmt.Errorf("%s is not a file or a folder of files", p)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("the documents chosen hold no files to build from")
	}
	if len(files) > MaxInputFiles {
		return nil, fmt.Errorf("a build In Agentry takes at most %d documents; these come to %d files", MaxInputFiles, len(files))
	}
	var total int64
	names := map[string]string{}
	out := make([]InputFile, 0, len(files))
	for _, p := range files {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		name := filepath.Base(p)
		if other, dup := names[name]; dup {
			return nil, fmt.Errorf("two documents are both named %s (%s and %s): rename one, since the build reads them from one folder", name, other, p)
		}
		names[name] = p
		total += info.Size()
		if total > MaxInputBytes {
			return nil, fmt.Errorf("documents are at most %d MB together for a build In Agentry; these come to %.1f MB",
				MaxInputBytes>>20, float64(total)/float64(1<<20))
		}
		out = append(out, InputFile{Path: p, Name: name, Size: info.Size()})
	}
	return out, nil
}
