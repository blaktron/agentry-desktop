package run

import (
	"errors"

	"github.com/blaktron/agentry/pkg/manifest"
)

// RefuseHostedBrowser refuses a run, a session or a schedule In Agentry of
// a release that declares a browser: no run In Agentry carries one until
// the browser testing plan's M7 (agentry-enterprise#565), and the run
// host's CLI refuses it again (engine.HostedBrowserRefusal, agentry-cli
// #915). The words are the site's (agentry-web#1644).
func RefuseHostedBrowser(m *manifest.Manifest) error {
	if m != nil && m.Capabilities.Browser != nil {
		return errors.New("a run In Agentry has no browser yet, so this QuickAgent runs on this computer for now")
	}
	return nil
}
