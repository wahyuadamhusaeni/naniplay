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

  function attach() {
    if (video.canPlayType("application/vnd.apple.mpegurl")) {
      video.src = src;
    } else if (window.Hls && Hls.isSupported()) {
      var hls = new Hls({ maxBufferLength: 30 });
      hls.loadSource(src);
      hls.attachMedia(video);
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

  // --- Subtitle: saved tracks, online search, upload, desync offset ---
  var subSelect = document.getElementById("subSelect");
  var subStatus = document.getElementById("subStatus");
  var subResults = document.getElementById("subResults");
  var subOffsetVal = document.getElementById("subOffsetVal");
  var subDelete = document.getElementById("subDelete");
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

  // Keyboard: space handled by native controls; arrows seek, N = next episode.
  document.addEventListener("keydown", function (e) {
    if (e.target && (e.target.tagName === "INPUT" || e.target.tagName === "TEXTAREA")) return;
    if (e.key === "ArrowRight") video.currentTime = Math.min(video.duration || 0, video.currentTime + 10);
    else if (e.key === "ArrowLeft") video.currentTime = Math.max(0, video.currentTime - 10);
    else if ((e.key === "n" || e.key === "N") && next) window.location.href = next;
  });
})();
