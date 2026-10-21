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

test('first-run setup and permissions screen is present and reusable', () => {
  const html = ui('index.html');
  const js = ui('app.js');
  assert.match(html, /id="setup-open"/);
  assert.match(html, /id="setup-dialog"/);
  assert.match(html, /id="setup-checks"/);
  assert.match(html, /id="setup-finish"/);
  assert.match(js, /api\('permissions'\)/);
  assert.match(js, /api\('permissions\/check'/);
  assert.match(js, /api\('setup\/complete'/);
  assert.match(js, /location\.hash==='#setup'/);
  assert.match(fs.readFileSync(path.join(__dirname, '../internal/native/window.js'), 'utf8'), /Setup & Permissions…/);
});

test('settings dialog covers startup, visibility, notifications, updates, maintenance', () => {
  const html = ui('index.html');
  const js = ui('app.js');
  const win = fs.readFileSync(path.join(__dirname, '../internal/native/window.js'), 'utf8');
  assert.match(html, /id="settings-open"/);
  assert.match(html, /id="settings-dialog"/);
  for (const id of ['set-login', 'set-menustart', 'set-dock', 'set-menubar', 'set-notify-repaired', 'set-notify-attention', 'set-autoupdate', 'set-update-check', 'set-refresh-cache', 'set-open-data']) {
    assert.match(html, new RegExp(`id="${id}"`));
  }
  assert.match(js, /api\('settings'/);
  assert.match(js, /api\('update-check'/);
  assert.match(js, /api\('refresh-icon-cache'/);
  assert.match(js, /api\('test-notification'/);
  // Window host applies visibility live and can start windowless.
  assert.match(win, /setActivationPolicy/);
  assert.match(win, /setVisible/);
  assert.match(win, /menu-only/);
  assert.match(win, /pollSettings/);
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

test('default apps tab, organise controls and relocated Add folder are wired', () => {
  const html = ui('index.html');
  const js = ui('app.js');
  const css = ui('style.css');
  assert.match(html, /id="tab-apps"/);
  assert.match(html, /id="tab-defaults"/);
  assert.match(html, /id="apps-view"/);
  assert.match(html, /id="defaults-view"/);
  assert.match(html, /id="defaults-list"/);
  assert.match(html, /id="defaults-refresh"/);
  assert.match(html, /id="sort"/);
  assert.match(html, /id="group"/);
  assert.match(html, /id="folder"/);
  assert.match(html, /Add apps folder/);
  assert.match(js, /api\('defaults'\)/);
  assert.match(js, /api\('defaults\/set'/);
  assert.match(js, /function setTab/);
  assert.match(js, /function loadDefaults/);
  assert.match(js, /function changeDefault/);
  assert.match(js, /function compareApps/);
  assert.match(js, /function groupOf/);
  assert.match(js, /api\('organize'/);
  assert.doesNotMatch(js, /localStorage/);
  assert.match(css, /\.group-head/);
  assert.match(css, /\.default-row/);
  assert.match(css, /\.tab\[aria-selected=true\]/);
});

test('group headers lead every group under any sort order', () => {
  const js = ui('app.js');
  // A single reused header node per group is moved by append() whenever the
  // groups interleave under a name sort, stranding the first run headerless.
  assert.doesNotMatch(js, /groupHeads/);
  assert.match(js, /groupHeadNodes/);
  // Grouping clusters rows by group while keeping the chosen sort inside.
  assert.match(js, /rank\.get\(groupOf\(x\)\)/);
});
