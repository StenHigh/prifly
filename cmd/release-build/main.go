// release-build creates the assets consumed by the public installer and
// updater. Publishing is intentionally a separate CI/owner decision.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/stenhigh/prifly/internal/release"
)

func main() {
	flags := flag.NewFlagSet("release-build", flag.ExitOnError)
	options := release.BuildOptions{}
	flags.Func("asset", "release asset as OS/ARCH=PATH; repeat for every supported platform", func(value string) error {
		platform, binary, ok := strings.Cut(value, "=")
		osName, arch, platformOK := strings.Cut(platform, "/")
		if !ok || !platformOK || osName == "" || arch == "" || binary == "" || strings.Contains(arch, "/") {
			return fmt.Errorf("asset must be OS/ARCH=PATH")
		}
		options.Assets = append(options.Assets, release.BuildAsset{OS: osName, Arch: arch, Binary: binary})
		return nil
	})
	flags.StringVar(&options.Installer, "installer", "", "optional install.sh asset")
	flags.StringVar(&options.Output, "output", "", "new asset directory")
	flags.StringVar(&options.Version, "version", "", "semantic release version")
	flags.StringVar(&options.PrivateKeyHex, "private-key-hex", "", "ed25519 signing key")
	flags.StringVar(&options.PublicKeyHex, "public-key-hex", "", "matching ed25519 public key")
	_ = flags.Parse(os.Args[1:])
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "release-build accepts flags only")
		os.Exit(2)
	}
	if options.PrivateKeyHex == "" {
		options.PrivateKeyHex = os.Getenv("PRIFLY_RELEASE_SIGNING_KEY")
	}
	manifest, err := release.Build(options)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("release assets created for %s (%d platforms)\n", manifest.Version, len(manifest.Assets))
}
