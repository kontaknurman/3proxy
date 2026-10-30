#!/bin/bash
# Installs 3proxy-panel on Ubuntu or Debian. Running it again updates it and
# keeps its settings and proxies.
#
#   sudo bash panel/install.sh            3proxy from apt, the panel as a systemd service
#   sudo bash panel/install.sh --docker   3proxy and the panel in a Docker container
#
# The first run asks for the password, the panel port and the range of proxy
# ports. Without a terminal they are taken from PANEL_PASSWORD, PANEL_PORT
# and PANEL_PROXY_PORTS, or made up.
set -euo pipefail

GO_VERSION=1.24.7
GO_MIN=1.22
DEFAULT_PORT=8088
DEFAULT_PROXY_PORTS=20000-29999
PANEL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(dirname "$PANEL_DIR")"
ENV_DIR=/etc/3proxy-panel
# Present when 3proxy was built from this repository rather than installed
# from 3proxy.org: later runs build it again, which updates it.
SOURCE_MARK=$ENV_DIR/3proxy-from-source
BIN=/usr/local/bin/3proxy-panel
UNIT=/etc/systemd/system/3proxy-panel.service

info() { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mPerhatian:\033[0m %s\n' "$*" >&2; }
die() { printf '\033[1;31mGagal:\033[0m %s\n' "$*" >&2; exit 1; }

mode=native
case "${1:-}" in
"") ;;
--docker) mode=docker ;;
*) die "pilihan tidak dikenal: $1 (tanpa pilihan untuk instalasi biasa, --docker untuk Docker)" ;;
esac

[ "$(id -u)" -eq 0 ] || die "jalankan dengan sudo: sudo bash $0 $*"
command -v apt-get >/dev/null || die "script ini untuk Ubuntu/Debian (apt)"
command -v systemctl >/dev/null || die "systemd tidak ditemukan"

interactive() { [ -t 0 ]; }

# ask PROMPT DEFAULT prints the answer, or DEFAULT without a terminal.
ask() {
	local answer=""
	if interactive; then
		read -rp "$1 [$2]: " answer
	fi
	printf '%s\n' "${answer:-$2}"
}

valid_port() {
	[[ "$1" =~ ^[0-9]+$ ]] && [ "$1" -ge 1 ] && [ "$1" -le 65535 ]
}

# port_user PORT names the programs listening on PORT, nothing if it is free.
port_user() {
	local lines names
	command -v ss >/dev/null || return 0
	lines=$(ss -Hltnp "sport = :$1" 2>/dev/null) || return 0
	[ -n "$lines" ] || return 0
	names=$(printf '%s\n' "$lines" | sed -n 's/.*users:(("\([^"]*\)".*/\1/p' | sort -u | paste -sd, -)
	printf '%s\n' "${names:-program lain}"
}

