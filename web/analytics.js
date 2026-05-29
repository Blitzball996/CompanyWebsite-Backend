/*!
 * Blitzball Labs — privacy-friendly analytics client (no cookies, no 3rd party)
 * Usage: <script defer src="https://YOUR-BACKEND/analytics.js"
 *                 data-endpoint="https://YOUR-BACKEND/api/collect"></script>
 */
(function () {
  "use strict";
  var script = document.currentScript;
  var ENDPOINT =
    (script && script.getAttribute("data-endpoint")) ||
    (location.origin.replace(/:\d+$/, "") + ":8090/api/collect");

  // ---- session id (sessionStorage, resets per tab session) ----
  function sid() {
    try {
      var s = sessionStorage.getItem("bz_sid");
      if (!s) {
        s = (Date.now().toString(36) + Math.random().toString(36).slice(2, 10));
        sessionStorage.setItem("bz_sid", s);
      }
      return s;
    } catch (e) {
      return "nostore";
    }
  }

  var qs = new URLSearchParams(location.search);
  function base() {
    return {
      session_id: sid(),
      page: location.pathname || "/",
      title: document.title,
      referrer: document.referrer || "",
      utm_source: qs.get("utm_source") || "",
      utm_medium: qs.get("utm_medium") || "",
      utm_campaign: qs.get("utm_campaign") || "",
      lang: (document.documentElement.lang || navigator.language || "").slice(0, 5),
      screen: screen.width + "x" + screen.height,
    };
  }

  function send(body, useBeacon) {
    var json = JSON.stringify(body);
    try {
      if (useBeacon && navigator.sendBeacon) {
        navigator.sendBeacon(ENDPOINT, new Blob([json], { type: "application/json" }));
        return;
      }
    } catch (e) {}
    try {
      fetch(ENDPOINT, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: json,
        keepalive: true,
        mode: "cors",
      }).catch(function () {});
    } catch (e) {}
  }

  // ---- pageview ----
  var start = Date.now();
  var maxScroll = 0;
  var pv = base();
  pv.type = "pageview";
  send(pv, false);

  // ---- scroll depth ----
  window.addEventListener(
    "scroll",
    function () {
      var h = document.documentElement;
      var denom = h.scrollHeight - h.clientHeight;
      if (denom > 0) {
        var pct = Math.round((h.scrollTop / denom) * 100);
        if (pct > maxScroll) maxScroll = Math.min(100, pct);
      }
    },
    { passive: true }
  );

  // ---- public event API: window.bzTrack('name', {extra}) ----
  window.bzTrack = function (name, meta) {
    var e = base();
    e.type = "event";
    e.name = name;
    e.meta = meta || {};
    send(e, false);
  };

  // ---- auto-track outbound / download / github clicks ----
  document.addEventListener(
    "click",
    function (ev) {
      var a = ev.target.closest && ev.target.closest("a");
      if (!a || !a.href) return;
      var href = a.href;
      var name = null;
      if (/github\.com/i.test(href)) name = "github_click";
      else if (/\.(zip|exe|dmg|pkg|appimage|tar\.gz)(\?|$)/i.test(href)) name = "download_click";
      else if (a.host && a.host !== location.host) name = "outbound_click";
      if (name) window.bzTrack(name, { href: href });
    },
    true
  );

  // ---- track language switcher (your site uses a lang switcher) ----
  document.addEventListener(
    "click",
    function (ev) {
      var el = ev.target.closest && ev.target.closest("[data-lang],[data-i18n-lang],.lang-switch a,.lang-option");
      if (el) {
        var lng = el.getAttribute("data-lang") || el.getAttribute("data-i18n-lang") || el.textContent.trim();
        window.bzTrack("lang_switch", { to: lng });
      }
    },
    true
  );

  // ---- send duration + scroll on leave ----
  function flush() {
    var e = base();
    e.type = "pageview";
    e.name = "engagement";
    e.duration_ms = Date.now() - start;
    e.scroll_pct = maxScroll;
    send(e, true);
  }
  document.addEventListener("visibilitychange", function () {
    if (document.visibilityState === "hidden") flush();
  });
  window.addEventListener("pagehide", flush);
})();
