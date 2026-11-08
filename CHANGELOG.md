# Changelog

Semua perubahan penting NaniPlay dicatat di sini.
Format mengikuti [Keep a Changelog](https://keepachangelog.com/id/1.1.0/),
versi mengikuti [Semantic Versioning](https://semver.org/lang/id/).

## [0.10.0] — 2026-11-08

### Ditambahkan
- **Katalog Film & Series (IDLIX)**: paket `internal/idlix` — klien
  full-Go untuk API situs IDLIX dengan sidik jari TLS/HTTP2 Chrome
  (uTLS) yang lolos penyaringan bot Cloudflare (terverifikasi dari
  server produksi: ±7.700 film & ±5.100 series). Cache respons
  in-memory per endpoint.
- **Halaman Film & Series**: jelajah katalog dengan penomoran halaman,
  cip genre, dan pencarian per seksi; rail "Film Populer" & "Series
  Populer" di beranda (sort popularitas).
- **Detail film**: hero backdrop, meta (rating/kualitas/durasi/
  sutradara), genre, sinopsis, pemeran, dan tombol Daftar Saya.
- **Detail series**: hero, meta, pemeran, pemilih season, dan daftar
  episode lengkap dengan still & sinopsis per episode.
- Daftar Saya & Riwayat kini campuran anime + film + series (tautan
  per jenis media).

### Diperbaiki
- **Domain HiAnime kini bisa diedit di Pengaturan** (sebelumnya hanya
  tersimpan lewat database/env, tanpa kolom UI) — satu formulir
  "Sumber" untuk Otakudesu & HiAnime.

### Catatan
- Tombol putar film/series ditandai "Segera" dengan jujur: pemutarnya
  tiba di v0.11.0. Dari IP yang skor botnya rendah (mis. sebagian
  jaringan perumahan), IDLIX dapat menantang akses; katalog menampilkan
  pesan yang jelas dalam keadaan itu.

## [0.9.0] — 2026-11-08

### Ditambahkan
- **Fondasi penggabungan anime + film + series (Fase 5)**: seluruh data
  pengguna (riwayat, favorit, trek subtitle) kini berkunci *media key*
  umum — `anime:{id}`, `movie:{slug}`, `series:{slug}` — bukan lagi ID
  AniList semata. Database lama dimigrasi otomatis di tempat dalam satu
  transaksi (skema v2, `PRAGMA user_version`); terverifikasi terhadap
  fixture lama dan snapshot database produksi.
- **Pengaturan IDLIX**: aktif/nonaktifkan seksi Film & Series dan alamat
  basisnya (default `https://z2.idlixku.com`, env `IDLIX_BASE`).
- **Seksi Film & Series** di navigasi (tampil bila IDLIX aktif) dengan
  halaman penampung informatif; katalognya terhubung di v0.10.0.

### Diubah
- API riwayat menerima `key` (media key); `anilistId` lama tetap
  diterima. Pemutar mengirim kunci media di setiap penyimpanan progres.

## [0.8.2] — 2026-11-08

### Diperbaiki
- Kunci versi aset diganti menjadi `?v=802`: percobaan deploy 0.8.1
  sempat mengambil unduhan rilis yang rusak dan URL `?v=801` terlanjur
  terisi konten lama di cache edge sebelum deploy berhasil. Tidak ada
  perubahan fungsi dari 0.8.1 selain kunci cache baru ini.

## [0.8.1] — 2026-11-08

### Diperbaiki
- **Tampilan mobile rusak setelah pembaruan 0.8.0**: berkas CSS/JS kini
  diberi versi pada URL-nya (`?v=801`) — sebelumnya peramban/edge dapat
  menyajikan CSS lama dari cache (masa simpan 4 jam) untuk HTML baru,
  sehingga drawer menu tampil sebagai blok kedua di bawah header dan
  control pemain tidak ber-gaya.
- Pengaman tata letak mobile: halaman tidak bisa lagi bergeser
  menyamping (overflow-x dikunci), drawer yang tertutup tidak pernah
  terlukis, dan pita peringatan tidak lagi menyisakan celah setinggi
  header di halaman detail.

## [0.8.0] — 2026-11-08

Rilis pengalaman menonton (dari masukan pengguna) + AI Fansub.

### Ditambahkan
- **AI Fansub (BYOK)**: terjemahkan subtitle dengan model AI lewat endpoint
  OpenAI-compatible apa pun (OpenAI, OpenRouter, Gemini, Groq, Ollama, 9router).
  Diatur di Pengaturan (alamat endpoint, model, kunci API — tersimpan lokal).
  Prompt fansub profesional: bahasa alami, sapaan khas anime dipertahankan.
  Baris yang dijatuhkan model diisi otomatis oleh terjemahan mesin GTX (hibrida —
  tidak pernah gagal total), dan kegagalan endpoint dilaporkan apa adanya.
- **Pemain kustom penuh**: control bar baru (putar/jeda, seek bar dengan indikator
  buffered, volume, waktu), tombol episode sebelum/berikutnya, menu kualitas
  video (Otomatis/1080p/720p/… dari level HLS), gambar-dalam-gambar, layar penuh,
  tombol CC, klik video untuk putar/jeda, klik ganda untuk layar penuh, control
  bar menyembunyi otomatis, dan pintasan keyboard (spasi/K, panah, F, M, N).
  Tombol Lewati OP/ED dipindah ke posisi yang tidak tertutup control bar.
- **Pemilihan sumber manual**: chip Otomatis/Otakudesu/HiAnime/AniChan di halaman
  detail dan tonton (`?src=`), menonaktifkan fallback otomatis; pesan jelas bila
  judul tidak tersedia di sumber pilihan.
- **Pemilih episode cerdas**: pencarian nomor episode, rentang per 100 episode
  untuk seri panjang (otomatis membuka rentang episode aktif), dan tanda progres
  tontonan per episode dari riwayat.
- **Jelajah genre**: halaman /genre (18 genre AniList) dan daftar per genre
  terpopuler dengan penomoran halaman; chip genre di detail kini bisa diklik.
- **Drawer menu mobile**: navigasi meluncur dari kanan (☰) lengkap dengan
  pencarian; tooltip pada tombol-tombol pemain dan pemilih sumber.

### Diperbaiki
- **Halaman pencarian rusak diam-diam** sejak awal: hasil tidak pernah tampil
  (definisi kartu poster tinggal di berkas template lain). Kartu kini dibagikan
  dari template dasar dan pencarian menampilkan jumlah hasil.
- Judul anime pada kartu poster dibatasi maksimal 2 baris dengan elipsis —
  judul panjang tidak lagi merusak tata letak rail/grid.

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
