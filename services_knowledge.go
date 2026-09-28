// KnowledgeAPI: the Knowledge panel (plan knowledge.md §4.3, §4.8;
// agentry-desktop#273, D-039). The packs are the local store's, which the
// CLI shares (internal/knowledge over agentry-cli's pkg/knowledge/store):
// list, Create knowledge and Add documents with the built-in Knowledge
// Creator on this computer, Upload to Agentry and Download, Export
// unencrypted…, Delete, and a page preview. The gates are the run
// surface's (D-030): documents come only from the native picker, an export
// and a delete are asked in a native dialog, an export's folder is the
// picker's by construction, and a build takes the run lock, so it waits
// on no run and no run waits on it.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	kn "github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/knowledge/store"

	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// eventKnowledgeLog carries a Creator build's progress, line by line.
const eventKnowledgeLog = "knowledge:build-log"

// KnowledgeAPI is the Knowledge panel's bound surface.
type KnowledgeAPI struct {
	app *App
	// run is the run surface: a build holds its lock and is tracked as
	// the run in flight, so quitting ends it and sweeps what it started.
	run *RunAPI
	// build is the Creator seam a test substitutes; nil is the CLI's
	// pkg/knowledge/build.
	build knowledge.Builder
	// openFolder reveals an exported folder; nil is the file manager.
	openFolder func(dir string) error
	// hosted is the build In Agentry this app follows
	// (services_knowledge_hosted.go).
	hosted hostedSlot
}

func (k *KnowledgeAPI) env() knowledge.Env { return k.run.knowledge() }

// List is the panel: the packs kept on this computer and, signed in to
// reg, the ones its account keeps in Agentry.
func (k *KnowledgeAPI) List(reg string) (*knowledge.List, error) {
	return k.env().List(k.app.ctx, reg)
}

// CreatorModel names the model the runner policy gives the Knowledge
// Creator, for the sentence the review step says before a build.
func (k *KnowledgeAPI) CreatorModel() string { return knowledge.PolicyModel() }

// Create makes a new pack named name from the documents with the
// Knowledge Creator on this computer and keeps it sealed. Every document
// must be one the native picker returned. The build takes the run lock;
// its progress streams as knowledge:build-log.
func (k *KnowledgeAPI) Create(reg, name string, documents []string) (*knowledge.BuildResult, error) {
	return k.runBuild(reg, name, documents, false)
}

// Add extends the kept pack name with the documents, under the key it had.
func (k *KnowledgeAPI) Add(reg, name string, documents []string) (*knowledge.BuildResult, error) {
	return k.runBuild(reg, name, documents, true)
}

func (k *KnowledgeAPI) runBuild(reg, name string, documents []string, add bool) (*knowledge.BuildResult, error) {
	for _, p := range documents {
		if err := k.app.requirePicked("document", p); err != nil {
			return nil, err
		}
	}
	// A build is a run: the Creator's containers carry the run id, so
	// quitting mid-build ends it and sweeps them (services_quit.go).
	var res *knowledge.BuildResult
	err := k.run.asRun("the Knowledge Creator", eventKnowledgeLog, func(ctx context.Context, runID string, logw io.Writer) error {
		req := knowledge.BuildRequest{Registry: reg, Name: name, Documents: documents, RunID: runID, Build: k.build, Log: logw}
		var err error
		if add {
			res, err = k.env().Add(ctx, req)
		} else {
			res, err = k.env().Create(ctx, req)
		}
		return err
	})
	return res, err
}

// Upload keeps a copy of the pack in Agentry, where it is scanned.
func (k *KnowledgeAPI) Upload(reg, name string) (string, error) {
	return k.env().Upload(k.app.ctx, reg, name)
}

// Download keeps Agentry's copy of the pack on this computer.
func (k *KnowledgeAPI) Download(reg, name string) (string, error) {
	return k.env().Download(k.app.ctx, reg, name)
}

// DownloadShared keeps a copy of org's shared pack on this computer
// (Knowledge M8): its key stays the organization's, asked for at every
// open, so it opens only while the person is a member and signed in.
func (k *KnowledgeAPI) DownloadShared(reg, org, name string) (string, error) {
	return k.env().DownloadShared(k.app.ctx, reg, org, name)
}

