# 3proxy Panel

Panel web sederhana untuk mengelola proxy [3proxy](../README.md) dan memantau
status VPS. Ditulis dengan Go (tanpa library tambahan), login cukup dengan
satu password.

**Fitur**

- Tambah, lihat, edit, hapus, aktif/nonaktifkan proxy (CRUD).
- Tipe proxy: HTTP(S), SOCKS5, atau HTTP + SOCKS5 di satu port.
- Setiap proxy punya port, username dan password sendiri, plus IP keluar
  (untuk VPS dengan banyak IP) dan catatan.
- Tombol salin `ip:port:user:pass` atau URL `socks5://user:pass@ip:port`,
  dan export semua proxy aktif dalam satu daftar.
- Status VPS yang diperbarui tiap 3 detik: CPU, RAM, swap, disk, jaringan,
  uptime, load, status service 3proxy, dan jumlah koneksi aktif per proxy.
- Login dikunci 10 menit setelah 5 kali salah password.

---

## Tutorial instalasi di Ubuntu

Untuk **Ubuntu 22.04 / 24.04** (atau Debian 12) dengan akses root/sudo. Script
installer otomatis memasang 3proxy (dari repository resmi 3proxy.org), Go,
dan panelnya.

### Langkah 1: Masuk ke VPS

Dari komputer kamu, login SSH ke VPS (ganti `IP-VPS` dengan IP VPS kamu):

```bash
ssh root@IP-VPS
```

### Langkah 2: Pasang git dan download kode

```bash
sudo apt update && sudo apt install -y git
```

```bash
git clone https://github.com/kontaknurman/3proxy.git ~/3proxy
```

### Langkah 3: Jalankan installer

```bash
cd ~/3proxy && sudo bash panel/install.sh
```

Installer akan meminta kamu membuat **password login panel** (ketik dua kali).
Kalau dikosongkan, password acak dibuat dan ditampilkan di akhir. Tunggu
sampai muncul:

```
==> Selesai! Buka panel di: http://IP-VPS:8088
```

### Langkah 4: Buka port di firewall

Panel memakai port **8088**, dan setiap proxy memakai port yang kamu pilih
(default mulai dari 3128). Kalau firewall `ufw` aktif, buka port-portnya:

```bash
sudo ufw allow 8088/tcp
```

```bash
sudo ufw allow 3128:3200/tcp
```

Kalau provider VPS kamu punya firewall sendiri (misalnya *Security Group* di
AWS, *Firewall* di DigitalOcean/Vultr), buka port yang sama di sana.

### Langkah 5: Login dan buat proxy

1. Buka `http://IP-VPS:8088` di browser, masukkan password.
2. Di bagian **Tambah proxy**, pilih tipe, isi port, klik **Acak** untuk
   username dan password, lalu klik **Tambah**.
3. Klik **Salin** pada proxy untuk menyalin `ip:port:user:pass`.

Kartu **Service 3proxy** bisa menunjukkan *Mati* sebelum proxy pertama
dibuat, karena config bawaan paket belum lengkap. Itu normal: begitu proxy
pertama ditambahkan, panel menulis config baru dan menyalakan 3proxy.

Tes proxy dari komputer kamu (ganti user, pass, IP dan port):

```bash
curl -x http://user:pass@IP-VPS:3128 https://api.ipify.org
```

```bash
curl -x socks5h://user:pass@IP-VPS:3129 https://api.ipify.org
```

Kalau yang muncul adalah IP VPS kamu, proxy sudah jalan.

---

## Tutorial update

### Update panel

Data proxy dan password tetap aman, installer hanya mem-build ulang panel:

```bash
cd ~/3proxy && git pull && sudo bash panel/install.sh
```

### Update 3proxy

```bash
sudo apt update && sudo apt-get install --only-upgrade -o Dpkg::Options::=--force-confold 3proxy
```

Opsi `--force-confold` menjaga config yang dibuat panel. Kalau tanpa opsi itu
muncul pertanyaan tentang `/usr/local/3proxy/conf/3proxy.cfg`, jawab **N**
(pertahankan versi sekarang). Kalau config terlanjur tertimpa, buka panel dan
klik **Terapkan ulang**.

---

## Perintah yang sering dipakai

| Keperluan | Perintah |
| --- | --- |
| Cek status panel | `sudo systemctl status 3proxy-panel` |
| Cek status 3proxy | `sudo systemctl status 3proxy` |
| Lihat log panel | `sudo journalctl -u 3proxy-panel -f` |
| Restart panel | `sudo systemctl restart 3proxy-panel` |
| Restart 3proxy | `sudo systemctl restart 3proxy` |
| Lihat config 3proxy | `sudo cat /usr/local/3proxy/conf/3proxy.cfg` |

### Ganti password atau port panel

Edit file pengaturan, ubah `PANEL_PASSWORD` atau `PANEL_LISTEN`, lalu restart:

```bash
sudo nano /etc/3proxy-panel/panel.env
```

```bash
sudo systemctl restart 3proxy-panel
```

Password tidak boleh berisi spasi, tanda kutip atau backslash.

---

