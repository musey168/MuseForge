import test from 'node:test';
import assert from 'node:assert/strict';
import { loadApp, settle } from '../lib/load-app.mjs';

test('issue drawer panes render the requirement, its delivery, and its activity', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const rendered = app.evaluate(`
    (() => {
      const issue={...state.data.issues[0], status:'in_progress', source:'local', tags:['bug','ui'],
        description:'Improve the dashboard',
        operations:[{action:'closed',createdAt:'2026-10-21T10:00:00Z',userName:'Tester'}]};
      const project=state.data.projects[0];
      const data={issue,repositories:state.data.repositories};
      state.issueDetailData=data;
      return [
        overviewPane(issue,data), deliveryPane(issue,data),
        operationsPane(issue), activityPane(issue,data),
        issueProgressActionsHTML(issue,{includeIssueActions:true})
      ].join('');
    })()
  `);
  assert.match(rendered, /bug、ui/);
  assert.equal(typeof sopPane, 'undefined');
  assert.equal(typeof artifactsPane, 'undefined');
  assert.equal(typeof acceptancePane, 'undefined');
});

test('the requirement list and detail no longer expose engine helpers', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const removed = app.evaluate(`[
    'normalizeAIWorkflow','parseStreamingWorkflow','shellForAIWorkflow','mergeAIEdit',
    'renderAssets','renderAssetDetail','renderProperties','clientIssueLanes','issueActor',
    'issueRunHealth','dispatchIssue','triggerIssue','openRunModal','renderSOPCatalog','renderRuns'
  ].filter(name => typeof window[name] !== 'undefined')`);
  assert.deepEqual([...removed], []);
  await settle();
});
