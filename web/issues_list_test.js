const assert = require('assert');
const fs = require('fs');
const vm = require('vm');

const src = fs.readFileSync(process.argv[2], 'utf8');
function slice(from, to) {
  const start = src.indexOf(from);
  assert.ok(start >= 0, `${from} not found`);
  const end = src.indexOf(to, start);
  assert.ok(end > start, `${to} not found after ${from}`);
  return src.slice(start, end);
}

const ctx = {
  escapeHTML: v => String(v ?? '').replace(/[&<>'"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[c])),
  issuePriorityLabel: v => ({ urgent: '紧急', high: '高', medium: '中', low: '低' }[v] || v),
  issueAssigneeName: i => i.name || '',
  issueKeyLabel: i => i.key,
};
vm.createContext(ctx);
vm.runInContext(slice('function issueStatusIcon(', 'function issueStatusText('), ctx);
vm.runInContext(slice('function issueKeyColumnWidth(', 'function issueTagTone('), ctx);

const tones = [
  [{ key: 'active', label: '进行中' }, 'info', 'lamp'],
  [{ key: 'active', label: '评审中' }, 'info', 'lamp'],
  [{ key: 'attention', label: '已阻塞' }, 'error', 'alert'],
  [{ key: 'closed', label: '已完成' }, 'success', 'check'],
  [{ key: 'closed', label: '已关闭' }, 'neutral', 'close'],
  [{ key: 'not_started', label: '待规划' }, 'neutral', 'circle'],
  [undefined, 'neutral', 'circle'],
];
for (const [stage, tone, icon] of tones) {
  assert.deepStrictEqual({ ...ctx.issueStageTone(stage) }, { tone, icon }, `stage ${JSON.stringify(stage)}`);
}

const running = ctx.issueStageBadge({ key: 'active', label: '编码实现' });
assert.match(running, /^<span class="hui-badge" data-tone="info" data-stage-icon="lamp"><span class="hui-lamp"/);
assert.ok(!running.includes('data-icon='), 'running badge uses the breathing lamp instead of an icon');
const failed = ctx.issueStageBadge({ key: 'attention', label: '失败 · <b>x</b>' });
assert.ok(failed.includes('class="hui-badge" data-tone="error"'));
assert.ok(failed.includes('data-icon="alert"'));
assert.ok(failed.includes('<span class="hui-badge-label">失败 · &lt;b&gt;x&lt;/b&gt;</span>'), failed);

assert.ok(ctx.huiIcon('search').startsWith('<svg class="agent-icon harness-icon" data-icon="search" width="14" height="14"'));
assert.ok(ctx.huiIcon('edit', 'popover-icon').startsWith('<svg class="agent-icon harness-icon popover-icon" data-icon="edit"'));
assert.ok(ctx.huiIcon('x"><img>').includes('data-icon="x&quot;&gt;&lt;img&gt;"'));

for (const [pri, icon] of [['urgent', 'priority-bolt'], ['high', 'priority-high'], ['medium', 'priority-medium'], ['low', 'priority-low'], ['', 'priority-medium']]) {
  const html = ctx.issuePriorityMarkup(pri);
  assert.ok(html.includes(`class="issue-priority ${pri || 'medium'}"`), html);
  assert.ok(html.includes(`data-icon="${icon}"`), html);
}

// 一个操作者一个身份：责任人的头像与姓名不再出现在列表里。
assert.strictEqual(typeof ctx.issueOwnerAvatar, 'undefined', 'the assignee avatar helper is gone');

// 来源标记：GitHub 与 GitLab 由外部同步，其余都算平台新建，取值与来源筛选器一致。
ctx.metricsIssueSource = issue => String(issue?.source || issue?.externalRef?.source || '').toLowerCase();
for (const [issue, label, tone] of [
  [{ source: 'github' }, 'GitHub', 'github'],
  [{ source: 'gitlab' }, 'GitLab', 'github'],
  [{ source: 'local' }, '平台新建', 'local'],
  [{}, '平台新建', 'local'],
  [{ externalRef: { source: 'gitlab' } }, 'GitLab', 'github'],
]) {
  assert.strictEqual(ctx.issueSourceLabel(issue), label, JSON.stringify(issue));
  const chip = ctx.issueSourceMarkup(issue);
  assert.ok(chip.includes(`class="issue-source-chip tone-${tone}"`), chip);
  assert.ok(chip.includes(`>${label}</span>`), chip);
}

assert.strictEqual(ctx.issueKeyColumnWidth([{ key: 'AF-36' }]), '76px');
assert.strictEqual(ctx.issueKeyColumnWidth([{ key: 'AF-1036' }]), '96px');
assert.strictEqual(ctx.issueKeyColumnWidth([{ key: '1010000000100012345' }]), '120px');
assert.strictEqual(ctx.issueKeyColumnWidth([{ key: '10100000001000123456789012' }]), '120px');

console.log('issues list markup ok');
