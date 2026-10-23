// Package upgrade is ttr upgrade: the newest release from GitHub, checked
// against its SHA256SUMS and put in place of the running program.
//
// The new binary is written beside the old one and renamed over it. On Linux
// and macOS the rename is atomic and the running program keeps the file it
// started from; on macOS a new file, rather than one rewritten in place, is
// also what the code-signature check expects, and a file ttr downloads
// itself carries no quarantine flag. Windows won't let a running program be
// replaced, but will let it be renamed, so it is moved aside first.
package upgrade

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"ttr/internal/fetch"
	"ttr/internal/launcher"
)

// LatestURL is GitHub's answer to "what is the newest release". A variable,
// so tests can point it at a server of their own.
var LatestURL = "https://api.github.com/repos/marbris/tutor/releases/latest"

// Release is the part of GitHub's release that an upgrade needs.
type Release struct {
	Tag    string  `json:"tag_name"`
	URL    string  `json:"html_url"`
	Assets []Asset `json:"assets"`
}

// Asset is one file of a release.
type Asset struct {
	FileName string `json:"name"`
	URL      string `json:"browser_download_url"`
	Size     int64  `json:"size"`
}

// Version is the release's version, without the v.
func (r Release) Version() string { return strings.TrimPrefix(r.Tag, "v") }

// Latest asks GitHub for the newest release.
func Latest() (Release, error) {
	body, err := fetch.Get(LatestURL)
	if err != nil {
		return Release{}, fmt.Errorf("couldn't ask GitHub for the latest release: %w", err)
	}
	var r Release
	if err := json.Unmarshal(body, &r); err != nil {
		return Release{}, fmt.Errorf("GitHub's answer didn't read as a release: %w", err)
	}
	if r.Tag == "" {
		return Release{}, fmt.Errorf("GitHub has no release of Tutor")
	}
	return r, nil
}

// Newer reports whether version a is newer than b, both x.y.z. Anything
// that doesn't read as a version — "dev" — is older than every release.
func Newer(a, b string) bool {
	pa, okA := parse(a)
	pb, okB := parse(b)
	switch {
	case !okA:
		return false
	case !okB:
		return true
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// AssetName is the release file built for a platform.
func AssetName(goos, goarch string) string {
	name := "ttr-" + goos + "-" + goarch
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// Find is the release's file for this machine, and its checksums.
func (r Release) Find(goos, goarch string) (bin, sums Asset, err error) {
	want := AssetName(goos, goarch)
	var have []string
	for _, a := range r.Assets {
		switch a.FileName {
		case want:
			bin = a
		case "SHA256SUMS":
			sums = a
		default:
			have = append(have, strings.TrimPrefix(strings.TrimSuffix(a.FileName, ".exe"), "ttr-"))
		}
	}
	if bin.URL == "" {
		return bin, sums, fmt.Errorf("%s has no build for %s/%s — it has %s",
			r.Tag, goos, goarch, strings.Join(have, ", "))
	}
	if sums.URL == "" {
		return bin, sums, fmt.Errorf("%s has no SHA256SUMS to check the download against", r.Tag)
	}
	return bin, sums, nil
}

// Download fetches the binary and checks it against SHA256SUMS. Nothing is
// written: a download that doesn't match is never put anywhere.
func Download(bin, sums Asset) ([]byte, error) {
	list, err := fetch.GetFile(sums.URL)
	if err != nil {
		return nil, fmt.Errorf("couldn't download SHA256SUMS: %w", err)
	}
	want, ok := sumFor(list, bin.FileName)
	if !ok {
		return nil, fmt.Errorf("SHA256SUMS doesn't list %s", bin.FileName)
	}
	body, err := fetch.GetFile(bin.URL)
	if err != nil {
		return nil, fmt.Errorf("couldn't download %s: %w", bin.FileName, err)
	}
	got := sha256.Sum256(body)
	if hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("%s doesn't match its checksum — not installed", bin.FileName)
	}
	return body, nil
}

// sumFor finds a file's checksum in sha256sum's output: "<hex>  <name>".
func sumFor(list []byte, name string) (string, bool) {
	sc := bufio.NewScanner(bytes.NewReader(list))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			return strings.ToLower(fields[0]), true
		}
	}
	return "", false
}

// managed reports whether a path belongs to a package manager, which would
// put back its own copy — or refuse — at the next update.
func managed(exe string) bool {
	for _, prefix := range []string{"/nix/store/", "/opt/homebrew/Cellar/", "/usr/local/Cellar/", "/usr/bin/", "/snap/"} {
		if strings.HasPrefix(exe, prefix) {
			return true
		}
	}
	return false
}

// Replace puts body in place of the program at exe.
func Replace(exe string, body []byte) error {
	if managed(exe) {
		return fmt.Errorf("%s was installed by a package manager — upgrade it with that", exe)
	}
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".ttr-upgrade-*")
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("can't write to %s — run `sudo ttr upgrade`", dir)
		}
		return err
	}
	done := false
	defer func() {
		if !done {
			os.Remove(tmp.Name())
		}
	}()
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		if err := launcher.MoveAside(exe); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp.Name(), exe); err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("can't replace %s — run `sudo ttr upgrade`", exe)
		}
		return err
	}
	done = true
	return nil
}
