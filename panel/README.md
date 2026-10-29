# 3proxy Panel: dokumentasi lanjutan

Instalasi, update dan pemakaian sehari-hari ada di
[README utama](../README.md). Halaman ini berisi HTTPS, semua pengaturan,
cara kerja panel, dan development.

## Keamanan dan HTTPS

Panel berjalan dengan HTTP biasa, jadi password dikirim tanpa enkripsi.
Pilih salah satu cara berikut kalau ingin lebih aman. Pengaturannya ada di
file yang sama dengan port dan password: `/etc/3proxy-panel/panel.env` (tanpa
Docker) atau `~/3proxy/panel/.env` (dengan Docker).

### Lewat SSH tunnel (tanpa domain)

Tambahkan di file pengaturan, lalu terapkan:

```
PANEL_LISTEN=127.0.0.1:8088
```

Panel sekarang hanya bisa dibuka dari VPS sendiri. Dari komputer kamu:

```bash
ssh -L 8088:127.0.0.1:8088 root@IP-VPS
```

Selama SSH itu terbuka, panel bisa dibuka di `http://localhost:8088`.

### HTTPS dengan Let's Encrypt

Butuh domain yang mengarah ke VPS, dan port 80 terbuka.

```bash
sudo apt install -y certbot
```

Tanpa Docker:

```bash
sudo certbot certonly --standalone -d panel.domainkamu.com --deploy-hook "systemctl restart 3proxy-panel"
```

Dengan Docker:

```bash
sudo certbot certonly --standalone -d panel.domainkamu.com --deploy-hook "docker restart 3proxy-panel"
```

Tambahkan di file pengaturan:

```
PANEL_TLS_CERT=/etc/letsencrypt/live/panel.domainkamu.com/fullchain.pem
PANEL_TLS_KEY=/etc/letsencrypt/live/panel.domainkamu.com/privkey.pem
```

Dengan Docker, izinkan juga container membaca sertifikatnya:

```bash
printf 'services:\n  panel:\n    volumes:\n      - /etc/letsencrypt:/etc/letsencrypt:ro\n' > ~/3proxy/panel/docker-compose.override.yml
```

Terapkan pengaturannya. Panel sekarang di `https://panel.domainkamu.com:8088`.

## Semua pengaturan

| Variabel | Default | Keterangan |
| --- | --- | --- |
| `PANEL_PASSWORD` | (wajib) | Password login panel |
| `PANEL_PORT` | `8088` | Port panel |
| `PANEL_PROXY_PORTS` | `20000-29999` dari installer; kalau tidak diisi: semua port (tanpa Docker mulai 1024) | Rentang port proxy |
| `PANEL_LISTEN` | (kosong) | Alamat dan port panel, contoh `127.0.0.1:8088`; menggantikan `PANEL_PORT` |
| `PANEL_HOST` | alamat yang dipakai membuka panel | IP/domain yang ditampilkan di daftar proxy |
| `PANEL_HEADER` | (kosong) | File pengaturan global 3proxy, lihat di bawah |
| `PANEL_TLS_CERT`, `PANEL_TLS_KEY` | (kosong) | Sertifikat dan key untuk HTTPS |
| `PANEL_DATA` | `/var/lib/3proxy-panel/proxies.json` | File data proxy |
| `PANEL_CONFIG` | `/usr/local/3proxy/conf/3proxy.cfg` | Config 3proxy yang ditulis panel |
| `PANEL_CHROOT` | `/usr/local/3proxy` | Folder chroot 3proxy, kosong kalau tidak memakai chroot |
| `PANEL_RELOAD` | `systemctl reload-or-restart 3proxy` | Perintah reload 3proxy |

Empat baris terakhir sudah diatur oleh installer dan image Docker, tidak
perlu diubah. Semua pengaturan juga bisa diberikan sebagai flag, lihat
`3proxy-panel -h`.

## Pengaturan global 3proxy

Bawaan panel:

```
nserver 1.1.1.1
nserver 8.8.8.8
nscache 65536
timeouts 1 5 30 60 180 1800 15 60
```

Untuk mengubahnya, misalnya menambah log, buat file berisi baris di atas
ditambah baris `log`, isi `PANEL_HEADER` dengan path file itu, terapkan
pengaturan, lalu klik **Terapkan ulang** di panel.

- Tanpa Docker: file `/etc/3proxy-panel/header.cfg` dengan baris
  `log /logs/3proxy-%y%m%d.log D` (log tersimpan di `/var/log/3proxy/`), dan
  `PANEL_HEADER=/etc/3proxy-panel/header.cfg`.
- Dengan Docker: file `~/3proxy/panel/data/header.cfg` dengan baris `log`
  saja (log muncul di `sudo docker compose logs`), dan
  `PANEL_HEADER=/data/header.cfg`.

## Cara kerja

- Tanpa Docker, proxy disimpan di `/var/lib/3proxy-panel/proxies.json` dan
  config 3proxy di `/usr/local/3proxy/conf/3proxy.cfg`. Paket apt menjalankan
  3proxy di dalam chroot sebagai user `proxy`, karena itu port proxy minimal
  1024.
- Dengan Docker, data dan config ada di `~/3proxy/panel/data/`. 3proxy jalan
  sebagai root di container yang sama dengan panel, dan dinyalakan ulang
  otomatis kalau mati. Container memakai jaringan VPS langsung
  (`network_mode: host`) supaya port proxy baru tidak perlu didaftarkan di
  `docker-compose.yml`.
- Setiap ada perubahan, panel menulis ulang config 3proxy lalu me-reload
  3proxy. Kalau reload gagal, 3proxy memuat config baru sendiri dalam 1 menit
  (`monitor`).
- Config 3proxy yang bukan buatan panel disalin dulu ke
  `3proxy.cfg.bak-<tanggal>` sebelum ditimpa pertama kali.
- Sebelum menyimpan, panel memeriksa bahwa port proxy ada di rentang port
  proxy, belum dipakai proxy lain, bukan port panel, dan tidak sedang dipakai
  program lain di VPS.
- Pengguna proxy tidak bisa mengakses alamat `127.0.0.0/8` di VPS, termasuk
  panel ini.
- 3proxy memakai satu daftar user untuk semua port, jadi username yang sama
  boleh dipakai di beberapa proxy hanya jika password-nya sama.
- Login dikunci 10 menit setelah 5 kali salah password. Sesi login disimpan
  di memori, jadi restart panel berarti login ulang.

## Development

Panel ditulis dengan Go tanpa library tambahan. Tampilannya ada di `web/`.

```bash
cd panel && go test ./... && go build -o 3proxy-panel .
```

Menjalankan panel tanpa 3proxy, untuk mencoba tampilannya:

```bash
PANEL_PASSWORD=rahasia PANEL_CHROOT= PANEL_CONFIG=/tmp/3proxy.cfg PANEL_DATA=/tmp/proxies.json PANEL_RELOAD= ./3proxy-panel
```

Membangun image Docker dari root repo:

```bash
docker build -f panel/Dockerfile -t 3proxy-panel .
```
