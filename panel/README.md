# 3proxy Panel

Panel web sederhana untuk mengelola proxy [3proxy](../README.md) dan memantau
status VPS. Ditulis dengan Go (tanpa library tambahan), login cukup dengan
satu password.

**Fitur**

- Tambah, lihat, edit, hapus, aktif/nonaktifkan proxy (CRUD).
- Tipe proxy: HTTP(S), SOCKS5, atau HTTP + SOCKS5 di satu port.
- Setiap proxy punya port, username dan password sendiri, plus IP keluar
  (untuk VPS dengan banyak IP) dan catatan.
- Port panel dan rentang port proxy bisa diatur. Port yang sudah dipakai
  program lain di VPS ditolak otomatis, jadi tidak ada yang bentrok.
- Tombol salin `ip:port:user:pass` atau URL `socks5://user:pass@ip:port`,
  dan export semua proxy aktif dalam satu daftar.
- Status VPS yang diperbarui tiap 3 detik: CPU, RAM, swap, disk, jaringan,
  uptime, load, status service 3proxy, dan jumlah koneksi aktif per proxy.
- Login dikunci 10 menit setelah 5 kali salah password.

**Daftar isi**

1. [Sebelum mulai](#1-sebelum-mulai)
2. [Cara A: Instalasi tanpa Docker](#2-cara-a-instalasi-tanpa-docker)
3. [Cara B: Instalasi dengan Docker](#3-cara-b-instalasi-dengan-docker)
4. [Memakai panel](#4-memakai-panel)
5. [Update](#5-update)
6. [Mengubah port atau password](#6-mengubah-port-atau-password)
7. [Perintah sehari-hari](#7-perintah-sehari-hari)
8. [Mengatasi masalah](#8-mengatasi-masalah)
9. [Keamanan dan HTTPS](#9-keamanan-dan-https)
10. [Cara kerja dan semua pengaturan](#10-cara-kerja-dan-semua-pengaturan)
11. [Menghapus panel](#11-menghapus-panel)

---

## 1. Sebelum mulai

**Yang dibutuhkan**

- VPS dengan **Ubuntu 22.04 atau 24.04** (Debian 12 juga bisa), RAM minimal
  512 MB, dan IP publik.
- Akses root atau user dengan `sudo`.
- Program SSH di komputer kamu (Terminal di Mac/Linux, PowerShell atau
  PuTTY di Windows).

**Pilih salah satu cara instalasi** (jangan dua-duanya di VPS yang sama):

| | Cara A: tanpa Docker | Cara B: dengan Docker |
| --- | --- | --- |
| 3proxy | paket apt resmi 3proxy.org | di-build dari repo ini, di dalam container |
| Panel | service systemd | container yang sama dengan 3proxy |
| Port proxy | minimal 1024 | semua port boleh |
| Cocok untuk | VPS khusus proxy, paling ringan | VPS yang sudah memakai Docker, atau yang ingin semuanya terpisah dari sistem |

**Tentukan port yang akan dipakai.** Installer akan menanyakan dua hal ini:

- **Port panel**, alamat untuk membuka panel di browser. Default `8088`.
- **Rentang port proxy**, port yang boleh dipakai proxy. Default
  `20000-29999`, artinya proxy bisa memakai port 20000 sampai 29999.

Kalau di VPS sudah ada program lain (web server, panel lain, database), pilih
port yang tidak dipakai program itu. Untuk melihat port yang sudah dipakai:

```bash
sudo ss -ltnp
```

Angka setelah titik dua di kolom *Local Address* adalah port-nya, misalnya
`0.0.0.0:80` berarti port 80 dipakai. Tidak perlu khawatir salah: installer
dan panel menolak port yang sudah dipakai program lain.

---

## 2. Cara A: Instalasi tanpa Docker

### Langkah 1: Masuk ke VPS

Dari komputer kamu (ganti `IP-VPS` dengan IP VPS kamu):

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

Installer memasang 3proxy dari repository resmi 3proxy.org, Go (untuk
mem-build panel) dan panelnya, lalu menanyakan pengaturan. Tekan **Enter**
untuk memakai nilai di dalam kurung siku:

```
Buat password login panel (kosongkan agar dibuat acak):
Ulangi password:
Port panel [8088]:
Rentang port proxy [20000-29999]:
Firewall ufw aktif. Buka port 8088 dan 20000-29999 di ufw? (y/n) [y]:
```

- Password tidak terlihat saat diketik, itu normal. Kalau dikosongkan,
  password acak dibuat dan ditampilkan di akhir.
- Kalau port yang kamu pilih sudah dipakai, installer memberi tahu program
  apa yang memakainya dan meminta port lain.
- Pertanyaan firewall hanya muncul kalau `ufw` aktif. Jawab `y` supaya port
  panel dan port proxy langsung dibuka.

Tunggu sampai muncul:

```
==> Selesai! Buka panel di: http://IP-VPS:8088
```

### Langkah 4: Buka port di firewall provider

Kalau provider VPS punya firewall sendiri (*Security Group* di AWS,
*Firewall* di DigitalOcean/Vultr/Hetzner, dsb.), buka port panel dan
rentang port proxy (TCP) di panel provider. Tanpa firewall provider, lewati
langkah ini.

### Langkah 5: Login dan buat proxy

Lanjut ke [Memakai panel](#4-memakai-panel).

Kartu **Service 3proxy** bisa menunjukkan *Mati* sebelum proxy pertama
dibuat, karena config bawaan paket belum lengkap. Itu normal: begitu proxy
pertama ditambahkan, panel menulis config baru dan menyalakan 3proxy.

---

## 3. Cara B: Instalasi dengan Docker

### Langkah 1: Masuk ke VPS

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

### Langkah 3: Jalankan installer mode Docker

```bash
cd ~/3proxy && sudo bash panel/install.sh --docker
```

Installer memasang Docker kalau belum ada, menanyakan pengaturan yang sama
seperti [Cara A](#langkah-3-jalankan-installer) (password, port panel,
rentang port proxy, firewall), menyimpannya di `~/3proxy/panel/.env`, lalu
mem-build dan menjalankan container. Build pertama butuh beberapa menit.

Tunggu sampai muncul:

```
==> Selesai! Buka panel di: http://IP-VPS:8088
```

### Langkah 4: Buka port di firewall provider

Sama seperti [Cara A langkah 4](#langkah-4-buka-port-di-firewall-provider).

### Langkah 5: Login dan buat proxy

Lanjut ke [Memakai panel](#4-memakai-panel).

### Alternatif: tanpa installer

Kalau ingin menjalankan Docker sendiri:

```bash
curl -fsSL https://get.docker.com | sudo sh
```

```bash
cd ~/3proxy/panel && cp .env.example .env && nano .env
```

Isi `PANEL_PASSWORD`, ubah `PANEL_PORT` dan `PANEL_PROXY_PORTS` kalau perlu,
simpan (Ctrl+O, Enter, Ctrl+X), lalu:

```bash
sudo docker compose up -d --build
```

Buka port di firewall sendiri, misalnya dengan ufw:

```bash
sudo ufw allow 8088/tcp && sudo ufw allow 20000:29999/tcp
```

**Catatan Docker:** container memakai jaringan VPS langsung
(`network_mode: host`), jadi port proxy yang dibuat dari panel langsung bisa
dipakai tanpa mengubah `docker-compose.yml`. Karena itu cara ini hanya untuk
VPS Linux, bukan Docker Desktop di Mac/Windows.

---

## 4. Memakai panel

1. Buka `http://IP-VPS:8088` (atau port yang kamu pilih) dan masukkan
   password.
2. Bagian atas menampilkan status VPS: CPU, RAM, disk, jaringan, uptime dan
   status service 3proxy.
3. Di bagian **Tambah proxy** isi:
   - **Tipe**: HTTP(S), SOCKS5, atau HTTP + SOCKS5 (keduanya di satu port).
   - **Port**: sudah terisi port kosong berikutnya di rentang port proxy.
   - **Username** dan **Password**: klik **Acak** untuk membuat otomatis.
   - **IP keluar** (opsional): untuk VPS dengan beberapa IP, pilih IP yang
     dipakai proxy ke internet. Kosongkan untuk otomatis.
   - **Catatan** (opsional), misalnya nama pemakai proxy.
4. Klik **Tambah**. Proxy langsung aktif.
5. Di tabel proxy: **Salin** menyalin `ip:port:user:pass`, **URL** menyalin
   `http://user:pass@ip:port`, **Matikan** menonaktifkan sementara, kolom
   **Koneksi** menunjukkan jumlah koneksi aktif. Tombol **Export** di atas
   tabel menampilkan semua proxy aktif sekaligus.

**Tes proxy** dari komputer kamu (ganti user, pass, IP dan port):

```bash
curl -x http://user:pass@IP-VPS:20000 https://api.ipify.org
```

```bash
curl -x socks5h://user:pass@IP-VPS:20001 https://api.ipify.org
```

Kalau yang muncul adalah IP VPS kamu, proxy sudah jalan.

---

## 5. Update

Data proxy, password dan pengaturan port tetap aman saat update.

**Cara A** (update panel):

```bash
cd ~/3proxy && git pull && sudo bash panel/install.sh
```

**Cara A** (update 3proxy dari apt):

```bash
sudo apt update && sudo apt-get install --only-upgrade -o Dpkg::Options::=--force-confold 3proxy
```

Opsi `--force-confold` menjaga config yang dibuat panel. Kalau tanpa opsi itu
muncul pertanyaan tentang `/usr/local/3proxy/conf/3proxy.cfg`, jawab **N**.
Kalau config terlanjur tertimpa, buka panel dan klik **Terapkan ulang**.

**Cara B** (update panel dan 3proxy sekaligus):

```bash
cd ~/3proxy && git pull && sudo bash panel/install.sh --docker
```

---

## 6. Mengubah port atau password

Semua pengaturan ada di satu file:

| | Cara A | Cara B |
| --- | --- | --- |
| File pengaturan | `/etc/3proxy-panel/panel.env` | `~/3proxy/panel/.env` |
| Edit | `sudo nano /etc/3proxy-panel/panel.env` | `sudo nano ~/3proxy/panel/.env` |
| Terapkan | `sudo systemctl restart 3proxy-panel` | `cd ~/3proxy/panel && sudo docker compose up -d` |

Baris yang bisa diubah:

```
PANEL_PASSWORD='passwordbaru'
PANEL_PORT=9090
PANEL_PROXY_PORTS=30000-30999
```

- Password tidak boleh berisi spasi, tanda kutip atau backslash.
- Di Cara A, rentang port proxy minimal 1024.
- Kalau baris `PANEL_PORT` atau `PANEL_PROXY_PORTS` belum ada (instalasi
  versi lama), tambahkan saja.

Setelah mengganti port, buka port baru di firewall dan tutup yang lama,
contoh untuk ufw:

```bash
sudo ufw allow 9090/tcp && sudo ufw delete allow 8088/tcp
```

```bash
sudo ufw allow 30000:30999/tcp && sudo ufw delete allow 20000:29999/tcp
```

Proxy lama yang port-nya di luar rentang baru tetap jalan, tapi panel tidak
mau menyimpan perubahan apa pun sampai proxy itu diedit ke port di dalam
rentang atau dihapus. Pesan errornya menyebut port mana yang bermasalah.

---

## 7. Perintah sehari-hari

**Cara A**

| Keperluan | Perintah |
| --- | --- |
| Cek status panel | `sudo systemctl status 3proxy-panel` |
| Cek status 3proxy | `sudo systemctl status 3proxy` |
| Lihat log panel | `sudo journalctl -u 3proxy-panel -f` |
| Restart panel | `sudo systemctl restart 3proxy-panel` |
| Restart 3proxy | `sudo systemctl restart 3proxy` |
| Lihat config 3proxy | `sudo cat /usr/local/3proxy/conf/3proxy.cfg` |

**Cara B** (jalankan dari folder `~/3proxy/panel`)

| Keperluan | Perintah |
| --- | --- |
| Cek status | `sudo docker compose ps` |
| Lihat log | `sudo docker compose logs -f` |
| Restart | `sudo docker compose restart` |
| Stop | `sudo docker compose down` |
| Start lagi | `sudo docker compose up -d` |
| Lihat config 3proxy | `sudo cat data/3proxy.cfg` |

---

## 8. Mengatasi masalah

**Panel tidak bisa dibuka di browser**

1. Cek panel jalan: `sudo systemctl status 3proxy-panel` (Cara A) atau
   `sudo docker compose ps` (Cara B).
2. Cek firewall: `sudo ufw status` harus menampilkan port panel dengan
   `ALLOW`, dan firewall provider juga harus membukanya.
3. Coba dari VPS sendiri: `curl -I http://127.0.0.1:8088/login`. Kalau ini
   berhasil tapi dari browser tidak, masalahnya di firewall.

**Log berisi `port 8088 is already used by another program`**

Port panel sudah dipakai program lain. Ganti `PANEL_PORT` (lihat
[Mengubah port](#6-mengubah-port-atau-password)) lalu terapkan.

**Panel menolak port: `sudah dipakai program lain di VPS`**

Pilih port lain di rentang port proxy. Untuk melihat program yang memakai
port itu (ganti 20000):

```bash
sudo ss -ltnp | grep :20000
```

**Proxy tidak bisa dipakai**

1. Pastikan proxy berstatus *aktif* di panel dan kartu **Service 3proxy**
   menunjukkan *Berjalan*.
2. Tes dari VPS sendiri (ganti user, pass, port). Kalau ini berhasil tapi
   dari komputer kamu tidak, buka rentang port proxy di firewall.

```bash
curl -x http://user:pass@127.0.0.1:20000 https://api.ipify.org
```

3. Kalau 3proxy *Mati*, lihat lognya: `sudo journalctl -u 3proxy -n 50`
   (Cara A) atau `sudo docker compose logs` (Cara B), lalu klik **Terapkan
   ulang** di panel.

**Lupa password panel**

```bash
sudo grep PANEL_PASSWORD /etc/3proxy-panel/panel.env
```

```bash
sudo grep PANEL_PASSWORD ~/3proxy/panel/.env
```

Perintah pertama untuk Cara A, kedua untuk Cara B.

**`Terlalu banyak percobaan gagal`**

Tunggu 10 menit, atau restart panel supaya kuncinya hilang.

---

## 9. Keamanan dan HTTPS

Panel berjalan dengan HTTP biasa, jadi password dikirim tanpa enkripsi. Pilih
salah satu cara berikut kalau ingin lebih aman.

**Cara 1: lewat SSH tunnel (tanpa domain).** Tambahkan baris ini di file
pengaturan (lihat [tabel di bagian 6](#6-mengubah-port-atau-password)) lalu
terapkan:

```
PANEL_LISTEN=127.0.0.1:8088
```

Panel sekarang hanya bisa dibuka dari VPS sendiri. Dari komputer kamu:

```bash
ssh -L 8088:127.0.0.1:8088 root@IP-VPS
```

Selama SSH itu terbuka, panel bisa dibuka di `http://localhost:8088`.

**Cara 2: HTTPS dengan Let's Encrypt** (butuh domain yang mengarah ke VPS,
dan port 80 terbuka).

```bash
sudo apt install -y certbot
```

Cara A:

```bash
sudo certbot certonly --standalone -d panel.domainkamu.com --deploy-hook "systemctl restart 3proxy-panel"
```

Cara B:

```bash
sudo certbot certonly --standalone -d panel.domainkamu.com --deploy-hook "docker restart 3proxy-panel"
```

Tambahkan di file pengaturan:

```
PANEL_TLS_CERT=/etc/letsencrypt/live/panel.domainkamu.com/fullchain.pem
PANEL_TLS_KEY=/etc/letsencrypt/live/panel.domainkamu.com/privkey.pem
```

Untuk Cara B, izinkan juga container membaca sertifikatnya:

```bash
printf 'services:\n  panel:\n    volumes:\n      - /etc/letsencrypt:/etc/letsencrypt:ro\n' > ~/3proxy/panel/docker-compose.override.yml
```

Terapkan pengaturannya. Panel sekarang di
`https://panel.domainkamu.com:8088`.

---

## 10. Cara kerja dan semua pengaturan

- Cara A: daftar proxy disimpan di `/var/lib/3proxy-panel/proxies.json` dan
  config 3proxy di `/usr/local/3proxy/conf/3proxy.cfg`. Paket apt
  menjalankan 3proxy di dalam chroot sebagai user `proxy`, karena itu port
  proxy minimal 1024.
- Cara B: data dan config ada di `~/3proxy/panel/data/` (`proxies.json` dan
  `3proxy.cfg`). 3proxy jalan sebagai root di dalam container dan dinyalakan
  ulang otomatis kalau mati.
- Setiap ada perubahan, panel menulis ulang config 3proxy lalu me-reload
  3proxy. Kalau reload gagal, 3proxy tetap memuat config baru sendiri dalam
  1 menit (`monitor`).
- Config 3proxy yang bukan buatan panel disalin dulu ke
  `3proxy.cfg.bak-<tanggal>` sebelum ditimpa pertama kali.
- Sebelum menyimpan, panel memeriksa bahwa port proxy ada di rentang port
  proxy, belum dipakai proxy lain, bukan port panel, dan tidak sedang dipakai
  program lain di VPS.
- Pengguna proxy tidak bisa mengakses alamat `127.0.0.0/8` di VPS (termasuk
  panel ini).
- 3proxy memakai satu daftar user untuk semua port, jadi username yang sama
  boleh dipakai di beberapa proxy hanya jika password-nya sama.

**Semua pengaturan**

| Variabel | Default | Keterangan |
| --- | --- | --- |
| `PANEL_PASSWORD` | (wajib) | Password login panel |
| `PANEL_PORT` | `8088` | Port panel |
| `PANEL_PROXY_PORTS` | `20000-29999` dari installer; kalau baris ini tidak ada: semua port (Cara A mulai 1024) | Rentang port proxy |
| `PANEL_LISTEN` | (kosong) | Alamat dan port panel, contoh `127.0.0.1:8088`; menggantikan `PANEL_PORT` |
| `PANEL_HOST` | alamat yang dipakai membuka panel | IP/domain yang ditampilkan di daftar proxy |
| `PANEL_HEADER` | (kosong) | File berisi pengaturan global 3proxy pengganti bawaan panel |
| `PANEL_TLS_CERT`, `PANEL_TLS_KEY` | (kosong) | Sertifikat dan key untuk HTTPS |
| `PANEL_DATA` | `/var/lib/3proxy-panel/proxies.json` | File data proxy (sudah diatur, tidak perlu diubah) |
| `PANEL_CONFIG` | `/usr/local/3proxy/conf/3proxy.cfg` | Config 3proxy yang ditulis panel (sudah diatur) |
| `PANEL_CHROOT` | `/usr/local/3proxy` | Folder chroot 3proxy, kosong kalau tidak memakai chroot (sudah diatur) |
| `PANEL_RELOAD` | `systemctl reload-or-restart 3proxy` | Perintah reload 3proxy (sudah diatur) |

**Pengaturan global 3proxy.** Bawaan panel:

```
nserver 1.1.1.1
nserver 8.8.8.8
nscache 65536
timeouts 1 5 30 60 180 1800 15 60
```

Untuk mengubahnya, misalnya menambah log, buat file berisi baris di atas
ditambah baris `log`, isi `PANEL_HEADER` dengan path file itu, terapkan, lalu
klik **Terapkan ulang** di panel.

- Cara A: file `/etc/3proxy-panel/header.cfg`, baris
  `log /logs/3proxy-%y%m%d.log D` (log tersimpan di `/var/log/3proxy/`),
  `PANEL_HEADER='/etc/3proxy-panel/header.cfg'`.
- Cara B: file `~/3proxy/panel/data/header.cfg`, baris `log` saja (log
  muncul di `sudo docker compose logs`), `PANEL_HEADER=/data/header.cfg`.

---

## 11. Menghapus panel

**Cara A**

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
`/usr/local/3proxy/conf/`. Untuk menghapus 3proxy juga:
`sudo apt remove 3proxy`.

**Cara B**

```bash
cd ~/3proxy/panel && sudo docker compose down --rmi all
```

```bash
sudo rm -rf ~/3proxy/panel/data ~/3proxy/panel/.env
```

Jangan lupa menutup port yang tadi dibuka di firewall.

---

## Development

```bash
cd panel && go test ./... && go build -o 3proxy-panel .
```

```bash
PANEL_PASSWORD=rahasia PANEL_CHROOT= PANEL_CONFIG=/tmp/3proxy.cfg PANEL_DATA=/tmp/proxies.json PANEL_RELOAD= ./3proxy-panel
```
