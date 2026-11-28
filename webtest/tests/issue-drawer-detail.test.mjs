import test from 'node:test';
import assert from 'node:assert/strict';
import { loadApp, settle } from '../lib/load-app.mjs';


test('the requirement drawer has no clarification surface left', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const removed = app.evaluate(`['issueClarifying','clarificationNeedsConfirm','clarifyPane',
    'clarificationOverviewCard','openIssueClarification','sendIssueClarification','collectIssueClarifyReply',
    'issueClarifyViewKey','clarifyQuestionCardsHTML','clarifyHistoryHTML','captureIssueClarifyDraft',
    'restoreIssueClarifyDraft','setIssueClarifyTab','attachIssueClarifyStream','stopIssueClarification',
    'retryIssueClarificationWriteBack','issueClarifyThinking']
    .filter(name => typeof window[name] !== 'undefined')`);
  assert.deepEqual([...removed], []);
});

test('the requirement drawer loads the surviving tabs only', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  await app.evaluate(`openIssueDetail('issue-1')`);
  await settle(4);
  const tabs = [...app.document.querySelectorAll('#issueDetailTabs .drawer-tab')].map(b => b.dataset.dtab);
  assert.deepEqual(tabs, ['overview', 'delivery', 'activity', 'operations']);
  for (const removed of ['clarify', 'sop', 'artifacts', 'conversations', 'acceptance']) {
    assert.equal(tabs.includes(removed), false, `${removed} tab is gone`);
  }
});
