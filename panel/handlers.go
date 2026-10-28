package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// proxyView is a proxy with the strings a client connects with.
type proxyView struct {
	Proxy
	Host string
	URL  string
	Line string
}

type pageData struct {
	Flash      string
	FlashError bool
	Proxies    []proxyView
	Form       Proxy
	FormError  string
	LocalIPs   []string
}

func (a *app) render(w http.ResponseWriter, page string, status int, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := a.pages[page].Execute(w, data); err != nil {
		log.Printf("render %s: %v", page, err)
	}
}

// publicHost is the address shown to connect to a proxy without an
// outgoing address of its own.
func (a *app) publicHost(r *http.Request) string {
	if a.host != "" {
		return a.host
	}
	if host, _, err := net.SplitHostPort(r.Host); err == nil {
		return host
	}
	return r.Host
}

func (a *app) view(r *http.Request, p Proxy) proxyView {
	host := p.ExtIP
	if host == "" {
		host = a.publicHost(r)
	}
	scheme := "http"
	if p.Type == "socks5" {
		scheme = "socks5"
	}
	port := strconv.Itoa(p.Port)
	return proxyView{
		Proxy: p,
		Host:  host,
		URL:   fmt.Sprintf("%s://%s:%s@%s", scheme, p.Username, p.Password, net.JoinHostPort(host, port)),
		Line:  fmt.Sprintf("%s:%s:%s:%s", host, port, p.Username, p.Password),
	}
}

