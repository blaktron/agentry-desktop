package hostedrun

// The vault's two write routes, as a token carrying secret:write reaches
// them (agentry-enterprise#251, DEV-165; plan hosted-credentials-vault.md
// D12, D13): a run In Agentry of a release that declares a secret sets
// each one the account does not keep, and deletes the ones it set when the
// run does not start. The list is a signed-in session's alone, so nothing
// here reads one; which names are missing is what run create's 409
// secret-missing says.
//
// A value is sent once, in the body, and is in no URL, no header and no log
// line; every text field of a refusal is scrubbed of an exact copy of it
// before it is returned, should a registry echo the body (D-024's rule for
// the model key).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// secretName is the manifest's grammar for a secret's name, as the vault
// holds it: an environment variable's, at most 128 characters.
var secretName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

// ValidSecretName reports whether n is a name the vault keeps.
func ValidSecretName(n string) bool { return secretName.MatchString(n) }

func (c *Client) secretPath(name string) string {
	return c.base + "/api/v1/x/vault/me/secrets/" + url.PathEscape(name)
}

// SetSecret sets the account's secret name to value; keep says whether the
// account keeps it for later runs (true) or holds it for one run (false),
// and is always sent. A second set of the name replaces it.
func (c *Client) SetSecret(ctx context.Context, name, value string, keep bool) error {
	if !ValidSecretName(name) {
		return fmt.Errorf("a secret's name is an environment variable's: a letter or _, then letters, digits or _, at most 128")
	}
	body, err := json.Marshal(struct {
		Value string `json:"value"`
		Keep  bool   `json:"keep"`
	}{value, keep})
	if err != nil {
		return err
	}
	req, err := c.request(ctx, http.MethodPut, c.secretPath(name), bytes.NewReader(body), "application/json")
	if err != nil {
		return err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("set %s: %w", name, scrub(err, value))
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusCreated {
		return nil
	}
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return scrubRefusal(refusalFrom("set "+name, res.StatusCode, raw), value)
}

// DeleteSecret deletes the account's secret name. A name the account does
// not hold is a refusal with the slug not-found.
func (c *Client) DeleteSecret(ctx context.Context, name string) error {
	if !ValidSecretName(name) {
		return fmt.Errorf("a secret's name is an environment variable's: a letter or _, then letters, digits or _, at most 128")
	}
	req, err := c.request(ctx, http.MethodDelete, c.secretPath(name), nil, "")
	if err != nil {
		return err
	}
	return c.do("delete "+name, req, http.StatusNoContent, nil)
}

// sourceHost is a host a key may be saved for, as the vault holds it:
// lower-case LDH labels (agentry-enterprise#532).
var sourceHost = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)

// SetSourceKey saves value in the account's vault as the key for an input
// source's host (agentry-enterprise#532): kept, used by a hosted run of the
// account's for a source on that host it gives no token for, and by a
// hosted schedule's fires. The host is the one the CLI's parse of a
// source's answer reads, never one the page names. A key the account saved
// from a signed-in session is refused (409 secret-kept): a token may not
// replace it (DEV-165).
func (c *Client) SetSourceKey(ctx context.Context, host, value string) error {
	if !sourceHost.MatchString(host) {
		return fmt.Errorf("no key can be saved in Agentry for the host %q", host)
	}
	body, err := json.Marshal(struct {
		Value string `json:"value"`
	}{value})
	if err != nil {
		return err
	}
	req, err := c.request(ctx, http.MethodPut, c.base+"/api/v1/x/vault/me/source-keys/"+url.PathEscape(host), bytes.NewReader(body), "application/json")
	if err != nil {
		return err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("save the key for %s: %w", host, scrub(err, value))
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusCreated {
		return nil
	}
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return scrubRefusal(refusalFrom("save the key for "+host, res.StatusCode, raw), value)
}

// scrubRefusal removes value from every field a refusal shows.
func scrubRefusal(r *Refusal, value string) *Refusal {
	if value == "" {
		return r
	}
	for _, f := range []*string{&r.Op, &r.Slug, &r.Title, &r.Detail, &r.Tier, &r.ResetsAt} {
		*f = strings.ReplaceAll(*f, value, "…")
	}
	return r
}

// scrub removes value from a transport error's text.
func scrub(err error, value string) error {
	if value == "" || !strings.Contains(err.Error(), value) {
		return err
	}
	return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), value, "…"))
}
