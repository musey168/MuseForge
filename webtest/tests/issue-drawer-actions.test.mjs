import test from 'node:test';
import assert from 'node:assert/strict';
import { loadApp, settle } from '../lib/load-app.mjs';




test('drawer actions write back clarification and resync TAPD with API payloads', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const issue = { ...app.evaluate('structuredClone(state.data.issues[0])'), source: 'tapd',
    externalRef: { externalId: 'T-1', externalUrl: 'https://tapd.example/T-1' },
    clarification: { status: 'ready', writeBackStatus: 'pending' } };
  const written = { ...issue, clarification: { status: 'ready', writeBackStatus: 'succeeded' } };
  const synced = { ...written, title: 'Synced from TAPD' };
  app.router.post('/api/issues/issue-1/clarification', written);
  app.router.post('/api/issues/issue-1/resync-tapd', synced);
  app.router.post('/api/issues/issue-1/chat-clarify', {
    issue: { ...written, clarification: { status: 'ready', summary: 'Clarified', writeBackStatus: 'succeeded' } },
    ready: true
  });
  app.router.get('/api/issues/issue-1/chat-clarify', {}, 404);
  app.router.post('/api/issues/issue-1/chat-clarify/cancel', { ok: true });
  app.evaluate(`state.data.issues=[${JSON.stringify(issue)}];
    state.issueDetailData={issue:state.data.issues[0],comments:[],run:null,task:null,requirement:null};
    state.issueDetailTab='clarify';state.drawerOpen=true;renderIssueDetailTab()`);
  await app.evaluate(`retryIssueClarificationWriteBack()`);
  await settle(3);
  assert.equal(app.evaluate(`state.issueDetailData.issue.clarification.writeBackStatus`), 'succeeded');
  await app.evaluate(`resyncTAPDIssueById('issue-1')`);
  await settle(3);
  assert.equal(app.evaluate(`state.data.issues[0].title`), 'Synced from TAPD');
  app.evaluate(`
    state.issueDetailData.issue.clarification={status:'in_progress',questions:[
      {question:'Browser?',options:['Chrome','Safari'],multiSelect:false}],history:[]};
    state.issueDetailTab='clarify';renderIssueDetailTab();
    document.querySelector('.issue-clarify-option')?.click();
    const input=document.querySelector('#issueClarifyInput');if(input)input.value='Safari details';
  `);
  assert.match(app.evaluate(`collectIssueClarifyReply()`), /Chrome|Safari details/);
  await app.evaluate(`sendIssueClarification('Safari is required')`);
  assert.equal(app.evaluate(`state.issueDetailData.issue.clarification.status`), 'ready');
  await app.evaluate(`attachIssueClarifyStream('issue-1')`);
  app.evaluate(`
    openIssueClarification(false,false);
    const box=document.querySelector('#issueClarifyThinking')||document.body.appendChild(Object.assign(document.createElement('div'),{id:'issueClarifyThinking'}));
    appendIssueClarifyThought(box,{kind:'status',text:'Checking repository'});
    appendIssueClarifyThought(box,{kind:'tool',tool:'search',detail:'issues'});
  `);
  assert.match(app.document.querySelector('#issueClarifyThinking').textContent, /Checking repository/);
  app.evaluate(`stopIssueClarification('issue-1')`);
  await settle(2);
  const calls = app.router.calls.filter(c => c.method === 'POST');
  assert.equal(calls.find(c => c.path.endsWith('/clarification')).body.retry, true);
  assert.equal(calls.find(c => c.path.endsWith('/resync-tapd')).path, '/api/issues/issue-1/resync-tapd');
  assert.equal(calls.find(c => c.path.endsWith('/chat-clarify')).body.message, 'Safari is required');
  assert.equal(calls.find(c => c.path.endsWith('/chat-clarify')).body.model, undefined);
});

test('another clarification round waits for the responsible account catalog and sends its selected model', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const catalog = Promise.withResolvers();
  app.router.get('/api/issues/issue-1/clarify-models', () => catalog.promise);
  app.router.post('/api/issues/issue-1/chat-clarify', { ready: true });
  app.evaluate(`
    state.issueDetailData={issue:state.data.issues[0],comments:[],run:null,task:null,requirement:null};
    state.drawerOpen=true;
    openIssueClarification(false,true);
  `);
  const posted = () => app.router.calls.find(call => call.method==='POST' && call.path.endsWith('/chat-clarify'));
  await settle(2);
  assert.equal(posted(), undefined);
  catalog.resolve({engines:[{id:'codex',name:'Codex',personal:true}],models:[{id:'codex:fixture-model',engine:'codex'}]});
  await settle(6);
  assert.equal(posted()?.body.model, 'codex:fixture-model');
});
