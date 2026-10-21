'use strict';
/* IconCraft site: live GitHub download/star counts (GitHub counts every
   release-asset download — no tracking of our own) + gentle reveals. */
document.documentElement.classList.add('js');
(function () {
  var reveal = function () {
    var els = document.querySelectorAll('.reveal');
    if (!('IntersectionObserver' in window)) {
      els.forEach(function (e) { e.classList.add('visible'); });
      return;
    }
    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (en) {
        if (en.isIntersecting) { en.target.classList.add('visible'); io.unobserve(en.target); }
      });
    }, { threshold: 0.12 });
    els.forEach(function (e) { io.observe(e); });
  };
  reveal();

  var fmt = function (n) { return n.toLocaleString('en-US'); };
  fetch('https://api.github.com/repos/gshipman223/IconCraft/releases?per_page=100', {
    headers: { 'Accept': 'application/vnd.github+json' }
  })
    .then(function (r) { if (!r.ok) throw new Error(r.status); return r.json(); })
    .then(function (releases) {
      var downloads = 0;
      releases.forEach(function (rel) {
        (rel.assets || []).forEach(function (a) {
          if (/-Installer\.pkg$|\.zip$/.test(a.name) && !/Source/.test(a.name)) downloads += a.download_count || 0;
        });
      });
      if (downloads > 0) {
        var chip = document.getElementById('dl-count');
        if (chip) {
          chip.textContent = fmt(downloads) + ' downloads so far · ';
          chip.hidden = false;
          var fb = document.querySelector('.dl-fallback');
          if (fb) fb.hidden = true;
        }
        var line = document.getElementById('dl-count-line');
        if (line) {
          line.textContent = 'Downloaded ' + fmt(downloads) + ' times from GitHub — and counting.';
          line.hidden = false;
        }
      }
    })
    .catch(function () { /* counts are a nicety; the page stands without them */ });

  fetch('https://api.github.com/repos/gshipman223/IconCraft', {
    headers: { 'Accept': 'application/vnd.github+json' }
  })
    .then(function (r) { if (!r.ok) throw new Error(r.status); return r.json(); })
    .then(function (repo) {
      var el = document.getElementById('star-count');
      if (el && typeof repo.stargazers_count === 'number') {
        el.textContent = '★ ' + fmt(repo.stargazers_count);
        el.hidden = false;
      }
    })
    .catch(function () {});
})();
