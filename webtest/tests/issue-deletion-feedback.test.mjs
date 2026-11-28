import test from 'node:test';
import assert from 'node:assert/strict';
import { loadApp, settle } from '../lib/load-app.mjs';

const reason = '仍有未完成的工作，请先处理后再删除';
const blockedIssue = {
  id: 'delete-feedback', projectId: 'project-1', title: '国际站套餐支持升级',
  status: 'closed', closeSource: 'manual', source: 'local', priority: 'medium',
  deletionAllowed: false, closureAllowed: false,
  deletionBlock: { code: 'active', reason },
};

function showMenu(app, issue) {
  app.evaluate(`{
    state.data.issues=[${JSON.stringify(issue)}];
    state.data.tasks=[];state.data.runs=[];
    const trigger=document.createElement('button');
    document.body.appendChild(trigger);
    showIssueActionMenu({preventDefault(){},stopPropagation(){},currentTarget:trigger},'${issue.id}');
  }`);
  return app.document.querySelector('#issueActionPopover');
}


test('ordinary, forced, blocked, and unknown deletion states are shown in the menu', async t => {
  const app = await loadApp();
  t.after(app.close);
  let menu = showMenu(app, { ...blockedIssue, deletionAllowed: true, deletionBlock: undefined });
  assert.equal(menu.querySelector('[data-action="delete"]').disabled, false);
  assert.equal(menu.querySelector('[data-action="delete"]').textContent, '删除需求');
  assert.equal(menu.querySelector('.issue-delete-reason'), null);
  menu = showMenu(app, { ...blockedIssue, deletionAllowed: false, deletionBlock: { code: 'clarifying', reason: '正在澄清，请先停止澄清后再删除' } });
  assert.equal(menu.querySelector('[data-action="delete"]').disabled, true);
  assert.match(menu.querySelector('.issue-delete-reason').textContent, /正在澄清/);
  assert.match(menu.querySelector('[data-issue-deletion-context]').textContent, /查看需求详情/);
  menu = showMenu(app, { ...blockedIssue, status: 'done', deletionAllowed: true, deletionRequiresForce: true, deletionBlock: undefined });
  assert.equal(menu.querySelector('[data-action="delete"]').textContent, '强制删除');
  assert.equal(menu.querySelector('[data-action="delete"]').disabled, false);
  menu = showMenu(app, { ...blockedIssue, deletionAllowed: undefined, deletionBlock: undefined });
  assert.match(menu.querySelector('.issue-delete-reason').textContent, /刷新后重试/);
  assert.doesNotMatch(menu.querySelector('.issue-delete-reason').textContent, /成功/);
});