func (a *app) loginPage(w http.ResponseWriter, r *http.Request) {
	if a.auth.loggedIn(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	a.render(w, "login", http.StatusOK, pageData{})
}

func (a *app) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "cross-site request rejected", http.StatusForbidden)
		return
	}
	if a.auth.locked(r) {
		a.render(w, "login", http.StatusTooManyRequests, pageData{
			Flash: "Terlalu banyak percobaan gagal. Coba lagi dalam 10 menit.", FlashError: true,
		})
		return
	}
	if !a.auth.login(w, r, r.PostFormValue("password")) {
		log.Printf("failed login from %s", clientIP(r))
		a.render(w, "login", http.StatusUnauthorized, pageData{Flash: "Password salah.", FlashError: true})
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *app) logoutSubmit(w http.ResponseWriter, r *http.Request) {
	a.auth.logout(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// indexData fills the dashboard; form is the content of the "add proxy"
// form.
func (a *app) indexData(r *http.Request, form Proxy, formErr string) pageData {
	list := a.list()
	views := make([]proxyView, len(list))
	for i, p := range list {
		views[i] = a.view(r, p)
	}
	if form.Type == "" {
		form = Proxy{Type: "http", Port: a.suggestPort(list), Enabled: true}
	}
	d := pageData{Proxies: views, Form: form, FormError: formErr, LocalIPs: localIPs()}
	d.Flash, d.FlashError = a.auth.takeFlash(r)
	return d
}

func (a *app) index(w http.ResponseWriter, r *http.Request) {
	a.render(w, "index", http.StatusOK, a.indexData(r, Proxy{}, ""))
}

// readForm fills p from the proxy form.
func readForm(r *http.Request, p *Proxy) {
	p.Type = r.PostFormValue("type")
	p.Port, _ = strconv.Atoi(strings.TrimSpace(r.PostFormValue("port")))
	p.Username = strings.TrimSpace(r.PostFormValue("username"))
	p.Password = strings.TrimSpace(r.PostFormValue("password"))
	p.ExtIP = strings.TrimSpace(r.PostFormValue("ext_ip"))
	p.Note = strings.TrimSpace(r.PostFormValue("note"))
	p.Enabled = r.PostFormValue("enabled") != ""
}

// done reports the outcome of a change on the next page.
func (a *app) done(w http.ResponseWriter, r *http.Request, msg string, reloadErr error) {
	if reloadErr != nil {
		log.Printf("reload: %v", reloadErr)
		msg += fmt.Sprintf(". Reload 3proxy gagal (%v), 3proxy akan memuat config baru sendiri dalam 1 menit.", reloadErr)
	}
	a.auth.setFlash(r, msg, reloadErr != nil)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *app) create(w http.ResponseWriter, r *http.Request) {
	p := Proxy{ID: newID(), CreatedAt: time.Now().UTC()}
	readForm(r, &p)
	reloadErr, err := a.change(func(list []Proxy) ([]Proxy, error) {
		return append(list, p), nil
	})
	if err != nil {
		a.render(w, "index", http.StatusBadRequest, a.indexData(r, p, err.Error()))
		return
	}
	a.done(w, r, fmt.Sprintf("Proxy port %d ditambahkan", p.Port), reloadErr)
}

var errNotFound = errors.New("proxy tidak ditemukan")

// find returns the index of the proxy with the given ID.
func find(list []Proxy, id string) (int, error) {
	for i := range list {
		if list[i].ID == id {
			return i, nil
		}
	}
	return -1, errNotFound
}

func (a *app) editPage(w http.ResponseWriter, r *http.Request) {
	list := a.list()
	i, err := find(list, r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a.render(w, "edit", http.StatusOK, pageData{Form: list[i], LocalIPs: localIPs()})
}

func (a *app) update(w http.ResponseWriter, r *http.Request) {
	list := a.list()
	i, err := find(list, r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p := list[i]
	readForm(r, &p)
	reloadErr, err := a.change(func(list []Proxy) ([]Proxy, error) {
		i, err := find(list, p.ID)
		if err != nil {
			return nil, err
		}
		list[i] = p
		return list, nil
	})
	if err != nil {
		a.render(w, "edit", http.StatusBadRequest, pageData{Form: p, FormError: err.Error(), LocalIPs: localIPs()})
		return
	}
	a.done(w, r, fmt.Sprintf("Proxy port %d disimpan", p.Port), reloadErr)
}

func (a *app) toggle(w http.ResponseWriter, r *http.Request) {
	var p Proxy
	reloadErr, err := a.change(func(list []Proxy) ([]Proxy, error) {
		i, err := find(list, r.PathValue("id"))
		if err != nil {
			return nil, err
		}
		list[i].Enabled = !list[i].Enabled
		p = list[i]
		return list, nil
	})
	if err != nil {
		a.auth.setFlash(r, err.Error(), true)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	state := "dinonaktifkan"
	if p.Enabled {
		state = "diaktifkan"
	}
	a.done(w, r, fmt.Sprintf("Proxy port %d %s", p.Port, state), reloadErr)
}

func (a *app) remove(w http.ResponseWriter, r *http.Request) {
	var p Proxy
	reloadErr, err := a.change(func(list []Proxy) ([]Proxy, error) {
		i, err := find(list, r.PathValue("id"))
		if err != nil {
			return nil, err
		}
		p = list[i]
		return append(list[:i], list[i+1:]...), nil
	})
	if err != nil {
		a.auth.setFlash(r, err.Error(), true)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	a.done(w, r, fmt.Sprintf("Proxy port %d dihapus", p.Port), reloadErr)
}

// reapply writes the configuration again and reloads 3proxy.
func (a *app) reapply(w http.ResponseWriter, r *http.Request) {
	reloadErr, err := a.change(func(list []Proxy) ([]Proxy, error) { return list, nil })
	if err != nil {
		a.auth.setFlash(r, err.Error(), true)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	a.done(w, r, "Config 3proxy ditulis ulang", reloadErr)
}

// export lists the enabled proxies as host:port:user:password lines.
func (a *app) export(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for _, p := range a.list() {
		if p.Enabled {
			fmt.Fprintln(w, a.view(r, p).Line)
		}
	}
}

func (a *app) status(w http.ResponseWriter, r *http.Request) {
	var ports []int
	for _, p := range a.list() {
		if p.Enabled {
			ports = append(ports, p.Port)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.sampler.status(ports))
}
