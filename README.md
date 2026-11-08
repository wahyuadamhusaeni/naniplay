# NaniPlay — *Nani?! Play.*

**Satu aplikasi untuk menonton anime, film, dan series** — self-hosted, satu binary Go.
Tanpa akun, tanpa iklan, tanpa runtime tambahan. Berjalan sebagai server web di
Linux/VPS atau sebagai aplikasi portable di Windows.

[![Rilis](https://img.shields.io/github/v/release/wahyuadamhusaeni/naniplay?style=flat-square&color=e50914&label=Rilis)](https://github.com/wahyuadamhusaeni/naniplay/releases/latest)
[![Bintang](https://img.shields.io/github/stars/wahyuadamhusaeni/naniplay?style=flat-square&logo=github&color=e3b341&label=Stars)](https://github.com/wahyuadamhusaeni/naniplay/stargazers)
[![Lisensi](https://img.shields.io/badge/Lisensi-MIT-blue.svg?style=flat-square)](LICENSE)
[![CI](https://img.shields.io/github/actions/workflow/status/wahyuadamhusaeni/naniplay/ci.yml?style=flat-square&label=CI)](https://github.com/wahyuadamhusaeni/naniplay/actions/workflows/ci.yml)

Antarmuka bergaya layanan streaming modern — tema gelap, aksen merah, hero
berputar, dan rail-rail poster — dengan katalog anime dari AniList dan katalog
film/series dari IDLIX, semuanya menyatu dalam satu beranda, satu pencarian,
dan satu pemutar.

## Fitur

### Menonton

- **Beranda campuran** — hero bergilir (anime trending, film & series
  terpopuler), rail *Lanjutkan Menonton*, *Episode Terbaru*, *Sedang Trending*,
  *Pilihan Untukmu* (dipersonalisasi dari genre tontonanmu), film & series
  populer, dan *Rating Tertinggi*
- **Anime dari 3 sumber dengan fallback otomatis** — Otakudesu (subtitle
  Indonesia) → HiAnime → AniChan; sumber bisa dipilih manual per judul, dan
  sumber utama bisa diatur di Pengaturan
- **Film & Series (IDLIX)** — ribuan judul dengan detail lengkap (sinopsis,
  pemeran, sutradara, kualitas), pemilih season/episode, dan status video
- **Film Indo (KitaNonton)** — film & series Indonesia dari arsip negara
  Indonesia KitaNonton: katalog, detail, dan putar langsung dari CDN
  penyedia ke perangkat penonton; domain sumber bisa diganti dari
  Pengaturan karena situsnya sering berpindah
- **Pemutar bergaya Netflix** — kontrol kustom penuh ikon SVG: seek bar
  dengan scrub & tooltip waktu, mundur/maju 10 detik, menu kualitas HLS,
  kecepatan putar 0,5×–2×, volume, PiP, layar penuh, tombol *Episode
  Berikutnya*, dan **putar otomatis episode berikutnya** dengan hitung mundur
- **Lanjutkan Menonton** — posisi putar tersimpan otomatis per episode di
  SQLite; karena self-hosted, semua perangkatmu berbagi riwayat yang sama
- **Lewati Opening/Ending/Rekap** — tombol skip dari data komunitas AniSkip
- **Jadwal Rilis** — jadwal tayang anime 7 hari ke depan (WIB), plus label
  *Episode Mendatang* di Daftar Saya & Riwayat
- **Pemilih episode pintar** — pencarian nomor, rentang per 100 episode,
  dan penanda progres per episode

### Subtitle

- **Panel subtitle modern** — trek tampil sebagai daftar baris (bahasa,
  sumber, penanda aktif); bahasa pilihanmu **dipelajari** dan otomatis
  diterapkan di episode berikutnya
- **Cari subtitle online** — anime: OpenSubtitles tanpa API key (validasi
  judul/season/episode + penilaian rilis); film & series: SubDL per ID IMDb
  (kunci API milikmu, gratis dari subdl.com)
- **Terjemahan mesin (Google GTX)** — subtitle Inggris apa pun menjadi
  Indonesia sekali klik, tanpa API key
- **AI Fansub (BYOK)** — terjemahan oleh model AI lewat endpoint
  OpenAI-compatible milikmu (OpenAI/Gemini/Groq/Ollama/9router, dll);
  kunci terdeteksi otomatis dari formatnya, prompt fansub bisa disunting,
  baris yang gagal diisi oleh GTX
- **Alat trek lengkap** — unggah file (.srt/.vtt/.ass), ambil dari URL,
  geser timing ±0,5 dtk, **gabung dua trek** menjadi dwibahasa, dan
  **sinkronisasi otomatis** (desync) mengikuti trek referensi

### Lainnya

- **Pencarian gabungan** — anime, film, dan series dalam satu halaman hasil
- **Jelajah genre** — genre anime (AniList) dan genre sinema (IDLIX)
- **Daftar Saya & Riwayat** — favorit dan riwayat campuran anime/film/series;
  riwayat bisa dihapus per judul atau seluruhnya
- **Pengaturan lengkap dari UI** — domain semua sumber (bisa diganti bila
  sumber pindah domain) lengkap dengan tombol *Tes Koneksi*, preferensi
  sumber/kualitas/bahasa subtitle, kunci AI & SubDL (tidak pernah ditampilkan
  kembali), dan cek versi/rilis baru
- **Satu binary ramping** — RAM belasan MB saat jalan; data lokal dalam satu
  berkas SQLite

## Menjalankan

### Dari rilis (Windows)

Unduh `naniplay-windows.zip` dari halaman Releases, ekstrak, jalankan
`naniplay.exe`. Browser terbuka di `http://127.0.0.1:8080`. Data (riwayat,
favorit, subtitle, pengaturan) tersimpan di folder `data/` di sebelah exe.

### Self-hosted (Linux/VPS)

```bash
./naniplay                    # dengarkan di 127.0.0.1:8080
APP_HOST=0.0.0.0 ./naniplay   # bisa diakses dari jaringan/internet
```

### Dari sumber

Butuh Go 1.27+.

```bash
git clone https://github.com/wahyuadamhusaeni/naniplay.git
cd naniplay
make run        # atau: go run ./cmd/naniplay
```

### Konfigurasi (variabel lingkungan)

| Variabel | Bawaan | Keterangan |
|---|---|---|
| `APP_HOST` | `127.0.0.1` | Alamat dengar server |
| `APP_PORT` | `8080` | Port server |
| `NANIPLAY_DATA` | `./data` | Folder data (SQLite) |
| `NANIPLAY_NO_OPEN` | — | Isi `1` agar browser tidak dibuka otomatis (mode server) |
| `OTAKU_BASE` | `https://ww2.otakudesu.biz` | Domain sumber Otakudesu |
| `HIANIME_BASE` | `https://hianime.at` | Domain sumber HiAnime |
| `IDLIX_BASE` | `https://z2.idlixku.com` | Domain sumber IDLIX |
| `KITANONTON_BASE` | `https://kitanonton2.garden` | Domain sumber KitaNonton (Film Indo) |

Sebagian besar pengaturan juga bisa diubah dari halaman **Pengaturan** di UI
dan tersimpan di SQLite (mengalahkan nilai bawaan lingkungan).

## Teknologi

Go (satu binary, tanpa CGO) · SQLite (modernc.org/sqlite) · hls.js ·
API publik AniList · klien TLS untuk sumber yang dilindungi Cloudflare ·
CI GitHub Actions (gofmt, vet, test, build Linux & Windows di setiap push;
rilis otomatis per tag semver).

## Kredit & Inspirasi

NaniPlay dikembangkan dari nol (clean-room) di Go. Desain awalnya terinspirasi
[Tatap](https://github.com/asfaraguest/tatap) oleh asfaraguest — terima kasih
atas referensi ekosistemnya. Fitur film/series terinspirasi alur aplikasi
[idlix-androidtv](https://github.com/samdgea/idlix-androidtv) oleh samdgea.
Metadata oleh [AniList](https://anilist.co); sumber stream adalah situs
publik masing-masing — NaniPlay tidak menyimpan berkas video apa pun.

## Lisensi

MIT — lihat [LICENSE](LICENSE).
