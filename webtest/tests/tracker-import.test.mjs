import test from 'node:test';
import assert from 'node:assert/strict';
import { loadApp, settle } from '../lib/load-app.mjs';

test('the GitHub import is triggered on demand and its last run is visible', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const run = {
    provider: 'github',
    status: 'partial',
    imported: 1,
    failed: 1,
    startedAt: '2026-11-20T08:00:00Z',
    finishedAt: '2026-11-20T08:00:05Z',
    reason: 'broken：GitHub returned HTTP 404',
    repositories: [
      { repositoryId: 'repo-good', projectId: 'project-1', name: 'good', status: 'success', imported: 1 },
      { repositoryId: 'repo-broken', projectId: 'project-1', name: 'broken', status: 'failed', reason: 'GitHub returned HTTP 404' },
    ],
  };
  app.router.get('/api/tracker/import', () => ({ run }));
  app.router.post('/api/tracker/import', () => ({ run }));

  app.evaluate('state.data.trackerImport=null;renderSync()');
  assert.match(app.document.querySelector('#trackerImportStatus').textContent, /尚未导入/);

  await app.evaluate('loadTrackerImport()');
  const status = app.document.querySelector('#trackerImportStatus').textContent;
  assert.match(status, /最近导入/);
  assert.match(status, /部分成功/);
  assert.match(status, /broken/);
  const repos = app.document.querySelector('#trackerImportRepos').textContent;
  assert.match(repos, /good/);
  assert.match(repos, /broken/);
  assert.match(repos, /GitHub returned HTTP 404/);

  app.document.querySelector('#trackerImportBtn').click();
  await settle(6);
  const call = app.router.calls.find(c => c.method === 'POST' && c.path === '/api/tracker/import');
  assert.ok(call, 'the trigger must post');
  assert.equal(call.body.provider, 'github');
  assert.match(app.document.querySelector('#trackerImportStatus').textContent, /部分成功/);
  assert.equal(app.document.querySelector('#trackerImportBtn').disabled, false);
});

test('a failed import shows its reason instead of a silent success', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  app.router.get('/api/tracker/import', () => ({
    run: {
      provider: 'github',
      status: 'failed',
      imported: 0,
      failed: 1,
      startedAt: '2026-11-20T08:00:00Z',
      finishedAt: '2026-11-20T08:00:05Z',
      reason: 'broken：GitHub returned HTTP 401',
      repositories: [
        { repositoryId: 'repo-broken', name: 'broken', status: 'failed', reason: 'GitHub returned HTTP 401' },
      ],
    },
  }));
  await app.evaluate('loadTrackerImport()');
  const status = app.document.querySelector('#trackerImportStatus');
  assert.match(status.textContent, /失败/);
  assert.match(status.textContent, /GitHub returned HTTP 401/);
  assert.equal(status.className.includes('failed'), true);
  assert.match(app.document.querySelector('#trackerImportRepos').textContent, /GitHub returned HTTP 401/);
});

test('the GitLab import is triggered from the provider choice and its last run is visible', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const run = {
    provider: 'gitlab',
    status: 'partial',
    imported: 1,
    failed: 1,
    startedAt: '2026-11-20T09:00:00Z',
    finishedAt: '2026-11-20T09:00:05Z',
    reason: 'broken：GitLab returned HTTP 401',
    repositories: [
      { repositoryId: 'repo-good', projectId: 'project-1', name: 'good', status: 'success', imported: 1 },
      { repositoryId: 'repo-broken', projectId: 'project-1', name: 'broken', status: 'failed', reason: 'GitLab returned HTTP 401' },
    ],
  };
  app.router.get('/api/tracker/import', () => ({ run }));
  app.router.post('/api/tracker/import', () => ({ run }));

  await app.evaluate(`$('#trackerImportProvider').value='gitlab';state.data.trackerImport=null;renderSync();loadTrackerImport()`);
  const load = app.router.calls.filter(c => c.method === 'GET' && c.path === '/api/tracker/import').at(-1);
  assert.equal(load.search, '?provider=gitlab');
  assert.match(app.document.querySelector('#trackerImportBtn').textContent, /GitLab/);
  assert.match(app.document.querySelector('#trackerImportHint').textContent, /GitLab/);
  assert.match(app.document.querySelector('#trackerImportStatus').textContent, /部分成功/);
  assert.match(app.document.querySelector('#trackerImportStatus').textContent, /GitLab returned HTTP 401/);
  const repos = app.document.querySelector('#trackerImportRepos').textContent;
  assert.match(repos, /good/);
  assert.match(repos, /broken/);
  assert.match(repos, /GitLab returned HTTP 401/);

  app.document.querySelector('#trackerImportBtn').click();
  await settle(6);
  const call = app.router.calls.find(c => c.method === 'POST' && c.path === '/api/tracker/import');
  assert.ok(call, 'the trigger must post');
  assert.equal(call.body.provider, 'gitlab');
  assert.equal(app.document.querySelector('#trackerImportBtn').disabled, false);
});