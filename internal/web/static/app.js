// NaniPlay UI helpers: navbar background on scroll + mobile drawer.
(function () {
  var nav = document.getElementById("topnav");
  if (nav) {
    function onScroll() {
      if (window.scrollY > 8) nav.classList.add("scrolled");
      else nav.classList.remove("scrolled");
    }
    window.addEventListener("scroll", onScroll, { passive: true });
    onScroll();
  }

  var drawer = document.getElementById("drawer");
  var backdrop = document.getElementById("drawerBackdrop");
  var burger = document.getElementById("navBurger");
  var closeBtn = document.getElementById("drawerClose");
  if (!drawer || !burger) return;
  function open() {
    drawer.classList.add("open");
    drawer.setAttribute("aria-hidden", "false");
    if (backdrop) backdrop.hidden = false;
    document.body.classList.add("no-scroll");
  }
  function close() {
    drawer.classList.remove("open");
    drawer.setAttribute("aria-hidden", "true");
    if (backdrop) backdrop.hidden = true;
    document.body.classList.remove("no-scroll");
  }
  burger.addEventListener("click", open);
  if (closeBtn) closeBtn.addEventListener("click", close);
  if (backdrop) backdrop.addEventListener("click", close);
  document.addEventListener("keydown", function (e) { if (e.key === "Escape") close(); });
  drawer.querySelectorAll("a").forEach(function (a) { a.addEventListener("click", close); });
})();

// Hero campuran: rotasi sorotan tiap 6,5 detik (berhenti saat disentuh/hover).
(function () {
  var slider = document.getElementById("heroSlider");
  if (!slider) return;
  var slides = slider.querySelectorAll(".hero-slide");
  var dots = slider.querySelectorAll(".hero-dot");
  if (slides.length < 2) return;
  var idx = 0, timer = null;
  function show(n) {
    idx = (n + slides.length) % slides.length;
    slides.forEach(function (el, i) { el.classList.toggle("active", i === idx); });
    dots.forEach(function (el, i) { el.classList.toggle("active", i === idx); });
  }
  function start() { timer = setInterval(function () { show(idx + 1); }, 6500); }
  function stop() { if (timer) clearInterval(timer); timer = null; }
  dots.forEach(function (d) {
    d.addEventListener("click", function () { stop(); show(parseInt(d.dataset.goto, 10)); start(); });
  });
  slider.addEventListener("pointerenter", stop);
  slider.addEventListener("pointerleave", start);
  start();
})();

// Rail navigation: chevron buttons scroll the track (desktop hover;
// touch devices keep swiping).
(function () {
  document.querySelectorAll(".rail-wrap").forEach(function (wrap) {
    var track = wrap.querySelector(".rail-track");
    var prev = wrap.querySelector(".rail-prev");
    var next = wrap.querySelector(".rail-next");
    if (!track) return;
    function upd() {
      if (prev) prev.disabled = track.scrollLeft <= 4;
      if (next) next.disabled = track.scrollLeft + track.clientWidth >= track.scrollWidth - 4;
    }
    if (prev) prev.addEventListener("click", function () {
      track.scrollBy({ left: -track.clientWidth * 0.85, behavior: "smooth" });
    });
    if (next) next.addEventListener("click", function () {
      track.scrollBy({ left: track.clientWidth * 0.85, behavior: "smooth" });
    });
    track.addEventListener("scroll", upd, { passive: true });
    window.addEventListener("resize", upd);
    upd();
  });
})();
