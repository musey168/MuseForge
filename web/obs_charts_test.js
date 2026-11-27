'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

// ---------- 极简 DOM ----------
class FakeClassList {
  constructor(el) { this.el = el; this.set = new Set(); }
  add(...c) { c.forEach(x => this.set.add(x)); }
  remove(...c) { c.forEach(x => this.set.delete(x)); }
  contains(c) { return this.set.has(c); }
  toggle(c, on) { if (on === undefined) on = !this.set.has(c); on ? this.set.add(c) : this.set.delete(c); return on; }
}
function camel(name) { return name.replace(/-([a-z])/g, (_, c) => c.toUpperCase()); }
function matches(el, sel) {
  return sel.split(',').some(part => {
    part = part.trim();
    const re = /(#[\w-]+)|(\.[\w-]+)|\[([\w-]+)(?:="([^"]*)")?\]|^([a-z]+)/g;
    let m, ok = true, any = false;
    while ((m = re.exec(part))) {
      any = true;
      if (m[1]) ok = ok && el.id === m[1].slice(1);
      else if (m[2]) ok = ok && el.classList.contains(m[2].slice(1));
      else if (m[3]) {
        const key = camel(m[3].replace(/^data-/, ''));
        const val = m[3].startsWith('data-') ? el.dataset[key] : el.attrs[m[3]];
        ok = ok && val !== undefined && (m[4] === undefined || String(val) === m[4]);
      } else if (m[5]) ok = ok && el.tagName === m[5].toUpperCase();
    }
    return any && ok;
  });
}
class FakeNode {
  constructor(doc) { this.ownerDocument = doc; this.parentNode = null; this.children = []; }
  get isConnected() { let n = this; while (n.parentNode) n = n.parentNode; return n === this.ownerDocument.root; }
  remove() { if (this.parentNode) { const list = this.parentNode.children; list.splice(list.indexOf(this), 1); this.parentNode = null; } }
  before(node) { node.remove(); const list = this.parentNode.children; list.splice(list.indexOf(this), 0, node); node.parentNode = this.parentNode; }
  replaceWith(node) { this.before(node); this.remove(); }
  appendChild(node) { node.remove(); this.children.push(node); node.parentNode = this; return node; }
}
class FakeComment extends FakeNode {}
class FakeElement extends FakeNode {
  constructor(doc, tag) {
    super(doc);
    this.tagName = String(tag || 'div').toUpperCase();
    this.dataset = {}; this.attrs = {}; this.style = {}; this.id = '';
    this.classList = new FakeClassList(this);
    this._text = ''; this.clientWidth = 400; this.hidden = false; this.listeners = {};
  }
  set innerHTML(v) { this.children.forEach(c => { c.parentNode = null; }); this.children = []; this._text = String(v); }
  get innerHTML() { return this._text; }
  set textContent(v) { this._text = String(v); }
  get textContent() { return this._text; }
  get elementChildren() { return this.children.filter(c => c instanceof FakeElement); }
  setAttribute(k, v) { this.attrs[k] = String(v); }
  getAttribute(k) { return this.attrs[k]; }
  addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); }
  insertAdjacentHTML(_pos, html) { this._text += html; }
  focus() {}
  scrollIntoView() {}
  contains(node) { for (let n = node; n; n = n.parentNode) if (n === this) return true; return false; }
  closest(sel) { for (let n = this; n instanceof FakeElement; n = n.parentNode) if (matches(n, sel)) return n; return null; }
  querySelectorAll(sel) {
    const out = [];
    const walk = n => n.elementChildren.forEach(c => { if (matches(c, sel)) out.push(c); walk(c); });
    walk(this);
    return out;
  }
  querySelector(sel) { return this.querySelectorAll(sel)[0] || null; }
}
function makeDocument() {
  const doc = {};
  doc.root = new FakeElement(doc, 'html');
  doc.documentElement = doc.root;
  doc.documentElement.dataset.theme = 'dark';
  doc.head = doc.root.appendChild(new FakeElement(doc, 'head'));
  doc.body = doc.root.appendChild(new FakeElement(doc, 'body'));
  doc.listeners = {};
  doc.createElement = tag => new FakeElement(doc, tag);
  doc.createComment = () => new FakeComment(doc);
  doc.getElementById = id => doc.root.querySelectorAll('#' + id)[0] || null;
  doc.querySelector = sel => doc.root.querySelector(sel);
  doc.querySelectorAll = sel => doc.root.querySelectorAll(sel);
  doc.addEventListener = (type, fn) => { (doc.listeners[type] ||= []).push(fn); };
  return doc;
}
function el(doc, tag, opts) {
  const node = doc.createElement(tag);
  Object.assign(node.dataset, opts?.data || {});
  if (opts?.id) node.id = opts.id;
  (opts?.cls || []).forEach(c => node.classList.add(c));
  if (opts?.text != null) node.textContent = opts.text;
  return node;
}

