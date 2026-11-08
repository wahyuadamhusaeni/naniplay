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

  // Keyboard: space handled by native controls; arrows seek, N = next episode.
  document.addEventListener("keydown", function (e) {
    if (e.target && (e.target.tagName === "INPUT" || e.target.tagName === "TEXTAREA")) return;
    if (e.key === "ArrowRight") video.currentTime = Math.min(video.duration || 0, video.currentTime + 10);
    else if (e.key === "ArrowLeft") video.currentTime = Math.max(0, video.currentTime - 10);
    else if ((e.key === "n" || e.key === "N") && next) window.location.href = next;
  });
})();
