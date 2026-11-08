# Rencana Pengembangan NaniPlay

Produk: pemutar & agregator anime subtitle Indonesia, **Web (self-hosted) + Windows**,
satu binary Go. Android menyusul sebagai konsumen API yang sama.

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

## Fase 3 — Sumber Kedua

- [ ] Provider HiAnime (katalog besar; token/embed)
- [ ] Provider AniChan (anichan.to — sumber kedua di ekosistem Tatap/Shisen; resolver HLS + subtitle bawaan)
- [ ] Fallback otomatis antar sumber bila satu gagal
- [ ] Indikator sumber di halaman detail

## Fase 4 — AI Fansub & Pelengkap

- [x] Terjemahan mesin Google GTX tanpa kunci (Inggris → Indonesia, tersimpan luring) *(v0.4.0)*
- [ ] AI fansub BYOK via endpoint OpenAI-compatible apa pun (Gemini/Groq/OpenAI/Ollama/9router sendiri) — tingkat di atas GTX: hybrid LLM mengisi baris yang gagal diterjemahkan mesin
- [x] AniSkip: deteksi & lewati opening/ending *(selesai lebih awal, di v0.2.0)*
- [x] Jadwal rilis mingguan (AniList airing schedule) *(selesai lebih awal, di v0.2.0)*
- [ ] Jelajah genre & filter katalog
- [ ] Episode mendatang untuk anime di Daftar Saya/Riwayat
- [ ] Mode TV (navigasi D-pad) di web

## Nanti

- [ ] Android (konsumen API yang sama)
- [ ] Sinkronisasi antar perangkat (opsional, self-hosted)