// ---------- 假 echarts / 观察者 ----------
function makeEcharts() {
  const stats = { init: 0, dispose: 0, setOption: 0, clear: 0, actions: [] };
  const live = new Set();
  return {
    stats, live,
    init(node, theme, opts) {
      stats.init++;
      assert.equal(opts.renderer, 'svg');
      const handlers = {}, zr = {};
      const chart = {
        node, options: [], disposed: false,
        setOption(o, notMerge) { stats.setOption++; this.options.push({ o, notMerge }); },
        on(name, fn) { (handlers[name] ||= []).push(fn); },
        emit(name, p) { (handlers[name] || []).forEach(fn => fn(p)); },
        getZr() { return { on: (n, fn) => { zr[n] = fn; } }; },
        zrClick(x) { zr.click && zr.click({ offsetX: x, offsetY: 1 }); },
        containPixel() { return true; },
        convertFromPixel(_g, p) { return [p[0]]; },
        dispatchAction(a) { stats.actions.push(a); },
        resize() {}, clear() { stats.clear++; },
        dispose() { stats.dispose++; this.disposed = true; live.delete(this); },
        isDisposed() { return this.disposed; },
      };
      live.add(chart);
      return chart;
    },
  };
}
function makeIO() {
  const observed = new Set();
  class IO {
    constructor(cb) { this.cb = cb; IO.instances.push(this); }
    observe(t) { observed.add(t); }
    unobserve(t) { observed.delete(t); }
    disconnect() { observed.clear(); }
    fire(targets) { this.cb(targets.map(target => ({ target, isIntersecting: true }))); }
  }
  IO.instances = [];
  IO.observed = observed;
  return IO;
}

function makeContext({ echarts, reduced } = {}) {
  const document = makeDocument();
  const IO = makeIO();
  const frames = [];
  const ctx = {
    console, document, Map, Set, WeakMap, Promise, Math, Date, Number, String, Object, Array, JSON, Intl, URLSearchParams,
    setTimeout: (fn) => { fn(); return 0; }, clearTimeout() {},
    requestAnimationFrame: fn => { frames.push(fn); return frames.length; },
    performance: { now: () => ctx.__now || 0 },
    matchMedia: () => ({ matches: !!reduced }),
    getComputedStyle: () => ({ getPropertyValue: () => '' }),
    IntersectionObserver: IO,
    ResizeObserver: class { observe() {} unobserve() {} },
    MutationObserver: class { observe() {} },
    localStorage: { getItem() { return null; }, setItem() {} },
    state: { usage: null, usageGrain: 'day', usageBucket: '', usageBuckets: [], usageMetric: 'tokens', data: { users: [], runs: [], issues: [] } },
    escapeHTML: v => String(v ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])),
    safeURL: v => String(v || ''),
    formatTokens: v => String(Math.round(Number(v) || 0)),
    duration: v => `${Math.round((Number(v) || 0) / 1000)}s`,
    projectQuery: () => '', currentProject: () => ({ id: 'p', name: 'P' }),
    toast() {}, userById() { return null; }, userDisplay: u => String(u?.id || ''),
    paginateList: (key, items) => ({ items, total: items.length, size: 20, page: 1, pages: 1, start: 0, end: items.length }),
    pagerHTML: () => '', bindPager() {}, resetListPage() {}, resetListPages() {}, listPageSize: () => 20,
  };
  ctx.window = ctx;
  if (echarts) ctx.echarts = echarts;
  ctx.__frames = frames;
  ctx.__IO = IO;
  vm.createContext(ctx);
  vm.runInContext(fs.readFileSync('app/obs-charts.js', 'utf8'), ctx, { filename: path.resolve('app/obs-charts.js') });
  vm.runInContext(fs.readFileSync('app/observability.js', 'utf8'), ctx, { filename: path.resolve('app/observability.js') });
  return ctx;
}
const plain = v => JSON.parse(JSON.stringify(v));
function same(a, b, msg) { assert.deepEqual(plain(a), plain(b), msg); }
async function flush() { for (let i = 0; i < 5; i++) await Promise.resolve(); }
function fireVisible(ctx) {
  const io = ctx.__IO.instances[0];
  if (io) io.fire([...ctx.__IO.observed]);
}

