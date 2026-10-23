package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"10.10.0", "10.9.0", true},
		{"10.9.0", "10.10.0", false},
		{"10.5.0", "10.5.0", false},
		{"v11.0.0", "10.99.99", true},
		{"10.5.1", "dev", true},
		{"dev", "10.5.0", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestAssetName(t *testing.T) {
	for goos, want := range map[string]string{
		"linux": "ttr-linux-amd64", "darwin": "ttr-darwin-amd64", "windows": "ttr-windows-amd64.exe",
	} {
		if got := AssetName(goos, "amd64"); got != want {
			t.Errorf("%s: %q, want %q", goos, got, want)
		}
	}
}

func TestFindSaysWhatTheReleaseHas(t *testing.T) {
	r := Release{Tag: "v10.6.0", Assets: []Asset{
		{FileName: "ttr-linux-amd64", URL: "x"}, {FileName: "SHA256SUMS", URL: "y"},
	}}
	if _, _, err := r.Find("linux", "amd64"); err != nil {
		t.Errorf("linux/amd64: %v", err)
	}
	_, _, err := r.Find("plan9", "386")
	if err == nil || !strings.Contains(err.Error(), "linux-amd64") {
		t.Errorf("plan9: %v", err)
	}
}

// release serves a release whose binary for this machine is body, with
// sums as its SHA256SUMS.
func release(t *testing.T, body []byte, sums string) {
	t.Helper()
	name := AssetName(runtime.GOOS, runtime.GOARCH)
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v10.6.0", "html_url": srv.URL + "/notes",
			"assets": []map[string]any{
				{"name": name, "browser_download_url": srv.URL + "/bin", "size": len(body)},
				{"name": "SHA256SUMS", "browser_download_url": srv.URL + "/sums"},
			},
		})
	})
	mux.HandleFunc("/bin", func(w http.ResponseWriter, r *http.Request) { w.Write(body) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(sums)) })
	was := LatestURL
	LatestURL = srv.URL + "/latest"
	t.Cleanup(func() { LatestURL = was })
}

func sumLine(body []byte, name string) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:]) + "  " + name + "\n"
}

func TestUpgradeInstallsAMatchingDownload(t *testing.T) {
	body := []byte("new ttr")
	name := AssetName(runtime.GOOS, runtime.GOARCH)
	release(t, body, sumLine([]byte("other"), "ttr-other")+sumLine(body, name))

	exe := filepath.Join(t.TempDir(), "ttr")
	os.WriteFile(exe, []byte("old ttr"), 0o755)

	r, err := Latest()
	if err != nil {
		t.Fatal(err)
	}
	if r.Version() != "10.6.0" || !Newer(r.Version(), "10.5.0") {
		t.Fatalf("latest is %q", r.Version())
	}
	bin, sums, err := r.Find(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Download(bin, sums)
	if err != nil {
		t.Fatal(err)
	}
	if err := Replace(exe, got); err != nil {
		t.Fatal(err)
	}
	now, _ := os.ReadFile(exe)
	info, _ := os.Stat(exe)
	if string(now) != "new ttr" || info.Mode().Perm() != 0o755 {
		t.Errorf("the program holds %q, mode %v", now, info.Mode().Perm())
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".ttr-upgrade-*"))
	if len(left) > 0 {
		t.Errorf("left behind %v", left)
	}
}

func TestADownloadThatDoesntMatchIsRefused(t *testing.T) {
	name := AssetName(runtime.GOOS, runtime.GOARCH)
	release(t, []byte("tampered"), sumLine([]byte("the real one"), name))
	r, _ := Latest()
	bin, sums, _ := r.Find(runtime.GOOS, runtime.GOARCH)
	if _, err := Download(bin, sums); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Errorf("a mismatch gave %v", err)
	}
}

func TestReplaceSaysToUseSudo(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs a directory this user can't write")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "ttr")
	os.WriteFile(exe, []byte("old"), 0o755)
	os.Chmod(dir, 0o555)
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	err := Replace(exe, []byte("new"))
	if err == nil || !strings.Contains(err.Error(), "sudo ttr upgrade") {
		t.Errorf("a read-only directory gave %v", err)
	}
	if now, _ := os.ReadFile(exe); string(now) != "old" {
		t.Errorf("the program was changed to %q", now)
	}
}

func TestAPackageManagersCopyIsLeftToIt(t *testing.T) {
	if err := Replace("/nix/store/abc-ttr/bin/ttr", nil); err == nil || !strings.Contains(err.Error(), "package manager") {
		t.Errorf("a nix store path gave %v", err)
	}
}
