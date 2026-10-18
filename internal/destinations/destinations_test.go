package destinations

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/destination"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
)

// env points the CLI's configuration folder and the keychain at throwaway state.
func env(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(configdir.EnvVar, dir)
	t.Setenv(modelkey.EnvNoKeyring, "")
	keyring.MockInit()
	t.Cleanup(keyring.MockInit)
	return dir
}

func TestASavedDestinationIsTheCLIsAndItsCredentialsNeverComeBack(t *testing.T) {
	dir := env(t)
	err := Save(Input{Name: "n8n", URL: "https://hooks.example.com/webhook/abc?token=URLTOKEN", Secret: "whsec_TYPED", Authorization: "Bearer TYPEDAUTH"})
	if err != nil {
		t.Fatal(err)
	}
	// The CLI's own readers see what the app saved.
	r, err := destination.Resolve([]string{"n8n"})
	if err != nil || len(r) != 1 || r[0].Secret != "whsec_TYPED" || r[0].Authorization != "Bearer TYPEDAUTH" || r[0].URL != "https://hooks.example.com/webhook/abc?token=URLTOKEN" {
		t.Fatalf("%+v %v", r, err)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, destination.File)); strings.Contains(string(raw), "TYPED") {
		t.Errorf("the file holds a credential: %s", raw)
	}
	st, err := Load()
	if err != nil || len(st.Destinations) != 1 {
		t.Fatalf("%+v %v", st, err)
	}
	b, _ := json.Marshal(st)
	for _, bad := range []string{"TYPED", "URLTOKEN", "/webhook/", "token="} {
		if strings.Contains(string(b), bad) {
			t.Errorf("the view carries %q: %s", bad, b)
		}
	}
	v := st.Destinations[0]
	if !v.HasSecret || !v.HasAuthorization || v.Where != "https://hooks.example.com" || v.DefaultFor == nil {
		t.Errorf("view: %+v", v)
	}
}

func TestSavingAgainKeepsTheCredentialsUnlessTheHostChanges(t *testing.T) {
	env(t)
	if err := Save(Input{Name: "n8n", URL: "https://one.example.com/a", Secret: "s1", Authorization: "Bearer A"}); err != nil {
		t.Fatal(err)
	}
	// A change of path keeps both.
	if err := Save(Input{Name: "n8n", URL: "https://one.example.com/b"}); err != nil {
		t.Fatal(err)
	}
	if s, a, _ := destination.Secrets("n8n"); s != "s1" || a != "Bearer A" {
		t.Errorf("after a path change: %q %q", s, a)
	}
	// A new secret replaces the secret and keeps the value.
	if err := Save(Input{Name: "n8n", URL: "https://one.example.com/b", Secret: "s2"}); err != nil {
		t.Fatal(err)
	}
	if s, a, _ := destination.Secrets("n8n"); s != "s2" || a != "Bearer A" {
		t.Errorf("after a new secret: %q %q", s, a)
	}
	// Removing the value is asked for.
	if err := Save(Input{Name: "n8n", URL: "https://one.example.com/b", ClearAuthorization: true}); err != nil {
		t.Fatal(err)
	}
	if _, a, _ := destination.Secrets("n8n"); a != "" {
		t.Errorf("a value kept after it was cleared: %q", a)
	}
	if err := Save(Input{Name: "n8n", URL: "https://one.example.com/b", Authorization: "Bearer B"}); err != nil {
		t.Fatal(err)
	}
	// Another host: the value saved for the first is not sent to it.
	if err := Save(Input{Name: "n8n", URL: "https://two.example.com/b"}); err != nil {
		t.Fatal(err)
	}
	if s, a, _ := destination.Secrets("n8n"); s != "s2" || a != "" {
		t.Errorf("after a change of host: %q %q", s, a)
	}
}

