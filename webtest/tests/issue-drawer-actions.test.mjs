import test from 'node:test';
import assert from 'node:assert/strict';
import { loadApp, settle } from '../lib/load-app.mjs';




test('the requirement drawer offers the surviving requirement actions', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const issue = { ...app.evaluate('structuredClone(state.data.issues[0])'), status: 'in_progress',
    source: 'local', closureAllowed: true, deletionAllowed: true };
  app.router.post('/api/issues/issue-1/close', { ...issue, status: 'closed' });
  app.evaluate(`window.afConfirm=async()=>true;
    state.data.issues=[${JSON.stringify(issue)}];
    state.issueDetailData={issue:state.data.issues[0],comments:[],requirement:null,images:[]};
    state.issueDetailTab='overview';state.drawerOpen=true;renderIssueDetailTab()`);
  const html = app.document.querySelector('#issueDetailBody').innerHTML;
  assert.match(html, /data-edit-issue/);
  assert.match(html, /data-close-issue/);
  assert.match(html, /data-delete-issue/);
  assert.equal(html.includes('data-push-tapd'), false, 'the TAPD tracker action is gone');
  assert.equal(html.includes('data-resync-tapd'), false, 'the TAPD tracker action is gone');
  app.document.querySelector('#issueDetailBody [data-close-issue]').click();
  await settle(4);
  assert.equal(app.router.calls.find(c => c.path.endsWith('/close'))?.method, 'POST');
});
