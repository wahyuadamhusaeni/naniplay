# Changelog

Semua perubahan penting NaniPlay dicatat di sini.
Format mengikuti [Keep a Changelog](https://keepachangelog.com/id/1.1.0/),
versi mengikuti [Semantic Versioning](https://semver.org/lang/id/).

## [0.6.0] - 2026-11-08

### Ditambahkan
- **Sumber ketiga: AniChan** — melengkapi rantai fallback menjadi
  Otakudesu → HiAnime → AniChan, persis urutan ekosistem
  Tatap/Shisen. AniChan berkunci ID AniList (tanpa menebak judul):
  sesi + triple kunci yang di-scrape otomatis dari bundle JS situs
  (kuncinya berotasi; nilai statis bawaan aplikasi hanya cadangan),
  payload daftar server terdekripsi AES-256-GCM (kunci diturunkan
  via HMAC-SHA256 dari token sesi), hanya server HLS langsung yang
  dipakai, dan subtitle bawaannya terimpor seperti HiAnime.
- **Server lama ZokoAnime di HiAnime** — embed lama yang membawa
  konfigurasi player dalam blob base64+XOR kini bisa di-resolve,
  jadi judul yang hanya punya server itu tetap bisa diputar.

### Diubah
- Domain HiAnime kini bisa diatur lewat env `HIANIME_BASE` atau
  pengaturan `hianime_base`, sama seperti Otakudesu.

## [0.5.0] - 2026-11-08

### Ditambahkan
- **Sumber kedua: HiAnime** — katalog raksasa hianime.at dengan softsub
  multi-bahasa. Pipeline: pencarian (nama Inggris + romaji), daftar
  episode, daftar server, lalu dekripsi AES-256-CBC payload Megaplay
  untuk master HLS-nya.
- **Fallback otomatis antar sumber** — Otakudesu tetap utama (Sub Indo
  hardsub); bila sebuah judul tidak ditemukan di sana, HiAnime
  mengambil alih tanpa terasa. Halaman detail kini menampilkan sumber
  yang melayani judul tersebut.
- **Subtitle bawaan HiAnime terimpor otomatis** — track softsub dari
  stream (Inggris diprioritaskan, maks 4) tersimpan sebagai track
  lokal: bisa dipilih di pemutar dan langsung diterjemahkan ke
  Indonesia dengan GTX.
- Proxy HLS kini mengenali segmen TS yang disamarkan dengan awalan
  sampah 252 byte (pola beberapa CDN sumber) dan membuangnya.

### Catatan
- Server lama ZokoAnime (satu-satunya pilihan untuk sebagian kecil
  judul) belum didukung; judul seperti itu menampilkan pesan sumber
  belum tersedia.

## [0.4.0] - 2026-11-08

### Ditambahkan
- **Terjemahan subtitle otomatis ke Indonesia (Google GTX)** — tombol
  "Terjemahkan ke Indonesia" pada track subtitle aktif menerjemahkan
  seluruh episode lewat endpoint publik Google Translate, tanpa API
  key dan tanpa akun. Hasilnya disimpan sebagai track baru
  ("… — Indonesia (GTX)") sehingga tetap tersedia secara luring;
  track asli tidak diubah. Kontraksi bahasa Inggris lisan diperluas
  dulu agar hasil mesin tidak kaku, dan timing tidak berubah sama
  sekali. Terverifikasi: 304 cue Naruto episode 1 selesai dalam
  ±11 detik.
  - Ini penyelamat untuk anime lama/ langka yang subtitle
    Indonesianya tidak pernah diterbitkan: ambil subtitle Inggrisnya,
    terjemahkan, tonton.

## [0.3.0] - 2026-11-08

### Ditambahkan
- **Mesin subtitle (softsub)** di pemutar:
  - **Cari Subtitle Online** — OpenSubtitles tanpa API key (API publik +
    resolusi IMDb otomatis). Kandidat divalidasi ketat terhadap judul,
    musim, dan episode, diberi skor (bahasa, grup rilis tepercaya, jumlah
    unduhan), dan dibersihkan dari iklan/watermark OpenSubtitles.
    Bahasa Indonesia diprioritaskan; Melayu tidak pernah tertukar sebagai
    Indonesia.
  - **Unggah file** subtitle sendiri (.srt/.vtt/.ass, maks 2 MiB).
  - **Ambil dari URL** — tempel tautan file subtitle dari mana pun.
  - Semua format dinormalisasi ke WebVTT; track tersimpan per episode di
    SQLite dan dapat dihapus dari pemutar.
  - **Koreksi desync manual** — geser timing subtitle ±0,5 detik langsung
    dari pemutar bila subtitle tidak sinkron dengan video.
- Unit test untuk parser SRT/VTT/ASS, pembersih iklan, pencocokan rilis
  subtitle, dan penyimpanan track.

### Catatan
- Sumber AnimeTosho ditunda: lampiran subtitle di sisi upstream sedang
  tidak dapat diunduh (tautan unduh resmi mereka sendiri membalas 404
  pada 2026-11-08). Akan ditinjau ulang secara berkala; arsitektur
  sumber subtitle memang dirancang agar sumber baru bisa ditambahkan
  tanpa mengubah pemutar.

## [0.2.0] - 2026-11-08

### Ditambahkan
- **AniSkip** — tombol "Lewati Opening / Lewati Ending / Lewati Rekap" di pemutar,
  memakai data komunitas AniSkip (disesuaikan otomatis dengan durasi berkas).
- **Jadwal Rilis** — halaman jadwal tayang 7 hari ke depan (WIB) dari AniList,
  terhubung ke halaman detail setiap anime.
- **Anime Terkait** — rail sekuel/prekuel/cerita sampingan di halaman detail.
- **Pengaturan** — domain sumber Otakudesu dapat diganti dari UI (tersimpan di
  database) karena sumber kerap berpindah domain; halaman ini juga menampilkan
  versi berjalan dan memberi tahu bila ada rilis baru.
- Endpoint `GET /api/health` dan `GET /api/version`.
- CI GitHub Actions: `gofmt`, `go vet`, `go test`, dan build Linux + Windows
  pada setiap push dan pull request.
- Unit test untuk pencocokan judul, penulisan ulang playlist HLS, dan store.

### Diubah
- Pencocokan judul AniList → Otakudesu sekarang berbasis skor (tumpang-tindih
  token + kesepakatan nomor musim), dengan jumlah episode sebagai penentu saat
  dua kandidat bersaing ketat. Tidak ada lagi tebakan buta ke hasil pertama.

## [0.1.0] - 2026-11-08

### Ditambahkan
- Rilis pertama: beranda bergaya layanan streaming (hero + rail) dari AniList
  (trending, musim berjalan, rating tertinggi), pencarian, dan detail anime.
- Streaming Otakudesu (Sub Indo): pencarian sumber otomatis, daftar episode,
  resolve stream melalui dekripsi konfigurasi pemutar, dan proxy HLS bawaan.
- Pemutar hls.js dengan resume otomatis dan penyimpanan progres per episode.
- Lanjutkan Menonton, Daftar Saya (favorit), dan Riwayat — tersimpan lokal di SQLite.
- Satu binary Go untuk Web (self-hosted) dan Windows (portable, membuka browser).
- Workflow rilis GitHub Actions: zip Windows + tarball Linux per tag versi.

[0.6.0]: https://github.com/wahyuadamhusaeni/naniplay/releases/tag/v0.6.0
[0.5.0]: https://github.com/wahyuadamhusaeni/naniplay/releases/tag/v0.5.0
[0.4.0]: https://github.com/wahyuadamhusaeni/naniplay/releases/tag/v0.4.0
[0.3.0]: https://github.com/wahyuadamhusaeni/naniplay/releases/tag/v0.3.0
[0.2.0]: https://github.com/wahyuadamhusaeni/naniplay/releases/tag/v0.2.0
[0.1.0]: https://github.com/wahyuadamhusaeni/naniplay/releases/tag/v0.1.0
