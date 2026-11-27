import test from 'node:test';
import assert from 'node:assert/strict';
import { loadApp, settle } from '../lib/load-app.mjs';

const SURVIVING_VIEWS = [
  'issues', 'observability', 'settings', 'projects', 'projectCredentials',
  'platform', 'runners', 'guide',
];

const REMOVED_VIEWS = [
  'collaboration', 'runs', 'approvals', 'sops', 'flow', 'agents', 'mcp',
  'rules', 'skills', 'sandboxes', 'knowledge', 'bots', 'aiSchedules', 'evolution',
  'metrics', 'businessDomains', 'businessDomainManagement',
];

test('boots with platform-shaped data and renders every routed view', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  assert.equal(app.evaluate('state.data.projects[0].name'), 'Atlas');
  assert.equal(app.evaluate('state.currentUser.username'), 'tester');

  for (const view of SURVIVING_VIEWS) {
    app.evaluate(`switchView(${JSON.stringify(view)})`);
    await settle(2);
    assert.ok(app.document.querySelector(`#${view}View`), `${view}View exists`);
  }
  assert.ok(app.router.calls.some((call) => call.path === '/api/bootstrap'));
  assert.ok(app.document.querySelectorAll('.view').length > 5);
  assert.equal(app.document.querySelector('#pageTitle').textContent, '访问密钥');
  for (const removed of REMOVED_VIEWS) {
    assert.equal(app.document.querySelector(`#${removed}View`), null, `${removed} was retired`);
    assert.equal(app.document.querySelector(`[data-view="${removed}"]`), null, `${removed} has no nav entry`);
  }
});

test('hash routing opens project detail and keeps removed surfaces unreachable', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  app.window.location.hash = '#/projects/project-1/repos';
  app.window.dispatchEvent(new app.window.HashChangeEvent('hashchange'));
  await settle(3);
  assert.match(app.window.location.hash, /^#\/projects/);
  for (const hash of ['#/runs/run-1', '#/flow/flow-1', '#/agents/agent-1', '#/knowledge']) {
    app.window.location.hash = hash;
    app.window.dispatchEvent(new app.window.HashChangeEvent('hashchange'));
    await settle(3);
    assert.ok(
      SURVIVING_VIEWS.includes(app.evaluate('state.currentView')),
      `${hash} stays on a surviving view`,
    );
  }
});

test('the requirement list offers no dispatch control and no engine columns', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  app.evaluate(`switchView('issues')`);
  await settle(2);
  assert.equal(app.document.querySelector('#dispatchIssue'), null);
  assert.equal(app.document.querySelector('#reqDelivery'), null);
  assert.equal(app.document.querySelector('#reqWorkflow'), null);
  assert.equal(app.document.querySelector('#runButton'), null);
  assert.equal(app.document.querySelector('#artifactModal'), null);
});
