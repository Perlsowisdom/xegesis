//go:build !store

package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/update/updatesig"
)

// DefaultEndpoint is the one address the updater asks: the latest published release's manifest.
// Draft and pre-release releases are never "latest", so they are not offered.
const DefaultEndpoint = "https://github.com/Perlsowisdom/xegesis/releases/latest/download/latest.json"

const (
	maxManifest  = 1 << 20
	maxInstaller = 500 << 20
)

//go:embed pubkey.txt
var pubkeyText string

// PlaceholderKey reports whether the embedded public key is still the placeholder: no update can
// verify against it, so updating does nothing until it is replaced (docs/releasing.md).
func PlaceholderKey() bool { return strings.Contains(pubkeyText, "PLACEHOLDER") }

// Manifest is latest.json, published with each release.
type Manifest struct {
	Version   string `json:"version" desc:"The release, e.g. 1.2.3"`
	URL       string `json:"url" desc:"Where the installer is"`
	SHA256    string `json:"sha256" desc:"The installer's SHA-256, hex"`
	Signature string `json:"signature" desc:"Ed25519 signature (base64) over the version and SHA-256"`
	Notes     string `json:"notes,omitempty" desc:"What changed"`
}

// ParseManifest reads and checks the shape of latest.json. allowHTTP is for tests.
func ParseManifest(data []byte, allowHTTP bool) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("the update manifest is not valid: %w", err)
	}
	if !parseVersion(m.Version).ok {
		return Manifest{}, fmt.Errorf("the update manifest has no usable version (%q)", m.Version)
	}
	u, err := url.Parse(m.URL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(allowHTTP && u.Scheme == "http")) {
		return Manifest{}, fmt.Errorf("the update manifest has no usable https address (%q)", m.URL)
	}
	if m.Signature == "" {
		return Manifest{}, errors.New("the update manifest is not signed")
	}
	if raw, err := hex.DecodeString(m.SHA256); err != nil || len(raw) != sha256.Size {
		return Manifest{}, fmt.Errorf("the update manifest has no usable SHA-256 (%q)", m.SHA256)
	}
	m.SHA256 = strings.ToLower(m.SHA256)
	return m, nil
}

// Config says where updates come from and how they are checked. Tests replace the parts that would
// touch the network or start programs.
type Config struct {
	Endpoint  string
	PubKey    ed25519.PublicKey
	Current   string
	HTTP      *http.Client
	AllowHTTP bool // tests only: serve from http://127.0.0.1
	TempDir   string
	Launch    func(installer string) error
	Now       func() time.Time
}

// DefaultConfig is the real updater: the public endpoint, the embedded key, this build's version.
func DefaultConfig() Config {
	key, _ := updatesig.ParsePublicKey(pubkeyText)
	return Config{Endpoint: DefaultEndpoint, PubKey: key, Current: core.Version, Launch: launchInstaller}
}

func (c Config) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 10 * time.Minute}
}

func (c Config) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c Config) get(ctx context.Context, address string, limit int64) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Xegesis")
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s answered %s", req.URL.Host, resp.Status)
	}
	return struct {
		io.Reader
		io.Closer
	}{io.LimitReader(resp.Body, limit+1), resp.Body}, nil
}

// Check fetches latest.json. It sends one HTTP GET and nothing else.
func (c Config) Check(ctx context.Context) (Manifest, error) {
	body, err := c.get(ctx, c.Endpoint, maxManifest)
	if err != nil {
		return Manifest{}, err
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		return Manifest{}, err
	}
	if len(data) > maxManifest {
		return Manifest{}, errors.New("the update manifest is too large")
	}
	return ParseManifest(data, c.AllowHTTP)
}

// Download fetches the installer of m into a new temporary folder and returns its path. The file is
// kept only if its SHA-256 equals the manifest's and the signature verifies against the app's key;
// otherwise it is deleted and an error says which check failed.
func (c Config) Download(ctx context.Context, m Manifest) (string, error) {
	if len(c.PubKey) != ed25519.PublicKeySize {
		return "", errors.New("this build has no update key")
	}
	if err := updatesig.Verify(c.PubKey, m.Version, m.SHA256, m.Signature); err != nil {
		return "", fmt.Errorf("refusing the update: %w", err) // before downloading anything large
	}
	body, err := c.get(ctx, m.URL, maxInstaller)
	if err != nil {
		return "", err
	}
	defer body.Close()
	dir, err := os.MkdirTemp(c.TempDir, "xegesis-update-")
	if err != nil {
		return "", err
	}
	name := path.Base(m.URL)
	if name == "." || name == "/" || filepath.Base(name) != name {
		name = "xegesis-setup.exe"
	}
	file := filepath.Join(dir, name)
	fail := func(err error) (string, error) {
		os.RemoveAll(dir)
		return "", err
	}
	f, err := os.Create(file)
	if err != nil {
		return fail(err)
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fail(err)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if st, err := os.Stat(file); err != nil || st.Size() > maxInstaller {
		return fail(errors.New("the installer is too large"))
	}
	if sum != m.SHA256 {
		return fail(errors.New("the download does not match its SHA-256; it was discarded"))
	}
	// The signature was checked for this hash above; the file's hash equals it.
	return file, nil
}
