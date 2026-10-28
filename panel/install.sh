#!/bin/bash
# Installs 3proxy-panel on Ubuntu or Debian, together with 3proxy and Go
# when they are missing. Running it again updates the panel and keeps its
# settings and proxies.
#
#   sudo bash panel/install.sh
set -euo pipefail

GO_VERSION=1.24.7
GO_MIN=1.22
PANEL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_DIR=/etc/3proxy-panel
ENV_FILE=$ENV_DIR/panel.env
BIN=/usr/local/bin/3proxy-panel
UNIT=/etc/systemd/system/3proxy-panel.service

info() { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mPerhatian:\033[0m %s\n' "$*"; }
die() { printf '\033[1;31mGagal:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "jalankan dengan sudo: sudo bash $0"
command -v apt-get >/dev/null || die "script ini untuk Ubuntu/Debian (apt)"
command -v systemctl >/dev/null || die "systemd tidak ditemukan"

# 1. 3proxy, from the signed repository at 3proxy.org.
if command -v 3proxy >/dev/null; then
	info "3proxy sudah terpasang: $(command -v 3proxy)"
else
	info "Memasang 3proxy dari repository 3proxy.org"
	apt-get update
	apt-get install -y curl ca-certificates
	curl -fsSL https://3proxy.org/repo/3proxy-release-key.asc -o /usr/share/keyrings/3proxy.asc
	cat >/etc/apt/sources.list.d/3proxy.sources <<'EOF'
Types: deb
URIs: https://3proxy.org/repo/deb
Suites: current
Components: main
Signed-By: /usr/share/keyrings/3proxy.asc
EOF
	apt-get update
	apt-get install -y 3proxy
fi
systemctl enable 3proxy >/dev/null 2>&1 || true

# 2. Go, only to build the panel.
go_ok() {
	local v
	v=$("$1" env GOVERSION 2>/dev/null) || return 1
	v=${v#go}
	[ "$(printf '%s\n%s\n' "$GO_MIN" "$v" | sort -V | head -n1)" = "$GO_MIN" ]
}
GO=""
for candidate in go /usr/local/go/bin/go; do
	if command -v "$candidate" >/dev/null && go_ok "$candidate"; then
		GO=$(command -v "$candidate")
		break
	fi
done
if [ -z "$GO" ]; then
	case "$(uname -m)" in
	x86_64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	armv6l | armv7l) arch=armv6l ;;
	*) die "arsitektur $(uname -m) tidak didukung" ;;
	esac
	command -v curl >/dev/null || apt-get install -y curl ca-certificates
	info "Memasang Go $GO_VERSION ke /usr/local/go"
	tmp=$(mktemp -d)
	curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" -o "$tmp/go.tar.gz"
	rm -rf /usr/local/go
	tar -C /usr/local -xzf "$tmp/go.tar.gz"
	rm -rf "$tmp"
	GO=/usr/local/go/bin/go
fi
info "Memakai $("$GO" version)"

# 3. The panel itself.
info "Build 3proxy-panel"
(
	cd "$PANEL_DIR"
	export HOME="${HOME:-/root}"
	CGO_ENABLED=0 GOTOOLCHAIN=local "$GO" build -buildvcs=false -trimpath -ldflags "-s -w" -o "$BIN.new" .
)
chmod 0755 "$BIN.new"
mv -f "$BIN.new" "$BIN"

# 4. Settings, written once and kept on updates.
mkdir -p "$ENV_DIR"
chmod 0700 "$ENV_DIR"
# The password is written between single quotes into the env file.
valid_password() {
	case "$1" in
	"" | *[\'\"\\\ ]*) return 1 ;;
	esac
}
generated=""
if [ ! -f "$ENV_FILE" ]; then
	password="${PANEL_PASSWORD:-}"
	if [ -n "$password" ] && ! valid_password "$password"; then
		die "PANEL_PASSWORD tidak boleh berisi spasi, kutip atau backslash"
	fi
	while [ -z "$password" ] && [ -t 0 ]; do
		read -rsp "Buat password login panel (kosongkan agar dibuat acak): " password
		echo
		if [ -z "$password" ]; then
			break
		elif ! valid_password "$password"; then
			warn "password tidak boleh berisi spasi, kutip atau backslash"
			password=""
		else
			read -rsp "Ulangi password: " again
			echo
			if [ "$password" != "$again" ]; then
				warn "password tidak sama, coba lagi"
				password=""
			fi
		fi
	done
	if [ -z "$password" ]; then
		password=$(head -c 48 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | cut -c1-16)
		generated=1
	fi
	cat >"$ENV_FILE" <<EOF
# Pengaturan 3proxy-panel. Setelah mengubah file ini jalankan:
#   sudo systemctl restart 3proxy-panel

# Password untuk login ke panel.
PANEL_PASSWORD='$password'

# Alamat dan port panel.
PANEL_LISTEN=':${PANEL_PORT:-8088}'

# Lokasi data proxy dan config 3proxy (paket apt menjalankan 3proxy
# di dalam chroot /usr/local/3proxy).
PANEL_DATA='/var/lib/3proxy-panel/proxies.json'
PANEL_CONFIG='/usr/local/3proxy/conf/3proxy.cfg'
PANEL_CHROOT='/usr/local/3proxy'
PANEL_RELOAD='systemctl reload-or-restart 3proxy'

# IP/domain yang ditampilkan di daftar proxy. Default: alamat yang dipakai
# untuk membuka panel.
#PANEL_HOST='203.0.113.10'

# HTTPS, misalnya dengan sertifikat Let's Encrypt.
#PANEL_TLS_CERT='/etc/letsencrypt/live/panel.example.com/fullchain.pem'
#PANEL_TLS_KEY='/etc/letsencrypt/live/panel.example.com/privkey.pem'
EOF
	chmod 0600 "$ENV_FILE"
	info "Pengaturan dibuat di $ENV_FILE"
else
	info "Pengaturan lama dipakai: $ENV_FILE"
fi

# 5. systemd service.
install -m 0644 "$PANEL_DIR/3proxy-panel.service" "$UNIT"
systemctl daemon-reload
systemctl enable 3proxy-panel >/dev/null 2>&1
systemctl restart 3proxy-panel
sleep 1
if ! systemctl is-active --quiet 3proxy-panel; then
	journalctl -u 3proxy-panel -n 20 --no-pager || true
	die "3proxy-panel tidak mau jalan, lihat log di atas"
fi

listen=$(sed -n "s/^PANEL_LISTEN='\{0,1\}\([^']*\)'\{0,1\}$/\1/p" "$ENV_FILE" | tail -n1)
port=${listen##*:}
ip=$(hostname -I 2>/dev/null | awk '{print $1}')
scheme=http
grep -q '^PANEL_TLS_CERT=' "$ENV_FILE" && scheme=https

echo
info "Selesai! Buka panel di: $scheme://${ip:-IP-VPS}:${port:-8088}"
if [ -n "$generated" ]; then
	echo "    Password login: $password"
	echo "    (tersimpan di $ENV_FILE)"
fi
if command -v ufw >/dev/null && ufw status 2>/dev/null | grep -q "Status: active"; then
	warn "firewall ufw aktif. Buka port panel dan port proxy, contoh:"
	echo "    sudo ufw allow ${port:-8088}/tcp"
	echo "    sudo ufw allow 3128:3200/tcp"
fi
