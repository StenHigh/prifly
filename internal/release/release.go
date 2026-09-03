// Package release builds and verifies the small public distribution contract.
// It is deliberately separate from project packages: a binary update never
// opens or mutates a Pri-Fly authority.
package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	ManifestVersion = "prifly-release/1"
	ReceiptVersion  = "prifly-managed-install/1"
	ReceiptName     = ".prifly-installation.json"
	BinaryName      = "prifly"

	DefaultReleaseBaseURL = "https://github.com/StenHigh/prifly/releases/latest/download"
	MaxManifestBytes      = 1 << 20
	MaxArchiveBytes       = 128 << 20
	MaxBinaryBytes        = 96 << 20
)

// PublicKeyHex is set only in a release build with -ldflags. Development
// builds deliberately refuse network updates rather than trusting a key in the
// source tree or an environment variable.
var PublicKeyHex string

var assetNamePattern = regexp.MustCompile(`^prifly-[a-z0-9]+-[a-z0-9]+\.tar\.gz$`)
var platformPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

type Asset struct {
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Archive string `json:"archive"`
	Binary  string `json:"binary"`
	SHA256  string `json:"sha256"`
}

type Manifest struct {
	SchemaVersion string  `json:"schema_version"`
	Version       string  `json:"version"`
	Stable        bool    `json:"stable"`
	Assets        []Asset `json:"assets"`
}

type Receipt struct {
	SchemaVersion string `json:"schema_version"`
	Binary        string `json:"binary"`
	Channel       string `json:"channel"`
}

type Result struct {
	SchemaVersion   string `json:"schema_version"`
	PreviousVersion string `json:"previous_version"`
	Version         string `json:"version"`
	Updated         bool   `json:"updated"`
	// Source names where this build looked. An installation that predates a
	// move keeps checking the old address and reports no update forever; the
	// address is the only thing that makes that visible without the sources.
	Source string `json:"source"`
}

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type Updater struct {
	ReleaseBaseURL string
	PublicKeyHex   string
	Executable     string
	CurrentVersion string
	OS             string
	Arch           string
	Client         Doer
}

func DefaultUpdater(currentVersion string) Updater {
	return Updater{
		ReleaseBaseURL: DefaultReleaseBaseURL,
		PublicKeyHex:   PublicKeyHex,
		CurrentVersion: currentVersion,
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		Client:         http.DefaultClient,
	}
}

func (u Updater) Update(ctx context.Context) (Result, error) {
	executable, receipt, err := u.managedExecutable()
	if err != nil {
		return Result{}, err
	}
	if receipt.Channel != "stable" {
		return Result{}, errors.New("managed installation has an unsupported update channel")
	}
	if u.CurrentVersion == "" || !validVersion(u.CurrentVersion) {
		return Result{}, errors.New("this binary is not a release version and cannot update itself")
	}
	if u.PublicKeyHex == "" {
		return Result{}, errors.New("public release updates are not configured in this development build")
	}
	manifestBytes, err := u.fetch(ctx, u.assetURL("release-manifest.json"), MaxManifestBytes)
	if err != nil {
		return Result{}, err
	}
	signature, err := u.fetch(ctx, u.assetURL("release-manifest.sig"), 4096)
	if err != nil {
		return Result{}, err
	}
	manifest, canonical, err := parseManifest(manifestBytes)
	if err != nil {
		return Result{}, err
	}
	if !manifest.Stable {
		return Result{}, errors.New("latest release is not a stable release")
	}
	if err := verify(canonical, signature, u.PublicKeyHex); err != nil {
		return Result{}, err
	}
	comparison, err := compareVersions(manifest.Version, u.CurrentVersion)
	if err != nil {
		return Result{}, err
	}
	if comparison <= 0 {
		return Result{SchemaVersion: "prifly-update/1", PreviousVersion: u.CurrentVersion, Version: u.CurrentVersion, Updated: false, Source: u.ReleaseBaseURL}, nil
	}
	asset, ok := assetFor(manifest, u.OS, u.Arch)
	if !ok {
		return Result{}, fmt.Errorf("no release asset supports %s/%s", u.OS, u.Arch)
	}
	archive, err := u.fetch(ctx, u.assetURL(asset.Archive), MaxArchiveBytes)
	if err != nil {
		return Result{}, err
	}
	actual := fmt.Sprintf("%x", sha256.Sum256(archive))
	if actual != asset.SHA256 {
		return Result{}, errors.New("release archive digest does not match the signed manifest")
	}
	binary, err := extractBinary(archive, asset.Binary)
	if err != nil {
		return Result{}, err
	}
	if err := replace(executable, binary); err != nil {
		return Result{}, err
	}
	return Result{SchemaVersion: "prifly-update/1", PreviousVersion: u.CurrentVersion, Version: manifest.Version, Updated: true, Source: u.ReleaseBaseURL}, nil
}

