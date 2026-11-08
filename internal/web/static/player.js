// NaniPlay player: attaches HLS (native or hls.js), resumes the saved
// position, and reports progress to the server so "Lanjutkan Menonton"
// stays current across devices.
(function () {
  var video = document.getElementById("player");
  if (!video) return;
  var src = video.dataset.src;
  var meta = {
    anilistId: parseInt(video.dataset.anilist, 10),
    title: video.dataset.title || "",
    poster: video.dataset.poster || "",
    episode: parseInt(video.dataset.episode, 10),
  };
  var saved = parseFloat(video.dataset.saved || "0");
  var next = video.dataset.next || "";

  var hls = null; // dipakai juga oleh pemilih kualitas
  function attach() {
    if (video.canPlayType("application/vnd.apple.mpegurl")) {
      video.src = src;
    } else if (window.Hls && Hls.isSupported()) {
      hls = new Hls({ maxBufferLength: 30 });
      hls.loadSource(src);
      hls.attachMedia(video);
      hls.on(Hls.Events.MANIFEST_PARSED, buildQualityMenu);
      hls.on(Hls.Events.LEVEL_SWITCHED, function (_e, data) {
        markQuality(data.level);
      });
    } else {
      video.src = src;
    }
  }
  attach();

  var resumed = false;
  video.addEventListener("loadedmetadata", function () {
    if (!resumed && saved > 10 && video.duration && saved < video.duration - 15) {
      video.currentTime = saved;
    }
    resumed = true;
    loadSkipTimes();
  });

  // --- AniSkip: OP/ED skip button -------------------------------------
  var skipBtn = document.getElementById("skipBtn");
  var intervals = [];
  var activeInterval = null;
  function loadSkipTimes() {
    fetch("/api/skiptimes/" + meta.anilistId + "/" + meta.episode)
      .then(function (r) { return r.json(); })
      .then(function (data) {
        if (!data.found || !data.results) return;
        intervals = data.results.map(function (res) {
          var scale = res.episodeLength > 0 ? video.duration / res.episodeLength : 1;
          return { type: res.type, start: res.start * scale, end: res.end * scale };
        });
      })
      .catch(function () {});
  }
  function skipLabel(type) {
    if (type.indexOf("op") >= 0) return "Lewati Opening";
    if (type.indexOf("ed") >= 0) return "Lewati Ending";
    if (type === "recap") return "Lewati Rekap";
    return "Lewati";
  }
  if (skipBtn) {
    skipBtn.addEventListener("click", function () {
      if (activeInterval) {
        video.currentTime = activeInterval.end + 0.1;
        skipBtn.hidden = true;
        activeInterval = null;
      }
    });
  }
  video.addEventListener("timeupdate", function () {
    if (!skipBtn || !intervals.length) return;
    var t = video.currentTime;
    var hit = null;
    for (var i = 0; i < intervals.length; i++) {
      if (t >= intervals[i].start && t < intervals[i].end - 1) { hit = intervals[i]; break; }
    }
    if (hit) {
      activeInterval = hit;
      skipBtn.textContent = skipLabel(hit.type) + " ⏭";
      skipBtn.hidden = false;
    } else if (activeInterval) {
      activeInterval = null;
      skipBtn.hidden = true;
    }
  });

  var lastSent = 0;
  function save(force) {
    var now = Date.now();
    if (!force && now - lastSent < 8000) return;
    if (!video.duration || video.currentTime < 3) return;
    lastSent = now;
    fetch("/api/history", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        anilistId: meta.anilistId,
        title: meta.title,
        poster: meta.poster,
        episode: meta.episode,
        position: video.currentTime,
        duration: video.duration,
      }),
      keepalive: true,
    }).catch(function () {});
  }
  video.addEventListener("timeupdate", function () { save(false); });
  video.addEventListener("pause", function () { save(true); });
  window.addEventListener("beforeunload", function () { save(true); });

  video.addEventListener("ended", function () {
    save(true);
    if (next) window.location.href = next;
  });

  // --- Control bar kustom: putar, seek, volume, kualitas, PiP ---
  var box = document.getElementById("playerBox");
  var pPlay = document.getElementById("pPlay");
  var pBigPlay = document.getElementById("pBigPlay");
  var pSeek = document.getElementById("pSeek");
  var pCur = document.getElementById("pCur");
  var pDur = document.getElementById("pDur");
  var pMute = document.getElementById("pMute");
  var pVol = document.getElementById("pVol");
  var pCC = document.getElementById("pCC");
  var pPiP = document.getElementById("pPiP");
  var pFull = document.getElementById("pFull");
  var qWrap = document.getElementById("pQualityWrap");
  var qBtn = document.getElementById("pQualityBtn");
  var qMenu = document.getElementById("pQualityMenu");
  var qLabel = document.getElementById("pQualityLabel");
  var seeking = false;
  var uiTimer = 0;

  function fmtT(t) {
    if (!isFinite(t) || t < 0) t = 0;
    var m = Math.floor(t / 60), sec = Math.floor(t % 60);
    var h = Math.floor(m / 60);
    if (h > 0) return h + ":" + String(m % 60).padStart(2, "0") + ":" + String(sec).padStart(2, "0");
    return m + ":" + String(sec).padStart(2, "0");
  }
  function togglePlay() {
    if (video.paused) video.play(); else video.pause();
  }
  function syncPlayUI() {
    var playing = !video.paused && !video.ended;
    if (pPlay) pPlay.textContent = playing ? "⏸" : "▶";
    if (box) {
      box.classList.toggle("paused", !playing);
      box.classList.toggle("playing", playing);
    }
  }
  function pokeUI() {
    if (!box) return;
    box.classList.add("ui-on");
    clearTimeout(uiTimer);
    uiTimer = setTimeout(function () { box.classList.remove("ui-on"); }, 2600);
  }
  function paintSeek() {
    if (!pSeek || !video.duration) return;
    var played = (video.currentTime / video.duration) * 100;
    var buffered = 0;
    try {
      if (video.buffered.length) buffered = (video.buffered.end(video.buffered.length - 1) / video.duration) * 100;
    } catch (e) { /* abaikan */ }
    pSeek.style.background = "linear-gradient(90deg, var(--red) " + played + "%, #8a8a8a " + played + "%, #8a8a8a " + buffered + "%, #4d4d4d " + buffered + "%)";
  }
  function toggleFull() {
    if (document.fullscreenElement) { document.exitFullscreen(); return; }
    if (box && box.requestFullscreen) box.requestFullscreen().catch(function () {});
    else if (video.webkitEnterFullscreen) video.webkitEnterFullscreen();
  }
  if (box && pPlay) {
    video.addEventListener("play", function () { syncPlayUI(); pokeUI(); });
    video.addEventListener("pause", function () { syncPlayUI(); pokeUI(); });
    video.addEventListener("click", function () { togglePlay(); });
    video.addEventListener("dblclick", function () { toggleFull(); });
    box.addEventListener("mousemove", pokeUI);
    box.addEventListener("touchstart", pokeUI, { passive: true });
    pPlay.addEventListener("click", togglePlay);
    if (pBigPlay) pBigPlay.addEventListener("click", togglePlay);
    pSeek.addEventListener("input", function () {
      seeking = true;
      if (video.duration) {
        video.currentTime = (pSeek.value / 1000) * video.duration;
        if (pCur) pCur.textContent = fmtT(video.currentTime);
      }
    });
    pSeek.addEventListener("change", function () { seeking = false; paintSeek(); });
    video.addEventListener("timeupdate", function () {
      if (!seeking && video.duration) {
        pSeek.value = String(Math.round((video.currentTime / video.duration) * 1000));
        paintSeek();
      }
      if (pCur) pCur.textContent = fmtT(video.currentTime);
    });
    video.addEventListener("progress", paintSeek);
    video.addEventListener("durationchange", function () { if (pDur) pDur.textContent = fmtT(video.duration); });
    pVol.addEventListener("input", function () {
      video.volume = pVol.value / 100;
      video.muted = false;
    });
    pMute.addEventListener("click", function () { video.muted = !video.muted; });
    video.addEventListener("volumechange", function () {
      pVol.value = String(Math.round(video.volume * 100));
      pMute.textContent = video.muted || video.volume === 0 ? "🔇" : video.volume < 0.5 ? "🔉" : "🔊";
    });
    if (pCC) pCC.addEventListener("click", function () {
      var panel = document.getElementById("subPanel");
      if (!panel) return;
      panel.scrollIntoView({ behavior: "smooth", block: "nearest" });
      panel.classList.add("flash");
      setTimeout(function () { panel.classList.remove("flash"); }, 1200);
      var sel = document.getElementById("subSelect");
      if (sel) sel.focus({ preventScroll: true });
    });
    if (pPiP) pPiP.addEventListener("click", function () {
      if (document.pictureInPictureElement) document.exitPictureInPicture();
      else if (video.requestPictureInPicture) video.requestPictureInPicture().catch(function () {});
    });
    if (pFull) pFull.addEventListener("click", toggleFull);
    syncPlayUI();
    pokeUI();
  }

  // --- Kualitas (level HLS) ---
  function markQuality(activeIdx) {
    if (!qMenu || !hls) return;
    var auto = hls.autoLevelEnabled;
    qMenu.querySelectorAll("button").forEach(function (b) {
      var lv = parseInt(b.dataset.level, 10);
      b.classList.toggle("active", auto ? lv === -1 : lv === activeIdx);
    });
    var label = "Otomatis";
    if (hls.levels && hls.levels[activeIdx]) {
      label = auto ? "Otomatis · " + hls.levels[activeIdx].height + "p" : hls.levels[activeIdx].height + "p";
    }
    if (qLabel) qLabel.textContent = label;
  }
  function buildQualityMenu() {
    if (!hls || !qWrap || !qMenu || !hls.levels || hls.levels.length < 2) return;
    qWrap.hidden = false;
    qMenu.innerHTML = "";
    var seen = {};
    var items = [{ level: -1, label: "Otomatis" }];
    var idxs = hls.levels.map(function (_l, i) { return i; }).sort(function (a, b) {
      return hls.levels[b].height - hls.levels[a].height;
    });
    idxs.forEach(function (i) {
      var h = hls.levels[i].height;
      if (!h || seen[h]) return;
      seen[h] = true;
      items.push({ level: i, label: h + "p" });
    });
    items.forEach(function (it) {
      var b = document.createElement("button");
      b.type = "button";
      b.dataset.level = String(it.level);
      b.textContent = it.label;
      b.addEventListener("click", function () {
        hls.currentLevel = it.level;
        qMenu.hidden = true;
        markQuality(it.level < 0 && hls.levels.length ? 0 : it.level);
        if (it.level < 0 && qLabel) qLabel.textContent = "Otomatis";
      });
      qMenu.appendChild(b);
    });
    markQuality(hls.currentLevel < 0 ? 0 : hls.currentLevel);
  }
  if (qBtn && qMenu) {
    qBtn.addEventListener("click", function () { qMenu.hidden = !qMenu.hidden; });
    document.addEventListener("click", function (e) {
      if (!qMenu.hidden && !e.target.closest(".p-quality")) qMenu.hidden = true;
    });
  }

  // --- Subtitle: saved tracks, online search, upload, desync offset ---
  var subSelect = document.getElementById("subSelect");
  var subStatus = document.getElementById("subStatus");
  var subResults = document.getElementById("subResults");
  var subOffsetVal = document.getElementById("subOffsetVal");
  var subDelete = document.getElementById("subDelete");
  var subTranslate = document.getElementById("subTranslate");
  var subTranslateAI = document.getElementById("subTranslateAI");
  var subBase = "/api/subtitles/" + meta.anilistId + "/" + meta.episode;
  var subOffset = 0;
  var subActiveId = "";

  function subSay(msg) { if (subStatus) subStatus.textContent = msg || ""; }
  function subFmtOffset() {
    var v = subOffset.toFixed(1).replace(".", ",") + "s";
    if (subOffset > 0) v = "+" + v;
    subOffsetVal.textContent = v;
  }
  function activateSub(id) {
    subActiveId = id || "";
    var old = video.querySelectorAll("track");
    for (var i = 0; i < old.length; i++) old[i].remove();
    for (var j = 0; j < video.textTracks.length; j++) video.textTracks[j].mode = "hidden";
    if (subDelete) subDelete.hidden = !subActiveId;
    if (subTranslate) subTranslate.hidden = !subActiveId;
    if (subTranslateAI) subTranslateAI.hidden = !subActiveId;
    if (!subActiveId) return;
    var tr = document.createElement("track");
    tr.kind = "subtitles";
    tr.label = "Subtitle";
    tr.srclang = "id";
    tr.src = "/api/subtitles/track/" + subActiveId + "?offset=" + subOffset;
    tr.default = true;
    video.appendChild(tr);
    var tries = 0;
    var iv = setInterval(function () {
      tries++;
      for (var k = 0; k < video.textTracks.length; k++) {
        if (video.textTracks[k].kind === "subtitles") {
          video.textTracks[k].mode = "showing";
          clearInterval(iv);
          return;
        }
      }
      if (tries > 25) clearInterval(iv);
    }, 100);
  }
  function renderSaved(saved) {
    var current = subSelect.value;
    subSelect.innerHTML = '<option value="">Tanpa subtitle</option>';
    (saved || []).forEach(function (t) {
      var o = document.createElement("option");
      o.value = t.id;
      o.textContent = t.label + (t.lang ? " [" + t.lang.toUpperCase() + "]" : "") + " · " + t.source;
      subSelect.appendChild(o);
    });
    if (current) subSelect.value = current;
  }
  function loadSubtitles() {
    return fetch(subBase)
      .then(function (r) { return r.json(); })
      .then(function (data) { renderSaved(data.saved); return data; })
      .catch(function () { return null; });
  }
  function activateById(id) {
    loadSubtitles().then(function () {
      subSelect.value = id;
      activateSub(id);
    });
  }
  if (subSelect) {
    loadSubtitles().then(function (data) {
      if (data && data.saved && data.saved.length) {
        subSelect.value = data.saved[0].id;
        activateSub(data.saved[0].id);
      }
    });
    subSelect.addEventListener("change", function () {
      activateSub(subSelect.value);
      subSay(subSelect.value ? "Subtitle aktif. Bila timing meleset, geser dengan tombol −0,5s/+0,5s." : "");
    });
    document.getElementById("subMinus").addEventListener("click", function () {
      subOffset = Math.max(-120, Math.round((subOffset - 0.5) * 10) / 10);
      subFmtOffset();
      if (subActiveId) activateSub(subActiveId);
    });
    document.getElementById("subPlus").addEventListener("click", function () {
      subOffset = Math.min(120, Math.round((subOffset + 0.5) * 10) / 10);
      subFmtOffset();
      if (subActiveId) activateSub(subActiveId);
    });
    subTranslateAI.addEventListener("click", function () {
      if (!subActiveId) return;
      subTranslateAI.disabled = true;
      subSay("Fansub AI sedang menerjemahkan… episode penuh bisa beberapa menit. Jangan tutup halaman ini.");
      fetch("/api/subtitles/track/" + subActiveId + "/translate-llm", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: "{}",
      })
        .then(function (r) { return r.json(); })
        .then(function (resp) {
          subTranslateAI.disabled = false;
          if (resp.error) { subSay("Gagal: " + resp.error); return; }
          subSay("Terjemahan AI Fansub tersimpan & aktif. Selamat menonton!");
          activateById(resp.id);
        })
        .catch(function () {
          subTranslateAI.disabled = false;
          subSay("Gagal menghubungi server untuk fansub AI.");
        });
    });
    subTranslate.addEventListener("click", function () {
      if (!subActiveId) return;
      subTranslate.disabled = true;
      subSay("Menerjemahkan dengan Google GTX… episode penuh bisa 1–2 menit. Jangan tutup halaman ini.");
      fetch("/api/subtitles/track/" + subActiveId + "/translate", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: "{}",
      })
        .then(function (r) { return r.json(); })
        .then(function (resp) {
          subTranslate.disabled = false;
          if (resp.error) { subSay("Gagal: " + resp.error); return; }
          subSay("Terjemahan Indonesia (mesin) tersimpan & aktif. Kualitasnya literal — anggap draf dari fansub kilat.");
          activateById(resp.id);
        })
        .catch(function () {
          subTranslate.disabled = false;
          subSay("Terjemahan gagal — coba lagi sebentar.");
        });
    });
    subDelete.addEventListener("click", function () {
      if (!subActiveId) return;
      fetch("/api/subtitles/track/" + subActiveId, { method: "DELETE" }).then(function () {
        activateSub("");
        subSelect.value = "";
        loadSubtitles();
        subSay("Track subtitle dihapus.");
      });
    });
    document.getElementById("subSearch").addEventListener("click", function () {
      subSay("Mencari subtitle online (OpenSubtitles)… bisa 10–30 detik.");
      subResults.innerHTML = "";
      fetch(subBase + "?q=online")
        .then(function (r) { return r.json(); })
        .then(function (data) {
          var results = data.results || [];
          if (!results.length) {
            subSay("Tidak ditemukan subtitle online yang cocok untuk episode ini. Coba unggah file atau tempel URL.");
            return;
          }
          subSay("Ditemukan " + results.length + " subtitle. Klik untuk memakai:");
          results.forEach(function (c) {
            var b = document.createElement("button");
            b.type = "button";
            b.className = "sub-result";
            b.textContent = "[" + (c.lang || "?").toUpperCase() + "] " + c.label;
            b.addEventListener("click", function () {
              b.disabled = true;
              subSay("Mengunduh subtitle…");
              fetch(subBase + "/fetch", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ source: c.source, id: c.id, label: c.label, lang: c.lang }),
              })
                .then(function (r) { return r.json(); })
                .then(function (resp) {
                  if (resp.error) { subSay("Gagal: " + resp.error); b.disabled = false; return; }
                  subResults.innerHTML = "";
                  subSay("Subtitle tersimpan & aktif.");
                  activateById(resp.id);
                })
                .catch(function () { subSay("Gagal mengunduh subtitle."); b.disabled = false; });
            });
            subResults.appendChild(b);
          });
        })
        .catch(function () { subSay("Pencarian online gagal."); });
    });
    document.getElementById("subFile").addEventListener("change", function (e) {
      var f = e.target.files && e.target.files[0];
      if (!f) return;
      var fd = new FormData();
      fd.append("file", f);
      subSay("Mengunggah " + f.name + "…");
      fetch(subBase + "/upload", { method: "POST", body: fd })
        .then(function (r) { return r.json(); })
        .then(function (resp) {
          if (resp.error) { subSay("Gagal: " + resp.error); return; }
          subSay("Subtitle unggahan tersimpan & aktif.");
          activateById(resp.id);
        })
        .catch(function () { subSay("Unggahan gagal."); });
      e.target.value = "";
    });
    document.getElementById("subUrlBtn").addEventListener("click", function () {
      var input = document.getElementById("subUrl");
      var url = (input.value || "").trim();
      if (!url) return;
      subSay("Mengambil subtitle dari URL…");
      fetch(subBase + "/external", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ url: url }),
      })
        .then(function (r) { return r.json(); })
        .then(function (resp) {
          if (resp.error) { subSay("Gagal: " + resp.error); return; }
          input.value = "";
          subSay("Subtitle dari URL tersimpan & aktif.");
          activateById(resp.id);
        })
        .catch(function () { subSay("Gagal mengambil URL."); });
    });
  }

  // Keyboard: spasi/K putar-jeda, panah seek/volume, F layar penuh,
  // M bisukan, N episode berikutnya.
  document.addEventListener("keydown", function (e) {
    if (e.target && (e.target.tagName === "INPUT" || e.target.tagName === "TEXTAREA" || e.target.tagName === "SELECT")) return;
    if (e.key === "ArrowRight") video.currentTime = Math.min(video.duration || 0, video.currentTime + 10);
    else if (e.key === "ArrowLeft") video.currentTime = Math.max(0, video.currentTime - 10);
    else if (e.key === "ArrowUp") { e.preventDefault(); video.volume = Math.min(1, video.volume + 0.1); }
    else if (e.key === "ArrowDown") { e.preventDefault(); video.volume = Math.max(0, video.volume - 0.1); }
    else if (e.key === " " || e.key === "k") {
      if (e.target === document.body || e.target === video) { e.preventDefault(); togglePlay(); }
    }
    else if (e.key === "f" || e.key === "F") toggleFull();
    else if (e.key === "m" || e.key === "M") video.muted = !video.muted;
    else if ((e.key === "n" || e.key === "N") && next) window.location.href = next;
  });
})();
