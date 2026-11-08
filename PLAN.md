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

## Fase 1 — MVP Bisa Memutar ✅ (target rilis v0.1)

- [x] Provider Otakudesu: pencarian, daftar episode, resolve stream (dekripsi config pemutar putarin, AES-256-GCM)
- [x] Pencocokan judul AniList → Otakudesu (normalisasi token + musim)
- [x] Proxy HLS: header Referer/UA, penulisan ulang playlist & segmen
- [x] Pemutar hls.js: resume otomatis, simpan progres berkala, episode berikutnya otomatis
- [x] Halaman: Beranda (hero + rail), Detail + episode, Tonton, Pencarian, Daftar Saya, Riwayat
- [ ] Rilis GitHub Actions: zip Windows + binary Linux per tag

## Fase 2 — Subtitle Engine

- [ ] Ambil subtitle dari OpenSubtitles & AnimeTosho; parser WebVTT/ASS
- [ ] Merge multi-sumber dengan prioritas (bawaan → komunitas → AI)
- [ ] Koreksi desync (geser timing) otomatis & manual
- [ ] Pilihan track subtitle di pemutar

## Fase 3 — Sumber Kedua

- [ ] Provider HiAnime (katalog besar; token/embed)
- [ ] Fallback otomatis antar sumber bila satu gagal
- [ ] Indikator sumber di halaman detail

## Fase 4 — AI Fansub & Pelengkap

- [ ] AI fansub BYOK via endpoint OpenAI-compatible apa pun (Gemini/Groq/OpenAI/Ollama/9router sendiri)
- [ ] AniSkip: deteksi & lewati opening/ending
- [ ] Jadwal rilis mingguan (AniList airing schedule)
- [ ] Mode TV (navigasi D-pad) di web

## Nanti

- [ ] Android (konsumen API yang sama)
- [ ] Sinkronisasi antar perangkat (opsional, self-hosted)
