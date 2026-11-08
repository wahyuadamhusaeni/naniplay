// NaniPlay player: attaches HLS (native or hls.js), resumes the saved
// position, and reports progress to the server so "Lanjutkan Menonton"
// stays current across devices.
(function () {
  var video = document.getElementById("player");
  if (!video) return;
  var src = video.dataset.src;
  var meta = {
    anilistId: parseInt(video.dataset.anilist, 10),
    key: video.dataset.key || "",
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
      hls.on(Hls.Events.MANIFEST_PARSED, applyQualityPref);
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
    if (meta.anilistId > 0) fetch("/api/skiptimes/" + meta.anilistId + "/" + meta.episode)
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
      skipBtn.textContent = skipLabel(hit.type);
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
        key: meta.key,
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

  // --- Control bar ala Netflix: ikon SVG, seek bar penuh, +/-10 dtk,
  // kecepatan putar, indikator buffering ---
  var box = document.getElementById("playerBox");
  var pPlay = document.getElementById("pPlay");
  var pPlayIco = document.getElementById("pPlayIco");
  var pBigPlay = document.getElementById("pBigPlay");
  var pSeek = document.getElementById("pSeek");
  var pPlayed = document.getElementById("pPlayed");
  var pBuffered = document.getElementById("pBuffered");
  var pKnob = document.getElementById("pKnob");
  var pSeekTip = document.getElementById("pSeekTip");
  var pCur = document.getElementById("pCur");
  var pDur = document.getElementById("pDur");
  var pBack10 = document.getElementById("pBack10");
  var pFwd10 = document.getElementById("pFwd10");
  var pMute = document.getElementById("pMute");
  var pVolIco = document.getElementById("pVolIco");
  var pVol = document.getElementById("pVol");
  var pCC = document.getElementById("pCC");
  var pPiP = document.getElementById("pPiP");
  var pFull = document.getElementById("pFull");
  var pFullIco = document.getElementById("pFullIco");
  var pSpinner = document.getElementById("pSpinner");
  var pFlash = document.getElementById("pFlash");
  var pFlashIco = document.getElementById("pFlashIco");
  var pSpeedBtn = document.getElementById("pSpeedBtn");
  var pSpeedMenu = document.getElementById("pSpeedMenu");
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
  function flash(icon) {
    if (!pFlash || !pFlashIco) return;
    pFlashIco.setAttribute("href", icon);
    pFlash.classList.remove("go");
    void pFlash.offsetWidth;
    pFlash.classList.add("go");
  }
  function togglePlay() {
    if (video.paused) { video.play(); flash("#i-play"); }
    else { video.pause(); flash("#i-pause"); }
  }
  function syncPlayUI() {
    var playing = !video.paused && !video.ended;
    if (pPlayIco) pPlayIco.setAttribute("href", playing ? "#i-pause" : "#i-play");
    if (box) {
      box.classList.toggle("paused", !playing);
      box.classList.toggle("playing", playing);
    }
  }
  function pokeUI() {
    if (!box) return;
    box.classList.add("ui-on");
    clearTimeout(uiTimer);
    uiTimer = setTimeout(function () {
      // Saat pause, kontrol tetap tampil — jangan sembunyikan.
      if (!video.paused) box.classList.remove("ui-on");
    }, 6000);
  }
  function paintSeek() {
    if (!video.duration) return;
    var played = (video.currentTime / video.duration) * 100;
    var buffered = 0;
    try {
      if (video.buffered.length) buffered = (video.buffered.end(video.buffered.length - 1) / video.duration) * 100;
    } catch (e) { /* abaikan */ }
    if (pPlayed) pPlayed.style.width = played + "%";
    if (pBuffered) pBuffered.style.width = buffered + "%";
    if (pKnob) pKnob.style.left = played + "%";
    if (pSeek) pSeek.setAttribute("aria-valuenow", String(Math.round(played)));
  }
  function toggleFull() {
    if (document.fullscreenElement) { document.exitFullscreen(); return; }
    function goVideo() {
      if (video.requestFullscreen) video.requestFullscreen().catch(function () {});
      else if (video.webkitEnterFullscreen) video.webkitEnterFullscreen();
    }
    if (box && box.requestFullscreen) box.requestFullscreen().catch(goVideo);
    else goVideo();
  }
  function syncVolIcon() {
    if (!pVolIco) return;
    var name = video.muted || video.volume === 0 ? "#i-vol-off" : video.volume < 0.5 ? "#i-vol-low" : "#i-vol-high";
    pVolIco.setAttribute("href", name);
  }
  function seekFraction(e) {
    var rect = pSeek.getBoundingClientRect();
    return Math.min(1, Math.max(0, (e.clientX - rect.left) / rect.width));
  }
  function moveTip(e) {
    if (!pSeekTip || !video.duration) return;
    var f = seekFraction(e);
    pSeekTip.textContent = fmtT(f * video.duration);
    pSeekTip.style.left = (f * 100) + "%";
  }
  if (box && pPlay) {
  // --- Putar otomatis episode berikutnya (gaya Netflix) ---
  var nextAnchor = box.querySelector(".np-next");
  if (nextAnchor) {
    var upNext = document.createElement("div");
    upNext.className = "np-upnext";
    upNext.hidden = true;
    upNext.innerHTML = '<p class="np-upnext-label">Episode Berikutnya</p>' +
      '<div class="np-upnext-row"><a class="np-upnext-play" href="' + nextAnchor.getAttribute("href") + '">▶ Putar Sekarang</a>' +
      '<span class="np-upnext-count"></span>' +
      '<button type="button" class="np-upnext-x" aria-label="Nanti saja">✕</button></div>';
    box.appendChild(upNext);
    var upDismissed = false;
    var upCount = upNext.querySelector(".np-upnext-count");
    upNext.querySelector(".np-upnext-x").addEventListener("click", function () {
      upDismissed = true;
      upNext.hidden = true;
    });
    video.addEventListener("timeupdate", function () {
      if (upDismissed || !video.duration) return;
      // Jangan tutupi tombol Lewati OP/ED yang sedang tampil.
      upNext.style.bottom = (skipBtn && !skipBtn.hidden && box.clientHeight >= 320) ? "176px" : "118px";
      var left = video.duration - video.currentTime;
      if (left <= 20 && left > 0) {
        upNext.hidden = false;
        upCount.textContent = "mulai dalam " + Math.ceil(left) + " dtk";
      } else if (left > 25) {
        upNext.hidden = true;
      }
    });
    video.addEventListener("ended", function () {
      if (!upDismissed) window.location.href = nextAnchor.getAttribute("href");
    });
  }

    video.addEventListener("play", function () {
      box.classList.add("started");
      syncPlayUI(); pokeUI();
    });
    video.addEventListener("pause", function () { syncPlayUI(); pokeUI(); });
    video.addEventListener("ended", function () { syncPlayUI(); pokeUI(); });
    // Klik/ketuk di area video TIDAK play/pause (itu tugas tombol
    // play): sekali klik hanya tampil/sembunyi kontrol, dua kali
    // klik = layar penuh. Timer membedakan klik tunggal vs ganda.
    var stageClickTimer = null;
    // Keadaan kontrol dicatat saat jari/tikus MENYENTUH (pointerdown),
    // sebelum event mouse sintetis dari ketukan (mousemove kompatibilitas)
    // sempat menyalakan UI lewat pokeUI. Tanpa ini, ketukan untuk
    // MENAMPILKAN kontrol justru membacanya sebagai "sudah tampil" lalu
    // mematikannya 280ms kemudian — kontrol terasa hilang sekejap.
    var stageWasOn = null, stageDownAt = 0;
    box.addEventListener("pointerdown", function (e) {
      if (e.target !== video && e.target !== box) return;
      stageWasOn = box.classList.contains("ui-on");
      stageDownAt = Date.now();
    });
    box.addEventListener("click", function (e) {
      if (e.target !== video && e.target !== box) return;
      if (stageClickTimer) {
        clearTimeout(stageClickTimer);
        stageClickTimer = null;
        toggleFull();
        return;
      }
      stageClickTimer = setTimeout(function () {
        stageClickTimer = null;
        var wasOn = (stageWasOn !== null && Date.now() - stageDownAt < 1500)
          ? stageWasOn : box.classList.contains("ui-on");
        if (wasOn) {
          box.classList.remove("ui-on");
          clearTimeout(uiTimer);
        } else pokeUI();
      }, 280);
    });
    video.addEventListener("waiting", function () { if (pSpinner) pSpinner.hidden = false; });
    video.addEventListener("playing", function () { if (pSpinner) pSpinner.hidden = true; });
    video.addEventListener("canplay", function () { if (pSpinner) pSpinner.hidden = true; });
    document.addEventListener("fullscreenchange", function () {
      if (pFullIco) pFullIco.setAttribute("href", document.fullscreenElement ? "#i-full-exit" : "#i-full");
    });
    box.addEventListener("mousemove", pokeUI);
    pPlay.addEventListener("click", togglePlay);
    if (pBigPlay) pBigPlay.addEventListener("click", togglePlay);
    if (pBack10) pBack10.addEventListener("click", function () {
      video.currentTime = Math.max(0, video.currentTime - 10);
      pokeUI();
    });
    if (pFwd10) pFwd10.addEventListener("click", function () {
      if (video.duration) video.currentTime = Math.min(video.duration, video.currentTime + 10);
      pokeUI();
    });
    if (pSeek) {
      pSeek.addEventListener("pointerdown", function (e) {
        seeking = true;
        pSeek.classList.add("scrubbing");
        if (pSeek.setPointerCapture) { try { pSeek.setPointerCapture(e.pointerId); } catch (err) {} }
        if (video.duration) video.currentTime = seekFraction(e) * video.duration;
        moveTip(e); paintSeek(); pokeUI();
      });
      pSeek.addEventListener("pointermove", function (e) {
        moveTip(e);
        if (seeking && video.duration) {
          video.currentTime = seekFraction(e) * video.duration;
          paintSeek();
          if (pCur) pCur.textContent = fmtT(video.currentTime);
        }
      });
      var endSeek = function () {
        seeking = false;
        pSeek.classList.remove("scrubbing");
        paintSeek();
      };
      pSeek.addEventListener("pointerup", endSeek);
      pSeek.addEventListener("pointercancel", endSeek);
    }
    video.addEventListener("timeupdate", function () {
      if (!seeking) paintSeek();
      if (pCur) pCur.textContent = fmtT(video.currentTime);
    });
    video.addEventListener("progress", paintSeek);
    video.addEventListener("durationchange", function () { if (pDur) pDur.textContent = fmtT(video.duration); });
    if (pVol) pVol.addEventListener("input", function () {
      video.volume = pVol.value / 100;
      video.muted = false;
    });
    if (pMute) pMute.addEventListener("click", function () { video.muted = !video.muted; });
    video.addEventListener("volumechange", function () {
      if (pVol) pVol.value = String(Math.round(video.volume * 100));
      syncVolIcon();
    });
    syncVolIcon();
    if (pCC) pCC.addEventListener("click", function () {
      var panel = document.getElementById("subPanel");
      if (!panel) return;
      panel.scrollIntoView({ behavior: "smooth", block: "nearest" });
      panel.classList.add("flash");
      setTimeout(function () { panel.classList.remove("flash"); }, 1200);
      var first = document.querySelector("#subTrackList .sub-track");
      if (first) first.focus({ preventScroll: true });
    });
    if (pPiP) pPiP.addEventListener("click", function () {
      if (document.pictureInPictureElement) document.exitPictureInPicture();
      else if (video.requestPictureInPicture) video.requestPictureInPicture().catch(function () {});
    });
    if (pFull) pFull.addEventListener("click", toggleFull);
    syncPlayUI();
    pokeUI();
    if (pSpinner) pSpinner.hidden = false;
  }

  // --- Kecepatan putar ---
  if (pSpeedBtn && pSpeedMenu) {
    [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2].forEach(function (rate) {
      var b = document.createElement("button");
      b.type = "button";
      b.textContent = (rate === 1 ? "1" : String(rate).replace(".", ",")) + "×";
      if (rate === 1) b.classList.add("active");
      b.addEventListener("click", function () {
        video.playbackRate = rate;
        pSpeedBtn.textContent = (rate === 1 ? "1" : String(rate)) + "×";
        pSpeedMenu.querySelectorAll("button").forEach(function (x) { x.classList.remove("active"); });
        b.classList.add("active");
        pSpeedMenu.hidden = true;
      });
      pSpeedMenu.appendChild(b);
    });
    pSpeedBtn.addEventListener("click", function () { pSpeedMenu.hidden = !pSpeedMenu.hidden; });
    document.addEventListener("click", function (e) {
      if (!pSpeedMenu.hidden && !e.target.closest(".np-speedwrap")) pSpeedMenu.hidden = true;
    });
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

  // Kualitas bawaan dari Pengaturan: pilih level tertinggi yang
  // tidak melampaui preferensi (mis. 720 -> 720p atau di bawahnya).
  var qualityApplied = false;
  function applyQualityPref() {
    if (qualityApplied || !hls || !hls.levels || !hls.levels.length) return;
    var want = parseInt(video.dataset.quality || "", 10);
    if (!want) return;
    qualityApplied = true;
    var best = -1, bestH = -1, low = -1, lowH = Infinity;
    for (var i = 0; i < hls.levels.length; i++) {
      var h = hls.levels[i].height || 0;
      if (h <= want && h > bestH) { bestH = h; best = i; }
      if (h > 0 && h < lowH) { lowH = h; low = i; }
    }
    var pick = best >= 0 ? best : low;
    if (pick >= 0) hls.nextLevel = pick;
  }

  // --- Subtitle: daftar trek sebagai baris yang bisa diklik, zona
  // ambil subtitle, dan alat lanjutan (gabung/sinkron) terlipat ---
  var subTrackList = document.getElementById("subTrackList");
  var subActiveNote = document.getElementById("subActiveNote");
  var subStatus = document.getElementById("subStatus");
  var subResults = document.getElementById("subResults");
  var subOffsetVal = document.getElementById("subOffsetVal");
  var subDelete = document.getElementById("subDelete");
  var subTranslate = document.getElementById("subTranslate");
  var subTranslateAI = document.getElementById("subTranslateAI");
  var subBase = video.dataset.subbase || ("/api/subtitles/" + meta.anilistId + "/" + meta.episode);
  var subPref = video.dataset.subpref || "id";
  var subManual = false;
  var subOffset = 0;
  var subActiveId = "";

  function subBucket(lang, label) {
    var t = ((lang || "") + " " + (label || "")).toLowerCase();
    if (t.indexOf("indones") >= 0 || t.indexOf("[id]") >= 0 || t.indexOf("indonesia") >= 0) return "id";
    if (t.indexOf("english") >= 0 || t.indexOf("[en]") >= 0) return "en";
    return "";
  }
  function subSay(msg) { if (subStatus) subStatus.textContent = msg || ""; }
  function subFmtOffset() {
    var v = subOffset.toFixed(1).replace(".", ",") + "s";
    if (subOffset > 0) v = "+" + v;
    if (subOffsetVal) subOffsetVal.textContent = v;
  }
  function markActiveTrack() {
    if (!subTrackList) return;
    var name = "Tanpa subtitle";
    subTrackList.querySelectorAll(".sub-track").forEach(function (row) {
      var on = (row.dataset.id || "") === subActiveId;
      row.classList.toggle("active", on);
      if (on && row.dataset.name) name = row.dataset.name;
    });
    if (subActiveNote) subActiveNote.textContent = name;
  }
  function applySubPref() {
    // Sumber hardsub (Otakudesu) sudah ber-subtitle Indonesia di dalam
    // videonya — jangan aktifkan softsub otomatis, nanti teksnya ganda.
    if (video.dataset.subauto === "off") return;
    if (subManual || subActiveId || subPref === "off" || !subTrackList) return;
    var rows = subTrackList.querySelectorAll(".sub-track");
    for (var i = 0; i < rows.length; i++) {
      if (rows[i].dataset.id && rows[i].dataset.bucket === subPref) {
        activateSub(rows[i].dataset.id);
        return;
      }
    }
  }
  function activateSub(id) {
    subActiveId = id || "";
    var old = video.querySelectorAll("track");
    for (var i = 0; i < old.length; i++) old[i].remove();
    for (var j = 0; j < video.textTracks.length; j++) video.textTracks[j].mode = "hidden";
    if (subDelete) subDelete.hidden = !subActiveId;
    if (subTranslate) subTranslate.hidden = !subActiveId;
    if (subTranslateAI) subTranslateAI.hidden = !subActiveId;
    markActiveTrack();
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
  function trackRow(id, name, lang, source, bucket) {
    var b = document.createElement("button");
    b.type = "button";
    b.className = "sub-track";
    b.dataset.id = id || "";
    b.dataset.name = name || "";
    b.dataset.bucket = bucket || "";
    if (lang) {
      var chip = document.createElement("span");
      chip.className = "sub-lang";
      chip.textContent = lang.toUpperCase();
      b.appendChild(chip);
    }
    var label = document.createElement("span");
    label.className = "sub-track-name";
    label.textContent = name;
    b.appendChild(label);
    if (source) {
      var src = document.createElement("span");
      src.className = "sub-src";
      src.textContent = source;
      b.appendChild(src);
    }
    var check = document.createElement("span");
    check.className = "sub-check";
    check.textContent = "✓";
    b.appendChild(check);
    return b;
  }
  function renderSaved(saved) {
    if (subTrackList) {
      subTrackList.innerHTML = "";
      subTrackList.appendChild(trackRow("", "Tanpa subtitle", "", "", "off"));
      (saved || []).forEach(function (t) {
        subTrackList.appendChild(trackRow(t.id, t.label, t.lang, t.source, subBucket(t.lang, t.label)));
      });
    }
    var mergeSel = document.getElementById("subMergeSel");
    var mergeCur = mergeSel ? mergeSel.value : "";
    if (mergeSel) {
      mergeSel.innerHTML = '<option value="">Trek kedua / referensi…</option>';
      (saved || []).forEach(function (t) {
        var o2 = document.createElement("option");
        o2.value = t.id;
        o2.textContent = t.label + (t.lang ? " [" + t.lang.toUpperCase() + "]" : "") + " · " + t.source;
        mergeSel.appendChild(o2);
      });
      if (mergeCur) mergeSel.value = mergeCur;
    }
    markActiveTrack();
  }
  function loadSubtitles() {
    return fetch(subBase)
      .then(function (r) { return r.json(); })
      .then(function (data) { renderSaved(data.saved); applySubPref(); return data; })
      .catch(function () { return null; });
  }
  function activateById(id) {
    loadSubtitles().then(function () { activateSub(id); });
  }
  if (subTrackList) {
    loadSubtitles().then(function (data) {
      if (video.dataset.subauto === "off") return;
      if (!subActiveId && data && data.saved && data.saved.length) {
        activateSub(data.saved[0].id);
      }
    });
    subTrackList.addEventListener("click", function (e) {
      var row = e.target.closest(".sub-track");
      if (!row) return;
      subManual = true;
      var bucket = row.dataset.id ? (row.dataset.bucket || "") : "off";
      if (bucket) {
        fetch("/api/prefs/sub-lang", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ lang: bucket })
        }).catch(function () {});
      }
      activateSub(row.dataset.id);
      subSay(row.dataset.id ? "Subtitle aktif: " + (row.dataset.name || "") + ". Bila timing meleset, buka bagian Kelola subtitle di bawah." : "Subtitle dimatikan.");
    });
    var subMinus = document.getElementById("subMinus");
    var subPlus = document.getElementById("subPlus");
    if (subMinus) subMinus.addEventListener("click", function () {
      subOffset = Math.max(-120, Math.round((subOffset - 0.5) * 10) / 10);
      subFmtOffset();
      if (subActiveId) activateSub(subActiveId);
    });
    if (subPlus) subPlus.addEventListener("click", function () {
      subOffset = Math.min(120, Math.round((subOffset + 0.5) * 10) / 10);
      subFmtOffset();
      if (subActiveId) activateSub(subActiveId);
    });
    var subMergeBtn = document.getElementById("subMergeBtn");
    var subSyncBtn = document.getElementById("subSyncBtn");
    var subMergeSel = document.getElementById("subMergeSel");
    if (subMergeBtn) subMergeBtn.addEventListener("click", function () {
      var a = subActiveId, b = subMergeSel ? subMergeSel.value : "";
      if (!a || !b || a === b) { subSay("Aktifkan dulu trek utama, lalu pilih trek kedua yang berbeda."); return; }
      subSay("Menggabungkan trek…");
      fetch("/api/subtitles/track/" + a + "/merge", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ otherId: b })
      }).then(function (r) { return r.json(); }).then(function (d) {
        if (d.id) { subSay("Trek gabungan dibuat dan diaktifkan."); activateById(d.id); }
        else subSay(d.error || "Gabung gagal.");
      }).catch(function () { subSay("Gabung gagal: jaringan."); });
    });
    if (subSyncBtn) subSyncBtn.addEventListener("click", function () {
      var a = subActiveId, b = subMergeSel ? subMergeSel.value : "";
      if (!a || !b || a === b) { subSay("Aktifkan dulu trek utama, lalu pilih trek referensi yang berbeda."); return; }
      subSay("Menghitung selisih timing…");
      fetch("/api/subtitles/track/" + a + "/autosync", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ referenceId: b })
      }).then(function (r) { return r.json(); }).then(function (d) {
        if (d.applied) { subSay("Offset " + d.offset.toFixed(1) + " detik diterapkan — trek hasil sinkron diaktifkan."); activateById(d.id); }
        else if (typeof d.offset === "number") subSay("Trek tampaknya sudah sinkron (selisih " + d.offset.toFixed(2) + " dtk).");
        else subSay(d.error || "Sinkron gagal.");
      }).catch(function () { subSay("Sinkron gagal: jaringan."); });
    });
    if (subTranslateAI) subTranslateAI.addEventListener("click", function () {
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
    if (subTranslate) subTranslate.addEventListener("click", function () {
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
    if (subDelete) subDelete.addEventListener("click", function () {
      if (!subActiveId) return;
      fetch("/api/subtitles/track/" + subActiveId, { method: "DELETE" }).then(function () {
        activateSub("");
        loadSubtitles();
        subSay("Track subtitle dihapus.");
      });
    });
    var subSearch = document.getElementById("subSearch");
    if (subSearch) subSearch.addEventListener("click", function () {
      subSay("Mencari subtitle online (" + (video.dataset.subsource || "OpenSubtitles") + ")… bisa 10–30 detik.");
      subResults.innerHTML = "";
      fetch(subBase + "?q=online")
        .then(function (r) { return r.json(); })
        .then(function (data) {
          var results = data.results || [];
          if (data.note) { subSay(data.note); return; }
          if (!results.length) {
            subSay("Tidak ditemukan subtitle online yang cocok untuk episode ini. Coba unggah file atau tempel URL.");
            return;
          }
          subSay("Ditemukan " + results.length + " subtitle. Klik untuk memakai:");
          results.forEach(function (c) {
            var b = document.createElement("button");
            b.type = "button";
            b.className = "sub-result";
            var chip = document.createElement("span");
            chip.className = "sub-lang";
            chip.textContent = (c.lang || "?").toUpperCase();
            b.appendChild(chip);
            var label = document.createElement("span");
            label.className = "sub-track-name";
            label.textContent = c.label;
            b.appendChild(label);
            if (c.downloads) {
              var dl = document.createElement("span");
              dl.className = "sub-src";
              dl.textContent = "⬇ " + c.downloads;
              b.appendChild(dl);
            }
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
    var subFile = document.getElementById("subFile");
    if (subFile) subFile.addEventListener("change", function (e) {
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
    var subUrlBtn = document.getElementById("subUrlBtn");
    if (subUrlBtn) subUrlBtn.addEventListener("click", function () {
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