func (u Updater) managedExecutable() (string, Receipt, error) {
	executable := u.Executable
	if executable == "" {
		var err error
		executable, err = os.Executable()
		if err != nil {
			return "", Receipt{}, err
		}
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", Receipt{}, err
	}
	directory := filepath.Dir(resolved)
	receiptBytes, err := os.ReadFile(filepath.Join(directory, ReceiptName))
	if err != nil {
		return "", Receipt{}, errors.New("prifly update requires a binary installed by the official installer")
	}
	var receipt Receipt
	if err := decode(receiptBytes, &receipt); err != nil || receipt.SchemaVersion != ReceiptVersion || receipt.Binary != BinaryName {
		return "", Receipt{}, errors.New("managed installation receipt is invalid")
	}
	expected := filepath.Join(directory, receipt.Binary)
	if resolved != expected {
		return "", Receipt{}, errors.New("prifly update refuses a copied or redirected binary")
	}
	return resolved, receipt, nil
}

func (u Updater) assetURL(name string) string {
	return strings.TrimRight(u.ReleaseBaseURL, "/") + "/" + name
}

func (u Updater) fetch(ctx context.Context, address string, limit int64) ([]byte, error) {
	client := u.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("release download failed: %s", response.Status)
	}
	if response.ContentLength > limit {
		return nil, errors.New("release download exceeds its declared size limit")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("release download exceeds its size limit")
	}
	return data, nil
}

func parseManifest(raw []byte) (Manifest, []byte, error) {
	if len(raw) == 0 || len(raw) > MaxManifestBytes {
		return Manifest{}, nil, errors.New("release manifest exceeds its size limit")
	}
	var manifest Manifest
	if err := decode(raw, &manifest); err != nil {
		return Manifest{}, nil, fmt.Errorf("invalid release manifest: %w", err)
	}
	if manifest.SchemaVersion != ManifestVersion || !stableVersion(manifest.Version) || len(manifest.Assets) == 0 || len(manifest.Assets) > 16 {
		return Manifest{}, nil, errors.New("release manifest has an unsupported shape")
	}
	seen := map[string]bool{}
	for _, asset := range manifest.Assets {
		if !platformPattern.MatchString(asset.OS) || !platformPattern.MatchString(asset.Arch) || !assetNamePattern.MatchString(asset.Archive) || asset.Binary != BinaryName || len(asset.SHA256) != 64 {
			return Manifest{}, nil, errors.New("release manifest contains an invalid asset")
		}
		if _, err := hex.DecodeString(asset.SHA256); err != nil {
			return Manifest{}, nil, errors.New("release manifest contains an invalid archive digest")
		}
		key := asset.OS + "/" + asset.Arch
		if seen[key] {
			return Manifest{}, nil, errors.New("release manifest declares a platform twice")
		}
		seen[key] = true
	}
	canonical, err := json.Marshal(manifest)
	if err != nil {
		return Manifest{}, nil, err
	}
	return manifest, canonical, nil
}

func decode(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("expected one JSON object")
	}
	return nil
}

func verify(manifest, signature []byte, publicKeyHex string) error {
	key, err := hex.DecodeString(publicKeyHex)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return errors.New("release public key is invalid")
	}
	rawSignature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(signature)))
	if err != nil || len(rawSignature) != ed25519.SignatureSize {
		return errors.New("release signature is invalid")
	}
	if !ed25519.Verify(ed25519.PublicKey(key), manifest, rawSignature) {
		return errors.New("release manifest signature does not verify")
	}
	return nil
}

func assetFor(manifest Manifest, osName, arch string) (Asset, bool) {
	for _, asset := range manifest.Assets {
		if asset.OS == osName && asset.Arch == arch {
			return asset, true
		}
	}
	return Asset{}, false
}

