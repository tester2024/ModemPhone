// Package router talks to the TEST-MODEL-1 web interface.
//
// The firmware keeps its SMS pages out of the visible menu but serves them
// anyway, so the app drives them directly. Authentication is the
// challenge/response scheme the firmware expects: it hands out a challenge
// and a pseudo public key, the password is folded into a private key with
// HMAC-MD5, and a second HMAC-MD5 of that private key is posted back.
//
// Every session expires after 170 seconds of inactivity, so the client
// re-authenticates transparently whenever the firmware answers with a
// redirect to the login page.
package router

import (
	"crypto/hmac"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Errors returned by the client.
var (
	ErrAuthFailed  = errors.New("router: authentication rejected")
	ErrNotLoggedIn = errors.New("router: not logged in")
)

// Client is a session with the router. It is safe for concurrent use.
type Client struct {
	base     string
	user     string
	password string

	mu sync.Mutex
	// hc keeps a single connection alive across the login page, the key
	// request and the setup request. The firmware ties a challenge to the
	// connection that asked for it, so these steps must not be split across
	// separate transports, and redirects are never followed automatically
	// because a redirect to the login page is the signal that the session
	// lapsed.
	hc      *http.Client
	cookie  string
	lastReq time.Time
	// lastAuth is when we last completed a login, used to refresh
	// proactively rather than waiting for a rejected request.
	lastAuth time.Time
}

// idleLimit is the firmware's inactivity window. Refreshing a little before it
// expires keeps a long-lived UI from bouncing through the login page.
const idleLimit = 150 * time.Second

// New returns a client for the router at base, for example
// "http://192.168.0.1". It does not connect.
func New(base, user, password string) *Client {
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "http://" + base
	}
	base = strings.TrimRight(base, "/")
	tr := &http.Transport{
		Proxy:               nil, // never send router traffic through a proxy
		MaxIdleConnsPerHost: 1,
		DisableCompression:  true,
		IdleConnTimeout:     90 * time.Second,
	}
	return &Client{
		base:     base,
		user:     user,
		password: password,
		hc: &http.Client{
			Timeout:   20 * time.Second,
			Transport: tr,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func hmacMD5Hex(key, msg string) string {
	m := hmac.New(md5.New, []byte(key))
	io.WriteString(m, msg)
	return strings.ToUpper(hex.EncodeToString(m.Sum(nil)))
}

func (c *Client) urlFor(path string, q url.Values) string {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// do issues a request carrying the session cookie. It does not follow
// redirects: a 302 to the login page is the signal that the session lapsed.
func (c *Client) do(method, path string, form url.Values) (body string, final string, status int, err error) {
	c.mu.Lock()
	cookie := c.cookie
	c.mu.Unlock()

	var rdr io.Reader
	if form != nil {
		rdr = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, c.urlFor(path, nil), rdr)
	if err != nil {
		return "", "", 0, err
	}
	// The firmware's own pages send these, and it is noticeably stricter about
	// requests that do not look like they came from its login page.
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", c.base+"/login.htm")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.Request.URL.String(), resp.StatusCode, err
	}
	loc := resp.Header.Get("Location")
	if resp.StatusCode == http.StatusFound && loc != "" {
		final = loc
	}
	if sc := resp.Header.Get("Set-Cookie"); sc != "" {
		if ck := parseCookie(sc); ck != "" {
			c.mu.Lock()
			c.cookie = ck
			c.mu.Unlock()
		}
	}
	c.mu.Lock()
	c.lastReq = time.Now()
	c.mu.Unlock()
	return string(raw), final, resp.StatusCode, nil
}

func parseCookie(setCookie string) string {
	parts := strings.SplitN(setCookie, ";", 2)
	if len(parts) == 0 {
		return ""
	}
	kv := strings.SplitN(parts[0], "=", 2)
	if len(kv) != 2 {
		return ""
	}
	return strings.TrimSpace(kv[0] + "=" + kv[1])
}

func loginRedirected(final string) bool {
	return strings.Contains(final, "login.htm")
}

// Login performs the challenge/response handshake. The challenge is scoped to
// the connection, so the key request and the setup request must share a
// cookie jar; requesting a key first also establishes the session.
func (c *Client) Login() error {
	// Load the login page first: the firmware ties a challenge to the
	// connection that requested it, so a client that jumps straight to the key
	// request gets a challenge the setup step will reject.
	if err := c.prime(); err != nil {
		return err
	}
	form := url.Values{"username": {c.user}}
	body, _, _, err := c.do(http.MethodPost, "/boafrm/formLoginKey", form)
	if err != nil {
		return err
	}
	// The firmware omits "status" entirely on success and sends status 0 only
	// to reject an unknown user, so the field is decoded as a pointer to tell
	// "rejected" apart from "not mentioned".
	var ch struct {
		Challenge string `json:"Challenge"`
		PublicKey string `json:"PublicKey"`
		Status    *int   `json:"status"`
		Msg       string `json:"msg"`
	}
	if err := decodeJSONLoose(body, &ch); err != nil {
		return fmt.Errorf("router: bad key response: %w", err)
	}
	if ch.Status != nil && *ch.Status == 0 {
		return fmt.Errorf("%w: %s", ErrAuthFailed, ch.Msg)
	}
	if ch.Challenge == "" || ch.PublicKey == "" {
		return errors.New("router: challenge response missing fields")
	}

	private := hmacMD5Hex(ch.PublicKey+c.password, ch.Challenge)
	loginPass := hmacMD5Hex(private, ch.Challenge)

	form = url.Values{"username": {c.user}, "password": {loginPass}}
	_, final, _, err := c.do(http.MethodPost, "/boafrm/formLoginSetup", form)
	if err != nil {
		return err
	}
	if loginRedirected(final) {
		return ErrAuthFailed
	}
	c.mu.Lock()
	c.lastAuth = time.Now()
	c.mu.Unlock()
	return nil
}

// prime loads the login page so that the challenge which follows arrives on a
// connection the firmware is willing to accept.
func (c *Client) prime() error {
	_, _, _, err := c.do(http.MethodGet, "/login.htm", nil)
	return err
}

func (c *Client) sessionFresh() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cookie != "" && time.Since(c.lastAuth) < idleLimit
}

// ensureAuth logs in when there is no session or the session is about to
// expire.
func (c *Client) ensureAuth() error {
	if c.sessionFresh() {
		return nil
	}
	return c.Login()
}

// get fetches a page, transparently re-authenticating once if the session has
// lapsed.
func (c *Client) get(path string) (string, error) {
	if err := c.ensureAuth(); err != nil {
		return "", err
	}
	body, final, _, err := c.do(http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	if loginRedirected(final) {
		if err := c.Login(); err != nil {
			return "", err
		}
		body, final, _, err = c.do(http.MethodGet, path, nil)
		if err != nil {
			return "", err
		}
		if loginRedirected(final) {
			return "", ErrNotLoggedIn
		}
	}
	return body, nil
}

// post submits a form, transparently re-authenticating once.
func (c *Client) post(path string, form url.Values) (string, error) {
	if err := c.ensureAuth(); err != nil {
		return "", err
	}
	body, final, _, err := c.do(http.MethodPost, path, form)
	if err != nil {
		return "", err
	}
	if loginRedirected(final) {
		if err := c.Login(); err != nil {
			return "", err
		}
		body, final, _, err = c.do(http.MethodPost, path, form)
		if err != nil {
			return "", err
		}
		if loginRedirected(final) {
			return "", ErrNotLoggedIn
		}
	}
	return body, nil
}

// Logout drops the session so the next call re-authenticates.
func (c *Client) Logout() {
	c.mu.Lock()
	c.cookie = ""
	c.lastAuth = time.Time{}
	c.mu.Unlock()
}
