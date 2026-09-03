package release

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

type releaseFixture struct {
	directory   string
	executable  string
	publicKey   string
	manifest    []byte
	signature   []byte
	archiveName string
	archive     []byte
	archives    map[string][]byte
	candidates  map[string]string
	os          string
	arch        string
}

func fixture(t *testing.T) releaseFixture {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	executable := filepath.Join(directory, BinaryName)
	if err := os.WriteFile(executable, []byte("old-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	receipt, err := jsonReceipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ReceiptName), receipt, 0600); err != nil {
		t.Fatal(err)
	}
	assetsToBuild := make([]BuildAsset, 0, 2)
	candidates := map[string]string{}
	for _, platform := range []struct{ os, arch string }{{"linux", "amd64"}, {"darwin", "arm64"}} {
		candidate := filepath.Join(directory, "candidate-"+platform.os+"-"+platform.arch)
		content := "new-" + platform.os + "-" + platform.arch
		if err := os.WriteFile(candidate, []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
		assetsToBuild = append(assetsToBuild, BuildAsset{Binary: candidate, OS: platform.os, Arch: platform.arch})
		candidates[platform.os+"/"+platform.arch] = content
	}
	assets := filepath.Join(directory, "assets")
	manifest, err := Build(BuildOptions{Assets: assetsToBuild, Output: assets, Version: "1.1.0", PrivateKeyHex: hex.EncodeToString(private), PublicKeyHex: hex.EncodeToString(public)})
	if err != nil {
		t.Fatal(err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(assets, "release-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	signature, err := os.ReadFile(filepath.Join(assets, "release-manifest.sig"))
	if err != nil {
		t.Fatal(err)
	}
	archives := map[string][]byte{}
	for _, asset := range manifest.Assets {
		archive, err := os.ReadFile(filepath.Join(assets, asset.Archive))
		if err != nil {
			t.Fatal(err)
		}
		archives[asset.Archive] = archive
	}
	archiveName := "prifly-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz"
	archive, ok := archives[archiveName]
	if !ok {
		t.Fatalf("test host %s/%s is absent from release matrix", runtime.GOOS, runtime.GOARCH)
	}
	return releaseFixture{directory: directory, executable: executable, publicKey: hex.EncodeToString(public), manifest: manifestBytes, signature: signature, archiveName: archiveName, archive: archive, archives: archives, candidates: candidates, os: runtime.GOOS, arch: runtime.GOARCH}
}

func jsonReceipt() ([]byte, error) {
	return []byte(`{"schema_version":"prifly-managed-install/1","binary":"prifly","channel":"stable"}`), nil
}

func server(t *testing.T, f releaseFixture, mutate func(string, []byte) []byte) (*httptest.Server, *int) {
	t.Helper()
	requests := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body []byte
		switch r.URL.Path {
		case "/release-manifest.json":
			body = f.manifest
		case "/release-manifest.sig":
			body = f.signature
		default:
			var ok bool
			body, ok = f.archives[strings.TrimPrefix(r.URL.Path, "/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
		}
		if mutate != nil {
			body = mutate(r.URL.Path, body)
		}
		_, _ = w.Write(body)
	})), &requests
}

func updater(f releaseFixture, address string) Updater {
	return Updater{ReleaseBaseURL: address, PublicKeyHex: f.publicKey, Executable: f.executable, CurrentVersion: "1.0.0", OS: f.os, Arch: f.arch}
}

func TestUpdateVerifiesAndAtomicallyReplacesManagedBinary(t *testing.T) {
	f := fixture(t)
	s, _ := server(t, f, nil)
	defer s.Close()
	result, err := updater(f, s.URL).Update(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.PreviousVersion != "1.0.0" || result.Version != "1.1.0" {
		t.Fatalf("unexpected update result: %+v", result)
	}
	binary, err := os.ReadFile(f.executable)
	if err != nil || string(binary) != f.candidates[f.os+"/"+f.arch] {
		t.Fatalf("binary not replaced: %q %v", binary, err)
	}
}

func TestUpdateSelectsEachSupportedPlatform(t *testing.T) {
	f := fixture(t)
	s, _ := server(t, f, nil)
	defer s.Close()
	for platform, expected := range f.candidates {
		t.Run(platform, func(t *testing.T) {
			if err := os.WriteFile(f.executable, []byte("old-binary"), 0755); err != nil {
				t.Fatal(err)
			}
			osName, arch, _ := strings.Cut(platform, "/")
			u := updater(f, s.URL)
			u.OS, u.Arch = osName, arch
			if _, err := u.Update(context.Background()); err != nil {
				t.Fatal(err)
			}
			binary, err := os.ReadFile(f.executable)
			if err != nil || string(binary) != expected {
				t.Fatalf("platform %s selected the wrong binary: %q %v", platform, binary, err)
			}
		})
	}
}

func TestUpdateRejectsChangedManifestAndArchiveWithoutReplacingBinary(t *testing.T) {
	for name, mutate := range map[string]func(string, []byte) []byte{
		"manifest": func(path string, body []byte) []byte {
			if path == "/release-manifest.json" {
				body = append([]byte(nil), body...)
				body[len(body)-2] ^= 1
			}
			return body
		},
		"archive": func(path string, body []byte) []byte {
			if path != "/release-manifest.json" && path != "/release-manifest.sig" {
				body = append([]byte(nil), body...)
				body[len(body)-1] ^= 1
			}
			return body
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := fixture(t)
			s, _ := server(t, f, mutate)
			defer s.Close()
			if _, err := updater(f, s.URL).Update(context.Background()); err == nil {
				t.Fatal("tampered release was accepted")
			}
			binary, err := os.ReadFile(f.executable)
			if err != nil || string(binary) != "old-binary" {
				t.Fatalf("failed update changed binary: %q %v", binary, err)
			}
		})
	}
}

func TestUpdateRefusesUnsupportedPlatformAndCurrentVersionWithoutArchive(t *testing.T) {
	f := fixture(t)
	s, requests := server(t, f, nil)
	defer s.Close()
	u := updater(f, s.URL)
	u.OS = "other"
	if _, err := u.Update(context.Background()); err == nil {
		t.Fatal("unsupported platform was accepted")
	}
	if *requests != 2 {
		t.Fatalf("unsupported platform fetched an archive: %d requests", *requests)
	}
	*requests = 0
	u = updater(f, s.URL)
	u.CurrentVersion = "1.1.0"
	result, err := u.Update(context.Background())
	if err != nil || result.Updated || *requests != 2 {
		t.Fatalf("current version behavior changed: result=%+v requests=%d err=%v", result, *requests, err)
	}
}

func TestInstallerUsesOnlyOneArchiveAndLeavesNoPartialBinary(t *testing.T) {
	f := fixture(t)
	s, _ := server(t, f, nil)
	defer s.Close()
	repository, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(filepath.Join(repository, "..", "..", "scripts", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(t.TempDir(), "install.sh")
	script = []byte(strings.Replace(string(script), DefaultReleaseBaseURL, s.URL, 1))
	if err := os.WriteFile(copyPath, script, 0755); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("sh", "-n", copyPath).CombinedOutput(); err != nil {
		t.Fatalf("installer has invalid shell syntax: %v\n%s", err, output)
	}
	destination := filepath.Join(t.TempDir(), "bin")
	command := exec.Command("sh", copyPath)
	command.Env = append(os.Environ(), "PRIFLY_INSTALL_DIR="+destination)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("installer failed: %v\n%s", err, output)
	}
	binary, err := os.ReadFile(filepath.Join(destination, BinaryName))
	if err != nil || string(binary) != f.candidates[f.os+"/"+f.arch] {
		t.Fatalf("installer wrote unexpected binary: %q %v", binary, err)
	}
	if _, err := os.Stat(filepath.Join(destination, ReceiptName)); err != nil {
		t.Fatalf("installer did not write receipt: %v", err)
	}
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	command = exec.Command("sh", copyPath)
	command.Env = append(os.Environ(), "PRIFLY_INSTALL_DIR="+blocked)
	if output, err := command.CombinedOutput(); err == nil {
		t.Fatalf("unwritable destination was accepted: %s", output)
	}
	content, err := os.ReadFile(blocked)
	if err != nil || string(content) != "occupied" {
		t.Fatalf("installer changed blocked destination: %q %v", content, err)
	}
}

func TestUpdateRefusesSourceBuildBeforeNetwork(t *testing.T) {
	f := fixture(t)
	s, requests := server(t, f, nil)
	defer s.Close()
	source := filepath.Join(t.TempDir(), BinaryName)
	if err := os.WriteFile(source, []byte("source-build"), 0755); err != nil {
		t.Fatal(err)
	}
	u := updater(f, s.URL)
	u.Executable = source
	if _, err := u.Update(context.Background()); err == nil {
		t.Fatal("source build update was accepted")
	}
	if *requests != 0 {
		t.Fatalf("source build made network requests: %d", *requests)
	}
}

func TestUpdateRefusesCopiedBinaryBeforeNetwork(t *testing.T) {
	f := fixture(t)
	s, requests := server(t, f, nil)
	defer s.Close()
	copied := filepath.Join(f.directory, "other-prifly")
	if err := os.WriteFile(copied, []byte("copied-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	u := updater(f, s.URL)
	u.Executable = copied
	if _, err := u.Update(context.Background()); err == nil {
		t.Fatal("copied binary update was accepted")
	}
	if *requests != 0 {
		t.Fatalf("copied binary made network requests: %d", *requests)
	}
}

func TestInterruptedDownloadLeavesInstalledBinaryUntouched(t *testing.T) {
	f := fixture(t)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release-manifest.json":
			_, _ = w.Write(f.manifest)
		case "/release-manifest.sig":
			_, _ = w.Write(f.signature)
		case "/" + f.archiveName:
			w.Header().Set("Content-Length", strconv.Itoa(len(f.archive)))
			_, _ = w.Write(f.archive[:len(f.archive)/2])
		default:
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	if _, err := updater(f, s.URL).Update(context.Background()); err == nil {
		t.Fatal("interrupted archive was accepted")
	}
	binary, err := os.ReadFile(f.executable)
	if err != nil || string(binary) != "old-binary" {
		t.Fatalf("interrupted update changed binary: %q %v", binary, err)
	}
}

func TestBuildRefusesMismatchedPublicKey(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), BinaryName)
	if err := os.WriteFile(binary, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	wrongKey := append([]byte(nil), public...)
	wrongKey[0] ^= 1
	if _, err := Build(BuildOptions{Assets: buildAssets(binary), Output: filepath.Join(t.TempDir(), "assets"), Version: "1.0.0", PrivateKeyHex: hex.EncodeToString(private), PublicKeyHex: hex.EncodeToString(wrongKey)}); err == nil {
		t.Fatal("mismatched public key was accepted")
	}
	if _, err := Build(BuildOptions{Assets: buildAssets(binary), Output: filepath.Join(t.TempDir(), "prerelease-assets"), Version: "1.0.0-rc.1", PrivateKeyHex: hex.EncodeToString(private), PublicKeyHex: hex.EncodeToString(public)}); err == nil {
		t.Fatal("stable release builder accepted a prerelease")
	}
}

func buildAssets(binary string) []BuildAsset {
	return []BuildAsset{{Binary: binary, OS: "linux", Arch: "amd64"}, {Binary: binary, OS: "darwin", Arch: "arm64"}}
}

func TestBuildRefusesIncompleteOrDuplicatePlatformAssets(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), BinaryName)
	if err := os.WriteFile(binary, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	options := BuildOptions{Output: filepath.Join(t.TempDir(), "assets"), Version: "1.0.0", PrivateKeyHex: hex.EncodeToString(private), PublicKeyHex: hex.EncodeToString(public)}
	options.Assets = []BuildAsset{{Binary: binary, OS: "linux", Arch: "amd64"}}
	if _, err := Build(options); err == nil {
		t.Fatal("release build accepted a missing macOS asset")
	}
	options.Output = filepath.Join(t.TempDir(), "duplicate-assets")
	options.Assets = append(buildAssets(binary), BuildAsset{Binary: binary, OS: "linux", Arch: "amd64"})
	if _, err := Build(options); err == nil {
		t.Fatal("release build accepted a duplicate platform")
	}
}

func TestStableManifestRejectsPrerelease(t *testing.T) {
	raw := []byte(`{"schema_version":"prifly-release/1","version":"1.0.0-rc.1","stable":true,"assets":[{"os":"linux","arch":"amd64","archive":"prifly-linux-amd64.tar.gz","binary":"prifly","sha256":"0000000000000000000000000000000000000000000000000000000000000000"}]}`)
	if _, _, err := parseManifest(raw); err == nil {
		t.Fatal("stable manifest accepted a prerelease")
	}
}

func TestBuildCopiesInstallerAndReleaseCIIsManual(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(repository, "..", ".."))
	assets := filepath.Join(t.TempDir(), "assets")
	if _, err := Build(BuildOptions{Assets: buildAssets(filepath.Join(root, "README.md")), Installer: filepath.Join(root, "scripts", "install.sh"), Output: assets, Version: "1.0.0", PrivateKeyHex: hex.EncodeToString(private), PublicKeyHex: hex.EncodeToString(public)}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(assets, "install.sh"))
	if err != nil || info.Mode()&0100 == 0 {
		t.Fatalf("installer asset is missing or not executable: %v", err)
	}
	ci, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"build-linux-amd64:", "build-darwin-arm64:", "environment: release", "contents: write", "release-manifest.json"} {
		if !strings.Contains(string(ci), required) {
			t.Fatalf("release CI omits %q", required)
		}
	}
}