func extractBinary(archive []byte, name string) ([]byte, error) {
	compressed, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, errors.New("release archive is not a gzip stream")
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	var binary []byte
	entries := 0
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid release archive: %w", err)
		}
		entries++
		if entries > 1 || header.Typeflag != tar.TypeReg || header.Name != name || header.Size < 1 || header.Size > MaxBinaryBytes {
			return nil, errors.New("release archive must contain exactly one bounded prifly binary")
		}
		binary, err = io.ReadAll(io.LimitReader(reader, header.Size+1))
		if err != nil || int64(len(binary)) != header.Size {
			return nil, errors.New("release archive binary is truncated")
		}
	}
	if len(binary) == 0 {
		return nil, errors.New("release archive does not contain prifly")
	}
	return binary, nil
}

func replace(target string, binary []byte) (err error) {
	temporary, err := os.CreateTemp(filepath.Dir(target), ".prifly-update-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(temporaryName)
		}
	}()
	if _, err = temporary.Write(binary); err != nil {
		_ = temporary.Close()
		return err
	}
	if err = temporary.Chmod(0755); err != nil {
		_ = temporary.Close()
		return err
	}
	if err = temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, target)
}

type BuildOptions struct {
	Assets        []BuildAsset
	Installer     string
	Output        string
	Version       string
	PrivateKeyHex string
	PublicKeyHex  string
}

type BuildAsset struct {
	Binary string
	OS     string
	Arch   string
}

func Build(options BuildOptions) (Manifest, error) {
	if len(options.Assets) == 0 || options.Output == "" || !stableVersion(options.Version) {
		return Manifest{}, errors.New("release build requires assets, output and semantic version")
	}
	private, err := hex.DecodeString(options.PrivateKeyHex)
	if err != nil || len(private) != ed25519.PrivateKeySize {
		return Manifest{}, errors.New("release build requires an ed25519 private key in hex")
	}
	public := ed25519.PrivateKey(private).Public().(ed25519.PublicKey)
	if options.PublicKeyHex == "" || !strings.EqualFold(options.PublicKeyHex, hex.EncodeToString(public)) {
		return Manifest{}, errors.New("release build public key does not match the signing key")
	}
	if err := os.MkdirAll(options.Output, 0755); err != nil {
		return Manifest{}, err
	}
	paths := []string{filepath.Join(options.Output, "release-manifest.json"), filepath.Join(options.Output, "release-manifest.sig")}
	seen := map[string]bool{}
	assets := append([]BuildAsset(nil), options.Assets...)
	sort.Slice(assets, func(i, j int) bool {
		return assets[i].OS+"/"+assets[i].Arch < assets[j].OS+"/"+assets[j].Arch
	})
	for _, asset := range assets {
		if asset.Binary == "" || !platformPattern.MatchString(asset.OS) || !platformPattern.MatchString(asset.Arch) {
			return Manifest{}, errors.New("release build requires a binary, OS and architecture for every asset")
		}
		key := asset.OS + "/" + asset.Arch
		if seen[key] {
			return Manifest{}, errors.New("release build declares a platform twice")
		}
		seen[key] = true
		paths = append(paths, filepath.Join(options.Output, fmt.Sprintf("prifly-%s-%s.tar.gz", asset.OS, asset.Arch)))
	}
	if !seen["linux/amd64"] || !seen["darwin/arm64"] {
		return Manifest{}, errors.New("release build requires linux/amd64 and darwin/arm64 assets")
	}
	if options.Installer != "" {
		paths = append(paths, filepath.Join(options.Output, "install.sh"))
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return Manifest{}, fmt.Errorf("refusing to overwrite existing release asset %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return Manifest{}, err
		}
	}
	manifest := Manifest{SchemaVersion: ManifestVersion, Version: options.Version, Stable: true}
	for _, asset := range assets {
		binary, err := os.ReadFile(asset.Binary)
		if err != nil || len(binary) == 0 || len(binary) > MaxBinaryBytes {
			return Manifest{}, errors.New("release build binary is unavailable or exceeds the size limit")
		}
		archiveName := fmt.Sprintf("prifly-%s-%s.tar.gz", asset.OS, asset.Arch)
		archivePath := filepath.Join(options.Output, archiveName)
		if err := writeArchive(archivePath, binary); err != nil {
			return Manifest{}, err
		}
		archiveBytes, err := os.ReadFile(archivePath)
		if err != nil {
			return Manifest{}, err
		}
		manifest.Assets = append(manifest.Assets, Asset{OS: asset.OS, Arch: asset.Arch, Archive: archiveName, Binary: BinaryName, SHA256: fmt.Sprintf("%x", sha256.Sum256(archiveBytes))})
	}
	canonical, err := json.Marshal(manifest)
	if err != nil {
		return Manifest{}, err
	}
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(private), canonical)) + "\n"
	for _, asset := range []struct {
		path, content string
		mode          os.FileMode
	}{
		{filepath.Join(options.Output, "release-manifest.json"), string(append(canonical, '\n')), 0644},
		{filepath.Join(options.Output, "release-manifest.sig"), signature, 0644},
	} {
		if err := writeNew(asset.path, []byte(asset.content), asset.mode); err != nil {
			return Manifest{}, err
		}
	}
	if options.Installer != "" {
		installer, err := os.ReadFile(options.Installer)
		if err != nil || len(installer) == 0 {
			return Manifest{}, errors.New("release installer is unavailable")
		}
		if err := writeNew(filepath.Join(options.Output, "install.sh"), installer, 0755); err != nil {
			return Manifest{}, err
		}
	}
	return manifest, nil
}

