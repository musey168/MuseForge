import test from 'node:test';
import assert from 'node:assert/strict';
import { loadApp } from '../lib/load-app.mjs';

// The sync configuration and issue push are the tracker import surface that
// survives the chat workbench removal: they persist project-scoped
// integration settings and push an issue to a configured tracker.

test('sync configuration and issue push persist project-scoped integration settings', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  app.router.put('/api/sync/config', call => ({ ...call.body, autoSyncEnabled: call.body.autoSyncEnabled }));
  app.router.put('/api/projects', call => call.body);
  app.router.post('/api/sync/create', call => ({ ...app.evaluate(`state.data.issues[0]`), source: call.body.target, externalRef: { externalIid: 42, externalId: 'T-42' } }));
  app.evaluate(`
    state.data.syncConfig={gongfengProjectId:'1001'};
    $('#syncGongfengURL').value='https://mcp.example/gongfeng';
    $('#syncTAPDURL').value='https://mcp.example/tapd';
    $('#syncTAPDWorkspace').value='ws-9';
    $('#syncTAPDIteration').value='iter-3';
    $('#syncInterval').value='600';
    $('#syncAutoEnabled').checked=true;
  `);
  await app.evaluate(`saveSyncConfig()`);
  const config = app.router.calls.find(c => c.method === 'PUT' && c.path === '/api/sync/config');
  assert.equal(config.body.gongfengMcpUrl, 'https://mcp.example/gongfeng');
  assert.equal(config.body.autoSyncIntervalSec, 600);
  assert.equal(config.body.autoSyncEnabled, true);
  const project = app.router.calls.find(c => c.method === 'PUT' && c.path === '/api/projects');
  assert.equal(project.body.tapdWorkspaceId, 'ws-9');
  assert.equal(project.body.tapdIterationId, 'iter-3');

  app.evaluate(`renderSync();$('#syncPushIssue').value='issue-1'`);
  await app.evaluate(`pushIssue('gongfeng')`);
  const push = app.router.calls.find(c => c.method === 'POST' && c.path === '/api/sync/create');
  assert.equal(push.body.issueId, 'issue-1');
  assert.equal(push.body.target, 'gongfeng');
  assert.equal(app.evaluate(`state.data.issues[0].source`), 'gongfeng');
});

test('external sync controls are platform-admin only because they sync every project', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const controls = ['#importGitHubIssues', '#syncNowBtn', '#syncAutoEnabledInline', '#syncAutoEnabled', '#syncInterval', '#saveSyncConfig'];
  const disabled = () => controls.map(id => app.document.querySelector(id).disabled);
  app.evaluate(`state.authRequired=true;state.data.syncing=false;renderSync()`);
  assert.deepEqual(disabled(), controls.map(() => false), 'platform admins keep every sync control');
  app.evaluate(`state.authRequired=false;state.currentUser.isPlatformAdmin=false;state.currentUser.isGuest=true;renderSync()`);
  assert.deepEqual(disabled(), controls.map(() => false), 'without login the server accepts every caller, so the controls stay usable');
  assert.equal(app.document.querySelector('#importGitHubIssues').title, '');
  app.evaluate(`state.authRequired=true;state.currentUser.isGuest=false;renderSync()`);
  assert.deepEqual(disabled(), controls.map(() => true), 'POST /api/sync/now and PUT /api/sync/config answer 403 for everyone else');
  assert.match(app.document.querySelector('#importGitHubIssues').title, /只有平台管理员/);
  assert.match(app.document.querySelector('#syncScopeHint').textContent, /项目 Owner/);
  app.evaluate(`state.currentUser.isPlatformAdmin=true;state.data.syncing=true;renderSync()`);
  assert.equal(app.document.querySelector('#importGitHubIssues').disabled, true, 'a running sync still blocks a second one');
  assert.equal(app.document.querySelector('#syncNowBtn').disabled, true);
});
