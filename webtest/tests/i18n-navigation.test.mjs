import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import vm from 'node:vm';
import { JSDOM } from 'jsdom';
import { loadApp, WEB_ROOT } from '../lib/load-app.mjs';

const HAN = /[\u3400-\u9fff]/;
const ATTRS = ['placeholder', 'title', 'aria-label', 'data-tip', 'data-tooltip'];

// A page in English mode: i18n.js rewrites the Chinese copy with i18n-en.js.
async function englishPage(html) {
  const dom = new JSDOM(html.replace('<html', '<html data-lang="en"'), { runScripts: 'outside-only', url: 'http://127.0.0.1/' });
  for (const file of ['i18n-en.js', 'i18n.js']) {
    new vm.Script(await readFile(path.join(WEB_ROOT, file), 'utf8'), { filename: file }).runInContext(dom.getInternalVMContext());
  }
  return dom.window;
}

function leftoverChinese(roots) {
  const left = [];
  for (const root of roots) {
    for (const el of [root, ...root.querySelectorAll('*')]) {
      if (el.closest('[data-i18n-skip]')) continue;
      for (const node of el.childNodes) if (node.nodeType === 3 && HAN.test(node.nodeValue)) left.push(node.nodeValue.trim());
      for (const name of ATTRS) {
        const value = el.getAttribute(name);
        if (value && HAN.test(value)) left.push(`${name}="${value}"`);
      }
    }
  }
  return left;
}

test('English mode leaves no Chinese in the navigation, the top bar or the 个人配置 heading', async () => {
  const window = await englishPage(await readFile(path.join(WEB_ROOT, 'index.html'), 'utf8'));
  const { document } = window;
  const roots = [...document.querySelectorAll('#platformRail, #settingsSidebar, .topbar, #settingsView .page-heading')];
  assert.equal(roots.length, 4);
  assert.deepEqual(leftoverChinese(roots), []);
  assert.equal(document.querySelector('#settingsSidebar [data-view="settings"] .nav-label').textContent, 'Personal settings');
  assert.equal(document.querySelector('#platformRail [data-view="observability"]').dataset.tooltip, 'Usage');
  window.close();
});

test('English mode translates every page title and the copy the app renders into the navigation', async (t) => {
  const app = await loadApp({ dispatchReady: false });
  t.after(app.close);
  const window = await englishPage('<html><body></body></html>');
  const titles = app.evaluate('viewTitles()');
  const untranslated = Object.entries(titles).filter(([, zh]) => HAN.test(window.lfTranslate(zh)));
  assert.deepEqual(untranslated, []);
  for (const text of ['收起设置导航', '展开设置导航', '收起导航', '展开导航', '返回首页', '拖动调整侧栏宽度，双击恢复默认', '调整侧栏宽度']) {
    assert.doesNotMatch(window.lfTranslate(text), HAN, text);
  }
  window.close();
});


test('English mode translates the 个人配置 credential panel and the hints that point to it', async (t) => {
  const app = await loadApp({ dispatchReady: false });
  t.after(app.close);
  const window = await englishPage('<html><body></body></html>');
  const panel = window.document.createElement('div');
  panel.innerHTML = app.evaluate('credentialPanelHTML({})');
  window.document.body.append(panel);
  await new Promise((resolve) => window.setTimeout(resolve, 0));
  assert.deepEqual(leftoverChinese([panel]), []);
  for (const text of ['需由责任人本人在个人配置中配置',
    '使用仓库进行需求分析、访问私有代码或提交 PR 前，请先配置 Git 访问令牌。LOOP 使用需求责任人的 Token，未指定时使用项目 Owner 的 Token。']) {
    assert.doesNotMatch(window.lfTranslate(text), HAN, text);
  }
  window.close();
});

