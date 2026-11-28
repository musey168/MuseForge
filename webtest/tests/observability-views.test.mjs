import test from 'node:test';
import assert from 'node:assert/strict';
import { chooseOption, loadApp, settle } from '../lib/load-app.mjs';

const usagePayload = {
  summary: {
    executions: 3, inputTokens: 100, outputTokens: 50, cacheReadTokens: 20, cacheWriteTokens: 5,
    totalTokens: 150, cacheHitRate: 20, totalCostUsd: 1.23, totalCostCny: 8.61,
    costCurrency: 'CNY', usdToCny: 7, costReportedUsd: 1, costEstimatedUsd: 0.23,
    reportedExecutions: 1, estimatedExecutions: 2, unknownCostExecutions: 0,
    costCoveragePct: 100, malformedReports: 0, skippedExecutions: 0,
  },
  trend: [
    { date: '2026-10-29', executions: 1, tokens: 50, inputTokens: 30, outputTokens: 10, cacheReadTokens: 8, cacheWriteTokens: 2, costUsd: 0.4, costCny: 2.8 },
    { date: '2026-10-30', executions: 2, tokens: 100, inputTokens: 70, outputTokens: 40, cacheReadTokens: 12, cacheWriteTokens: 3, costUsd: 0.83, costCny: 5.81 },
  ],
  models: [
    { key: 'glm-5.3', name: 'glm-5.3', executions: 2, tokens: 100, inputTokens: 70, outputTokens: 40, cacheReadTokens: 12, cacheWriteTokens: 3, costUsd: 0.83, costCny: 5.81, tokenSharePct: 66.7, costSharePct: 67.5 },
  ],
  providers: [
    { key: 'p', name: 'p', executions: 3, tokens: 150, inputTokens: 100, outputTokens: 50, cacheReadTokens: 20, cacheWriteTokens: 5, costUsd: 1.23, costCny: 8.61, tokenSharePct: 100, costSharePct: 100 },
  ],
  executions: [
    { id: 'ext-1', externalExecutionId: 'build-1', workflowId: 'ci', workflowName: 'CI', status: 'succeeded', startedAt: '2026-10-30T01:00:00Z', finishedAt: '2026-10-30T01:05:00Z', durationMs: 300000, tokens: 100, costUsd: 0.83, costCny: 5.81, costStatus: 'reported', usage: [{ provider: 'p', model: 'glm-5.3', inputTokens: 70, outputTokens: 40, cacheReadTokens: 12, cacheWriteTokens: 3, costUsd: 0.83, costStatus: 'reported' }] },
    { id: 'ext-2', externalExecutionId: 'build-2', workflowId: 'ci', workflowName: 'CI', status: 'failed', startedAt: '2026-10-30T02:00:00Z', durationMs: 120000, tokens: 50, costStatus: 'unknown', usage: [] },
  ],
  empty: false,
  timeZone: 'Asia/Shanghai',
  grain: 'day',
};

async function prepare(app, payload = usagePayload) {
  app.router.get(/^\/api\/usage\?/, () => payload);
  app.evaluate(`
    state.currentUser={id:1,isGuest:false,isPlatformAdmin:true};
    state.data.users=[{id:1,loginName:'alice'}];
    state.data.projects[0].ownerId=1;
    state.usage=null;state.usageLoading=false;state.usageError='';state.usageGrain='day';state.usageBucket='';state.usageBuckets=[];state.usageMetric='tokens';state.usageExpanded='';
  `);
}

test('the usage console renders token, cost, cache, execution, trend, breakdown and execution panels', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  await prepare(app);
  await app.evaluate('refreshObs()');
  await settle(4);
  const body = app.document.querySelector('#obsBody');
  const cards = [...body.querySelectorAll('[data-obs-card]')].map(el => el.dataset.obsCard);
  assert.deepEqual(cards, ['trend', 'models', 'providers', 'executions']);
  const text = body.textContent;
  assert.match(text, /Token 用量/);
  assert.match(text, /费用/);
  assert.match(text, /缓存命中率/);
  assert.match(text, /执行数/);
  assert.match(text, /¥8\.61/);
  assert.match(text, /模型用量/);
  assert.match(text, /供应商用量/);
  assert.match(text, /执行明细/);
  assert.doesNotMatch(text, /成功率|P95|人工介入|自动化率|返工|漏斗|负责人|成员汇总|每人每日用量/);
  const call = app.router.calls.find(c => c.path === '/api/usage');
  assert.equal(call.url.searchParams.get('grain'), 'day');
  assert.equal(call.url.searchParams.get('tz'), app.evaluate('obsTimeZone()'));
});