func TestASaveTheCLIWouldRefuseIsRefused(t *testing.T) {
	env(t)
	for _, in := range []Input{
		{Name: "a", URL: "http://hooks.example.com/x", Secret: "s"},
		{Name: "a", URL: "https://10.0.0.5/x", Secret: "s"},
		{Name: "a", URL: "https://127.0.0.1:8443/x", OnMyNetwork: true, Secret: "s"},
		{Name: "a", URL: "https://user:pw@hooks.example.com/x", Secret: "s"},
		{Name: "Bad Name", URL: "https://hooks.example.com/x", Secret: "s"},
		{Name: "a", URL: "https://hooks.example.com/x"},
		{Name: "a", URL: "https://hooks.example.com/x", Secret: "s", Authorization: "Bearer x\r\nX: y"},
	} {
		if err := Save(in); err == nil {
			t.Errorf("%+v was saved", in)
		}
	}
	if st, _ := Load(); len(st.Destinations) != 0 {
		t.Errorf("a refused save kept %+v", st.Destinations)
	}
	if destination.HasSecret("a") {
		t.Error("a refused save kept a secret")
	}
	if got := Check("https://10.0.0.5/x", false); !strings.Contains(got, "On my network") {
		t.Errorf("Check: %q", got)
	}
	if got := Check("https://10.0.0.5/x", true); got != "" {
		t.Errorf("Check on my network: %q", got)
	}
}

func TestRemoveTakesTheCredentialsAndDefaultsWithIt(t *testing.T) {
	env(t)
	if err := Save(Input{Name: "n8n", URL: "https://hooks.example.com/x", Secret: "s", Authorization: "Bearer A"}); err != nil {
		t.Fatal(err)
	}
	if err := SetDefault("acme/weekly", []string{"n8n"}); err != nil {
		t.Fatal(err)
	}
	if got := DefaultFor("acme/weekly@1.0.0"); len(got) != 1 || got[0] != "n8n" {
		t.Errorf("default: %v", got)
	}
	if st, _ := Load(); len(st.Destinations[0].DefaultFor) != 1 {
		t.Errorf("view: %+v", st.Destinations)
	}
	if err := Remove("n8n"); err != nil {
		t.Fatal(err)
	}
	if destination.HasSecret("n8n") || destination.HasAuthorization("n8n") || len(DefaultFor("acme/weekly")) != 0 {
		t.Error("a removed destination left something behind")
	}
	if err := Remove("n8n"); err == nil {
		t.Error("removing a destination twice succeeded")
	}
	if got := DefaultFor("acme/other"); got == nil {
		t.Error("the default set is nil, which marshals as null")
	}
}

func TestTheKeychainBeingOffRefusesASave(t *testing.T) {
	env(t)
	t.Setenv(modelkey.EnvNoKeyring, "1")
	if err := Save(Input{Name: "a", URL: "https://hooks.example.com/x", Secret: "s"}); err == nil {
		t.Error("a destination was saved with the keychain off")
	}
	if st, _ := Load(); !st.KeychainOff {
		t.Error("the status does not say the keychain is off")
	}
}

// TestAnAuthorizationValueStaysWithTheReceiverItWasSavedFor: the same host and
// port under a weaker scheme, or under a wider rule for an address, is another
// receiver, and the saved value does not go to it.
func TestAnAuthorizationValueStaysWithTheReceiverItWasSavedFor(t *testing.T) {
	env(t)
	for name, c := range map[string]struct{ first, second Input }{
		"a scheme downgrade": {
			Input{Name: "a", URL: "https://hooks.example.com/a", Secret: "s", Authorization: "Bearer A"},
			Input{Name: "a", URL: "http://hooks.example.com:443/a", OnMyNetwork: true},
		},
		"a wider rule": {
			Input{Name: "a", URL: "https://hooks.example.com/a", Secret: "s", Authorization: "Bearer A"},
			Input{Name: "a", URL: "https://hooks.example.com/a", OnMyNetwork: true},
		},
		"another port": {
			Input{Name: "a", URL: "https://nas.lan:8443/a", OnMyNetwork: true, Secret: "s", Authorization: "Bearer A"},
			Input{Name: "a", URL: "https://nas.lan:9443/a", OnMyNetwork: true},
		},
	} {
		keyring.MockInit()
		_ = Remove("a")
		if err := Save(c.first); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := Save(c.second); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, a, _ := destination.Secrets("a"); a != "" {
			t.Errorf("%s: the saved value went to the other receiver", name)
		}
	}
}

