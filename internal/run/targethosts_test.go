package run

// Target hosts in the desktop's run setup (agentry-desktop#812): each host
// is readied by the CLI's own targethost.Prepare against a real SSH server,
// with the sign-in the setup gave (never a variable of the process's
// environment) or the one saved in the keychain; a host key no file holds
// is put to the native dialog with its type and SHA256 fingerprint and,
// accepted, kept in the runner home's file; a changed key refuses with both
// fingerprints; a key file the picker did not return refuses; and nothing
// the run writes or says carries a credential.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/ssh"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/intake"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/targethost"
)

const (
	testHostPassword = "the-hosts-password-7f3a"
	testSudoPassword = "the-sudo-password-c21e"
	testPassphrase   = "the-keys-passphrase-90bd"
)

// sshHost is an SSH server that offers a host key and lets no one in: the
// readying reads the key and signs in to nothing.
type sshHost struct {
	addr netip.Addr
	port int
	key  ssh.Signer
}

// nonLoopback is an IPv4 address of this machine that is not loopback or
// link-local, which a target host may be (plan D8); the test is skipped on
// a machine with none.
func nonLoopback(t *testing.T) netip.Addr {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skipf("no interface addresses: %v", err)
	}
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip, ok := netip.AddrFromSlice(n.IP)
		if ok && ip.Unmap().Is4() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
			return ip.Unmap()
		}
	}
	t.Skip("this machine has no address but loopback; a target host may not be loopback (plan D8)")
	return netip.Addr{}
}

func newSigner(t *testing.T) ssh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func startSSHHost(t *testing.T) *sshHost {
	t.Helper()
	ip := nonLoopback(t)
	ln, err := net.Listen("tcp", net.JoinHostPort(ip.String(), "0"))
	if err != nil {
		t.Skipf("listen on %s: %v", ip, err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	h := &sshHost{addr: ip, port: ln.Addr().(*net.TCPAddr).Port, key: newSigner(t)}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				cfg := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
					return nil, errors.New("no")
				}}
				cfg.AddHostKey(h.key)
				_, _, _, _ = ssh.NewServerConn(c, cfg)
			}()
		}
	}()
	return h
}

func (h *sshHost) answer() string {
	return "admin@" + h.addr.String() + ":" + strconv.Itoa(h.port)
}

func (h *sshHost) fingerprint() string { return ssh.FingerprintSHA256(h.key.PublicKey()) }