// ---------- option 纯函数 ----------
(() => {
  const ctx = makeContext();
  const theme = vm.runInContext('OBS_THEME_PRESETS.dark', ctx);
  const points = [
    { date: '2026-08', executions: 2, tokens: 100, inputTokens: 60, outputTokens: 20, cacheReadTokens: 15, cacheWriteTokens: 5, costUsd: 1, costCny: 7 },
    { date: '2026-09', executions: 5, tokens: 300, inputTokens: 200, outputTokens: 60, cacheReadTokens: 30, cacheWriteTokens: 10, costUsd: 3, costCny: 21 },
    { date: '2026-10', executions: 4, tokens: 200, inputTokens: 100, outputTokens: 60, cacheReadTokens: 30, cacheWriteTokens: 10, costUsd: 2, costCny: 14 },
  ];
  const opts = { theme, reduced: false, fmtTokens: v => String(v), fmtCost: v => '¥' + Number(v).toFixed(2) };

  const tokens = ctx.obsUsageTrendOption(points, 'tokens', opts);
  assert.equal(tokens.series.length, 4, 'token mode splits input/output/cache read/cache write');
  same(tokens.series.map(s => s.name), ['输入', '输出', '缓存读', '缓存写']);
  same(tokens.series.map(s => s.data[1]), [200, 60, 30, 10]);
  assert.ok(tokens.series.every(s => s.stack === 'tokens'));
  assert.equal(tokens.aria.enabled, true);

  const cost = ctx.obsUsageTrendOption(points, 'cost', opts);
  assert.equal(cost.series.length, 1);
  same(cost.series[0].data, [7, 21, 14]);
  assert.equal(cost.yAxis.axisLabel.formatter(21), '¥21.00');
  assert.match(cost.tooltip.formatter([{ dataIndex: 1 }]), /¥21\.00/);

  const executions = ctx.obsUsageTrendOption(points, 'executions', opts);
  assert.equal(executions.series[0].type, 'bar');
  same(executions.series[0].data, [2, 5, 4]);

  const tip = ctx.obsUsageTrendOption([{ date: 'x', label: '<script>alert(1)</script>', tokens: 1, executions: 1, costCny: 1 }], 'tokens', { theme, label: p => p.label }).tooltip.formatter([{ dataIndex: 0 }]);
  assert.ok(!tip.includes('<script>'), 'tooltip escapes labels');
  assert.ok(tip.includes('&lt;script&gt;'));

  const reduced = ctx.obsUsageTrendOption(points, 'tokens', { theme, reduced: true });
  assert.equal(reduced.animation, false);
  assert.equal(reduced.animationDuration, 0);
  assert.equal(ctx.obsUsageTrendOption(points, 'tokens', opts).animation, true);

  const rows = [{ name: 'a', tokens: 70, costCny: 0 }, { name: '<b>', tokens: 30, costCny: 14 }];
  const pie = ctx.obsBreakdownOption(rows, 'tokens', opts);
  assert.equal(pie.series[0].data.reduce((n, d) => n + d.value, 0), ctx.obsBreakdownTotal(rows, 'tokens'));
  assert.equal(ctx.obsBreakdownOption(rows, 'cost', opts).series[0].data[1].value, 14, 'pie uses CNY amounts');
  assert.ok(!pie.tooltip.formatter({ name: '<b>', value: 30 }).includes('<b><'), 'pie tooltip escapes');
  assert.equal(ctx.obsBreakdownOption(rows, 'tokens', { theme, reduced: true }).animation, false);
})();