## Keamanan dan HTTPS

Panel berjalan dengan HTTP biasa, jadi password dikirim tanpa enkripsi. Pilih
salah satu cara berikut kalau ingin lebih aman.

**Cara 1: lewat SSH tunnel (tanpa domain).** Set `PANEL_LISTEN='127.0.0.1:8088'`
di `/etc/3proxy-panel/panel.env`, restart panel, lalu dari komputer kamu:

```bash
ssh -L 8088:127.0.0.1:8088 root@IP-VPS
```

Selama SSH itu terbuka, panel bisa dibuka di `http://localhost:8088`.

**Cara 2: HTTPS dengan Let's Encrypt (butuh domain yang mengarah ke VPS, dan
port 80 terbuka).**

```bash
sudo apt install -y certbot
```

```bash
sudo certbot certonly --standalone -d panel.domainkamu.com --deploy-hook "systemctl restart 3proxy-panel"
```

Lalu di `/etc/3proxy-panel/panel.env` aktifkan (hapus `#`) dan sesuaikan:

```
PANEL_TLS_CERT='/etc/letsencrypt/live/panel.domainkamu.com/fullchain.pem'
PANEL_TLS_KEY='/etc/letsencrypt/live/panel.domainkamu.com/privkey.pem'
```

```bash
sudo systemctl restart 3proxy-panel
```

Panel sekarang di `https://panel.domainkamu.com:8088`.

---

## Cara kerja

- Daftar proxy disimpan di `/var/lib/3proxy-panel/proxies.json`.
- Setiap ada perubahan, panel menulis ulang config 3proxy
  `/usr/local/3proxy/conf/3proxy.cfg` (sama dengan
  `/etc/3proxy/conf/3proxy.cfg`) lalu menjalankan
  `systemctl reload-or-restart 3proxy`. Kalau perintah itu gagal, 3proxy tetap
  memuat config baru sendiri dalam 1 menit (`monitor`).
- Config bawaan yang bukan buatan panel disalin dulu ke
  `3proxy.cfg.bak-<tanggal>` sebelum ditimpa pertama kali.
- Paket apt menjalankan 3proxy di dalam chroot sebagai user `proxy`, jadi
  port proxy harus **1024 ke atas**.
- Pengguna proxy tidak bisa mengakses alamat `127.0.0.0/8` di VPS (termasuk
  panel ini).
- 3proxy memakai satu daftar user untuk semua port, jadi username yang sama
  boleh dipakai di beberapa proxy hanya jika password-nya sama.

### Pengaturan

Semua pengaturan ada di `/etc/3proxy-panel/panel.env`:

| Variabel | Default | Keterangan |
| --- | --- | --- |
| `PANEL_PASSWORD` | (wajib) | Password login panel |
| `PANEL_LISTEN` | `:8088` | Alamat dan port panel |
| `PANEL_DATA` | `/var/lib/3proxy-panel/proxies.json` | File data proxy |
| `PANEL_CONFIG` | `/usr/local/3proxy/conf/3proxy.cfg` | Config 3proxy yang ditulis panel |
| `PANEL_CHROOT` | `/usr/local/3proxy` | Folder chroot 3proxy, kosongkan kalau 3proxy tidak memakai chroot |
| `PANEL_RELOAD` | `systemctl reload-or-restart 3proxy` | Perintah setelah config berubah |
| `PANEL_HOST` | alamat yang dipakai membuka panel | IP/domain yang ditampilkan di daftar proxy |
| `PANEL_HEADER` | (kosong) | File berisi pengaturan global 3proxy pengganti bawaan panel |
| `PANEL_TLS_CERT`, `PANEL_TLS_KEY` | (kosong) | Sertifikat dan key untuk HTTPS |

Pengaturan global bawaan panel adalah:

```
nserver 1.1.1.1
nserver 8.8.8.8
nscache 65536
timeouts 1 5 30 60 180 1800 15 60
```

Untuk menambah misalnya log, buat file (contoh `/etc/3proxy-panel/header.cfg`)
berisi baris di atas ditambah `log /logs/3proxy-%y%m%d.log D` (path di dalam
chroot, jadi file log ada di `/var/log/3proxy/`), isi
`PANEL_HEADER='/etc/3proxy-panel/header.cfg'`, restart panel lalu klik
**Terapkan ulang**.

---

## Menghapus panel

```bash
sudo systemctl disable --now 3proxy-panel
```

```bash
sudo rm -rf /usr/local/bin/3proxy-panel /etc/systemd/system/3proxy-panel.service /etc/3proxy-panel /var/lib/3proxy-panel
```

```bash
sudo systemctl daemon-reload
```

Config 3proxy lama bisa dikembalikan dari file `3proxy.cfg.bak-*` di
`/usr/local/3proxy/conf/`.

---

## Development

```bash
cd panel && go test ./... && go build -o 3proxy-panel .
```

```bash
PANEL_PASSWORD=rahasia PANEL_CHROOT= PANEL_CONFIG=/tmp/3proxy.cfg PANEL_DATA=/tmp/proxies.json PANEL_RELOAD= ./3proxy-panel
```
