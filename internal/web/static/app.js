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
