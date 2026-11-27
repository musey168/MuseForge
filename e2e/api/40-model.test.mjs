// Scenarios that call the real model with the token owner's CodeBuddy key:
// an agent node run, AI run evaluation and the AI assistant. Skipped with
// AGENTFLOW_E2E_MODEL=0.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { api, get, post, put, del, list, cleanup, ensureProject, waitRun, waitFor, nodeOf, track } from '../lib/api.mjs';
import { cfg } from '../lib/env.mjs';
import { createWorkflow, createAgent, agentNode, script, startRun, repoSkip, childSOPHandoffScenario } from '../lib/fixtures.mjs';

const skip = cfg.model ? false : 'AGENTFLOW_E2E_MODEL=0';
let agentRunId;

before(ensureProject);
after(cleanup);

test('agent node: the real executor writes the declared artifact', { skip, timeout: 900_000 }, async () => {
  const agentId = await createAgent('writer', [
    '你是 e2e 自动化测试角色。严格按以下步骤执行，不要做任何额外的事：',
    '1. 在当前工作目录创建文件 answer.md，内容只有一行：MODEL-OK',
    '2. 回复一句话说明已完成。',
    '不要访问网络，不要安装依赖，不要修改其他文件。',
  ].join('\n'));
  const wf = await createWorkflow('model', [
    script('seed', 'echo "seed" > seed.txt'),
    agentNode('write', agentId, { artifactOutputs: [{ name: 'answer', path: 'answer.md', mediaType: 'text/markdown', required: true }] }),
    script('verify', 'grep -q MODEL-OK answer.md && echo VERIFY_OK'),
  ], { sandboxProfileId: cfg.modelSandbox });
  const { id } = await startRun(wf, 'e2e 真实模型执行');
  agentRunId = id;
  const run = await waitRun(id, r => r.status === 'succeeded', { label: 'succeeded', timeoutMs: 840_000 });
  const node = nodeOf(run, 'write');
  assert.equal(node.status, 'succeeded');
  assert.ok((run.usage?.totalTokens || node.usage?.totalTokens || 0) > 0, `no token usage recorded: ${JSON.stringify(run.usage)}`);
  const artifact = (run.artifacts || []).find(a => a.name === 'answer');
  assert.ok(artifact, 'declared artifact missing');
  const body = await api('GET', `/api/runs/${id}/artifacts/${encodeURIComponent(artifact.id)}`, undefined, { raw: true });
  assert.match(body.text, /MODEL-OK/);
  const logs = await api('GET', `/api/runs/${id}/logs.txt`, undefined, { raw: true });
  assert.match(logs.text, /VERIFY_OK/);
  const telemetry = await get(`/api/runs/${id}/telemetry?nodeId=write`);
  assert.ok((telemetry.events || []).length > 0, 'agent node produced no telemetry');
  if (run.timing) assert.ok(run.timing.executionMs > 0);
});

// 真实 Agent 在子 LOOP 里改父流程检出的代码，父流程下一个节点能直接读到；节点不提交、不推送。
test('agent node in a child LOOP edits the parent checkout', { skip: skip || repoSkip, timeout: 900_000 }, async () => {
  const agentId = await createAgent('repo-writer', [
    '你是 e2e 自动化测试角色。严格按以下步骤执行，不要做任何额外的事：',
    '1. 在当前工作目录下找到代码仓库子目录（里面有 parent.txt 文件）。',
    '2. 在 parent.txt 所在的同一个目录创建文件 child.txt，内容只有一行：child',
    '3. 不要 git add、git commit 或 git push，不要修改其他文件，不要访问网络。',
    '4. 回复一句话说明已完成。',
  ].join('\n'));
  const { run } = await childSOPHandoffScenario({
    label: 'model-repo-sop', waitRun, timeoutMs: 840_000, extra: { sandboxProfileId: cfg.modelSandbox },
    childNodes: [agentNode('edit', agentId)],
  });
  assert.equal(run.status, 'succeeded');
});

test('AI run evaluation completes for a finished run', { skip, timeout: 600_000 }, async () => {
  assert.ok(agentRunId, 'depends on the agent node run');
  await post(`/api/runs/${agentRunId}/eval`, {}, { expect: [200, 201, 202] });
  await api('POST', `/api/runs/${agentRunId}/eval`, {}, { expect: [200, 201, 202, 409] });
  const done = await waitFor('evaluation to finish', async () => {
    const evals = list(await get(`/api/runs/${agentRunId}/evals`));
    const latest = evals[0];
    return latest && latest.status !== 'running' ? latest : null;
  }, { timeoutMs: 540_000, intervalMs: 5_000 });
  assert.equal(done.status, 'succeeded', `evaluation ended ${done.status}: ${done.error || ''}`);
  const current = await get(`/api/runs/${agentRunId}/eval`);
  assert.ok(current);
});

test('evaluation refuses a run that is still active', { skip, timeout: 120_000 }, async () => {
  const wf = await createWorkflow('eval-active', [script('slow', 'sleep 30')]);
  const { id } = await startRun(wf);
  await waitRun(id, r => r.nodes?.slow?.status === 'running', { label: 'running' });
  await api('POST', `/api/runs/${id}/eval`, {}, { expect: 409 });
  await post(`/api/runs/${id}/cancel`, {});
});



// A draft is applied by the chat UI with the platform MCP tools/call, the same
async function applyDraft(draft) {
  const args = {};
  for (const key of ['path', 'query', 'body']) if (draft[key]) args[key] = draft[key];
  const res = await api('POST', '/mcp', { jsonrpc: '2.0', id: Date.now(), method: 'tools/call', params: { name: draft.tool, arguments: args } }, {
    headers: { Accept: 'application/json, text/event-stream' }, raw: true, expect: [200, 202],
  });
  const json = res.text.trim().startsWith('{') ? JSON.parse(res.text) : JSON.parse((res.text.match(/^data: (.*)$/m) || [])[1] || '{}');
  assert.ok(!json.error && !json.result?.isError, `apply ${draft.tool}: ${res.text.slice(0, 300)}`);
  return json.result;
}


