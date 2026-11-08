# Rencana Pengembangan NaniPlay

Produk: pemutar & agregator anime, film, dan series subtitle Indonesia,
**Web (self-hosted, mobile & desktop) + Windows**, satu aplikasi Go.
Mode TV dan aplikasi Android **tidak direncanakan** (keputusan
2026-11-08) — fokus penuh ke pengalaman web & Windows.

Sejak keputusan 2026-11-08, NaniPlay adalah SATU aplikasi untuk anime
(AniList + banyak sumber) DAN film/series (katalog IDLIX) — satu web,
satu container, satu repo. Implementasi selalu clean-room: perilaku API
publik situs sumber yang dipelajari, bukan kode proyek lain.

## Prinsip

- Satu codebase Go; distribusi = satu binary (Linux server / Windows portable).
- Sumber streaming di belakang interface `source.Provider`; kerusakan satu sumber terisolasi.
- UI bergaya layanan streaming modern (gelap + aksen merah, hero, rail poster) — pola yang sudah familiar bagi pengguna.
- Data pengguna lokal (SQLite): riwayat, favorit, posisi resume. Tanpa akun, tanpa telemetri.

## Fase 0 — Fondasi ✅

- [x] Skeleton repo, struktur paket, build lokal & Windows (Makefile)
- [x] Design system (tema gelap #141414, aksen #E50914, rail + hero)
- [x] Skema SQLite: history, favorites
- [x] Klien AniList: trending, musim berjalan, top, pencarian, detail

## Fase 1 — MVP Bisa Memutar ✅ (rilis v0.1.0)

- [x] Provider Otakudesu: pencarian, daftar episode, resolve stream (dekripsi config pemutar putarin, AES-256-GCM)
- [x] Pencocokan judul AniList → Otakudesu (normalisasi token + musim)
- [x] Proxy HLS: header Referer/UA, penulisan ulang playlist & segmen
- [x] Pemutar hls.js: resume otomatis, simpan progres berkala, episode berikutnya otomatis
- [x] Halaman: Beranda (hero + rail), Detail + episode, Tonton, Pencarian, Daftar Saya, Riwayat
- [x] Rilis GitHub Actions: zip Windows + binary Linux per tag

## Rilis v0.2.0 — Pengalaman & Fondasi Profesional ✅

- [x] AniSkip: tombol Lewati Opening/Ending/Rekap di pemutar
- [x] Jadwal Rilis 7 hari (AniList) + Anime Terkait di detail
- [x] Pencocokan judul berbasis skor + arbitrase jumlah episode
- [x] Pengaturan: ganti domain sumber dari UI, cek versi & rilis baru
- [x] Standar repo profesional: CI (gofmt/vet/test/build), unit test, CHANGELOG, badge README

## Fase 2 — Subtitle Engine *(sebagian besar selesai di v0.3.0)*

- [x] Ambil subtitle dari OpenSubtitles (tanpa API key); parser SRT/WebVTT/ASS → WebVTT
- [ ] AnimeTosho — **tertunda**: endpoint unduhnya pulih sebagian per
      2026-11-08 sore (bukan 404 lagi — /download/{id}/subs/all kini
      mengalihkan ke pack lampiran .7z yang dibuat dinamis, 36–42 MB
      per torrent), tetapi arsipnya tidak terbaca oleh pembaca 7z
      murni Go maupun Python yang tersedia (header ditolak keduanya),
      jadi klien belum bisa dibangun di atasnya; tinjau ulang berkala
- [x] Merge multi-sumber dengan prioritas (bawaan → komunitas → AI) — *prioritas & pilihan sumber ada sejak Fase 2/3; penggabungan isi dua trek menjadi satu trek dwibahasa selesai di v1.0.0*
- [x] Koreksi desync manual (offset ±0,5s di pemutar); otomatis belum
- [x] Pilihan track subtitle di pemutar + unggah file + ambil dari URL

## Fase 3 — Sumber Kedua *(SELESAI di v0.6.0)*

- [x] Provider HiAnime (katalog besar; dekripsi payload Megaplay AES-256-CBC; softsub bawaan terimpor & bisa diterjemahkan GTX)
- [x] Server lama ZokoAnime untuk judul yang hanya punya server itu (blob XOR window.__P)
- [x] Provider AniChan (anichan.to — berkunci ID AniList; sesi + triple kunci hasil scrape bundle; payload server AES-256-GCM; subtitle bawaan terimpor)
- [x] Fallback otomatis antar sumber bila satu gagal (HiAnime → AniChan → Otakudesu sejak v1.11.0)
- [x] Indikator sumber di halaman detail

## Fase 4 — AI Fansub & Pelengkap

- [x] Terjemahan mesin Google GTX tanpa kunci (Inggris → Indonesia, tersimpan luring) *(v0.4.0)*
- [x] AI fansub BYOK via endpoint OpenAI-compatible apa pun (Gemini/Groq/OpenAI/Ollama/9router sendiri) — tingkat di atas GTX; baris yang dijatuhkan model diisi GTX *(v0.8.0)*
- [x] AniSkip: deteksi & lewati opening/ending *(selesai lebih awal, di v0.2.0)*
- [x] Jadwal rilis mingguan (AniList airing schedule) *(selesai lebih awal, di v0.2.0)*
- [x] Jelajah genre (daftar per genre AniList + penomoran halaman) *(v0.8.0)*
- [x] Pemain kustom (menu kualitas, PiP, pintasan keyboard), pemilihan sumber manual, pemilih episode cerdas, drawer mobile *(v0.8.0, masukan pengguna)*
- [x] Episode mendatang untuk anime di Daftar Saya/Riwayat *(v1.0.0)*

## Fase 5 — Sinema: Gabung Film & Series IDLIX *(keputusan 2026-11-08)*

Temuan kunci yang membuat fase ini mungkin (terverifikasi 2026-11-08):

- Situs IDLIX (`z2.idlixku.com`, dapat diganti dari Pengaturan) memiliki
  **API JSON-nya sendiri**: `/api/movies`, `/api/movies/{slug}`,
  `/api/series`, `/api/series/{slug}`, `/api/series/{slug}/season/{s}/episode/{e}`,
  `/api/genres`, `/api/search`, serta alur putar
  `/api/watch/play-info/{type}/{id}` → gate token + hitung mundur buka kunci →
  `POST /api/watch/session/claim` → redeem ke host video (majorplay) →
  master HLS + trek subtitle. NaniPlay mengimplementasikan klien API ini
  secara clean-room di Go.
- Situs berada di balik penyaringan bot Cloudflare ("Just a moment",
  HTTP 403 untuk klien HTTP biasa — curl langsung & via proxy sama-sama
  ditolak). Namun gerbangnya berbasis **skor sidik jari TLS/HTTP2**, bukan
  kewajiban mengeksekusi JavaScript: klien Go murni dengan sidik jari
  Chrome (uTLS, `bogdanfinn/tls-client`) LOLOS — terverifikasi dari server
  produksi 2026-11-08: `/api/movies` 200 JSON, `/api/genres` 200 JSON,
  beranda 200 HTML asli. **NaniPlay tetap 100% Go, satu binary, tanpa
  browser/Node.** Alur putar (gerbang token → klaim → redeem) memakai
  klien yang sama dan diverifikasi end-to-end pada v0.11.0.
- Konsekuensi: container tetap ramping (kelas ~20 MB); dependensi baru
  hanya pustaka sidik jari TLS murni Go.

### v0.9.0 — Fondasi gabung

- [x] Generalisasi kunci media di store: `anilist_id` (int) → kunci media
      string (`anime:{id}`, `movie:{slug}`, `series:{slug}`) untuk
      history, favorites, dan subtitle_tracks; migrasi data lama otomatis
      *(v0.9.0 — terverifikasi pada snapshot DB produksi)*
- [x] Seksi navigasi Anime | Film | Series; Pengaturan: aktif/nonaktifkan
      IDLIX + URL basisnya *(v0.9.0)*

### v0.10.0 — Katalog sinema

- [x] Paket `internal/idlix`: klien API katalog full-Go (sidik jari TLS
      Chrome via uTLS, cache respons, domain dapat diganti dari Pengaturan)
      *(v0.10.0)*
- [x] Halaman Film & Series: rail beranda per seksi, jelajah + genre,
      detail series dengan musim/episode, pencarian per seksi *(v0.10.0)*

### v0.11.0 — Putar film & series

- [x] Resolusi putar: play-info → klaim sesi → redeem → master HLS,
      diputar lewat proxy & pemain yang sama dengan anime *(v0.11.0)*
- [x] Trek subtitle dari host video diimpor ke sistem subtitle yang ada
      (termasuk terjemahan GTX/AI untuk trek asing) *(v0.11.0)*
- [x] Riwayat & Daftar Saya campuran anime + film + series *(v0.10.0–0.11.0)*

### v0.12.0 — Satu beranda, satu pencarian

- [x] Pencarian gabungan (anime + film + series dalam satu hasil) *(v0.12.0)*
- [x] Hero & rail campuran di beranda; halaman genre film *(v0.12.0)*

### v1.0.0 — Penyempurnaan

- [x] Episode mendatang untuk judul di Daftar Saya/Riwayat *(sisa Fase 4)* *(v1.0.0)*
- [x] Gabung dua trek subtitle menjadi satu *(sisa Fase 2)* + desync otomatis *(v1.0.0)*

## Fase 6 — Pengaturan ala Tatap & SubDL (dari audit repo pembanding)

- [x] Pengaturan Pemutaran: sumber utama, kualitas bawaan, bahasa
      subtitle bawaan *(v1.1.0)*
- [x] Bahasa subtitle dipelajari dari pilihan di pemutar dan
      diterapkan kembali di episode berikutnya *(v1.1.0)*
- [x] Prompt Fansub AI dapat disunting; deteksi penyedia dari kunci
      API; tombol Tes koneksi untuk semua alamat *(v1.1.0–v1.1.1)*
- [x] Subtitle online Film & Series via SubDL (kunci API pengguna,
      pencarian per IMDb ID + season/episode) *(v1.2.0)*

## Fase 7 — Poles UX (audit menyeluruh 2026-11-08)

- [x] Kontrol pemutar ala Netflix: ikon SVG, seek bar penuh dengan
      scrub & tooltip, ±10 detik, kecepatan putar, spinner, judul
      di bar *(v1.3.0)*
- [x] Beranda: panah geser rail, Lihat Semua, Daftar Saya di hero,
      Putar hero langsung memutar, rail personal per genre,
      hero mobile ramping *(v1.3.0)*
- [x] Soft-404 diperbaiki (status 404 asli) *(v1.3.0)*

## Fase 8 — Panel subtitle & beranda hidup (2026-11-08)

- [x] Panel subtitle: daftar trek baris, aksi per trek, alat
      lanjutan terlipat penuh *(v1.4.0–v1.4.1)*
- [x] Klik area video = tampil/sembunyi kontrol saja *(v1.4.2)*
- [x] Rail Episode Terbaru di beranda *(v1.5.0)*

## Di luar rencana (keputusan 2026-11-08)

- Mode TV dan aplikasi Android — tidak dikerjakan kecuali pemilik
  memutuskan sebaliknya.
- Sinkronisasi antar perangkat — **tidak diperlukan**: aplikasi ini
  self-hosted, semua data (riwayat, daftar, subtitle, pengaturan)
  hidup di database server yang sama, jadi setiap perangkat yang
  membuka web-nya otomatis melihat data yang sama. Satu-satunya
  pengecualian adalah biner Windows mandiri dengan database
  lokalnya sendiri; untuk itu cukup arahkan pemakaian ke instance
  server, bukan membangun mekanisme sinkronisasi.

## Fase 9 — Penyempurnaan menyeluruh & README (v1.6.0)

- [x] README ditulis ulang: anime + film + series, fitur berkelompok, tabel env, kredit di akhir
- [x] Autoplay episode berikutnya (kartu hitung mundur 20 dtk terakhir + lanjut otomatis)
- [x] Hapus riwayat per judul & hapus semua (endpoint memakai DeleteHistory yang sudah ada)
- [x] Penanda hari ini di Jadwal; meta/OG/theme-color; footer mencerminkan cakupan penuh
- [x] v1.6.1: perbaiki lomba event ketuk-vs-klik pada kontrol pemutar (keputusan dari pointerdown), kontrol menetap saat pause, HTML no-cache, tumpukan kartu episode berikutnya vs tombol skip
- [x] v1.6.2: penjaga SSRF proxy (validasi IP di lapisan dial), soft-404 terakhir dibereskan; audit XSS panel subtitle & resume dinyatakan bersih
- [x] v1.6.3: audit ganda (kode + browser live) — yatim terjemahan sinema, panic merge/autosync, bocor kunci via tes koneksi, kepemilikan autosync, SSRF pengambil subtitle, subtitle ganda hardsub, dedupe trek, cache AniList terbatas
- [x] v1.7.0: UX halaman putar — panel subtitle lipat (bawaan tertutup di HP, diingat) + blok info sedang diputar (poster, meta, sinopsis, Daftar Saya, genre) untuk anime & sinema
- [x] v1.7.1: pemilih sumber — dihapus dari halaman putar (diganti info pasif + tautan pemulihan di alert), diturunkan jadi tautan teks tenang di detail

## Fase 10 — Film Indo / KitaNonton (v1.8.0)

- [x] Klien KitaNonton full-Go (paket `internal/kitanonton`): katalog arsip `/country/indonesia/` berpaginasi, detail, daftar server halaman putar (film & series per episode), domain basis dari Pengaturan (`kita_base`, env `KITANONTON_BASE`)
- [x] Pembongkar blob `_juicycodes` (JuicyCodes): garam 3 char terakhir, base64 URL-safe, substitusi peta simbol 10 huruf, potongan 4 digit → konfigurasi JW polos (master HLS `*.groovy.monster` s.d. 1080p)
- [x] Token identitas JuicyCodes = `base64(IP + "~~" + UA)` penonton (tanpa kunci rahasia; terbukti membuka master dari dua server) — URL master ditandatangani untuk penonton dan diserahkan ke browser; segmen mengalir langsung dari CDN; token ditempel ulang ke permintaan anak via `xhrSetup` hls.js
- [x] Seksi "🇮🇩 Film Indo": `/film-indo`, `/film-indo/{slug}`, `/series-indo/{slug}` + putar, rel beranda, kunci media `indo:`/`indoser:`, subtitle daring via SubDL berdasarkan judul
- [ ] Server cadangan AbyssPlayer ("ex") belum didukung — hanya server "dv" (JuicyCodes) yang dipakai; judul yang hanya punya server "ex" menampilkan pesan jujur
