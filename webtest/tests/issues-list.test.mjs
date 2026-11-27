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
  assert.match(head, /当前环节/);
  assert.equal(head.includes('关联 LOOP'), false);
  assert.equal(head.includes('进度'), false);
  assert.equal(app.document.querySelector('#dispatchIssue'), null);
});
