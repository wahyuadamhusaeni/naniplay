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