test('the console requests the selected grain and the selected bucket', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  await prepare(app);
  await app.evaluate('refreshObs()');
  await settle(4);
  app.document.querySelector('[data-obs-usage-grain="week"]').click();
  await settle(4);
  let call = app.router.calls.filter(c => c.path === '/api/usage').at(-1);
  assert.equal(call.url.searchParams.get('grain'), 'week');
  const bucket = app.document.querySelector('#obsUsageBucket');
  assert.ok(bucket, 'the period picker lists the reported buckets');
  chooseOption(bucket, '2026-10-30');
  await settle(4);
  call = app.router.calls.filter(c => c.path === '/api/usage').at(-1);
  assert.equal(call.url.searchParams.get('bucket'), '2026-10-30');
});

test('an execution row drills into its per-model usage entries', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  await prepare(app);
  await app.evaluate('refreshObs()');
  await settle(4);
  const body = app.document.querySelector('#obsBody');
  assert.equal(body.querySelectorAll('.obs-exec-detail').length, 0);
  body.querySelector('[data-obs-exec-toggle="ext-1"]').click();
  await settle(2);
  const detail = app.document.querySelector('.obs-exec-detail');
  assert.ok(detail, 'the model breakdown expands');
  assert.match(detail.textContent, /glm-5\.3/);
  const unknown = [...body.querySelectorAll('td')].some(td => /未识别/.test(td.textContent));
  assert.ok(unknown, 'an unknown cost never reads as ¥0');
});

test('no reported executions renders an explicit empty state instead of zeros', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  await prepare(app, { ...usagePayload, empty: true, summary: { ...usagePayload.summary, executions: 0 } });
  await app.evaluate('refreshObs()');
  await settle(4);
  const body = app.document.querySelector('#obsBody');
  assert.ok(body.querySelector('[data-obs-card="empty"]'));
  assert.match(body.textContent, /暂无上报的执行/);
  assert.match(body.textContent, /上报凭据/);
  assert.match(body.textContent, /omp 会话/);
  assert.equal(body.querySelector('[data-obs-card="executions"]'), null);
  assert.doesNotMatch(body.textContent, /¥0\.00/);
});

test('a failed usage request surfaces the error instead of a blank page', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  app.router.get(/^\/api\/usage\?/, () => ({ status: 500, body: { error: 'usage unavailable' } }));
  await app.evaluate('refreshObs()');
  await settle(4);
  const body = app.document.querySelector('#obsBody');
  assert.ok(body.querySelector('[role="alert"]'));
  assert.match(body.textContent, /usage unavailable/);
});

test('the credential button shows for the account that owns the reporting credential', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  await prepare(app);
  await app.evaluate('refreshObs()');
  await settle(4);
  assert.equal(app.document.querySelector('#obsManageSources').hidden, false);
  app.evaluate(`state.currentUser={id:99,isGuest:true};renderObservability()`);
  assert.equal(app.document.querySelector('#obsManageSources').hidden, true);
});

test('credential management still issues, shows and revokes reporting credentials', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  app.router.get('/api/insights/v1/sources?projectId=project-1', [{ id: 'src-1', name: 'Review CI', tokenPrefix: 'af_x', expiresAt: '2027-01-01T00:00:00Z' }]);
  app.router.post('/api/insights/v1/sources', call => ({ ...call.body, id: 'src-2', token: 'secret-token' }));
  await prepare(app);
  await app.evaluate('openObsSources()');
  await settle(3);
  const dialog = app.document.querySelector('.obs-source-dialog');
  assert.match(dialog.textContent, /Review CI|af_x|外部数据源/);
  dialog.querySelector('[data-add]').click();
  const row = dialog.querySelector('[data-mapping]');
  row.querySelector('input').value = 'github:alice';
  const form = dialog.querySelector('[data-create]');
  form.elements.name.value = 'GitHub Actions';
  form.elements.days.value = '30';
  form.dispatchEvent(new app.window.Event('submit', { bubbles: true, cancelable: true }));
  await settle(4);
  const call = app.router.calls.find(c => c.method === 'POST' && c.path === '/api/insights/v1/sources');
  assert.equal(call.body.name, 'GitHub Actions');
  assert.equal(call.body.expiresInDays, 30);
  assert.equal(dialog.querySelector('[data-token]').value, 'secret-token');
});

// The cost panel reads the summary's total, not only a row's own amount: a
// reported cost must show as the total, never as ¥0.00.
test('the cost panel shows the summary total', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const html = app.evaluate(`(() => {
    state.usage = { summary: { totalCostUsd: 0.25, totalCostCny: 1.75, costCurrency: 'CNY', usdToCny: 7,
      costReportedUsd: 0.25, costEstimatedUsd: 0, costCoveragePct: 100, reportedExecutions: 1, executions: 1 }, trend: [], models: [], providers: [], workflows: [], executions: [] };
    return obsUsageCostPanel();
  })()`);
  assert.match(html, /¥1\.75/, html);
});
