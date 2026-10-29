package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testProxy(id string, port int, user, pass string) Proxy {
	return Proxy{ID: id, Type: "http", Port: port, Username: user, Password: pass, Enabled: true}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		list []Proxy
		ok   bool
	}{
		{"valid", []Proxy{testProxy("a", 3128, "u1", "p1"), testProxy("b", 3129, "u2", "p2")}, true},
		{"shared user, same password", []Proxy{testProxy("a", 3128, "u", "p"), testProxy("b", 3129, "u", "p")}, true},
		{"shared user, other password", []Proxy{testProxy("a", 3128, "u", "p"), testProxy("b", 3129, "u", "q")}, false},
		{"duplicate port", []Proxy{testProxy("a", 3128, "u1", "p"), testProxy("b", 3128, "u2", "p")}, false},
		{"panel port", []Proxy{testProxy("a", 8088, "u", "p")}, false},
		{"port out of range", []Proxy{testProxy("a", 70000, "u", "p")}, false},
		{"quote in password", []Proxy{testProxy("a", 3128, "u", `p"x`)}, false},
		{"space in username", []Proxy{testProxy("a", 3128, "u x", "p")}, false},
		{"empty password", []Proxy{testProxy("a", 3128, "u", "")}, false},
		{"bad type", []Proxy{{ID: "a", Type: "ftp", Port: 21, Username: "u", Password: "p"}}, false},
		{"bad ext ip", []Proxy{{ID: "a", Type: "http", Port: 3128, Username: "u", Password: "p", ExtIP: "1.2.3"}}, false},
		{"newline in note", []Proxy{{ID: "a", Type: "http", Port: 3128, Username: "u", Password: "p", Note: "a\nb"}}, false},
	}
	all := portRange{1, 65535}
	for _, tt := range tests {
		err := validate(tt.list, 8088, all)
		if (err == nil) != tt.ok {
			t.Errorf("%s: validate() = %v, want ok=%v", tt.name, err, tt.ok)
		}
	}
	low := []Proxy{testProxy("a", 80, "u", "p")}
	if validate(low, 8088, all) != nil || validate(low, 8088, portRange{1024, 65535}) == nil {
		t.Errorf("port 80 should only be refused with a range from 1024")
	}
	if validate([]Proxy{testProxy("a", 30000, "u", "p")}, 8088, portRange{20000, 29999}) == nil {
		t.Errorf("port above the range accepted")
	}
}

func TestParsePortRange(t *testing.T) {
	tests := []struct {
		in     string
		lowest int
		want   portRange
		ok     bool
	}{
		{"", 1024, portRange{1024, 65535}, true},
		{"", 1, portRange{1, 65535}, true},
		{"20000-29999", 1024, portRange{20000, 29999}, true},
		{" 3128 - 3200 ", 1, portRange{3128, 3200}, true},
		{"80-100", 1, portRange{80, 100}, true},
		{"80-100", 1024, portRange{}, false},
		{"3200-3128", 1, portRange{}, false},
		{"20000-70000", 1, portRange{}, false},
		{"20000", 1, portRange{}, false},
		{"a-b", 1, portRange{}, false},
	}
	for _, tt := range tests {
		got, err := parsePortRange(tt.in, tt.lowest)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("parsePortRange(%q, %d) = %v, %v; want %v ok=%v", tt.in, tt.lowest, got, err, tt.want, tt.ok)
		}
	}
}

func TestCheckFree(t *testing.T) {
	current := []Proxy{testProxy("a", 3128, "u", "p")}
	listening := map[int]bool{22: true, 3128: true, 3306: true}
	ok := [][]Proxy{
		// 3128 is 3proxy's own.
		{testProxy("a", 3128, "u", "p"), testProxy("b", 3129, "u", "p")},
		// The edited proxy left 3128, another one takes it over.
		{testProxy("a", 3130, "u", "p"), testProxy("b", 3128, "u", "p")},
		// A disabled proxy may sit on a busy port.
		{testProxy("a", 3128, "u", "p"), {ID: "b", Type: "http", Port: 3306, Username: "u", Password: "p"}},
	}
	for i, next := range ok {
		if err := checkFree(next, current, listening); err != nil {
			t.Errorf("case %d: %v", i, err)
		}
	}
	if checkFree([]Proxy{testProxy("b", 3306, "u", "p")}, current, listening) == nil {
		t.Errorf("port used by another program accepted")
	}
}

