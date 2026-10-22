'use strict';
// Localisation contract tests: the page keeps English as its authored
// default, every translatable element carries a catalogue key, the four
// catalogues cover the same keys, and the runtime helpers behave as the
// Go side expects. Pure Node — no WebKit or macOS execution is claimed.
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const root = path.join(__dirname, '..');
const read = p => fs.readFileSync(path.join(root, p), 'utf8');
const catalogue = l => JSON.parse(read(`internal/i18n/locales/${l}.json`));
const LOCALES = ['en', 'zh-Hans', 'ja', 'ko'];
const en = catalogue('en');
const decode = s => s.replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"');

test('index.html English text matches en.json for every data-i18n key', () => {
  const html = read('internal/server/ui/index.html');
  const re = /<([a-z0-9]+)\b[^>]*\bdata-i18n="([a-zA-Z.]+)"[^>]*>([^<]*)</g;
  let m, count = 0;
  while ((m = re.exec(html))) {
    count++;
    const [, , key, text] = m;
    assert.ok(key in en.ui, `missing en.json key ${key}`);
    assert.equal(decode(text.trim()), en.ui[key], `HTML default text for ${key} drifted from en.json`);
  }
  assert.equal(count, (html.match(/\bdata-i18n="/g) || []).length, 'every data-i18n element was checked');
  assert.ok(count > 60, `expected many keyed elements, found ${count}`);
  // Attribute translations point at real keys too.
  for (const attr of ['title', 'aria', 'placeholder', 'alt', 'rich']) {
    const are = new RegExp(`data-i18n-${attr}="([a-zA-Z.]+)"`, 'g');
    while ((m = are.exec(html))) assert.ok(m[1] in en.ui, `missing en.json key ${m[1]} (${attr})`);
  }
  // Rich (emphasis) paragraphs: the HTML's <strong> runs equal the **…** runs.
  const rich = /data-i18n-rich="([a-zA-Z.]+)"[^>]*>([\s\S]*?)<\/p>/g;
  while ((m = rich.exec(html))) {
    const expected = en.ui[m[1]].replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
    assert.equal(decode(m[2].trim()), expected, `rich text for ${m[1]} drifted`);
  }
  // strings.js loads before app.js so the first render is already localised.
  assert.ok(html.indexOf('<script src="strings.js">') < html.indexOf('<script src="app.js">'));
  assert.match(html, /<html lang="en"/);
});

test('every locale carries every key, with identical placeholders', () => {
  const ph = s => (s.match(/\{[a-zA-Z]+\}/g) || []).sort().join(',');
  for (const l of LOCALES.slice(1)) {
    const c = catalogue(l);
    for (const k of Object.keys(en.ui)) {
      assert.ok(typeof c.ui[k] === 'string' && c.ui[k].trim(), `${l}: ui.${k} missing`);
      assert.equal(ph(c.ui[k]), ph(en.ui[k]), `${l}: ui.${k} placeholders`);
      assert.equal((c.ui[k].match(/\*\*/g) || []).length % 2, 0, `${l}: ui.${k} unbalanced **`);
      assert.ok(!/\bIconCraft\b/.test(en.ui[k]) || c.ui[k].includes('IconCraft'), `${l}: ui.${k} must keep the product name`);
    }
    for (const k of Object.keys(en.messages)) assert.ok(typeof c.messages[k] === 'string' && c.messages[k].trim(), `${l}: messages[${JSON.stringify(k)}] missing`);
    assert.deepEqual(Object.keys(c.ui).sort(), Object.keys(en.ui).sort(), `${l}: extra ui keys`);
    assert.deepEqual(Object.keys(c.messages).sort(), Object.keys(en.messages).sort(), `${l}: extra message keys`);
  }
});

// Run the top of app.js (helpers only) against a tiny DOM stand-in.
function helpers(strings, locale) {
  const src = read('internal/server/ui/app.js');
  const end = src.indexOf('const model=');
  assert.ok(end > 0);
  const nodes = [];
  const el = (dataset, attrs, text) => {
    const e = { dataset, attrs, textContent: text, children: [], lang: '',
      getAttribute: a => attrs[a], setAttribute(a, v) { attrs[a] = v; },
      replaceChildren() { this.children = []; }, append(...k) { this.children.push(...k); } };
    nodes.push(e); return e;
  };
  const doc = {
    documentElement: { lang: 'en' },
    createElement: tag => ({ tag, className: '', textContent: '' }),
    createTextNode: t => ({ text: t }),
    querySelectorAll: sel => nodes.filter(n => {
      const m = sel.match(/^\[data-i18n(?:-([a-z]+))?\]$/);
      if (!m) return false;
      if (!m[1]) return 'i18n' in n.dataset;
      return ('data-i18n-' + m[1]) in n.attrs;
    }),
    getElementById: () => null,
  };
  const ctx = vm.createContext({ window: { ICONCRAFT_I18N: { locale, strings } }, document: doc, Date, Number, String, Object, JSON });
  vm.runInContext(src.slice(0, end), ctx, { filename: 'app.js' });
  // `node` is defined later in app.js; supply the equivalent for rich().
  vm.runInContext('function node(tag,cls,text){const e=document.createElement(tag);if(cls)e.className=cls;if(text!==undefined)e.textContent=text;return e;}', ctx);
  return { ctx, el, doc };
}

test('t() substitutes placeholders, tn() picks plural forms, missing keys echo the key', () => {
  const { ctx } = helpers({ 'a.b': 'Hello {name}, {n} left', 'x.one': '{n} item', 'x.other': '{n} items' }, 'en');
  assert.equal(ctx.t('a.b', { name: 'Ann', n: 2 }), 'Hello Ann, 2 left');
  assert.equal(ctx.t('nope'), 'nope');
  assert.equal(ctx.tn(1, 'x'), '1 item');
  assert.equal(ctx.tn(3, 'x'), '3 items');
});

test('localizeDocument applies text, attributes and ** emphasis from the served strings', () => {
  const ja = catalogue('ja').ui;
  const { ctx, el, doc } = helpers(ja, 'ja');
  const title = el({ i18n: 'header.settings' }, {}, 'Settings');
  const search = el({}, { 'data-i18n-placeholder': 'search.placeholder', 'data-i18n-aria': 'search.aria', placeholder: 'Find an app…' }, '');
  const rich = el({ i18nRich: 'setup.note' }, { 'data-i18n-rich': 'setup.note' }, '');
  ctx.localizeDocument();
  assert.equal(doc.documentElement.lang, 'ja');
  assert.equal(title.textContent, ja['header.settings']);
  assert.equal(search.attrs.placeholder, ja['search.placeholder']);
  assert.equal(search.attrs['aria-label'], ja['search.aria']);
  assert.ok(rich.children.some(c => c.tag === 'strong'), 'emphasis rendered as <strong>');
  assert.ok(!rich.children.some(c => (c.text || c.textContent || '').includes('**')), 'markers stripped');
});

test('window.js reads menu titles from the i18n argument and falls back to English', () => {
  const win = read('internal/native/window.js');
  assert.match(win, /indexOf\('i18n='\)/);
  for (const key of ['menu.quit', 'menu.open', 'menu.setup', 'menu.keepOn', 'menu.keepOff', 'menu.keeperOn', 'menu.keeperOff', 'menu.appManagement', 'menu.support']) {
    assert.match(win, new RegExp(`L\\('${key.replace('.', '\\.')}'`), `window.js uses ${key}`);
    assert.ok(key in en.ui, `${key} in en.json`);
  }
  // The fallback literals equal the English catalogue, so a missing
  // argument still yields the shipped English menu.
  const lit = /L\('([a-zA-Z.]+)', '((?:[^'\\]|\\.)*)'\)/g;
  let m, n = 0;
  while ((m = lit.exec(win))) { n++; assert.equal(m[2], en.ui[m[1]], `fallback for ${m[1]}`); }
  assert.ok(n >= 15);
});

test('bridge.js uses the localised open-panel title supplied by Go', () => {
  const bridge = read('internal/native/bridge.js');
  assert.match(bridge, /r\.title/);
  for (const k of ['Choose a folder', 'Choose an IconCraft profile', 'Choose an original icon']) assert.ok(k in en.messages, k);
});
