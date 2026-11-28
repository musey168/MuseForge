import test from 'node:test';
import assert from 'node:assert/strict';
import { chooseOption, loadApp, sampleBootstrap, settle } from '../lib/load-app.mjs';

test('list paging, URL safety, and shell helpers return concrete domain results', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const result = app.evaluate(`(() => {
    const page=paginateList('issues',Array.from({length:45},(_,i)=>i+1),20);
    state.listPages.issues=3;
    const last=paginateList('issues',Array.from({length:45},(_,i)=>i+1),20);
    return {
      page:[page.page,page.pages,page.start,page.end,page.items[0]],
      last:[last.page,last.start,last.end,last.items],
      pager:pagerHTML('issues',last),
      labels:[
        clarifyOptionLabel({label:'Choice A'}),
        clarifyOptionLabel('{"text":"Choice B"}'),
        clarifyQuestionOptions({options:'[{"label":"One"},"Two"]'})
      ],
      urls:[safeURL('https://example.com/a'),safeURL('javascript:alert(1)')],
      colors:[safeColor('#abc'),safeColor('red')],
      unconfigured:(state.appDomain='',isOAHost('loopforge.example.com')),
      hosts:[isLoopbackHost('127.0.0.1'),(state.appDomain='loopforge.example.com',isOAHost('loopforge.example.com')),shouldAutoSSORedirect({isGuest:true,hostname:'loopforge.example.com',bypassAutoAuth:false,authErr:'',loginUrl:'https://login'})]
    };
  })()`);
  assert.equal(JSON.stringify(result.page), JSON.stringify([1, 3, 0, 20, 1]));
  assert.equal(result.last[0], 3);
  assert.equal(result.last[1], 40);
  assert.equal(result.last[2], 45);
  assert.match(result.pager, /第 41–45 条|上一页|disabled/);
  assert.equal(result.labels[0], 'Choice A');
  assert.equal(result.labels[1], 'Choice B');
  assert.equal(JSON.stringify(result.labels[2]), JSON.stringify(['One', 'Two']));
  assert.equal(result.urls[1], '#');
  assert.equal(JSON.stringify(result.colors), JSON.stringify(['#abc', '#74829a']));
  assert.equal(JSON.stringify(result.hosts), JSON.stringify([true, true, true]));
  assert.equal(result.unconfigured, false, 'no SSO redirect host unless the deployment configures one');
});

test('code editor keyboard controls indent, outdent, auto-indent, save, and close the modal', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  app.evaluate(`openCodeEditorModal({title:'Python',language:'python',value:'if ready:',onSave:(value,lang)=>window.__editorSaved={value,lang}})`);
  const editor = app.document.querySelector('#codeEditorTextarea');
  editor.selectionStart = editor.selectionEnd = editor.value.length;
  editor.dispatchEvent(new app.window.KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
  assert.equal(editor.value, 'if ready:\n  ');
  editor.value = 'line';
  editor.selectionStart = editor.selectionEnd = 0;
  editor.dispatchEvent(new app.window.KeyboardEvent('keydown', { key: 'Tab', bubbles: true }));
  assert.equal(editor.value, '  line');
  editor.selectionStart = editor.selectionEnd = 2;
  editor.dispatchEvent(new app.window.KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true }));
  assert.equal(editor.value, 'line');
  editor.value = 'print("saved")';
  editor.dispatchEvent(new app.window.KeyboardEvent('keydown', { key: 's', ctrlKey: true, bubbles: true }));
  assert.equal(app.evaluate(`window.__editorSaved.value`), 'print("saved")');
  assert.equal(app.evaluate(`window.__editorSaved.lang`), 'python');
  assert.equal(app.document.querySelector('#codeEditorModal').classList.contains('open'), false);

  app.evaluate(`openCodeEditorModal({title:'Close',value:'x'})`);
  editor.dispatchEvent(new app.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  assert.equal(app.document.querySelector('#codeEditorModal').classList.contains('open'), false);
});



test('project switching and shell keyboard interactions update state', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  app.evaluate(`state.data.projects=[...state.data.projects,{id:'project-2',name:'Second',identifier:'SEC',status:'active'}];`);
  const select = app.document.querySelector('#projectSwitcher');
  app.evaluate(`renderProjectSwitcher()`);
  chooseOption(select, 'project-2');
  await settle(6);
  assert.equal(app.evaluate(`state.projectFilter`), 'project-2');
  assert.equal(app.window.localStorage.getItem('agentflow.activeProject'), 'project-2');

  app.document.dispatchEvent(new app.window.KeyboardEvent('keydown', { key: 'k', ctrlKey: true, bubbles: true }));
  assert.equal(app.document.querySelector('#commandPalette').hidden, false);
  app.document.dispatchEvent(new app.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  assert.equal(app.document.querySelector('#commandPalette').hidden, true);

  const resize = app.document.querySelector('.sidebar-resize');
  resize.dispatchEvent(new app.window.KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }));
  assert.ok(Number(app.window.localStorage.getItem('agentflow.sidebarWidth')) >= 220);
  resize.dispatchEvent(new app.window.MouseEvent('dblclick', { bubbles: true }));
  assert.equal(app.window.localStorage.getItem('agentflow.sidebarWidth'), null);
});

test('boot lands on the requirement list without a sign-in', async (t) => {
  const app = await loadApp({ dispatchReady: false });
  t.after(app.close);
  app.window.history.replaceState({}, '', '/?projectId=project-1#/issues');
  await app.evaluate(`boot()`);
  assert.equal(app.evaluate(`state.projectFilter`), 'project-1');
  assert.match(app.window.location.hash, /issues/);
  assert.equal(app.document.querySelector('#pageTitle').textContent, '需求列表');
  assert.equal(app.document.querySelector('#loginView'), null);
});

test('boot selects the account\'s project before an unrelated first project', async (t) => {
  const shell = sampleBootstrap();
  shell.projects = [
    { id: 'someone-else', name: 'Someone else', key: 'ELSE', ownerId: 1 },
    { id: 'owned', name: 'Owned', key: 'OWN', ownerId: 7 },
  ];
  shell.users = [{ id: 7, loginName: 'owner', isGuest: false }];
  const app = await loadApp({ routes: [
    { path: '/api/bootstrap', body: shell },
  ] });
  t.after(app.close);
  await settle(6);
  assert.equal(app.evaluate('state.currentUser.loginName'), 'owner');
  assert.equal(app.evaluate('state.projectFilter'), 'owned');
});