func writeArchive(path string, binary []byte) (err error) {
	archive, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	compressed := gzip.NewWriter(archive)
	compressed.ModTime = time.Unix(0, 0)
	bundle := tar.NewWriter(compressed)
	if err = bundle.WriteHeader(&tar.Header{Name: BinaryName, Mode: 0755, Size: int64(len(binary)), ModTime: time.Unix(0, 0)}); err == nil {
		_, err = bundle.Write(binary)
	}
	if closeErr := bundle.Close(); err == nil {
		err = closeErr
	}
	if closeErr := compressed.Close(); err == nil {
		err = closeErr
	}
	if closeErr := archive.Close(); err == nil {
		err = closeErr
	}
	return err
}

func writeNew(path string, content []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

type parsedVersion struct {
	major, minor, patch int
	pre                 string
}

func validVersion(value string) bool {
	_, err := parseVersion(value)
	return err == nil
}

func stableVersion(value string) bool {
	parsed, err := parseVersion(value)
	return err == nil && parsed.pre == ""
}

func parseVersion(value string) (parsedVersion, error) {
	if strings.HasPrefix(value, "v") || strings.Contains(value, "+") {
		return parsedVersion{}, errors.New("version must be a stable semantic version without a v prefix or build metadata")
	}
	parts := strings.SplitN(value, "-", 2)
	base := strings.Split(parts[0], ".")
	if len(base) != 3 {
		return parsedVersion{}, errors.New("version must be major.minor.patch")
	}
	parsed := parsedVersion{}
	values := []*int{&parsed.major, &parsed.minor, &parsed.patch}
	for index, item := range base {
		if item == "" || len(item) > 1 && item[0] == '0' {
			return parsedVersion{}, errors.New("version numbers must be canonical")
		}
		value, err := strconv.Atoi(item)
		if err != nil || value < 0 {
			return parsedVersion{}, errors.New("version numbers must be non-negative integers")
		}
		*values[index] = value
	}
	if len(parts) == 2 {
		if parts[1] == "" || !regexp.MustCompile(`^[0-9A-Za-z.-]+$`).MatchString(parts[1]) {
			return parsedVersion{}, errors.New("version prerelease is invalid")
		}
		parsed.pre = parts[1]
	}
	return parsed, nil
}

func compareVersions(left, right string) (int, error) {
	a, err := parseVersion(left)
	if err != nil {
		return 0, err
	}
	b, err := parseVersion(right)
	if err != nil {
		return 0, err
	}
	for _, pair := range [][2]int{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if pair[0] < pair[1] {
			return -1, nil
		}
		if pair[0] > pair[1] {
			return 1, nil
		}
	}
	if a.pre == b.pre {
		return 0, nil
	}
	if a.pre == "" {
		return 1, nil
	}
	if b.pre == "" {
		return -1, nil
	}
	if a.pre < b.pre {
		return -1, nil
	}
	return 1, nil
}