// ---------- 挂载 / 释放 / 泄漏 ----------
(async () => {
  const ec = makeEcharts();
  const ctx = makeContext({ echarts: ec });
  const doc = ctx.document;
  const host = doc.body.appendChild(el(doc, 'div', { id: 'obsBody' }));
  const render = n => {
    ctx.obsDisposeCharts(host);
    host.innerHTML = '';
    for (let i = 0; i < n; i++) {
      ctx.obsChartSlot('c' + i, { option: () => ({ series: [] }), on: { click: () => {} } }, 200);
      host.appendChild(el(doc, 'div', { data: { obsChart: 'c' + i } }));
    }
    ctx.obsMountCharts(host);
  };
  render(3);
  assert.equal(ec.stats.init, 0, 'charts wait until visible');
  fireVisible(ctx);
  await flush();
  assert.equal(ctx.obsChartCount(), 3);
  assert.equal(ec.stats.init, 3);
  ctx.obsMountCharts(host);
  fireVisible(ctx);
  await flush();
  assert.equal(ec.stats.init, 3, 'repeat mount does not re-init');
  ctx.obsDisposeCharts();
  assert.equal(ctx.obsChartCount(), 0);
  assert.equal(ec.live.size, 0);
  for (let i = 0; i < 50; i++) { render(3); fireVisible(ctx); await flush(); }
  assert.equal(ctx.obsChartCount(), 3, 'no leaked instances after 50 renders');
  assert.equal(ec.live.size, 3);
  assert.equal(ctx.__IO.observed.size, 0, 'no stale observers');
  assert.ok(ctx.obsReplayChart('c0'));
  assert.equal(ec.stats.clear, 1);
  assert.equal(ctx.obsReplayChart('missing'), false);

  // axisClick 走 zr 点击 → 下标
  let clicked = -1;
  ctx.obsDisposeCharts(host); host.innerHTML = '';
  ctx.obsChartSlot('axis', { option: () => ({}), axisClick: idx => { clicked = idx; } }, 200);
  host.appendChild(el(doc, 'div', { data: { obsChart: 'axis' } }));
  ctx.obsMountCharts(host); fireVisible(ctx); await flush();
  ctx.obsChartEntry('axis').chart.zrClick(2.2);
  assert.equal(clicked, 2);
  ctx.obsDisposeCharts();

  // 空数据不 init
  ctx.obsChartSlot('empty', { empty: true, option: () => ({}) }, 100);
  const emptyEl = host.appendChild(el(doc, 'div', { data: { obsChart: 'empty', empty: '1' } }));
  ctx.obsMountCharts(host); fireVisible(ctx); await flush();
  assert.equal(ctx.obsChartEntry('empty'), null);
  emptyEl.remove();
})().then(async () => {
  // ---------- 加载失败 → SVG 兜底 ----------
  const ctx = makeContext();
  const doc = ctx.document;
  const script = { set onerror(fn) { this._err = fn; } };
  const origCreate = doc.createElement;
  let inserted = null;
  doc.createElement = tag => { const n = origCreate(tag); if (tag === 'script') inserted = n; return n; };
  doc.head.appendChild = n => { if (n.tagName === 'SCRIPT') setImmediate(() => n.onerror && n.onerror()); return n; };
  const card = doc.body.appendChild(el(doc, 'section', { data: { obsCard: 'trend' } }));
  const head = card.appendChild(el(doc, 'header', { cls: ['obs-card-head'] }));
  head.appendChild(el(doc, 'div', { cls: ['obs-card-tools'] }));
  const points = [{ date: '2026-09', executions: 1, tokens: 10 }, { date: '2026-10', executions: 1, tokens: 20 }];
  ctx.obsChartSlot('trend', { option: () => ({}), fallback: () => '<div class="obs-bd-bars"></div>' }, 200);
  const slot = card.appendChild(el(doc, 'div', { data: { obsChart: 'trend' } }));
  ctx.obsMountCharts(card); fireVisible(ctx);
  await new Promise(r => setImmediate(r)); await flush();
  assert.ok(inserted && inserted.src.startsWith('/vendor/echarts/echarts.min.js?v=6.1.0'), 'loads same-origin echarts');
  assert.ok(slot.innerHTML.includes('obs-bd-bars'), 'falls back to the plain chart');
  assert.ok(slot.classList.contains('is-fallback'));
  assert.ok(head.querySelector('.obs-card-tools').innerHTML.includes('简版图表'));
  assert.equal(ctx.obsChartCount(), 0);
  const firstScript = inserted;
  ctx.obsMountCharts(card);
  const retry = ctx.obsEnsureECharts();
  assert.notEqual(inserted, firstScript, 'next mount retries loading after a failure');
  await new Promise(r => setImmediate(r));
  assert.equal(await retry, null);
}).then(() => {
  // ---------- 数字滚动 ----------
  const ctx = makeContext();
  const doc = ctx.document;
  const num = doc.body.appendChild(el(doc, 'b', { text: '1,234.5k' }));
  ctx.obsCountUp(num);
  assert.equal(num.textContent, '0.0k');
  ctx.__now = 350; ctx.__frames.shift()(350);
  assert.notEqual(num.textContent, '1,234.5k');
  ctx.__now = 900; ctx.__frames.shift()(900);
  assert.equal(num.textContent, '1,234.5k', 'ends on original text');
  const dur = doc.body.appendChild(el(doc, 'b', { text: '3m 12s' }));
  ctx.obsCountUp(dur);
  assert.equal(dur.textContent, '3m 12s', 'multi-number values are left alone');
  const rctx = makeContext({ reduced: true });
  const still = rctx.document.body.appendChild(el(rctx.document, 'b', { text: '42' }));
  rctx.obsCountUp(still);
  assert.equal(still.textContent, '42');
  assert.equal(rctx.__frames.length, 0, 'reduced motion skips animation');
}).then(() => {
  // ---------- 放大窗口 ----------
  const ctx = makeContext({ echarts: makeEcharts() });
  const doc = ctx.document;
  const view = doc.body.appendChild(el(doc, 'section', { id: 'observabilityView' }));
  const grid = view.appendChild(el(doc, 'div'));
  const before = grid.appendChild(el(doc, 'div'));
  const card = grid.appendChild(el(doc, 'section', { data: { obsCard: 'trend' } }));
  const btn = card.appendChild(el(doc, 'button', { data: { obsExpand: 'trend' } }));
  const after = grid.appendChild(el(doc, 'div'));
  const modal = view.appendChild(el(doc, 'div', { id: 'obsExpand' }));
  modal.hidden = true;
  const box = modal.appendChild(el(doc, 'div', { cls: ['obs-expand-box'] }));
  ctx.obsExpand(card);
  assert.equal(card.parentNode, box);
  assert.equal(modal.hidden, false);
  assert.ok(doc.body.classList.contains('obs-expand-open'));
  assert.ok(card.classList.contains('is-expanded'));
  assert.equal(btn.attrs['aria-label'], '关闭放大窗口');
  doc.listeners.keydown.forEach(fn => fn({ key: 'Escape' }));
  assert.equal(card.parentNode, grid);
  assert.ok(grid.children.length === 3 && grid.children[0] === before && grid.children[1] === card && grid.children[2] === after, 'card returns to its slot');
  assert.equal(modal.hidden, true);
  assert.ok(!doc.body.classList.contains('obs-expand-open'));
  assert.ok(!card.classList.contains('is-expanded'));
  assert.equal(btn.attrs['aria-label'], '放大查看');
}).then(() => {
  console.log('obs charts ok');
}).catch(err => { console.error(err); process.exit(1); });
