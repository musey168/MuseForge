import test from 'node:test';
import assert from 'node:assert/strict';
import { loadApp, settle } from '../lib/load-app.mjs';


test('issue clarification helpers handle thinking, protocol recovery, and action controls', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  app.evaluate(`
    const issue={...state.data.issues[0],clarification:{status:'analyzing',questions:[],history:[]}};
    state.issueDetailData={issue,comments:[],run:null,task:null,requirement:null};
    state.issueDetailTab='clarify';state.drawerOpen=true;renderIssueDetailTab();
  `);
  app.evaluate(`
    issueClarifyThinking({kind:'thinking',text:'Inspecting requirement'},'issue-1');
    issueClarifyThinking({kind:'tool',tool:'search',text:'Searching'},'issue-1');
    issueClarifyThinking({kind:'answer',text:'Draft answer'},'issue-1');
    setIssueClarifyLastError('issue-1','Temporary failure');
  `);
  const recovered = app.evaluate(`
    [
      recoverClarifyView('prefix {"summary":"Clear","questions":[],"score":88} suffix'),
      clarifyExtractProtocol('text {"result":{"summary":"Done"}}'),
      clarifyVisibleSummary('Visible summary'),
      clarifyThoughtShouldHide('submit_clarification_result payload')
    ]
  `);
  assert.ok(recovered.length === 4);
  app.evaluate(`finishIssueClarifyJob('issue-1',true);closeIssueDrawer()`);
});

test('the requirement drawer loads the surviving tabs only', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  await app.evaluate(`openIssueDetail('issue-1')`);
  await settle(4);
  const tabs = [...app.document.querySelectorAll('#issueDetailTabs .drawer-tab')].map(b => b.dataset.dtab);
  assert.deepEqual(tabs, ['overview', 'clarify', 'delivery', 'activity', 'operations']);
  for (const removed of ['sop', 'artifacts', 'conversations', 'acceptance']) {
    assert.equal(tabs.includes(removed), false, `${removed} tab is gone`);
  }
});
