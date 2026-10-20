'use strict';
// Regression tests for the 0.5 UI: every installed app must be listed (not
// only catalogue-matched ones), app icons must actually load in WebKit, and
// the branding/support furniture must be present.
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const ui = p => fs.readFileSync(path.join(__dirname, '../internal/server/ui', p), 'utf8');

test('all installed apps are listed, not only catalogue matches', () => {
  const js = ui('app.js');
  assert.match(js, /function relevant\(\)\{return \[\.\.\.model\.apps\]/);
  assert.doesNotMatch(js, /filter\(a=>a\.craftId\|\|active\(a\)\)/);
});

test('icons are not lazy-loaded while hidden (WebKit would never fetch them)', () => {
  const js = ui('app.js');
  assert.doesNotMatch(js, /loading\s*=\s*'lazy'|loading="lazy"/);
  assert.match(ui('style.css'), /\.icon-wrap:not\(\.has-image\) \.icon\{display:none\}/);
});

test('real logo, version and PayPal support bar are in the page', () => {
  const html = ui('index.html');
  assert.match(html, /<img class="brand" src="logo\.svg"/);
  assert.match(html, /id="support-paypal"/);
  assert.match(html, /Contribute via PayPal/);
  assert.match(html, /IconCraft is free &amp; open source/);
  assert.match(html, /id="version"/);
  assert.match(fs.readFileSync(path.join(__dirname, '../internal/server/ui/logo.svg'), 'utf8'), /<svg/);
});
