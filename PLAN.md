# Rencana Pengembangan NaniPlay

Produk: pemutar & agregator anime, film, dan series subtitle Indonesia,
**Web (self-hosted) + Windows**, satu aplikasi Go. Android menyusul sebagai
konsumen API yang sama.

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
- [ ] AnimeTosho — **tertunda**: lampiran subtitle upstream sedang rusak (tautan unduh resmi mereka sendiri membalas 404 per 2026-11-08, terverifikasi dari dua jaringan); tinjau ulang berkala
- [ ] Merge multi-sumber dengan prioritas (bawaan → komunitas → AI) — *parsial: prioritas & pilihan sumber sudah ada; penggabungan isi dua track menjadi satu belum*
- [x] Koreksi desync manual (offset ±0,5s di pemutar); otomatis belum
- [x] Pilihan track subtitle di pemutar + unggah file + ambil dari URL

## Fase 3 — Sumber Kedua *(SELESAI di v0.6.0)*

- [x] Provider HiAnime (katalog besar; dekripsi payload Megaplay AES-256-CBC; softsub bawaan terimpor & bisa diterjemahkan GTX)
- [x] Server lama ZokoAnime untuk judul yang hanya punya server itu (blob XOR window.__P)
- [x] Provider AniChan (anichan.to — berkunci ID AniList; sesi + triple kunci hasil scrape bundle; payload server AES-256-GCM; subtitle bawaan terimpor)
- [x] Fallback otomatis antar sumber bila satu gagal (Otakudesu → HiAnime → AniChan)
- [x] Indikator sumber di halaman detail

## Fase 4 — AI Fansub & Pelengkap

- [x] Terjemahan mesin Google GTX tanpa kunci (Inggris → Indonesia, tersimpan luring) *(v0.4.0)*
- [x] AI fansub BYOK via endpoint OpenAI-compatible apa pun (Gemini/Groq/OpenAI/Ollama/9router sendiri) — tingkat di atas GTX; baris yang dijatuhkan model diisi GTX *(v0.8.0)*
- [x] AniSkip: deteksi & lewati opening/ending *(selesai lebih awal, di v0.2.0)*
- [x] Jadwal rilis mingguan (AniList airing schedule) *(selesai lebih awal, di v0.2.0)*
- [x] Jelajah genre (daftar per genre AniList + penomoran halaman) *(v0.8.0)*
- [x] Pemain kustom (menu kualitas, PiP, pintasan keyboard), pemilihan sumber manual, pemilih episode cerdas, drawer mobile *(v0.8.0, masukan pengguna)*
- [ ] Episode mendatang untuk anime di Daftar Saya/Riwayat
- [ ] Mode TV (navigasi D-pad) di web

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

- [ ] Generalisasi kunci media di store: `anilist_id` (int) → kunci media
      string (`anime:{id}`, `movie:{slug}`, `series:{slug}`) untuk
      history, favorites, dan subtitle_tracks; migrasi data lama otomatis
- [ ] Seksi navigasi Anime | Film | Series; Pengaturan: aktif/nonaktifkan
      IDLIX + URL basisnya

### v0.10.0 — Katalog sinema

- [ ] Paket `internal/idlix`: klien API katalog full-Go (sidik jari TLS
      Chrome via uTLS, cache respons, antrean tunggal, domain dapat diganti)
- [ ] Halaman Film & Series: rail beranda per seksi, jelajah + genre,
      detail series dengan musim/episode, pencarian per seksi

### v0.11.0 — Putar film & series

- [ ] Resolusi putar: play-info → klaim sesi → redeem → master HLS,
      diputar lewat proxy & pemain yang sama dengan anime
- [ ] Trek subtitle dari host video diimpor ke sistem subtitle yang ada
      (termasuk terjemahan GTX/AI untuk trek asing)
- [ ] Riwayat & Daftar Saya campuran anime + film + series

### v0.12.0 — Satu beranda, satu pencarian

- [ ] Pencarian gabungan (anime + film + series dalam satu hasil)
- [ ] Hero & rail campuran di beranda; halaman genre film

### v1.0.0 — Mode TV & penyempurnaan

- [ ] Mode TV di web: navigasi D-pad, fokus membesar/bercahaya pada kartu
      (ide tata letak TV diadopsi bersih dari aplikasi TV komunitas)
- [ ] Episode mendatang untuk judul di Daftar Saya/Riwayat *(sisa Fase 4)*
- [ ] Gabung dua trek subtitle menjadi satu *(sisa Fase 2)* + desync otomatis

## Nanti

- [ ] Android (konsumen API yang sama)
- [ ] Sinkronisasi antar perangkat (opsional, self-hosted)
