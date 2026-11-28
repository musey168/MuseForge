import test from 'node:test';
import assert from 'node:assert/strict';
import { loadApp } from '../lib/load-app.mjs';

test('the project header counts the requirements and the ones still open', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const result = await app.evaluate(`(() => {
    state.data.projects=[{id:'project-1',key:'AF',name:'AgentFlow'}];
    state.data.repositories=[];
    state.data.issues=[
      {id:'a',projectId:'project-1',number:1,title:'one',status:'in_progress'},
      {id:'b',projectId:'project-1',number:2,title:'two',status:'done'},
      {id:'c',projectId:'project-1',number:3,title:'three',status:'blocked'},
      {id:'d',projectId:'project-1',number:4,title:'four',status:'todo'}
    ];
    state.projectFilter='project-1';
    state.issueDeliveryFilter='';
    renderProjectContext(state.data.issues);
    const metrics=$('#projectContextMetrics').innerHTML;
    state.issueDeliveryFilter='open';
    const open=filteredIssues().map(i=>i.id);
    state.issueDeliveryFilter='closed';
    const closed=filteredIssues().map(i=>i.id);
    state.issueDeliveryFilter='';
    return {metrics,open,closed};
  })()`);
  assert.match(result.metrics, /<span>\u9700\u6c42<\/span><b>4<\/b>/);
  assert.match(result.metrics, /<span>\u672a\u5b8c\u6210<\/span><b>3<\/b>/);
  assert.equal(result.metrics.includes('MR'), false);
  assert.deepEqual([...result.open].sort(), ['a', 'c', 'd']);
  assert.deepEqual([...result.closed], ['b']);
});