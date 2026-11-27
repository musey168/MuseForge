'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ctx = {
  lfT: zh => zh,
  formatTokens: x => String(x),
  escapeHTML: x => String(x),
  duration: ms => `${Math.round(Number(ms || 0) / 1000)}s`,
  state: { usageGrain: 'day', usageBucket: '', usageBuckets: [], data: { users: [] } },
  projectQuery: () => '?projectId=p',
  currentProject: () => ({ id: 'p', ownerId: 7, members: [{ userId: 8, role: 'maintainer' }] }),
  paginateList: (key, items) => ({ items, total: items.length, size: 20, page: 1, pages: 1, start: 0, end: items.length }),
  pagerHTML: () => '',
  bindPager() {},
};
vm.createContext(ctx);
vm.runInContext(fs.readFileSync('app/obs-charts.js', 'utf8'), ctx, { filename: path.resolve('app/obs-charts.js') });
vm.runInContext(fs.readFileSync('app/observability.js', 'utf8'), ctx, { filename: path.resolve('app/observability.js') });

// The request carries the project scope, the grain, the reader time zone and
// the selected bucket.
ctx.state.currentUser = { id: 7 };
assert.match(ctx.obsUsageQuery(), /projectId=p/);
assert.match(ctx.obsUsageQuery(), /grain=day/);
assert.match(ctx.obsUsageQuery(), /tz=/);
assert.doesNotMatch(ctx.obsUsageQuery(), /bucket=/);
ctx.state.usageBucket = '2026-10-30';
assert.match(ctx.obsUsageQuery(), /bucket=2026-10-30/);
ctx.state.usageGrain = 'week';
assert.match(ctx.obsUsageQuery(), /grain=week/);
ctx.state.usageGrain = 'month';
assert.match(ctx.obsUsageQuery(), /grain=month/);
ctx.state.usageGrain = 'nonsense';
assert.equal(ctx.obsUsageGrain(), 'day', 'an unknown grain falls back to day');

// Costs render in the reader currency; an unknown cost never reads as ¥0.
assert.equal(ctx.obsUsageMoney(8.61, 'CNY'), '¥8.61');
assert.equal(ctx.obsUsageMoney(1.23, 'USD'), '$1.23');
assert.match(ctx.obsUsageCostText(0, 'unknown', 'CNY'), /未识别/);
assert.equal(ctx.obsUsageCostText(2.5, 'reported', 'CNY'), '¥2.50');
assert.equal(ctx.obsUsageCostText(2.5, 'estimated', 'CNY'), '¥2.50');

// The console is empty when the server says so or when nothing was reported.
ctx.state.usage = { empty: true, summary: { executions: 0 } };
assert.equal(ctx.obsUsageIsEmpty(), true);
ctx.state.usage = { summary: { executions: 0 } };
assert.equal(ctx.obsUsageIsEmpty(), true);
ctx.state.usage = { summary: { executions: 2 } };
assert.equal(ctx.obsUsageIsEmpty(), false);
ctx.state.usage = null;
assert.equal(ctx.obsUsageIsEmpty(), false);

// Managing reporting credentials follows the project role.
ctx.state.currentUser = { id: 7 };
assert.equal(ctx.obsCanManageSources(), true);
ctx.state.currentUser = { id: 8 };
assert.equal(ctx.obsCanManageSources(), true);
ctx.state.currentUser = { id: 9 };
assert.equal(ctx.obsCanManageSources(), false);
ctx.state.currentUser = { id: 9, isGuest: true };
assert.equal(ctx.obsCanManageSources(), false);

// The period picker walks the buckets the server reported.
const buckets = ['2026-10-01', '2026-10-02', '2026-10-03'];
assert.equal(ctx.obsUsageStepBucket(buckets, '2026-10-02', 1), '2026-10-03');
assert.equal(ctx.obsUsageStepBucket(buckets, '2026-10-02', -1), '2026-10-01');
assert.equal(ctx.obsUsageStepBucket(buckets, '2026-10-01', -1), null, 'the picker stops at the first bucket');
assert.equal(ctx.obsUsageStepBucket(buckets, '2026-10-03', 1), null, 'the picker stops at the last bucket');
assert.equal(ctx.obsUsageStepBucket([], '', 1), null);

// Run-quality surfaces are gone from the usage console.
const code = fs.readFileSync('app/observability.js', 'utf8');
for (const unwanted of ['successRate', 'p95', 'humanInterventions', 'automationRate', 'rework', 'funnel', 'ownerRows', 'weekHour', 'delivery']) {
  assert.ok(!code.includes(unwanted), `the usage console must not contain ${unwanted}`);
}
console.log('external usage frontend checks passed');
