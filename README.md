# NaniPlay — *Nani?! Play.*

**Pemutar & agregator anime subtitle Indonesia** untuk Web (self-hosted) dan Windows.
Satu binary Go: ringan, tanpa akun, tanpa iklan, tanpa runtime tambahan.

[![Rilis](https://img.shields.io/github/v/release/wahyuadamhusaeni/naniplay?style=flat-square&color=e50914&label=Rilis)](https://github.com/wahyuadamhusaeni/naniplay/releases/latest)
[![Bintang](https://img.shields.io/github/stars/wahyuadamhusaeni/naniplay?style=flat-square&logo=github&color=e3b341&label=Stars)](https://github.com/wahyuadamhusaeni/naniplay/stargazers)
[![Lisensi](https://img.shields.io/badge/Lisensi-MIT-blue.svg?style=flat-square)](LICENSE)
[![CI](https://img.shields.io/github/actions/workflow/status/wahyuadamhusaeni/naniplay/ci.yml?style=flat-square&label=CI)](https://github.com/wahyuadamhusaeni/naniplay/actions/workflows/ci.yml)

> Terinspirasi dari [Tatap](https://github.com/asfaraguest/tatap), ditulis ulang dari nol dengan Go
> dan antarmuka bergaya layanan streaming modern yang sudah familiar: tema gelap,
> aksen merah, hero banner, dan baris-baris poster.

## Fitur

- **Beranda bergaya streaming** — hero anime trending + rail *Sedang Trending*, *Populer Musim Ini*, *Rating Tertinggi*
- **Lanjutkan Menonton** — posisi putar tersimpan otomatis per episode (SQLite lokal), lanjut di perangkat mana pun
- **Lewati Opening/Ending** — tombol skip OP/ED dari data komunitas AniSkip, menyesuaikan durasi berkas
- **Subtitle lunak (softsub)** — cari subtitle Indonesia/Inggris dari OpenSubtitles tanpa API key, unggah file (.srt/.vtt/.ass), atau ambil dari URL; semua dikonversi ke WebVTT, tersimpan per episode, dengan koreksi desync ±0,5s dari pemutar
- **Jadwal Rilis** — jadwal tayang 7 hari ke depan (WIB)
- **Anime Terkait** — sekuel, prekuel, dan cerita sampingan di setiap halaman detail
- **Daftar Saya** — favorit tersimpan lokal
- **Metadata AniList** — katalog, pencarian, musim, dan detail anime dari API publik AniList
- **Stream Sub Indo (Otakudesu)** — pencarian sumber otomatis berdasarkan judul (pencocokan berbasis skor + jumlah episode), daftar episode, dan pemutar HLS lewat proxy bawaan
- **Pengaturan** — ganti domain sumber dari UI bila sumber berpindah domain; cek versi & rilis baru
- **Satu binary** — jalan di VPS/home server sebagai web, atau sebagai aplikasi portable di Windows (klik, browser terbuka)

## Menjalankan

### Dari rilis (Windows)

Unduh `naniplay-windows.zip` dari halaman Releases, ekstrak, jalankan `naniplay.exe`.
Aplikasi membuka `http://127.0.0.1:8080` di browser. Data (riwayat & favorit) tersimpan di folder `data/` di sebelah exe.

### Self-hosted (Linux/VPS)

```bash
./naniplay            # dengarkan di 127.0.0.1:8080
APP_HOST=0.0.0.0 ./naniplay   # bisa diakses dari LAN/internet
```

### Dari sumber

Butuh Go 1.27+.

```bash
git clone https://github.com/wahyuadamhusaeni/naniplay.git
cd naniplay
make run        # atau: go run ./cmd/naniplay
make windows    # hasil: dist/naniplay.exe
make build      # hasil: dist/naniplay
```

## Konfigurasi (environment)

| Variabel | Default | Keterangan |
|---|---|---|
| `APP_HOST` | `127.0.0.1` | Alamat dengar. `0.0.0.0` untuk akses jaringan |
| `APP_PORT` | `8080` | Port server |
| `NANIPLAY_DATA` | `./data` | Lokasi database SQLite |
| `NANIPLAY_NO_OPEN` | — | `1` = jangan buka browser otomatis |
| `OTAKU_BASE` | `https://ww2.otakudesu.biz` | Domain sumber Otakudesu (mereka kerap pindah domain) |

## Arsitektur

```
cmd/naniplay        entry point (server + buka browser di desktop)
internal/anilist    klien GraphQL AniList (metadata) + cache
internal/source     kontrak Provider sumber streaming
  └ otakudesu       implementasi Otakudesu (scraper + dekripsi config pemutar)
internal/proxy      proxy HLS (header Referer/UA + penulisan ulang playlist)
internal/store      SQLite (riwayat, favorit) — modernc.org/sqlite, tanpa CGO
internal/web        halaman (template tertanam) + API JSON
```

Setiap sumber streaming hidup di belakang interface `source.Provider`
(`Search`, `Episodes`, `Resolve`) supaya kerusakan satu sumber tidak merembet.

## Roadmap

Lihat [PLAN.md](PLAN.md). Ringkasnya: subtitle engine (OpenSubtitles/AnimeTosho + merge + desync),
AniSkip, sumber kedua (HiAnime), AI fansub BYOK (endpoint OpenAI-compatible, termasuk router sendiri), lalu Android.

## Catatan

NaniPlay adalah agregator: berkas video tidak disimpan di server ini dan berasal dari
sumber publik. Metadata oleh AniList. Hormati hak cipta dan ketentuan sumber konten.

## Lisensi

MIT — lihat [LICENSE](LICENSE).