// TestAFailedSaveOfASavedDestinationLeavesItsCredentialsAsTheyWere: a change
// that cannot be written to the file puts the keychain back.
func TestAFailedSaveOfASavedDestinationLeavesItsCredentialsAsTheyWere(t *testing.T) {
	dir := env(t)
	if err := Save(Input{Name: "a", URL: "https://hooks.example.com/a", Secret: "old", Authorization: "Bearer OLD"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Skip("cannot make the folder read-only here")
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if f, err := os.CreateTemp(dir, "probe"); err == nil {
		_ = f.Close()
		t.Skip("the folder is writable whatever its mode here")
	}
	if err := Save(Input{Name: "a", URL: "https://other.example.com/a", Secret: "new"}); err == nil {
		t.Fatal("the save did not fail")
	}
	if s, a, _ := destination.Secrets("a"); s != "old" || a != "Bearer OLD" {
		t.Errorf("the keychain after a failed save: %q %q", s, a)
	}
	// A new name leaves nothing.
	if err := Save(Input{Name: "b", URL: "https://hooks.example.com/b", Secret: "s"}); err == nil {
		t.Fatal("the save did not fail")
	}
	if destination.HasSecret("b") {
		t.Error("a failed save of a new destination kept its secret")
	}
}

func bucketInput(over func(*Input)) Input {
	in := Input{Name: "reports", Kind: "s3", Bucket: "my-reports", Prefix: "agentry", Region: "eu-west-1", AccessKeyID: "AKIATYPEDKEYID", Secret: "TYPEDSECRETACCESSKEY"}
	if over != nil {
		over(&in)
	}
	return in
}

func TestASavedBucketIsTheCLIsAndItsKeyPairNeverComesBack(t *testing.T) {
	dir := env(t)
	if err := Save(bucketInput(nil)); err != nil {
		t.Fatal(err)
	}
	r, err := destination.Resolve([]string{"reports"})
	if err != nil || len(r) != 1 || r[0].Kind != destination.KindS3 || r[0].AccessKeyID != "AKIATYPEDKEYID" || r[0].Secret != "TYPEDSECRETACCESSKEY" ||
		r[0].Bucket != "my-reports" || r[0].Prefix != "agentry" || r[0].Region != "eu-west-1" {
		t.Fatalf("%+v %v", r, err)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, destination.File)); strings.Contains(string(raw), "TYPED") {
		t.Errorf("the file holds a credential: %s", raw)
	}
	st, err := Load()
	if err != nil || len(st.Destinations) != 1 {
		t.Fatalf("%+v %v", st, err)
	}
	b, _ := json.Marshal(st)
	for _, bad := range []string{"TYPED", "AKIA"} {
		if strings.Contains(string(b), bad) {
			t.Errorf("the view carries %q: %s", bad, b)
		}
	}
	v := st.Destinations[0]
	if v.Kind != "s3" || !v.HasSecret || !v.HasAccessKey || v.HasAuthorization || !v.CanSend || v.Bucket != "my-reports" || v.Prefix != "agentry" || v.Region != "eu-west-1" ||
		v.Where != "s3://my-reports/agentry at https://my-reports.s3.eu-west-1.amazonaws.com:443" {
		t.Errorf("view: %+v", v)
	}
}

func TestABucketAtAnEndpointDefaultsItsRegionAndTakesTheTick(t *testing.T) {
	env(t)
	if err := Save(bucketInput(func(in *Input) {
		in.Name, in.Region, in.Prefix, in.Endpoint, in.OnMyNetwork = "minio", "", "/a/b/", "http://192.168.1.20:9000", true
	})); err != nil {
		t.Fatal(err)
	}
	r, err := destination.Resolve([]string{"minio"})
	if err != nil || r[0].Region != "us-east-1" || r[0].Prefix != "a/b" || r[0].Endpoint != "http://192.168.1.20:9000" || !r[0].OnMyNetwork {
		t.Fatalf("%+v %v", r, err)
	}
	st, _ := Load()
	if v := st.Destinations[0]; v.Endpoint != "http://192.168.1.20:9000" || !v.OnMyNetwork {
		t.Errorf("view: %+v", v)
	}
}

func TestABucketTheCLIWouldRefuseIsRefused(t *testing.T) {
	env(t)
	for name, in := range map[string]Input{
		"a private endpoint without the tick": bucketInput(func(in *Input) { in.Endpoint = "https://10.0.0.5:9000" }),
		"a loopback endpoint":                 bucketInput(func(in *Input) { in.Endpoint, in.OnMyNetwork = "http://127.0.0.1:9000", true }),
		"an endpoint with a path":             bucketInput(func(in *Input) { in.Endpoint = "https://s3.example.com/base" }),
		"an endpoint with a login":            bucketInput(func(in *Input) { in.Endpoint = "https://user:pw@s3.example.com" }),
		"the tick without an endpoint":        bucketInput(func(in *Input) { in.OnMyNetwork = true }),
		"a prefix that climbs":                bucketInput(func(in *Input) { in.Prefix = "../elsewhere" }),
		"a bucket with capitals":              bucketInput(func(in *Input) { in.Bucket = "My_Reports" }),
		"no bucket":                           bucketInput(func(in *Input) { in.Bucket = "" }),
		"no region":                           bucketInput(func(in *Input) { in.Region = "" }),
		"no access key id":                    bucketInput(func(in *Input) { in.AccessKeyID = "" }),
		"no secret access key":                bucketInput(func(in *Input) { in.Secret = "" }),
		"a secret with a line break":          bucketInput(func(in *Input) { in.Secret = "abc\r\nx" }),
		"a bad name":                          bucketInput(func(in *Input) { in.Name = "Bad Name" }),
		"another kind":                        bucketInput(func(in *Input) { in.Kind = "ftp" }),
	} {
		if err := Save(in); err == nil {
			t.Errorf("%s was saved", name)
		}
	}
	if st, _ := Load(); len(st.Destinations) != 0 {
		t.Errorf("a refused save kept %+v", st.Destinations)
	}
	if destination.HasSecret("reports") || destination.HasAccessKey("reports") {
		t.Error("a refused save kept a credential")
	}
}

func TestCheckBucketJudgesTheFieldsAsTheyAreTyped(t *testing.T) {
	if got := CheckBucket(Input{Endpoint: "https://10.0.0.5:9000"}); got == "" {
		t.Error("a private endpoint passed before the bucket was typed")
	}
	if got := CheckBucket(Input{Endpoint: "https://10.0.0.5:9000", OnMyNetwork: true}); got != "" {
		t.Errorf("a private endpoint on my network: %q", got)
	}
	if got := CheckBucket(Input{Bucket: "UPPER", Region: "us-east-1"}); got == "" {
		t.Error("a bucket with capitals passed")
	}
	if got := CheckBucket(Input{Bucket: "ok-bucket", Region: "us-east-1", Prefix: "a/../b"}); got == "" {
		t.Error("a climbing prefix passed")
	}
	if got := CheckBucket(Input{Bucket: "ok-bucket", Region: "us-east-1"}); got != "" {
		t.Errorf("a plain bucket: %q", got)
	}
}

// TestAKeyPairStaysWithTheServiceItWasSavedFor: a saved pair is kept only while
// the change leaves the service the same, and a changed one with no new pair
// is refused and leaves what was saved as it was.
func TestAKeyPairStaysWithTheServiceItWasSavedFor(t *testing.T) {
	env(t)
	minio := func(over func(*Input)) Input {
		return bucketInput(func(in *Input) {
			in.Region, in.Endpoint, in.OnMyNetwork = "", "https://nas.lan:9000", true
			if over != nil {
				over(in)
			}
		})
	}
	pair := func() (string, string) {
		id, _ := destination.AccessKeyID("reports")
		s, _, _ := destination.Secrets("reports")
		return id, s
	}
	if err := Save(minio(nil)); err != nil {
		t.Fatal(err)
	}
	// A new bucket and prefix at the same service keep the pair.
	if err := Save(Input{Name: "reports", Kind: "s3", Bucket: "other-bucket", Prefix: "p", Endpoint: "https://nas.lan:9000", OnMyNetwork: true}); err != nil {
		t.Fatalf("a change of bucket at the same service: %v", err)
	}
	if id, s := pair(); id != "AKIATYPEDKEYID" || s != "TYPEDSECRETACCESSKEY" {
		t.Errorf("after a change of bucket: %q %q", id, s)
	}
	for name, change := range map[string]Input{
		"another host":    {Endpoint: "https://elsewhere.example.com:9000", OnMyNetwork: true},
		"another port":    {Endpoint: "https://nas.lan:9001", OnMyNetwork: true},
		"a scheme change": {Endpoint: "http://nas.lan:9000", OnMyNetwork: true},
		"a narrower rule": {Endpoint: "https://nas.example.com:443", OnMyNetwork: false},
		"Amazon itself":   {Region: "eu-west-1"},
		"another network": {Endpoint: "https://nas.lan:9000", OnMyNetwork: false},
	} {
		change.Name, change.Kind, change.Bucket = "reports", "s3", "other-bucket"
		if err := Save(change); err == nil {
			t.Errorf("%s: the pair went to another service without a new one", name)
		}
		if id, s := pair(); id != "AKIATYPEDKEYID" || s != "TYPEDSECRETACCESSKEY" {
			t.Errorf("%s: a refused change altered the pair: %q %q", name, id, s)
		}
		if st, _ := Load(); st.Destinations[0].Endpoint != "https://nas.lan:9000" {
			t.Errorf("%s: a refused change altered the file: %+v", name, st.Destinations[0])
		}
	}
	// Half a pair is not enough for another service, and nothing of the old one stays.
	if err := Save(Input{Name: "reports", Kind: "s3", Bucket: "other-bucket", Endpoint: "https://elsewhere.example.com:9000", OnMyNetwork: true, AccessKeyID: "AKIANEWKEYID"}); err == nil {
		t.Error("a new key id was saved with the old secret")
	}
	// A whole new pair replaces it at the other service, and the old one is gone.
	if err := Save(Input{Name: "reports", Kind: "s3", Bucket: "other-bucket", Endpoint: "https://elsewhere.example.com:9000", OnMyNetwork: true, AccessKeyID: "AKIANEWKEYID", Secret: "NEWSECRET"}); err != nil {
		t.Fatal(err)
	}
	if id, s := pair(); id != "AKIANEWKEYID" || s != "NEWSECRET" {
		t.Errorf("after a new pair: %q %q", id, s)
	}
	// A new secret alone replaces the secret at the same service.
	if err := Save(Input{Name: "reports", Kind: "s3", Bucket: "other-bucket", Endpoint: "https://elsewhere.example.com:9000", OnMyNetwork: true, Secret: "ROTATED"}); err != nil {
		t.Fatal(err)
	}
	if id, s := pair(); id != "AKIANEWKEYID" || s != "ROTATED" {
		t.Errorf("after a rotated secret: %q %q", id, s)
	}
	// Amazon's own regions are one service: an access key reaches every bucket of its account.
	env(t)
	if err := Save(bucketInput(nil)); err != nil {
		t.Fatal(err)
	}
	if err := Save(Input{Name: "reports", Kind: "s3", Bucket: "another", Region: "us-west-2"}); err != nil {
		t.Fatalf("a change of region at Amazon: %v", err)
	}
	if id, s := pair(); id != "AKIATYPEDKEYID" || s != "TYPEDSECRETACCESSKEY" {
		t.Errorf("after a change of region: %q %q", id, s)
	}
}

// TestASecretAccessKeyIsNeverAWebhooksSigningSecretNorTheReverse: saving a name
// again as the other kind keeps no credential of the first, and leaves none a
// delivery would send to a receiver it was not made for.
func TestASecretAccessKeyIsNeverAWebhooksSigningSecretNorTheReverse(t *testing.T) {
	env(t)
	if err := Save(bucketInput(nil)); err != nil {
		t.Fatal(err)
	}
	if err := Save(Input{Name: "reports", URL: "https://hooks.example.com/x"}); err == nil {
		t.Error("a bucket's secret access key was kept as a webhook's signing secret")
	}
	if s, _, _ := destination.Secrets("reports"); s != "TYPEDSECRETACCESSKEY" {
		t.Errorf("a refused change altered the secret: %q", s)
	}
	if err := Save(Input{Name: "reports", URL: "https://hooks.example.com/x", Secret: "whsec"}); err != nil {
		t.Fatal(err)
	}
	if destination.HasAccessKey("reports") {
		t.Error("a webhook kept a bucket's access key id")
	}
	if st, _ := Load(); st.Destinations[0].Kind != "webhook" || st.Destinations[0].Bucket != "" || st.Destinations[0].CanSend != true {
		t.Errorf("view: %+v", st.Destinations[0])
	}
	// And back: a webhook's secret and Authorization value are no bucket's.
	if err := Save(Input{Name: "reports", URL: "https://hooks.example.com/x", Authorization: "Bearer A"}); err != nil {
		t.Fatal(err)
	}
	if err := Save(bucketInput(func(in *Input) { in.AccessKeyID, in.Secret = "", "" })); err == nil {
		t.Error("a webhook's credentials were kept for a bucket")
	}
	if err := Save(bucketInput(nil)); err != nil {
		t.Fatal(err)
	}
	if destination.HasAuthorization("reports") {
		t.Error("a bucket kept a webhook's Authorization value")
	}
	if id, s := func() (string, string) {
		id, _ := destination.AccessKeyID("reports")
		s, _, _ := destination.Secrets("reports")
		return id, s
	}(); id != "AKIATYPEDKEYID" || s != "TYPEDSECRETACCESSKEY" {
		t.Errorf("the pair: %q %q", id, s)
	}
}

func TestABucketIsSentToOnlyWithBothHalvesOfItsKeyPair(t *testing.T) {
	env(t)
	if err := Save(bucketInput(nil)); err != nil {
		t.Fatal(err)
	}
	if !CanSend("reports") || !Sendable()["reports"] {
		t.Fatal("a whole bucket cannot be sent to")
	}
	if err := destination.DeleteSecrets("reports"); err != nil {
		t.Fatal(err)
	}
	if err := destination.SaveS3Credentials("reports", "AKIAONLYTHEID", ""); err != nil {
		t.Fatal(err)
	}
	if CanSend("reports") || Sendable()["reports"] {
		t.Error("a bucket with half its pair can be sent to")
	}
	if st, _ := Load(); st.Destinations[0].CanSend || !st.Destinations[0].HasAccessKey || st.Destinations[0].HasSecret {
		t.Errorf("view: %+v", st.Destinations[0])
	}
	if !UsesBucket([]string{"reports"}) || UsesBucket([]string{"nothing"}) {
		t.Error("UsesBucket")
	}
}

func TestRemovingABucketTakesItsKeyPairToo(t *testing.T) {
	env(t)
	if err := Save(bucketInput(nil)); err != nil {
		t.Fatal(err)
	}
	if err := Remove("reports"); err != nil {
		t.Fatal(err)
	}
	if destination.HasSecret("reports") || destination.HasAccessKey("reports") {
		t.Error("a removed bucket left a credential behind")
	}
}

func TestKeepsKeyPairSaysWhenAChangeWouldKeepTheSavedPair(t *testing.T) {
	env(t)
	if err := Save(bucketInput(func(in *Input) { in.Region, in.Endpoint, in.OnMyNetwork = "", "https://nas.lan:9000", true })); err != nil {
		t.Fatal(err)
	}
	same := Input{Name: "reports", Kind: "s3", Bucket: "another", Endpoint: "https://nas.lan:9000", OnMyNetwork: true}
	if !KeepsKeyPair(same) {
		t.Error("the same service does not keep the pair")
	}
	for name, in := range map[string]Input{
		"another host": {Name: "reports", Bucket: "b", Endpoint: "https://elsewhere.example.com:9000", OnMyNetwork: true},
		"Amazon":       {Name: "reports", Bucket: "b", Region: "eu-west-1"},
		"not saved":    {Name: "nothing", Bucket: "b", Endpoint: "https://nas.lan:9000", OnMyNetwork: true},
	} {
		if KeepsKeyPair(in) {
			t.Errorf("%s keeps the pair", name)
		}
	}
}
