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

test('renamed or full-skinned rows show the original underlying app', () => {
  const js = ui('app.js');
  const css = ui('style.css');
  // A dedicated line under the installed app, fed by the saved original.
  assert.match(js, /orig-line/);
  assert.match(js, /t\('row\.originally',\{name:origName\}\)/);
  assert.match(js, /model\.vault\[a\.bundleId\]/);
  assert.match(js, /saved\?\.iconAssetId/);
  // Only when the worn identity actually differs from the original.
  assert.match(js, /origName!==a\.name&&\(a\.renamed\|\|model\.rebrands\[a\.bundleId\]\)/);
  assert.match(css, /\.orig-line/);
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

test('restores and full-skin changes rescan the inventory instead of trusting the cached session', () => {
  const js = ui('app.js');
  // Names written inside a bundle (full skin / its restore) only change in
  // the inventory on a scan; a plain session() left the old name on screen.
  assert.match(js, /async function rescan\(\)\{try\{await api\('scan',\{\}\);\}catch\(_\)\{\}await session\(\);\}/);
  assert.match(js, /api\('restore-all',\{confirmed:true\}\);[^\n]*await rescan\(\)/);
  assert.match(js, /api\('restore-app',\{appId:a\.id,confirmed:true\}\);[^\n]*await rescan\(\)/);
  // The editor's single Save (rename / rebrand / apply) rescans once after its steps.
  const save = js.slice(js.indexOf('async function saveEdit()'), js.indexOf('async function restoreFromEdit()'));
  assert.match(save, /api\('rebrand'/);
  assert.match(save, /await rescan\(\)/);
});

test('notification and update toggles each carry a visible label', () => {
  const html = ui('index.html');
  const css = ui('style.css');
  for (const id of ['set-notify-repaired', 'set-notify-attention', 'set-autoupdate']) {
    // <label class="switch-row"><span>text</span><span class="switch"><input id=…>
    const re = new RegExp(`<label class="switch-row"><span[^>]*>[^<]+</span><span class="switch"><input type="checkbox" id="${id}">`);
    assert.match(html, re, `${id} has its own labelled row`);
  }
  assert.doesNotMatch(html, /<label class="switch" title=/, 'tooltip-only switches are gone');
  assert.match(css, /\.switch-row\{display:flex/);
});

test('dialog action footers stay in view while long dialogs scroll', () => {
  const html = ui('index.html');
  const css = ui('style.css');
  for (const id of ['rename-cancel', 'settings-cancel', 'setup-later']) {
    assert.match(html, new RegExp(`<div class="dialog-footer dialog-actions[^"]*">(?:(?!</div>)[\\s\\S]){0,160}id="${id}"`), `${id} footer is sticky`);
  }
  assert.match(css, /\.dialog-actions\{position:sticky;bottom:-24px/);
});

test('Edit app is one panel: preview, name, icon choice, full-skin switch, single Save, restore path', () => {
  const html = ui('index.html');
  const js = ui('app.js');
  // Sections and controls.
  for (const id of ['edit-now-icon', 'edit-after-icon', 'edit-after-sub', 'name-input', 'name-original', 'edit-change-icon', 'edit-icon-keep', 'edit-icon-apply', 'edit-fullskin', 'fullskin-state', 'vault-state', 'name-restore', 'name-save', 'rename-cancel']) {
    assert.match(html, new RegExp(`id="${id}"`), id);
  }
  // The two old separate actions are gone.
  assert.doesNotMatch(html, /id="name-internal"|id="name-unskin"/);
  // One Save performs rename → full skin, or rename → Finder icon, in order.
  const save = js.slice(js.indexOf('async function saveEdit()'), js.indexOf('async function restoreFromEdit()'));
  const order = ['api(\'rename\'', 'api(\'rebrand\'', 'api(\'apply\''].map(k => save.indexOf(k));
  assert.ok(order.every(i => i >= 0) && order[0] < order[1] && order[1] < order[2], 'rename before rebrand before apply');
  // The full-skin safety confirm is folded into the Save button (arm, then confirm within 5 s).
  assert.match(save, /if\(s\.fullAction&&!editArmed\)\{/);
  assert.match(save, /editArmed=setTimeout\(\(\)=>\{editArmed=0;drawEdit\(\);\},5000\)/);
  assert.match(js, /save\.textContent=editArmed\?t\('edit\.confirmSkin',\{name:s\.name\|\|a\.name\}\):t\('edit\.save'\)/);
  // Partial failure is reported as such, and the inventory is rescanned either way.
  assert.match(save, /t\('edit\.partial',\{done:done\.join\(SEP\),error:failed\}\)/);
  assert.match(save, /await rescan\(\);busy=false;/);
  // Restore is one click from the editor and uses the vault restore.
  assert.match(js, /async function restoreFromEdit\(\)\{[^\n]*await restoreOne\(a\);/);
  // Choosing an icon from inside the editor selects it and refreshes the preview.
  assert.match(js, /if\(editApp\)\{editIconMode='apply';disarmEdit\(\);drawEdit\(\);\}/);
});
