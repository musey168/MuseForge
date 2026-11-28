import test from 'node:test';
import assert from 'node:assert/strict';
import { loadApp, settle } from '../lib/load-app.mjs';


test('issue row actions close and delete eligible issues through named controls', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const issue = { ...app.evaluate('structuredClone(state.data.issues[0])'), status: 'backlog',
    closureAllowed: true, deletionAllowed: true };
  app.router.post('/api/issues/issue-1/close', { ...issue, status: 'closed', closeSource: 'manual' });
  app.router.delete('/api/issues/issue-1', { ok: true });
  app.evaluate(`state.data.issues=[${JSON.stringify(issue)}];state.projectFilter='project-1';
    window.afConfirm=async()=>true;renderIssues()`);
  app.evaluate(`showIssueActionMenu({preventDefault(){},stopPropagation(){},currentTarget:document.querySelector('[data-issue-menu]')},'issue-1')`);
  app.document.querySelector('#issueActionPopover [data-action="close"]').click();
  await settle(4);
  assert.equal(app.evaluate(`state.data.issues[0].status`), 'closed');
  assert.equal(app.router.calls.find(c => c.path.endsWith('/close'))?.method, 'POST');
  app.evaluate(`state.data.issues[0].status='backlog';state.data.issues[0].closeSource='';renderIssues();
    showIssueActionMenu({preventDefault(){},stopPropagation(){},currentTarget:document.querySelector('[data-issue-menu]')},'issue-1')`);
  app.document.querySelector('#issueActionPopover [data-action="delete"]').click();
  await settle(5);
  assert.equal(app.router.calls.find(c => c.method === 'DELETE')?.path, '/api/issues/issue-1');
});


test('the requirement list renders the surviving columns and no dispatch control', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  await app.evaluate(`switchView('issues'); renderIssues();`);
  await settle(3);
  const head = app.document.querySelector('.issue-table-head').textContent;
  assert.match(head, /编号/);
  assert.match(head, /状态/);
  assert.equal(head.includes('当前环节'), false);
  assert.equal(head.includes('关联 LOOP'), false);
  assert.equal(head.includes('进度'), false);
  assert.equal(app.document.querySelector('#dispatchIssue'), null);
});

// The refresh button and every view change have to see the current list: a
// single in-flight fetch may be shared, but it must not latch forever.
test('refreshing the requirement list fetches again after the first load', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const before = app.router.calls.filter(c => c.path === '/api/issues' && c.method === 'GET').length;
  app.evaluate(`refreshIssues()`);
  await settle(4);
  app.evaluate(`refreshIssues()`);
  await settle(4);
  const after = app.router.calls.filter(c => c.path === '/api/issues' && c.method === 'GET').length;
  assert.equal(after - before, 2, 'each refresh must fetch the list');
});
