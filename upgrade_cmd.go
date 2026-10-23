package main

import (
	"fmt"
	"os"
	"runtime"

	"ttr/internal/launcher"
	"ttr/internal/upgrade"
)

// ttr upgrade: the newest release from GitHub, in place of this program.

const upgradeUsage = `Usage:
  ttr upgrade               Install the newest release over this ttr
  ttr upgrade -f            Install it even if it isn't newer, or over a build from source

Downloads the release for this machine from github.com/marbris/tutor,
checks it against the release's SHA256SUMS, and puts it where this ttr is.
If that place isn't yours to write to, run it with sudo.`

func runUpgrade(args []string) {
	force := false
	for _, a := range args {
		switch a {
		case "-f", "--force":
			force = true
		case "-h", "--help", "help":
			fmt.Println(upgradeUsage)
			return
		default:
			fmt.Println(upgradeUsage)
			os.Exit(1)
		}
	}

	exe, ok := launcher.Executable()
	if !ok {
		fail(fmt.Errorf("this ttr is a temporary build (go run, or a test); there's nothing to upgrade"))
	}
	if version == "dev" && !force {
		fail(fmt.Errorf("this ttr was built from source — `git pull && make build` updates it, " +
			"or `ttr upgrade -f` replaces it with the release"))
	}

	rel, err := upgrade.Latest()
	if err != nil {
		fail(err)
	}
	if !upgrade.Newer(rel.Version(), version) && !force {
		fmt.Printf("ttr %s is the latest.\n", version)
		return
	}
	bin, sums, err := rel.Find(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		fail(err)
	}
	fmt.Printf("Downloading %s %s (%.1f MB)…\n", bin.FileName, rel.Tag, float64(bin.Size)/1e6)
	body, err := upgrade.Download(bin, sums)
	if err != nil {
		fail(err)
	}
	fmt.Println("Checksum matches SHA256SUMS.")
	if err := upgrade.Replace(exe, body); err != nil {
		fail(err)
	}
	fmt.Printf("ttr %s → %s, installed at %s\n", version, rel.Version(), exe)
	if rel.URL != "" {
		fmt.Println("What's new: " + rel.URL)
	}
}
