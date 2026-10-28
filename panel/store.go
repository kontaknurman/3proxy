package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// portRange is the range of ports proxies may listen on.
type portRange struct{ Min, Max int }

func (r portRange) String() string { return fmt.Sprintf("%d-%d", r.Min, r.Max) }

func (r portRange) contains(port int) bool { return port >= r.Min && port <= r.Max }

// parsePortRange reads a "MIN-MAX" range. An empty one allows every port
// from lowest up.
func parsePortRange(s string, lowest int) (portRange, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return portRange{lowest, 65535}, nil
	}
	lo, hi, ok := strings.Cut(s, "-")
	min, err1 := strconv.Atoi(strings.TrimSpace(lo))
	max, err2 := strconv.Atoi(strings.TrimSpace(hi))
	if !ok || err1 != nil || err2 != nil || min < 1 || max > 65535 || min > max {
		return portRange{}, fmt.Errorf("port range %q is not valid, use for example 20000-29999", s)
	}
	if min < lowest {
		return portRange{}, fmt.Errorf("port range %q: 3proxy runs unprivileged and cannot use ports below %d", s, lowest)
	}
	return portRange{min, max}, nil
}

// Proxy types and the 3proxy service each of them is served by.
var proxyServices = map[string]string{
	"http":   "proxy",
	"socks5": "socks",
	"auto":   "auto",
}

// Proxy is one listening port with its own login.
type Proxy struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Port      int       `json:"port"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	ExtIP     string    `json:"ext_ip,omitempty"`
	Note      string    `json:"note,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// Usernames and passwords end up in the 3proxy configuration, so only
// characters which need no quoting there are accepted.
var credRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func newID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// check validates a single proxy.
func (p *Proxy) check() error {
	if _, ok := proxyServices[p.Type]; !ok {
		return fmt.Errorf("tipe proxy tidak dikenal: %q", p.Type)
	}
	if p.Port < 1 || p.Port > 65535 {
		return errors.New("port harus 1-65535")
	}
	if !credRe.MatchString(p.Username) {
		return errors.New("username harus 1-64 karakter: huruf, angka, titik, minus atau underscore")
	}
	if !credRe.MatchString(p.Password) {
		return errors.New("password harus 1-64 karakter: huruf, angka, titik, minus atau underscore")
	}
	if p.ExtIP != "" && net.ParseIP(p.ExtIP) == nil {
		return fmt.Errorf("IP keluar tidak valid: %q", p.ExtIP)
	}
	if len(p.Note) > 200 || strings.ContainsAny(p.Note, "\r\n") {
		return errors.New("catatan maksimal 200 karakter dan satu baris")
	}
	return nil
}

// validate checks every proxy and the rules between them: a port is in the
// allowed range and used once, and a username shared by several proxies
// has the same password, since 3proxy keeps one user list for all
// services.
func validate(list []Proxy, reservedPort int, allowed portRange) error {
	ports := map[int]string{}
	passwords := map[string]string{}
	for i := range list {
		p := &list[i]
		if err := p.check(); err != nil {
			return err
		}
		if !allowed.contains(p.Port) {
			return fmt.Errorf("port %d di luar rentang port proxy %s", p.Port, allowed)
		}
		if p.Port == reservedPort {
			return fmt.Errorf("port %d dipakai oleh panel", p.Port)
		}
		if other, ok := ports[p.Port]; ok && other != p.ID {
			return fmt.Errorf("port %d sudah dipakai proxy lain", p.Port)
		}
		ports[p.Port] = p.ID
		if pw, ok := passwords[p.Username]; ok && pw != p.Password {
			return fmt.Errorf("username %q sudah dipakai proxy lain dengan password berbeda", p.Username)
		}
		passwords[p.Username] = p.Password
	}
	return nil
}

// checkFree refuses enabled proxies on ports another program listens on.
// The ports of the proxies enabled in current are held by 3proxy itself.
func checkFree(next, current []Proxy, listening map[int]bool) error {
	held := map[int]bool{}
	for _, p := range current {
		if p.Enabled {
			held[p.Port] = true
		}
	}
	for _, p := range next {
		if p.Enabled && !held[p.Port] && listening[p.Port] {
			return fmt.Errorf("port %d sudah dipakai program lain di VPS, pilih port lain", p.Port)
		}
	}
	return nil
}

func sortProxies(list []Proxy) {
	sort.Slice(list, func(i, j int) bool { return list[i].Port < list[j].Port })
}

func loadProxies(path string) ([]Proxy, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []Proxy
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	sortProxies(list)
	return list, nil
}

func saveProxies(path string, list []Proxy) error {
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'), 0o600, false)
}

// writeFileAtomic replaces path with data through a temporary file in the
// same directory. If path exists its mode is kept and, with keepOwner, its
// owner and group too, so a file 3proxy reads stays readable for it.
func writeFileAtomic(path string, data []byte, mode os.FileMode, keepOwner bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
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
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
		if keepOwner {
			chownLike(tmp.Name(), fi)
		}
	} else if keepOwner {
		if fi, err := os.Stat(dir); err == nil {
			chownLike(tmp.Name(), fi)
		}
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