// keyFile writes an ed25519 private key, locked with passphrase when it is
// not "", and returns its path.
func keyFile(t *testing.T, passphrase string) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var b *pem.Block
	if passphrase == "" {
		b, err = ssh.MarshalPrivateKey(priv, "")
	} else {
		b, err = ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
	}
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(p, pem.EncodeToMemory(b), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// hostKeychain isolates the run's home, the keychain and the recent hosts.
func hostKeychain(t *testing.T) string {
	t.Helper()
	home := isolateRunEnv(t)
	keyring.MockInit()
	t.Setenv(modelkey.EnvNoKeyring, "")
	return home
}

// dialog is the native dialog: it records each prompt and answers yes.
type dialog struct {
	asked  []string
	answer bool
}

func (d *dialog) trust(prompt string) (bool, error) {
	d.asked = append(d.asked, prompt)
	return d.answer, nil
}

// readyOne readies one host question, "server", answered by answer.
func readyOne(t *testing.T, home, answer string, given map[string]HostSignIn, picked func(string, string) error, d *dialog, keep bool) (*intake.Answers, string, error) {
	t.Helper()
	a := &intake.Answers{Hosts: map[string]*targethost.Host{}}
	ans, err := targethost.ParseAnswer(answer)
	if err != nil {
		t.Fatal(err)
	}
	a.Hosts["server"] = &targethost.Host{ID: "server", Answer: ans}
	var said strings.Builder
	r := hostReadiness{pol: policy.Default(), home: home, given: given, picked: picked, say: func(s string) { said.WriteString(s + "\n") }, keep: keep}
	if d != nil {
		r.trust = d.trust
	}
	err = readyTargetHosts(context.Background(), a, []string{"server"}, r)
	return a, said.String(), err
}

func pickedOnly(paths ...string) func(string, string) error {
	return func(what, p string) error {
		for _, q := range paths {
			if p == q {
				return nil
			}
		}
		return fmt.Errorf("%s %q was not chosen through the file dialog", what, p)
	}
}

func noCredential(t *testing.T, what, s string) {
	t.Helper()
	for _, v := range []string{testHostPassword, testSudoPassword, testPassphrase, "PRIVATE KEY"} {
		if strings.Contains(s, v) {
			t.Fatalf("%s carries a credential (%q): %s", what, v, s)
		}
	}
}

func reasonOf(t *testing.T, err error) (int, *receipt.Reason) {
	t.Helper()
	var ce *engine.ClassifiedError
	if !errors.As(err, &ce) {
		t.Fatalf("not classified: %v", err)
	}
	return ce.Code, receipt.ReasonOf(err)
}

// An unknown host key is put to the dialog with its type and fingerprint;
// accepted, it is kept in the runner home's file, and the next run asks
// nothing. The sign-in is the setup's key file, read by the CLI's code.
func TestATargetHostKeyIsAcceptedInTheDialogAndKeptForTheNextRun(t *testing.T) {
	home := hostKeychain(t)
	h := startSSHHost(t)
	key := keyFile(t, "")
	given := map[string]HostSignIn{"server": {KeyFile: key, Sudo: testSudoPassword}}
	d := &dialog{answer: true}

	a, said, err := readyOne(t, home, h.answer(), given, pickedOnly(key), d, false)
	if err != nil {
		t.Fatalf("ready: %v\n%s", err, said)
	}
	if len(d.asked) != 1 || !strings.Contains(d.asked[0], "ED25519 "+h.fingerprint()) || !strings.Contains(d.asked[0], "Accept it") {
		t.Fatalf("the dialog was asked %q; want the key's type and fingerprint %s", d.asked, h.fingerprint())
	}
	got := a.Hosts["server"]
	if got.HostKey.From != targethost.TrustedNow || got.Credential.Auth != "key" || got.Credential.KeyFile != key ||
		got.Credential.From != targethost.FromPrompt || got.Credential.Sudo != testSudoPassword || got.Credential.SudoFrom != targethost.FromPrompt {
		t.Fatalf("host %+v, credential from %q, sudo from %q", got.HostKey, got.Credential.From, got.Credential.SudoFrom)
	}
	b, err := os.ReadFile(filepath.Join(home, "known_hosts"))
	if err != nil || !strings.Contains(string(b), "["+h.addr.String()+"]:"+strconv.Itoa(h.port)) {
		t.Fatalf("the runner's known_hosts %q (%v)", b, err)
	}
	// The receipt line says what the run signed in with, in the CLI's
	// words, never a value.
	line := got.ReceiptLine(0, 0)
	if !strings.Contains(line, "typed at the run's start") || strings.Contains(line, "given by a flag") {
		t.Fatalf("receipt line %q", line)
	}
	noCredential(t, "the receipt line", line)
	noCredential(t, "what the readying said", said)
	if os.Getenv("AGENTRY_DESKTOP_HOST_SUDO_server") != "" {
		t.Fatal("a sign-in was put in the process's environment")
	}
	if r := RecentHosts(); len(r) != 1 || r[0] != h.answer() {
		t.Fatalf("recent hosts %q", r)
	}

	d2 := &dialog{answer: true}
	a, _, err = readyOne(t, home, h.answer(), given, pickedOnly(key), d2, false)
	if err != nil || len(d2.asked) != 0 || a.Hosts["server"].HostKey.From != targethost.TrustRunnersFile {
		t.Fatalf("the second run: %v, asked %q", err, d2.asked)
	}
}

// A key the person declines, and one no one is there to accept, refuse the
// run: host-refused, host-key-unknown (exit 11).
func TestAnUnknownHostKeyNotAcceptedRefusesTheRun(t *testing.T) {
	home := hostKeychain(t)
	h := startSSHHost(t)
	given := map[string]HostSignIn{"server": {Password: testHostPassword}}
	for name, d := range map[string]*dialog{"declined": {answer: false}, "no one to ask": nil} {
		t.Run(name, func(t *testing.T) {
			_, _, err := readyOne(t, home, h.answer(), given, nil, d, false)
			code, r := reasonOf(t, err)
			if code != engine.ExitPolicy || r == nil || r.Code != receipt.ReasonHostRefused || r.Detail != receipt.HostKeyUnknown {
				t.Fatalf("exit %d, reason %+v: %v", code, r, err)
			}
			noCredential(t, "the refusal", err.Error())
		})
	}
	if _, err := os.Stat(filepath.Join(home, "known_hosts")); !os.IsNotExist(err) {
		t.Fatalf("a key no one accepted was kept: %v", err)
	}
}

// A host that offers another key than the one on file refuses the run,
// naming both fingerprints, whatever the dialog would say.
func TestAChangedHostKeyRefusesWithBothFingerprints(t *testing.T) {
	home := hostKeychain(t)
	h := startSSHHost(t)
	given := map[string]HostSignIn{"server": {Password: testHostPassword}}
	if _, _, err := readyOne(t, home, h.answer(), given, nil, &dialog{answer: true}, false); err != nil {
		t.Fatal(err)
	}
	was := h.fingerprint()
	h.key = newSigner(t)
	d := &dialog{answer: true}
	_, _, err := readyOne(t, home, h.answer(), given, nil, d, false)
	code, r := reasonOf(t, err)
	if code != engine.ExitPolicy || r == nil || r.Detail != receipt.HostKeyChanged {
		t.Fatalf("exit %d, reason %+v: %v", code, r, err)
	}
	if !strings.Contains(err.Error(), was) || !strings.Contains(err.Error(), h.fingerprint()) || len(d.asked) != 0 {
		t.Fatalf("the refusal %q names not both %s and %s, or the dialog was asked %q", err, was, h.fingerprint(), d.asked)
	}
}

// The sign-in: a password typed here, a locked key with its passphrase, a
// locked key without one, nothing at all, a key file the picker did not
// return, and a sign-in for a question that is no target host.
func TestATargetHostSignsInWithWhatTheSetupGaveAndRefusesWhatItCannot(t *testing.T) {
	home := hostKeychain(t)
	h := startSSHHost(t)
	locked := keyFile(t, testPassphrase)

	a, _, err := readyOne(t, home, h.answer(), map[string]HostSignIn{"server": {Password: testHostPassword}}, nil, &dialog{answer: true}, false)
	if c := a.Hosts["server"].Credential; err != nil || c.Auth != "password" || c.Password != testHostPassword || c.Sudo != "" {
		t.Fatalf("a password: %+v %v", c.Auth, err)
	}
	a, _, err = readyOne(t, home, h.answer(), map[string]HostSignIn{"server": {KeyFile: locked, Passphrase: testPassphrase}}, pickedOnly(locked), nil, false)
	if c := a.Hosts["server"].Credential; err != nil || c.Auth != "key" || c.Passphrase != testPassphrase {
		t.Fatalf("a locked key with its passphrase: %v", err)
	}

	for name, c := range map[string]struct {
		given  map[string]HostSignIn
		picked func(string, string) error
		code   int
		detail string
		says   string
	}{
		"a locked key, no passphrase":   {map[string]HostSignIn{"server": {KeyFile: locked}}, pickedOnly(locked), engine.ExitPolicy, receipt.HostCredential, "has a passphrase"},
		"the wrong passphrase":          {map[string]HostSignIn{"server": {KeyFile: locked, Passphrase: "nope"}}, pickedOnly(locked), engine.ExitPolicy, receipt.HostCredential, "does not unlock"},
		"nothing given, nothing saved":  {nil, nil, engine.ExitPolicy, receipt.HostCredential, "no sign-in was given"},
		"a key the picker did not give": {map[string]HostSignIn{"server": {KeyFile: locked}}, pickedOnly(), engine.ExitUsage, "", "file dialog"},
		"a sign-in for no target host":  {map[string]HostSignIn{"other": {Password: testHostPassword}}, nil, engine.ExitCLI, "", "not one of this run's target hosts"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := readyOne(t, home, h.answer(), c.given, c.picked, &dialog{answer: true}, false)
			code, r := reasonOf(t, err)
			if code != c.code || !strings.Contains(err.Error(), c.says) {
				t.Fatalf("exit %d: %v; want exit %d saying %q", code, err, c.code, c.says)
			}
			if c.detail != "" && (r == nil || r.Code != receipt.ReasonHostRefused || r.Detail != c.detail) {
				t.Fatalf("reason %+v, want %s", r, c.detail)
			}
			noCredential(t, "the refusal", err.Error())
		})
	}
}

