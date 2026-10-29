# 3proxy Panel

Panel web untuk membuat dan mengelola proxy HTTP/SOCKS5 di VPS, sekaligus
memantau status VPS-nya. Proxy dijalankan oleh
[3proxy](https://github.com/3proxy/3proxy), panelnya dibuka di browser dan
dilindungi satu password.

- Buat, edit, hapus, aktif/nonaktifkan proxy HTTP(S), SOCKS5, atau keduanya
  di satu port.
- Setiap proxy punya port, username dan password sendiri, dan bisa memakai
  IP keluar berbeda di VPS yang punya banyak IP.
- Port panel dan rentang port proxy bisa diatur. Port yang sudah dipakai
  program lain di VPS ditolak otomatis.
- Status VPS langsung di panel: CPU, RAM, disk, jaringan, uptime, status
  3proxy, dan jumlah koneksi per proxy.
- Dipasang dengan satu perintah, langsung di sistem atau dengan Docker.

## Persyaratan

- VPS **Ubuntu 22.04 / 24.04** atau **Debian 12** dengan IP publik.
- Akses root atau user dengan `sudo`.

## Instalasi

Pilih **salah satu** cara, jangan dua-duanya di VPS yang sama:

| | Tanpa Docker | Dengan Docker |
| --- | --- | --- |
| Cocok untuk | VPS khusus proxy, paling ringan | VPS yang sudah memakai Docker |
| 3proxy | paket apt resmi 3proxy.org | di-build dari repo ini, di dalam container |
| Port proxy | 1024 ke atas | semua port |

### 1. Download kode

Masuk ke VPS (ganti `IP-VPS` dengan IP VPS kamu):

```bash
ssh root@IP-VPS
```

```bash
sudo apt update && sudo apt install -y git
```

```bash
git clone https://github.com/kontaknurman/3proxy.git ~/3proxy
```

### 2. Jalankan installer

**Tanpa Docker:**

```bash
cd ~/3proxy && sudo bash panel/install.sh
```

**Dengan Docker** (Docker dipasang otomatis kalau belum ada):

```bash
cd ~/3proxy && sudo bash panel/install.sh --docker
```

Installer menanyakan beberapa hal. Tekan **Enter** untuk memakai nilai di
dalam kurung siku:

```
Buat password login panel (kosongkan agar dibuat acak):
Ulangi password:
Port panel [8088]:
Rentang port proxy [20000-29999]:
Firewall ufw aktif. Buka port 8088 dan 20000-29999 di ufw? (y/n) [y]:
```

- **Password** tidak terlihat saat diketik. Kalau dikosongkan, password acak
  dibuat dan ditampilkan di akhir.
- **Port panel** adalah port untuk membuka panel di browser.
- **Rentang port proxy** adalah port yang boleh dipakai proxy.
- Kalau port yang dipilih sudah dipakai program lain, installer menyebut
  programnya dan meminta port lain.
- Pertanyaan firewall hanya muncul kalau `ufw` aktif.

Selesai kalau muncul:

```
==> Selesai! Buka panel di: http://IP-VPS:8088
```

Build Docker pertama kali butuh beberapa menit.

### 3. Buka port di firewall provider

Kalau provider VPS punya firewall sendiri (*Security Group* di AWS,
*Firewall* di DigitalOcean/Vultr/Hetzner, dsb.), buka port panel dan rentang
port proxy (TCP) di sana. Kalau tidak ada, lewati langkah ini.

### 4. Login

Buka `http://IP-VPS:8088` di browser dan masukkan password.

### Docker tanpa installer

Kalau ingin mengatur Docker sendiri:

```bash
cd ~/3proxy/panel && cp .env.example .env && nano .env
```

Isi `PANEL_PASSWORD`, ubah port kalau perlu, simpan (Ctrl+O, Enter, Ctrl+X),
lalu jalankan:

```bash
sudo docker compose up -d --build
```

Container memakai jaringan VPS langsung, jadi port proxy baru langsung bisa
dipakai. Cara ini hanya untuk VPS Linux, bukan Docker Desktop di Mac/Windows.

## Membuat proxy

1. Di bagian **Tambah proxy**, pilih tipe: HTTP(S), SOCKS5, atau HTTP +
   SOCKS5.
2. Port sudah terisi dengan port kosong berikutnya. Klik **Acak** untuk
   username dan password.
3. **IP keluar** boleh dikosongkan. Isi hanya kalau VPS punya beberapa IP.
4. Klik **Tambah**. Proxy langsung aktif.
5. Klik **Salin** untuk menyalin `ip:port:user:pass`, atau **URL** untuk
   `http://user:pass@ip:port`. **Export** menampilkan semua proxy aktif.

Tes dari komputer kamu (ganti user, pass, IP dan port):

```bash
curl -x http://user:pass@IP-VPS:20000 https://api.ipify.org
```

Kalau yang muncul IP VPS kamu, proxy sudah jalan.

## Update

Data proxy, password dan pengaturan tetap aman.

**Tanpa Docker:**

```bash
cd ~/3proxy && git pull && sudo bash panel/install.sh
```

Untuk meng-update 3proxy-nya juga:

```bash
sudo apt update && sudo apt-get install --only-upgrade -o Dpkg::Options::=--force-confold 3proxy
```

**Dengan Docker** (panel dan 3proxy sekaligus):

```bash
cd ~/3proxy && git pull && sudo bash panel/install.sh --docker
```

## Mengubah password atau port

| | Tanpa Docker | Dengan Docker |
| --- | --- | --- |
| Edit pengaturan | `sudo nano /etc/3proxy-panel/panel.env` | `sudo nano ~/3proxy/panel/.env` |
| Terapkan | `sudo systemctl restart 3proxy-panel` | `cd ~/3proxy/panel && sudo docker compose up -d` |

Baris yang bisa diubah:

```
PANEL_PASSWORD='passwordbaru'
PANEL_PORT=9090
PANEL_PROXY_PORTS=30000-30999
```

Password tidak boleh berisi spasi, tanda kutip atau backslash. Setelah
mengganti port, buka port baru di firewall, misalnya:

```bash
sudo ufw allow 9090/tcp && sudo ufw allow 30000:30999/tcp
```

Kalau ada proxy lama di luar rentang port yang baru, edit atau hapus dulu
proxy itu di panel sebelum membuat perubahan lain.

## Perintah berguna

| Keperluan | Tanpa Docker | Dengan Docker (dari `~/3proxy/panel`) |
| --- | --- | --- |
| Status | `sudo systemctl status 3proxy-panel 3proxy` | `sudo docker compose ps` |
| Log | `sudo journalctl -u 3proxy-panel -f` | `sudo docker compose logs -f` |
| Restart | `sudo systemctl restart 3proxy-panel` | `sudo docker compose restart` |
| Stop | `sudo systemctl stop 3proxy-panel 3proxy` | `sudo docker compose down` |
| Lihat password | `sudo grep PASSWORD /etc/3proxy-panel/panel.env` | `sudo grep PASSWORD .env` |

## Masalah umum

**Panel tidak bisa dibuka.** Cek panel jalan (lihat *Status* di atas), lalu
coba dari VPS sendiri:

```bash
curl -I http://127.0.0.1:8088/login
```

Kalau ini berhasil tapi dari browser tidak, port panel belum dibuka di
firewall (`sudo ufw status`, dan firewall provider).

**Log berisi `port 8088 is already used by another program`.** Port panel
dipakai program lain. Ganti `PANEL_PORT` (lihat
[Mengubah password atau port](#mengubah-password-atau-port)).

**Panel menolak port: `sudah dipakai program lain di VPS`.** Pilih port
lain. Untuk melihat siapa yang memakainya (ganti 20000):

```bash
sudo ss -ltnp | grep :20000
```

**Proxy tidak bisa dipakai.** Pastikan proxy *aktif* dan kartu **Service
3proxy** di panel *Berjalan*. Lalu tes dari VPS sendiri:

```bash
curl -x http://user:pass@127.0.0.1:20000 https://api.ipify.org
```

Kalau dari VPS berhasil tapi dari luar tidak, buka rentang port proxy di
firewall.

**Service 3proxy *Mati* sebelum ada proxy (tanpa Docker).** Normal: 3proxy
dinyalakan begitu proxy pertama dibuat.

**`Terlalu banyak percobaan gagal`.** Tunggu 10 menit atau restart panel.

## Menghapus

**Tanpa Docker:**

```bash
sudo systemctl disable --now 3proxy-panel
```

```bash
sudo rm -rf /usr/local/bin/3proxy-panel /etc/systemd/system/3proxy-panel.service /etc/3proxy-panel /var/lib/3proxy-panel
```

```bash
sudo systemctl daemon-reload
```

3proxy-nya bisa dihapus dengan `sudo apt remove 3proxy`.

**Dengan Docker:**

```bash
cd ~/3proxy/panel && sudo docker compose down --rmi all
```

```bash
sudo rm -rf ~/3proxy/panel/data ~/3proxy/panel/.env
```

## Dokumentasi lain

- [panel/README.md](panel/README.md): HTTPS, semua pengaturan, cara kerja
  panel, dan development.
- [README-3proxy.md](README-3proxy.md): dokumentasi asli 3proxy.

3proxy dibuat oleh Vladimir "3APA3A" Dubrovin, lihat [copying](copying)
untuk lisensinya.
