// NaniPlay UI helpers: navbar background on scroll.
(function () {
  var nav = document.getElementById("topnav");
  if (!nav) return;
  function onScroll() {
    if (window.scrollY > 8) nav.classList.add("scrolled");
    else nav.classList.remove("scrolled");
  }
  window.addEventListener("scroll", onScroll, { passive: true });
  onScroll();
})();