// Save keeps the sign-in in the CLI's keychain entry, host:<user>@<host>:
// <port>, as the key's path, never its bytes; the next run with nothing
// typed signs in with it; the status says it is saved and never returns
// it; and Forget removes it.
func TestASavedSignInIsTheCLIsEntryAndTheNextRunUsesIt(t *testing.T) {
	home := hostKeychain(t)
	h := startSSHHost(t)
	key := keyFile(t, "")
	st := GetTargetHostStatus(h.answer())
	if st.Problem != "" || st.Saved || st.Account != "host:admin@"+h.addr.String()+":"+strconv.Itoa(h.port) {
		t.Fatalf("status %+v", st)
	}
	_, _, err := readyOne(t, home, h.answer(), map[string]HostSignIn{"server": {KeyFile: key, Sudo: testSudoPassword, Save: true}}, pickedOnly(key), &dialog{answer: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	v, err := keyring.Get(modelkey.Service, st.Account)
	if err != nil || !strings.Contains(v, key) || strings.Contains(v, "PRIVATE KEY") {
		t.Fatalf("the keychain entry %q (%v)", v, err)
	}
	if st := GetTargetHostStatus(h.answer()); !st.Saved {
		t.Fatalf("status after the save %+v", st)
	}
	noCredential(t, "the status", fmt.Sprintf("%+v", GetTargetHostStatus(h.answer())))

	a, _, err := readyOne(t, home, h.answer(), nil, nil, nil, false)
	if c := a.Hosts["server"].Credential; err != nil || c.From != targethost.FromKeychain || c.KeyFile != key || c.Sudo != testSudoPassword {
		t.Fatalf("a run with nothing typed: %v, from %q", err, c.From)
	}
	if err := ForgetHostSignIn(h.answer()); err != nil {
		t.Fatal(err)
	}
	if st := GetTargetHostStatus(h.answer()); st.Saved {
		t.Fatal("the sign-in is still saved")
	}
	if err := ForgetHostSignIn(h.answer()); err != nil {
		t.Fatalf("forgetting nothing: %v", err)
	}
}

// A schedule keeps the sign-in whatever Save says (plan D12).
func TestAScheduleKeepsTheSignIn(t *testing.T) {
	home := hostKeychain(t)
	h := startSSHHost(t)
	if _, _, err := readyOne(t, home, h.answer(), map[string]HostSignIn{"server": {Password: testHostPassword}}, nil, &dialog{answer: true}, true); err != nil {
		t.Fatal(err)
	}
	if !GetTargetHostStatus(h.answer()).Saved {
		t.Fatal("a schedule's sign-in was not kept")
	}
}

func TestAHostAnswerIsReadWithTheCLIsParser(t *testing.T) {
	hostKeychain(t)
	for answer, want := range map[string]string{
		"admin:pw@server":   "password",
		"server/path":       "is not a host",
		"[2001:db8::1":      "no closing bracket",
		"admin@server:0":    "is not a port",
		"admin@2001:db8::1": "in brackets",
	} {
		if st := GetTargetHostStatus(answer); !strings.Contains(st.Problem, want) || st.Account != "" {
			t.Errorf("%q: %+v, want a problem saying %q", answer, st, want)
		}
	}
	st := GetTargetHostStatus("Admin@Server.Example.com")
	if st.Problem != "" || st.Answer != "Admin@Server.Example.com" || st.Port != 22 || st.Account != "host:Admin@server.example.com:22" {
		t.Fatalf("%+v", st)
	}
	t.Setenv(modelkey.EnvNoKeyring, "1")
	if st := GetTargetHostStatus("admin@server"); !st.KeychainOff || st.Saved {
		t.Fatalf("keychain off: %+v", st)
	}
}

// The recent hosts hold addresses only, newest first, ten at most, and a
// line the run would not take is not offered.
func TestRecentHostsAreAddressesNewestFirst(t *testing.T) {
	hostKeychain(t)
	for i := range 12 {
		a, _ := targethost.ParseAnswer(fmt.Sprintf("admin@host%d", i))
		if err := rememberHost(a); err != nil {
			t.Fatal(err)
		}
	}
	a, _ := targethost.ParseAnswer("admin@host3")
	_ = rememberHost(a)
	r := RecentHosts()
	if len(r) != maxRecentHosts || r[0] != "admin@host3" || r[1] != "admin@host11" {
		t.Fatalf("recent %q", r)
	}
	if err := ForgetRecentHost("admin@host3"); err != nil || RecentHosts()[0] != "admin@host11" {
		t.Fatalf("forget: %v %q", err, RecentHosts())
	}
	p, _ := recentHostsPath()
	if err := os.WriteFile(p, []byte(`{"hosts":["admin:pw@x","ok@host"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := RecentHosts(); len(r) != 1 || r[0] != "ok@host" {
		t.Fatalf("recent from a hand-edited file %q", r)
	}
}

// A run or a schedule In Agentry of a release with a target host is
// refused, naming it (plan D11).
func TestARunInAgentryOfATargetHostReleaseIsRefused(t *testing.T) {
	m := &manifest.Manifest{Capabilities: manifest.Capabilities{TargetHosts: []string{"server"}}}
	if err := RefuseHostedTargetHosts(m); err == nil || !strings.Contains(err.Error(), "only a run on this computer") {
		t.Fatalf("%v", err)
	}
	if err := RefuseHostedTargetHosts(&manifest.Manifest{}); err != nil {
		t.Fatal(err)
	}
}

const hostRunManifest = `schemaVersion: "2"
name: server-helper
namespace: acme
modes: [autonomous]
version: 1.0.0
summary: "desktop run fixture that asks for a server"
entrypoints:
  instruction: SKILL.md
intake:
  - id: server
    prompt: "The server to set up"
    type: host
    required: true
capabilities:
  filesystem:
    read: workspace
    write: workspace
  targetHosts: [server]
publish:
  include: [agent.yaml, SKILL.md]
`

// A run prepared from the desktop's request readies its target host before
// the engine: the engine is handed the host with its address, its key and
// the setup's sign-in, and the log and the plan carry no credential.
func TestAPreparedRunGivesTheEngineTheReadiedTargetHost(t *testing.T) {
	hostKeychain(t)
	saveCataloguedPolicy(t)
	h := startSSHHost(t)
	fr := &fakeRunRegistry{
		manifestDoc: manifestDocFromYAML(t, hostRunManifest),
		archive: buildRunArchive(t, map[string]string{
			"agent.yaml": hostRunManifest,
			"SKILL.md":   "# Server helper\n\nSet the server up.\n",
		}),
	}
	regURL := startRunRegistry(t, fr)
	fake := readyImagesFake()
	scriptHappyAgent(fake)
	withFakeDocker(t, fake)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-key")

	d := &dialog{answer: true}
	req := request(regURL, filepath.Join(t.TempDir(), "out"))
	req.Intake = map[string]string{"server": h.answer()}
	req.HostSignIns = map[string]HostSignIn{"server": {Password: testHostPassword, Sudo: testSudoPassword}}
	req.TrustHostKey = d.trust
	var log strings.Builder
	p, err := prepare(context.Background(), req, &log, manifest.ModeAutonomous)
	if err != nil {
		t.Fatalf("prepare: %v\n%s", err, log.String())
	}
	got := p.params.Intake.Hosts["server"]
	if got == nil || got.Address != h.addr || got.HostKey.Key == nil || got.Credential.Password != testHostPassword || got.Credential.Sudo != testSudoPassword {
		t.Fatalf("the engine's host %+v", got)
	}
	if len(d.asked) != 1 || p.plan.HostLink == nil {
		t.Fatalf("dialog asked %d times, host link %v", len(d.asked), p.plan.HostLink)
	}
	noCredential(t, "the log", log.String())
	noCredential(t, "the plan", fmt.Sprintf("%+v", *p.plan))

	// A run with no sign-in and none saved is refused before the engine.
	req.HostSignIns = nil
	_, err = prepare(context.Background(), req, &strings.Builder{}, manifest.ModeAutonomous)
	if code, r := reasonOf(t, err); code != engine.ExitPolicy || r == nil || r.Detail != receipt.HostCredential {
		t.Fatalf("no sign-in: exit %d %+v: %v", code, r, err)
	}
}

// The dialog is titled "Accept this host key?": only the CLI's host key
// question reaches it, never another of its confirms.
func TestOnlyAHostKeyQuestionReachesTheDialog(t *testing.T) {
	d := &dialog{answer: true}
	a := setupAsker{trust: d.trust}
	if ok, _ := a.Confirm("Save this sign-in for admin@h in the system keychain, for later runs?", false); ok || len(d.asked) != 0 {
		t.Fatalf("another question reached the dialog: %v %q", ok, d.asked)
	}
	if ok, _ := a.Confirm("The host key of admin@h (192.0.2.1, port 22) is not known:\n  ED25519 SHA256:abc\nAccept it?", false); !ok || len(d.asked) != 1 {
		t.Fatalf("the host key question: %v %q", ok, d.asked)
	}
}
