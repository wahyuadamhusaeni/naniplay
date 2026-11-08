# Changelog

Semua perubahan penting NaniPlay dicatat di sini.
Format mengikuti [Keep a Changelog](https://keepachangelog.com/id/1.1.0/),
versi mengikuti [Semantic Versioning](https://semver.org/lang/id/).

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

[0.2.0]: https://github.com/wahyuadamhusaeni/naniplay/releases/tag/v0.2.0
[0.1.0]: https://github.com/wahyuadamhusaeni/naniplay/releases/tag/v0.1.0
