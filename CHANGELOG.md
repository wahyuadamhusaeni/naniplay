# Changelog

Semua perubahan penting NaniPlay dicatat di sini.
Format mengikuti [Keep a Changelog](https://keepachangelog.com/id/1.1.0/),
versi mengikuti [Semantic Versioning](https://semver.org/lang/id/).

## [1.8.0] — 2026-11-08

### Ditambahkan
- **Seksi Film Indo (KitaNonton)** — film & series Indonesia dari arsip
  negara Indonesia KitaNonton (±1.400 judul, 62 halaman): katalog
  berpaginasi, halaman detail (sinopsis, genre, sutradara, pemeran,
  durasi, rating), daftar episode untuk series, dan rel "Film Indo
  Terbaru" di beranda. Domain sumber bisa diganti dari Pengaturan
  (situsnya sering berpindah domain), lengkap dengan tombol tes
  koneksi. Kunci media baru `indo:` / `indoser:` berbagi sistem
  Daftar Saya, Riwayat, dan subtitle dengan seksi lain; pencarian
  subtitle daringnya memakai SubDL berdasarkan judul.
- **Putar langsung dari CDN** — server video KitaNonton (JuicyCodes)
  mengikat token identitasnya ke IP + User-Agent penonton, jadi
  server hanya membongkar konfigurasi pemutar (blob `_juicycodes`
  terdekode penuh di sisi server) lalu menyerahkan URL master HLS
  yang ditandatangani untuk penonton; playlist dan segmen mengalir
  langsung dari CDN ke perangkat penonton, dan pemutar menempelkan
  token yang sama ke permintaan anak lewat `xhrSetup` hls.js.

## [1.7.1] — 2026-11-08

### Diubah
- Pemilih sumber dihapus dari halaman putar (diganti info sumber
  pasif + tautan "Coba sumber lain" pada alert galat) dan diturunkan
  menjadi tautan teks tenang di halaman detail. Hierarki final:
  Pengaturan = bawaan global, Detail = pengecualian per judul,
  Putar = tanpa pemilih.

## [1.7.0] — 2026-11-08

### Ditambahkan
- Panel subtitle yang bisa dilipat di halaman putar (bawaan tertutup
  di HP, pilihan diingat) dan blok info "sedang diputar" (poster,
  meta, sinopsis, tombol Daftar Saya, chip genre) untuk anime dan
  sinema.

## [1.6.3] — 2026-11-08

### Diperbaiki
- Audit ganda (tinjauan kode + audit browser langsung): terjemahan
  subtitle sinema yang yatim, panic pada gabung/sinkron trek tak
  dikenal, kebocoran kunci API lewat tes koneksi, pemeriksaan
  kepemilikan autosync, pengambil subtitle kini lewat klien
  terproteksi SSRF, subtitle ganda pada sumber hardsub, dedupe
  penyimpanan trek, dan batas cache AniList.

## [1.6.2] — 2026-11-08

### Diperbaiki
- Penjaga SSRF pada proxy HLS (validasi IP tujuan di lapisan dial)
  dan soft-404 terakhir yang kini dirender sebagai 404 betulan.

## [1.6.1] — 2026-11-08

### Diperbaiki
- Kontrol pemutar yang hilang-timbul di layar sentuh (keputusan
  tampil/sembunyi diambil dari keadaan saat pointerdown), kontrol
  menetap selama jeda, HTML kini `Cache-Control: no-cache`, dan
  masukan formulir 16px di HP (anti zoom fokus iOS).

## [1.6.0] — 2026-11-08

### Ditambahkan
- Putar otomatis episode berikutnya dengan kartu hitung mundur,
  hapus riwayat per judul & hapus semua, penanda hari ini di Jadwal.

### Diubah
- README ditulis ulang mencerminkan cakupan penuh (anime + film +
  series); Tatap dan idlix-androidtv dikreditkan sewajarnya.

## [1.5.2] — 2026-11-08

### Diperbaiki
- Pemilih episode: 6 tombol per baris di HP dan bilah cari selebar
  penuh.

## [1.5.1] — 2026-11-08

### Diubah
- Sembunyi otomatis kontrol pemutar diperpanjang menjadi 5 detik.

## [1.5.0] — 2026-11-08

### Ditambahkan
- Rel "🆕 Episode Terbaru" di beranda (episode yang baru tayang
  72 jam terakhir, tautan langsung ke halaman putar).

## [1.4.2] — 2026-11-08

### Diubah
- Klik area video hanya menampilkan/menyembunyikan kontrol; putar/
  jeda lewat tombol khusus, spasi, atau tombol putar besar.

## [1.4.1] — 2026-11-08

### Diperbaiki
- Tombol layar penuh yang hilang di HP (baris kontrol meluber) dan
  semua tombol kelola subtitle dilipat ke satu panel "Kelola".

## [1.4.0] — 2026-11-08

### Diubah
- Panel subtitle didesain ulang: daftar trek sebagai baris yang bisa
  diklik (bukan dropdown), tombol cari subtitle daring selebar
  penuh, perkakas dilipat dalam satu `<details>`.

## [1.3.0] — 2026-11-08

### Ditambahkan
- Kontrol pemutar didesain ulang bergaya Netflix (sprite SVG, seek
  bar kustom), navigasi rel beranda, hero dengan tombol Daftar Saya
  per slide, dan rel personal "Pilihan {Genre} Untukmu".

### Diperbaiki
- Soft-404 di beranda dan lima titik sinema kini 404 betulan.

## [1.2.0] — 2026-11-08

### Ditambahkan
- Subtitle film & series dari SubDL (kunci API pengguna sendiri):
  pencarian daring di panel subtitle untuk judul sinema berdasarkan
  ID IMDb.

## [1.1.1] — 2026-11-08

### Diperbaiki
- Tes koneksi AI kini memakai panggilan chat completion minimal
  (bukan GET /models yang bisa menggantung).

## [1.1.0] — 2026-11-08

### Ditambahkan
- Pengaturan pemutaran ala Tatap: sumber utama, kualitas bawaan,
  bahasa subtitle yang diingat antar episode, prompt fansub kustom,
  deteksi penyedia AI dari kunci, dan tombol tes koneksi per alamat.

## [1.0.1] — 2026-11-08

### Diperbaiki
- **Halaman detail anime terpotong**: chip Episode Mendatang dirujuk
  sebagai `.NextLabel` di dalam blok `{{with .Anime}}` sehingga
  eksekusi template gagal dan halaman berhenti setelah meta hero
  (daftar episode hilang). Kini dirujuk dari akar data (`$.NextLabel`)
  dan halaman tampil utuh lagi.

## [1.0.0] — 2026-11-08

Rilis 1.0: NaniPlay lengkap untuk web (mobile & desktop) dan Windows —
satu aplikasi untuk anime, film, dan series, dengan subtitle
Indonesia sebagai fokusnya. (Mode TV dan aplikasi Android secara
eksplisit di luar rencana; lihat PLAN.md.)

### Ditambahkan
- **Episode Mendatang**: judul anime yang sedang tayang di Daftar
  Saya, Riwayat, dan halaman detail kini menampilkan episode
  berikutnya beserta hari/tanggal/jam tayangnya (WIB), dari data
  airing AniList.
- **Gabung dua trek subtitle**: tombol "Gabung ke Trek Aktif" di
  panel subtitle menggabungkan trek aktif dengan trek kedua dari
  episode yang sama menjadi satu trek dwibahasa (baris sekunder
  menempel pada jendela cue primer; sisa sekunder berdiri sendiri).
- **Sinkron otomatis (desync)**: tombol "Sinkronkan ke Referensi"
  mengestimasi selisih timing trek aktif terhadap trek referensi
  (histogram selisih awal cue, resolusi 0,25 detik, rentang ±30
  detik) dan menyimpan salinan terkoreksi bila selisihnya nyata
  (≥0,25 detik).

## [0.12.0] — 2026-11-08

### Ditambahkan
- **Pencarian gabungan**: halaman Cari kini menampilkan hasil anime
  (AniList) plus seksi 🎬 Film dan 📺 Series dari katalog sinema dalam
  satu halaman, masing-masing dengan kartunya sendiri. Saat sumber
  sinema tidak terjangkau, alasannya ditampilkan dengan jujur dan
  hasil anime tetap tampil.
- **Hero beranda campuran**: sorotan utama kini berganti otomatis
  (±6,5 detik, berhenti saat kursor menyentuh) antara anime trending
  #1, film terpopuler, dan series terpopuler — lengkap dengan titik
  navigasi. Tombol Putar series langsung menuju episode pertamanya.
- **Halaman genre sinema** (`/genre-sinema`): semua genre katalog
  sebagai ubin dengan tautan jelajah Film dan Series per genre;
  terhubung dari cip genre di halaman Film/Series.

## [0.11.1] — 2026-11-08

### Diperbaiki
- **Putar sinema benar-benar mengalir**: playlist host video tersaji
  dengan nama berkas non-playlist (mis. `config-*.json`) dan setiap
  permintaan anak (varian, audio, segmen) wajib membawa token query
  dari URL master (skema Shaka Packager). Proxy kini (1) mengenali
  playlist dari isi `#EXTM3U` tanpa bergantung ekstensi/tipe konten,
  dan (2) meneruskan query induk ke URL anak yang belum punya query
  bila aliran memintanya (parameter `pq=1`, hanya dipakai putar
  sinema — aliran anime tidak berubah). Terverifikasi end-to-end:
  master → varian → segmen media 200 lewat proxy publik.

## [0.11.0] — 2026-11-08

### Ditambahkan
- **Pemutar Film & Series**: alur gerbang putar penyedia diimplementasikan
  penuh secara full-Go — play-info menerbitkan token gerbang, klaim
  sesi setelah hitung mundur ±15 detik, redeem di host video menjadi
  master HLS + berkas subtitle. Halaman putar memakai pemutar kustom
  yang sama dengan anime (kontrol penuh, kualitas HLS, PiP, keyboard).
- **Subtitle sinema terimpor otomatis**: trek subtitle bawaan server
  masuk ke penyimpanan trek lokal — panel subtitle, unggah, ambil dari
  URL, terjemahan GTX dan Fansub AI semuanya bekerja untuk film/series.
  Rute subtitle digeneralisasi per kunci media
  (`/api/subtitles/k/{key}/{ep}`).
- Riwayat "Lanjut Menonton" series membawa label S/E (mis. S2 E3) dan
  tautan langsung ke episode yang sama.
- Paket `idlix`: `ResolvePlayback`, `Episode`, `gateDID`, POST JSON
  ber-sidik jari TLS; tes alur putar dengan fetcher fixture.

### Diperbaiki
- Detail series tanpa `defaultSeason` dari API kini mundur ke season
  pertama (sebelumnya daftar episode kosong).
- Judul katalog tanpa video diberi lencana "Belum ada video" dan
  tombol putarnya dinonaktifkan dengan jujur.
- Validasi domain HiAnime di Pengaturan kini menampilkan pesan error
  (sebelumnya input tidak valid diabaikan diam-diam).
- Redaksi total pencarian sinema diperjelas ("total di semua seksi").

### Catatan
- Sesi putar bersifat sekali pakai: memuat ulang halaman putar akan
  menyiapkan sesi baru (±15 detik), sama seperti aplikasi resmi
  sumbernya.

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
