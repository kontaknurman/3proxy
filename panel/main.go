// Command 3proxy-panel is a small web panel to manage the proxies of a
// 3proxy server and to watch the VPS it runs on. It is protected by a single
// password, stores the proxies in a JSON file and generates the 3proxy
// configuration from them.
package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed web
var webFS embed.FS

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func main() {
	port := flag.String("port", envOr("PANEL_PORT", "8088"), "port the panel listens on, on every address")
	listen := flag.String("listen", envOr("PANEL_LISTEN", ""), "address the panel listens on, such as 127.0.0.1:8088, instead of -port")
	proxyPorts := flag.String("proxy-ports", envOr("PANEL_PROXY_PORTS", ""), "range of ports proxies may use, such as 20000-29999, default every port 3proxy can use")
	dataFile := flag.String("data", envOr("PANEL_DATA", "/var/lib/3proxy-panel/proxies.json"), "file the proxies are stored in")
	// The defaults match the deb and rpm packages: 3proxy chroots into
	// /usr/local/3proxy and includes /conf/3proxy.cfg from there.
	cfgFile := flag.String("config", envOr("PANEL_CONFIG", "/usr/local/3proxy/conf/3proxy.cfg"), "3proxy configuration file the panel generates")
	chroot := flag.String("chroot", envOr("PANEL_CHROOT", "/usr/local/3proxy"), "directory 3proxy chroots into, empty if it does not")
	headerFile := flag.String("header", envOr("PANEL_HEADER", ""), "file with global 3proxy settings (nserver, log, ...) used instead of the defaults")
	// reload-or-restart also starts a 3proxy which is not running.
	reloadCmd := flag.String("reload", envOr("PANEL_RELOAD", "systemctl reload-or-restart 3proxy"), "command run after the configuration changes, empty to rely on 'monitor' only")
	host := flag.String("host", envOr("PANEL_HOST", ""), "public address shown in proxy strings, default the address the panel is opened at")
	tlsCert := flag.String("tls-cert", envOr("PANEL_TLS_CERT", ""), "TLS certificate file, enables HTTPS together with -tls-key")
	tlsKey := flag.String("tls-key", envOr("PANEL_TLS_KEY", ""), "TLS key file")
	flag.Parse()

	password := os.Getenv("PANEL_PASSWORD")
	if password == "" {
		log.Fatal("PANEL_PASSWORD is not set: export the panel password in it")
	}

	header := defaultHeader
	if *headerFile != "" {
		data, err := os.ReadFile(*headerFile)
		if err != nil {
			log.Fatal(err)
		}
		header = string(data)
	}

	cfgPath := *cfgFile
	if *chroot != "" {
		root := strings.TrimSuffix(*chroot, "/")
		rel, ok := strings.CutPrefix(*cfgFile, root+"/")
		if !ok {
			log.Fatalf("-config %s is not inside -chroot %s", *cfgFile, *chroot)
		}
		cfgPath = "/" + rel
	}

	// A chrooted 3proxy has dropped root and cannot bind low ports.
	lowest := 1
	if *chroot != "" {
		lowest = 1024
	}
	allowed, err := parsePortRange(*proxyPorts, lowest)
	if err != nil {
		log.Fatalf("PANEL_PROXY_PORTS: %v", err)
	}

	proxies, err := loadProxies(*dataFile)
	if err != nil {
		log.Fatal(err)
	}

	addr := *listen
	if addr == "" {
		if n, err := strconv.Atoi(*port); err != nil || n < 1 || n > 65535 {
			log.Fatalf("PANEL_PORT %q is not a port number", *port)
		}
		addr = ":" + *port
	}
	panelPort := 0
	if _, p, err := net.SplitHostPort(addr); err == nil {
		panelPort, _ = strconv.Atoi(p)
	}
	ln, err := net.Listen("tcp", addr)
	if errors.Is(err, syscall.EADDRINUSE) {
		log.Fatalf("port %d is already used by another program, set another PANEL_PORT", panelPort)
	} else if err != nil {
		log.Fatal(err)
	}

	useTLS := *tlsCert != "" && *tlsKey != ""
	a := &app{
		auth:      newAuth(password, useTLS),
		sampler:   &sampler{},
		pages:     parsePages(),
		dataFile:  *dataFile,
		cfgFile:   *cfgFile,
		cfgPath:   cfgPath,
		chrooted:  *chroot != "",
		header:    header,
		reloadCmd: *reloadCmd,
		host:      *host,
		panelPort: panelPort,
		ports:     allowed,
		proxies:   proxies,
	}
	// Without a configuration 3proxy cannot start, as in a new container.
	// An existing one is left alone until the first change.
	if _, err := os.Stat(a.cfgFile); errors.Is(err, os.ErrNotExist) {
		if err := writeConfig(a.cfgFile, renderConfig(proxies, header, cfgPath, a.chrooted)); err != nil {
			log.Fatal(err)
		}
		log.Printf("wrote %s", a.cfgFile)
	}
	go a.sampler.run(2 * time.Second)

	srv := &http.Server{
		Handler:           a.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Printf("3proxy-panel listening on %s, %d proxies on ports %s, config %s", addr, len(proxies), allowed, *cfgFile)
	if useTLS {
		err = srv.ServeTLS(ln, *tlsCert, *tlsKey)
	} else {
		err = srv.Serve(ln)
	}
	log.Fatal(err)
}

type app struct {
	auth      *auth
	sampler   *sampler
	pages     map[string]*template.Template
	dataFile  string
	cfgFile   string
	cfgPath   string // cfgFile as 3proxy sees it
	chrooted  bool
	header    string
	reloadCmd string
	host      string
	panelPort int
	ports     portRange // allowed for proxies

	mu      sync.Mutex
	proxies []Proxy
}

func parsePages() map[string]*template.Template {
	funcs := template.FuncMap{
		"types": func() []string { return []string{"http", "socks5", "auto"} },
	}
	pages := map[string]*template.Template{}
	for _, name := range []string{"login", "index", "edit"} {
		pages[name] = template.Must(template.New("layout.html").Funcs(funcs).
			ParseFS(webFS, "web/layout.html", "web/"+name+".html"))
	}
	return pages
}

func (a *app) routes() http.Handler {
	static, err := fs.Sub(webFS, "web/static")
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /login", a.loginPage)
	mux.HandleFunc("POST /login", a.loginSubmit)
	mux.HandleFunc("POST /logout", a.protect(a.logoutSubmit))
	mux.HandleFunc("GET /{$}", a.protect(a.index))
	mux.HandleFunc("POST /proxies", a.protect(a.create))
	mux.HandleFunc("GET /proxies/{id}", a.protect(a.editPage))
	mux.HandleFunc("POST /proxies/{id}", a.protect(a.update))
	mux.HandleFunc("POST /proxies/{id}/toggle", a.protect(a.toggle))
	mux.HandleFunc("POST /proxies/{id}/delete", a.protect(a.remove))
	mux.HandleFunc("POST /apply", a.protect(a.reapply))
	mux.HandleFunc("GET /export.txt", a.protect(a.export))
	mux.HandleFunc("GET /api/status", a.protect(a.status))
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		// Not no-referrer: browsers then send "Origin: null" with forms,
		// which sameOrigin cannot tell from a foreign request.
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// protect lets logged in users through and checks the origin of changes.
func (a *app) protect(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.auth.loggedIn(r) {
			if r.URL.Path == "/api/status" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			} else {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
			}
			return
		}
		if r.Method == http.MethodPost && !sameOrigin(r) {
			http.Error(w, "cross-site request rejected", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// list returns a copy of the proxies, sorted by port.
func (a *app) list() []Proxy {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Proxy(nil), a.proxies...)
}

// change applies fn to a copy of the proxies, then writes the 3proxy
// configuration and the proxy list. Nothing changes if fn or validation
// fails. reloadErr reports a failed reload command: the change is saved
// and 3proxy picks it up through 'monitor'.
func (a *app) change(fn func([]Proxy) ([]Proxy, error)) (reloadErr, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	next, err := fn(append([]Proxy(nil), a.proxies...))
	if err != nil {
		return nil, err
	}
	sortProxies(next)
	if err := validate(next, a.panelPort, a.ports); err != nil {
		return nil, err
	}
	if err := checkFree(next, a.proxies, listeningPorts()); err != nil {
		return nil, err
	}
	if err := writeConfig(a.cfgFile, renderConfig(next, a.header, a.cfgPath, a.chrooted)); err != nil {
		return nil, fmt.Errorf("gagal menulis config 3proxy: %w", err)
	}
	if err := saveProxies(a.dataFile, next); err != nil {
		// Put the configuration back in line with the list we keep.
		_ = writeConfig(a.cfgFile, renderConfig(a.proxies, a.header, a.cfgPath, a.chrooted))
		return nil, fmt.Errorf("gagal menyimpan data: %w", err)
	}
	a.proxies = next
	return runReload(a.reloadCmd), nil
}

// suggestPort proposes a port for a new proxy: the one after the highest
// proxy port, or the first free one after it in the allowed range. 0 means
// the range is full.
func suggestPort(list []Proxy, allowed portRange, panelPort int, listening map[int]bool) int {
	used := map[int]bool{panelPort: true}
	start := 3128
	if !allowed.contains(start) {
		start = allowed.Min
	}
	for _, p := range list {
		used[p.Port] = true
		if p.Port >= start && p.Port < allowed.Max {
			start = p.Port + 1
		}
	}
	size := allowed.Max - allowed.Min + 1
	for i := 0; i < size; i++ {
		port := allowed.Min + (start-allowed.Min+i)%size
		if !used[port] && !listening[port] {
			return port
		}
	}
	return 0
}

// localIPs lists the addresses of the VPS a proxy may connect out from.
func localIPs() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var v4, v6 []string
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.IsLinkLocalUnicast() {
			continue
		}
		if ipnet.IP.To4() != nil {
			v4 = append(v4, ipnet.IP.String())
		} else {
			v6 = append(v6, ipnet.IP.String())
		}
	}
	return append(v4, v6...)
}