// ExportView is where an export wrote the pack.
type ExportView struct {
	Dir string `json:"dir"`
}

// Export writes the pack out unencrypted, in this order (plan §4.8): a
// native dialog asks, then the native folder picker chooses where, then the
// pack is written into a new folder named for it there, and the file
// manager shows that folder. The desktop never opens a page (D-035). A No
// or a cancelled picker writes nothing.
func (k *KnowledgeAPI) Export(reg, name string) (*ExportView, error) {
	en, err := k.env().Find(reg, name)
	if err != nil {
		return nil, err
	}
	shared := ""
	if en.Key.Org != "" {
		shared = "It is " + en.Key.Org + "'s shared pack: Agentry exports it, to the organization's admins and, once an admin marks it exportable, to its members, and the copy you save stays yours if you leave. "
	}
	if err := k.app.askConsent("Export "+name+" unencrypted?",
		"This writes the knowledge pack "+name+" ("+kn.CountPages(en.Manifest.Pages)+") and the documents it was made from, in the clear, into a folder you choose. "+
			shared+"The copy there is not encrypted: anyone who can read that folder can read it. Continue?"); err != nil {
		return nil, err
	}
	parent, err := k.app.chooseDirectory("Export " + name + " into…")
	if err != nil {
		return nil, err
	}
	if parent == "" {
		return nil, errors.New("export cancelled: no folder was chosen")
	}
	dir, err := k.env().Export(k.app.ctx, reg, name, parent)
	if err != nil {
		return nil, err
	}
	open := k.openFolder
	if open == nil {
		open = openFolderInFileManager
	}
	if err := open(dir); err != nil {
		return &ExportView{Dir: dir}, fmt.Errorf("exported to %s, but the file manager could not show it: %w", dir, err)
	}
	return &ExportView{Dir: dir}, nil
}

// Delete removes the pack from this computer — the sealed tree, its
// wrapped key and the key the keychain caches — after a native dialog;
// with everywhere, Agentry's copy too. The dialog says when this
// computer's copy is the only one.
func (k *KnowledgeAPI) Delete(reg, name string, everywhere bool) error {
	en, err := k.env().Find(reg, name)
	if err != nil {
		return err
	}
	msg := "This deletes the knowledge pack " + name + " from this computer."
	switch {
	case everywhere && en.Key.Agentry != nil:
		msg = "This deletes the knowledge pack " + name + " from this computer and from Agentry. Neither copy can be recovered."
	case en.Key.Agentry == nil:
		msg += " It is the only copy: Agentry keeps none, and it cannot be recovered."
	default:
		msg += " Agentry keeps its copy, which Download brings back."
	}
	if err := k.app.askConsent("Delete "+name+"?", msg+" Continue?"); err != nil {
		return err
	}
	return k.env().Delete(k.app.ctx, reg, name, everywhere)
}

// Preview is the pack's index and the pages it has, for the preview. The
// pack is opened into a 0700 directory inside the store and removed
// before the answer returns.
func (k *KnowledgeAPI) Preview(reg, name string) (*knowledge.Preview, error) {
	return k.env().Preview(k.app.ctx, reg, name)
}

// ReadPage is one page of the pack, as the preview lists it.
func (k *KnowledgeAPI) ReadPage(reg, name, page string) (*knowledge.Page, error) {
	return k.env().ReadPage(k.app.ctx, reg, name, page)
}

// rewrapKnowledge moves the packs this computer's key wraps to the account
// signed in at reg (D20), best effort: a sign-in is never refused for it,
// and a pack not moved now moves at the next build or upload.
func rewrapKnowledge(ctx context.Context, env knowledge.Env, reg string) ([]string, string) {
	moved, err := env.Rewrap(ctx, settings.Normalize(reg))
	if err != nil && !errors.Is(err, store.ErrSignedOut) {
		return moved, "Some knowledge packs are still encrypted to this computer's key: " + err.Error()
	}
	return moved, ""
}
