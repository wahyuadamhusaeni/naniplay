// NaniPlay episode picker: search filter + 100-episode ranges, so
// long-running series stay navigable. Works on any .ep-picker block.
(function () {
  document.querySelectorAll(".ep-picker").forEach(function (root) {
    var btns = Array.prototype.slice.call(root.querySelectorAll(".ep-btn"));
    if (!btns.length) return;
    var search = root.querySelector(".ep-search");
    var range = root.querySelector(".ep-range");
    var count = root.querySelector(".ep-count");
    var none = root.querySelector(".ep-none");
    var CHUNK = 100;
    var chunks = [];
    if (btns.length > 60) {
      var min = parseInt(btns[0].dataset.num, 10);
      var max = parseInt(btns[btns.length - 1].dataset.num, 10);
      for (var start = Math.floor((min - 1) / CHUNK) * CHUNK + 1; start <= max; start += CHUNK) {
        chunks.push([start, Math.min(start + CHUNK - 1, max)]);
      }
      chunks.forEach(function (c, i) {
        var o = document.createElement("option");
        o.value = String(i);
        o.textContent = "Episode " + c[0] + "\u2013" + c[1];
        range.appendChild(o);
      });
      range.hidden = false;
      // Open on the chunk containing the active episode.
      var active = root.querySelector(".ep-btn.active");
      if (active) {
        var n = parseInt(active.dataset.num, 10);
        for (var i = 0; i < chunks.length; i++) {
          if (n >= chunks[i][0] && n <= chunks[i][1]) { range.value = String(i); break; }
        }
      }
    }
    function apply() {
      var q = (search.value || "").trim();
      var visible = 0;
      btns.forEach(function (b) {
        var num = parseInt(b.dataset.num, 10);
        var show = true;
        if (q) {
          show = String(num).indexOf(q) >= 0;
        } else if (chunks.length) {
          var c = chunks[parseInt(range.value || "0", 10)];
          show = num >= c[0] && num <= c[1];
        }
        b.style.display = show ? "" : "none";
        if (show) visible++;
      });
      if (count) count.textContent = "Menampilkan " + visible + " dari " + btns.length + " episode";
      if (none) none.hidden = visible !== 0;
    }
    search.addEventListener("input", apply);
    range.addEventListener("change", apply);
    apply();
  });
})();