func TestSuggestPort(t *testing.T) {
	all := portRange{1, 65535}
	none := map[int]bool{}
	tests := []struct {
		list      []Proxy
		allowed   portRange
		listening map[int]bool
		want      int
	}{
		{nil, all, none, 3128},
		{nil, portRange{20000, 29999}, none, 20000},
		{[]Proxy{testProxy("a", 3128, "u", "p")}, all, none, 3129},
		{[]Proxy{testProxy("a", 20005, "u", "p")}, portRange{20000, 29999}, none, 20006},
		// Skips the panel and other programs.
		{nil, portRange{8087, 8100}, map[int]bool{8087: true}, 8089},
		// Wraps around at the end of the range.
		{[]Proxy{testProxy("a", 20002, "u", "p")}, portRange{20000, 20002}, none, 20000},
		{[]Proxy{testProxy("a", 20000, "u", "p")}, portRange{20000, 20000}, none, 0},
	}
	for i, tt := range tests {
		if got := suggestPort(tt.list, tt.allowed, 8088, tt.listening); got != tt.want {
			t.Errorf("case %d: suggestPort() = %d, want %d", i, got, tt.want)
		}
	}
}

func TestRenderConfig(t *testing.T) {
	list := []Proxy{
		testProxy("a", 3128, "alice", "pw1"),
		{ID: "b", Type: "socks5", Port: 1080, Username: "bob", Password: "pw2", ExtIP: "203.0.113.5", Enabled: true},
		{ID: "c", Type: "auto", Port: 8000, Username: "alice", Password: "pw1", Enabled: true},
		{ID: "d", Type: "http", Port: 9000, Username: "carol", Password: "pw3"},
	}
	got := string(renderConfig(list, defaultHeader, "/conf/3proxy.cfg", true))
	for _, want := range []string{
		"config /conf/3proxy.cfg\n",
		"monitor /conf/3proxy.cfg\n",
		"auth strong\n",
		"users \"alice:CL:pw1\"\n",
		"users \"bob:CL:pw2\"\n",
		"flush\ndeny * * 127.0.0.0/8\nallow alice\nproxy -p3128\n",
		"allow bob\nsocks -p1080 -e203.0.113.5\n",
		"allow alice\nauto -p8000\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("config lacks %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "users ") != 2 {
		t.Errorf("users should be listed once each:\n%s", got)
	}
	if strings.Contains(got, "carol") || strings.Contains(got, "9000") {
		t.Errorf("disabled proxy in config:\n%s", got)
	}
	if !strings.HasPrefix(got, cfgMarker) {
		t.Errorf("config does not start with the marker")
	}

	plain := string(renderConfig(nil, defaultHeader, "/etc/3proxy/3proxy.cfg", false))
	if strings.Contains(plain, "\nconfig ") || strings.Contains(plain, "auth strong") {
		t.Errorf("unexpected content without chroot or proxies:\n%s", plain)
	}
}

func TestWriteConfigBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "3proxy.cfg")
	if err := os.WriteFile(path, []byte("proxy -p3128\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := writeConfig(path, renderConfig(nil, defaultHeader, path, false)); err != nil {
		t.Fatal(err)
	}
	if err := writeConfig(path, renderConfig(nil, defaultHeader, path, false)); err != nil {
		t.Fatal(err)
	}
	backups, _ := filepath.Glob(path + ".bak-*")
	if len(backups) != 1 {
		t.Fatalf("want one backup of the hand written config, got %v", backups)
	}
	if data, _ := os.ReadFile(backups[0]); string(data) != "proxy -p3128\n" {
		t.Errorf("backup content %q", data)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o640 {
		t.Errorf("mode %v, want 0640", fi.Mode().Perm())
	}
}

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "proxies.json")
	list, err := loadProxies(path)
	if err != nil || list != nil {
		t.Fatalf("missing file: %v %v", list, err)
	}
	want := []Proxy{testProxy("b", 3129, "u", "p"), testProxy("a", 3128, "u", "p")}
	if err := saveProxies(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadProxies(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Port != 3128 || got[1].ID != "b" {
		t.Errorf("loaded %+v", got)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", fi.Mode().Perm())
	}
}