# Passwords are written between single quotes into the settings file.
valid_password() {
	case "$1" in
	"" | *[\'\"\\\ ]*) return 1 ;;
	esac
}

# ask_password prints the password typed twice, nothing to have one made up.
ask_password() {
	local password again
	interactive || return 0
	while :; do
		read -rsp "Buat password login panel (kosongkan agar dibuat acak): " password
		echo >&2
		if [ -z "$password" ]; then
			return 0
		elif ! valid_password "$password"; then
			warn "password tidak boleh berisi spasi, tanda kutip atau backslash"
			continue
		fi
		read -rsp "Ulangi password: " again
		echo >&2
		if [ "$password" = "$again" ]; then
			printf '%s\n' "$password"
			return 0
		fi
		warn "password tidak sama, coba lagi"
	done
}

# ask_panel_port prints a free port for the panel.
ask_panel_port() {
	local port user default=${PANEL_PORT:-$DEFAULT_PORT}
	while :; do
		port=$(ask "Port panel" "$default")
		if ! valid_port "$port"; then
			warn "$port bukan nomor port (1-65535)"
		else
			user=$(port_user "$port")
			if [ -z "$user" ] || [ "$user" = "3proxy-panel" ]; then
				printf '%s\n' "$port"
				return 0
			fi
			warn "port $port sudah dipakai oleh $user, pilih port lain"
		fi
		interactive || die "pilih port panel lain lewat PANEL_PORT"
		default=$DEFAULT_PORT
	done
}

# ask_proxy_ports LOWEST prints the range of ports proxies may use.
ask_proxy_ports() {
	local range min max
	while :; do
		range=$(ask "Rentang port proxy" "${PANEL_PROXY_PORTS:-$DEFAULT_PROXY_PORTS}")
		range=${range// /}
		if [[ "$range" =~ ^([0-9]+)-([0-9]+)$ ]]; then
			min=$((10#${BASH_REMATCH[1]}))
			max=$((10#${BASH_REMATCH[2]}))
			if [ "$min" -ge "$1" ] && [ "$max" -le 65535 ] && [ "$min" -le "$max" ]; then
				printf '%s-%s\n' "$min" "$max"
				return 0
			fi
		fi
		warn "tulis rentang port antara $1 dan 65535, contoh: $DEFAULT_PROXY_PORTS"
		interactive || die "PANEL_PROXY_PORTS tidak valid"
	done
}

# ask_settings LOWEST sets password, generated, panel_port and proxy_ports.
ask_settings() {
	echo
	info "Pengaturan panel"
	password=${PANEL_PASSWORD:-}
	if [ -n "$password" ]; then
		valid_password "$password" || die "PANEL_PASSWORD tidak boleh berisi spasi, tanda kutip atau backslash"
	else
		password=$(ask_password)
	fi
	generated=""
	if [ -z "$password" ]; then
		password=$(head -c 48 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | cut -c1-16)
		generated=1
	fi
	panel_port=$(ask_panel_port)
	proxy_ports=$(ask_proxy_ports "$1")
}

# open_firewall PORT RANGE opens the panel and proxy ports in ufw, if it is
# on.
open_firewall() {
	local answer ufw_range=${2/-/:}
	command -v ufw >/dev/null || return 0
	ufw status 2>/dev/null | grep "Status: active" >/dev/null || return 0
	answer=$(ask "Firewall ufw aktif. Buka port $1 dan $2 di ufw? (y/n)" y)
	case "$answer" in
	[yY]*)
		ufw allow "$1/tcp" >/dev/null
		ufw allow "$ufw_range/tcp" >/dev/null
		info "Port $1 dan $2 dibuka di ufw"
		;;
	*) warn "buka sendiri nanti: sudo ufw allow $1/tcp && sudo ufw allow $ufw_range/tcp" ;;
	esac
}

# get_env FILE KEY prints the value of KEY in a settings file.
get_env() {
	sed -n "s/^$2=//p" "$1" | tail -n1 | tr -d "'\""
}

# finish FILE tells where the panel is.
finish() {
	local listen port ports ip scheme=http
	listen=$(get_env "$1" PANEL_LISTEN)
	port=${listen##*:}
	[ -n "$port" ] || port=$(get_env "$1" PANEL_PORT)
	port=${port:-$DEFAULT_PORT}
	ports=$(get_env "$1" PANEL_PROXY_PORTS)
	[ -n "$(get_env "$1" PANEL_TLS_CERT)" ] && scheme=https
	ip=$(hostname -I 2>/dev/null | awk '{print $1}')

	echo
	info "Selesai! Buka panel di: $scheme://${ip:-IP-VPS}:$port"
	if [ -n "${generated:-}" ]; then
		echo "    Password login: $password"
	fi
	echo "    Pengaturan: $1"
	echo "    Port proxy: ${ports:-semua port yang bisa dipakai 3proxy}"
	echo "    Kalau provider VPS punya firewall sendiri (Security Group dsb.),"
	echo "    buka juga port $port dan ${ports:-port proxy} (TCP) di sana."
}

# install_3proxy_apt installs 3proxy from the signed repository at
# 3proxy.org, and fails when the VPS cannot reach it. It runs as an if
# condition, where set -e does not apply, so every step is checked.
install_3proxy_apt() {
	local key=/usr/share/keyrings/3proxy.asc list=/etc/apt/sources.list.d/3proxy.sources
	info "Memasang 3proxy dari repository 3proxy.org"
	apt-get update || return 1
	apt-get install -y curl ca-certificates || return 1
	curl -fsSL --max-time 60 https://3proxy.org/repo/3proxy-release-key.asc -o "$key.new" || return 1
	mv -f "$key.new" "$key"
	cat >"$list" <<'EOF'
Types: deb
URIs: https://3proxy.org/repo/deb
Suites: current
Components: main
Signed-By: /usr/share/keyrings/3proxy.asc
EOF
	if ! { apt-get update && apt-get install -y 3proxy; }; then
		# A repository apt cannot reach spoils every later apt update.
		rm -f "$list"
		return 1
	fi
}

# install_3proxy_source builds 3proxy from this repository and installs it
# the way the deb package does: binaries in /bin, chrooted into
# /usr/local/3proxy, run by the 3proxy systemd service.
install_3proxy_source() {
	local tmp
	info "Build 3proxy dari kode di repo ini"
	if ! command -v make >/dev/null || ! command -v gcc >/dev/null; then
		if ! { apt-get update && apt-get install -y build-essential; }; then
			die "build-essential tidak bisa dipasang"
		fi
	fi
	tmp=$(mktemp -d)
	tar -C "$REPO_DIR" --exclude=.git --exclude=./panel -cf - . | tar -C "$tmp" -xf -
	if ! (cd "$tmp" && make -f Makefile.Linux -j"$(nproc)" && make -f Makefile.Linux install) >"$tmp/build.log" 2>&1; then
		tail -n 30 "$tmp/build.log" >&2
		die "build 3proxy gagal, log lengkap: $tmp/build.log"
	fi
	rm -rf "$tmp"
	systemctl daemon-reload
	mkdir -p "$ENV_DIR"
	touch "$SOURCE_MARK"
	info "3proxy terpasang dari kode: $(command -v 3proxy)"
}

# find_go prints a Go of at least GO_MIN, installing one when needed: from
# go.dev, else from apt.
find_go() {
	local candidate arch tmp
	go_ok() {
		local v
		v=$("$1" env GOVERSION 2>/dev/null) || return 1
		v=${v#go}
		[ "$(printf '%s\n%s\n' "$GO_MIN" "$v" | sort -V | head -n1)" = "$GO_MIN" ]
	}
	for candidate in go /usr/local/go/bin/go /usr/lib/go/bin/go; do
		if command -v "$candidate" >/dev/null && go_ok "$candidate"; then
			command -v "$candidate"
			return 0
		fi
	done
	case "$(uname -m)" in
	x86_64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	armv6l | armv7l) arch=armv6l ;;
	*) die "arsitektur $(uname -m) tidak didukung" ;;
	esac
	command -v curl >/dev/null || apt-get install -y curl ca-certificates >&2
	info "Memasang Go $GO_VERSION ke /usr/local/go" >&2
	tmp=$(mktemp -d)
	if curl -fsSL --max-time 300 "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" -o "$tmp/go.tar.gz"; then
		rm -rf /usr/local/go
		tar -C /usr/local -xzf "$tmp/go.tar.gz"
		rm -rf "$tmp"
		echo /usr/local/go/bin/go
		return 0
	fi
	rm -rf "$tmp"
	warn "go.dev tidak bisa diakses, mencoba Go dari apt"
	apt-get install -y golang-go >&2 || true
	if go_ok /usr/lib/go/bin/go; then
		echo /usr/lib/go/bin/go
		return 0
	fi
	die "Go $GO_MIN atau lebih baru tidak bisa dipasang"
}

install_native() {
	local env_file=$ENV_DIR/panel.env
	if command -v docker >/dev/null && docker ps -a --format '{{.Names}}' 2>/dev/null | grep -x 3proxy-panel >/dev/null; then
		die "panel versi Docker sudah terpasang. Update dengan: sudo bash $0 --docker"
	fi

	# 3proxy from 3proxy.org, or from source when it was built from source
	# before (which updates it) or 3proxy.org cannot be reached.
	if [ -f "$SOURCE_MARK" ]; then
		install_3proxy_source
	elif command -v 3proxy >/dev/null; then
		info "3proxy sudah terpasang: $(command -v 3proxy)"
	elif ! install_3proxy_apt; then
		warn "repository 3proxy.org tidak bisa diakses dari VPS ini (sering karena diblokir DNS provider), 3proxy di-build dari kode di repo ini"
		install_3proxy_source
	fi
	systemctl enable 3proxy >/dev/null 2>&1 || true

	local go
	go=$(find_go)
	info "Memakai $("$go" version)"

	info "Build 3proxy-panel"
	(
		cd "$PANEL_DIR"
		export HOME="${HOME:-/root}"
		CGO_ENABLED=0 GOTOOLCHAIN=local "$go" build -buildvcs=false -trimpath -ldflags "-s -w" -o "$BIN.new" .
	)
	chmod 0755 "$BIN.new"
	mv -f "$BIN.new" "$BIN"

	# Settings, written once and kept on updates.
	mkdir -p "$ENV_DIR"
	chmod 0700 "$ENV_DIR"
	if [ -f "$env_file" ]; then
		info "Pengaturan lama dipakai: $env_file"
	else
		# The packaged 3proxy runs chrooted without root: no ports below 1024.
		ask_settings 1024
		cat >"$env_file" <<EOF
# Pengaturan 3proxy-panel. Setelah mengubah file ini jalankan:
#   sudo systemctl restart 3proxy-panel

# Password login panel.
PANEL_PASSWORD='$password'

# Port panel.
PANEL_PORT='$panel_port'

# Rentang port yang boleh dipakai proxy (minimal 1024). Port yang sudah
# dipakai program lain di VPS ditolak otomatis.
PANEL_PROXY_PORTS='$proxy_ports'

# Lokasi data proxy dan config 3proxy (paket apt menjalankan 3proxy di dalam
# chroot /usr/local/3proxy).
PANEL_DATA='/var/lib/3proxy-panel/proxies.json'
PANEL_CONFIG='/usr/local/3proxy/conf/3proxy.cfg'
PANEL_CHROOT='/usr/local/3proxy'
PANEL_RELOAD='systemctl reload-or-restart 3proxy'

# IP/domain yang ditampilkan di daftar proxy. Default: alamat yang dipakai
# untuk membuka panel.
#PANEL_HOST='203.0.113.10'

# Panel hanya bisa dibuka dari VPS sendiri (untuk SSH tunnel). Menggantikan
# PANEL_PORT.
#PANEL_LISTEN='127.0.0.1:8088'

# HTTPS, misalnya dengan sertifikat Let's Encrypt.
#PANEL_TLS_CERT='/etc/letsencrypt/live/panel.domainkamu.com/fullchain.pem'
#PANEL_TLS_KEY='/etc/letsencrypt/live/panel.domainkamu.com/privkey.pem'
EOF
		chmod 0600 "$env_file"
		info "Pengaturan dibuat di $env_file"
		open_firewall "$panel_port" "$proxy_ports"
	fi

	install -m 0644 "$PANEL_DIR/3proxy-panel.service" "$UNIT"
	systemctl daemon-reload
	systemctl enable 3proxy-panel >/dev/null 2>&1
	systemctl restart 3proxy-panel
	sleep 2
	if ! systemctl is-active --quiet 3proxy-panel; then
		journalctl -u 3proxy-panel -n 20 --no-pager || true
		die "3proxy-panel tidak mau jalan, lihat log di atas"
	fi
	finish "$env_file"
}

install_docker() {
	local env_file=$PANEL_DIR/.env started state
	if systemctl is-active --quiet 3proxy-panel 2>/dev/null; then
		die "panel versi tanpa Docker sedang jalan. Hentikan dulu: sudo systemctl disable --now 3proxy-panel 3proxy"
	fi
	if systemctl is-active --quiet 3proxy 2>/dev/null; then
		warn "service 3proxy dari apt sedang jalan dan bisa bentrok port. Hentikan dengan: sudo systemctl disable --now 3proxy"
	fi

	if ! command -v docker >/dev/null; then
		info "Memasang Docker"
		command -v curl >/dev/null || { apt-get update && apt-get install -y curl ca-certificates; }
		curl -fsSL https://get.docker.com | sh
	fi
	systemctl enable --now docker >/dev/null 2>&1 || true
	if ! docker compose version >/dev/null 2>&1; then
		info "Memasang Docker Compose"
		if ! { apt-get update && apt-get install -y docker-compose-plugin; }; then
			die "Docker Compose tidak bisa dipasang"
		fi
	fi
	info "Memakai $(docker --version)"

	if [ -f "$env_file" ]; then
		info "Pengaturan lama dipakai: $env_file"
	else
		# In the container 3proxy runs as root, every port can be used.
		ask_settings 1
		cat >"$env_file" <<EOF
# Pengaturan 3proxy-panel untuk Docker. Setelah mengubah file ini jalankan:
#   sudo docker compose up -d

# Password login panel.
PANEL_PASSWORD='$password'

# Port panel.
PANEL_PORT=$panel_port

# Rentang port yang boleh dipakai proxy. Port yang sudah dipakai program
# lain di VPS ditolak otomatis.
PANEL_PROXY_PORTS=$proxy_ports

# Pengaturan lain ada di .env.example.
EOF
		chmod 0600 "$env_file"
		info "Pengaturan dibuat di $env_file"
		open_firewall "$panel_port" "$proxy_ports"
	fi

	info "Build dan jalankan container (pertama kali butuh beberapa menit)"
	(cd "$PANEL_DIR" && docker compose up -d --build)
	# A panel which cannot start is restarted by Docker over and over.
	started=$(docker inspect -f '{{.State.StartedAt}}' 3proxy-panel 2>/dev/null || true)
	sleep 4
	state=$(docker inspect -f '{{.State.Status}} {{.State.StartedAt}}' 3proxy-panel 2>/dev/null || true)
	if [ "$state" != "running $started" ]; then
		(cd "$PANEL_DIR" && docker compose logs --tail 20) || true
		die "container 3proxy-panel tidak mau jalan, lihat log di atas"
	fi
	finish "$env_file"
}

if [ "$mode" = docker ]; then
	install_docker
else
	install_native
fi
