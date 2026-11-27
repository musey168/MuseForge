import test from 'node:test';
import assert from 'node:assert/strict';
import { loadApp } from '../lib/load-app.mjs';

test('the requirement model derives stages, outcomes, and action markup from the requirement alone', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const result = structuredClone(app.evaluate(`(() => {
    const base={id:'i1',projectId:'p1',kind:'requirement',source:'local',status:'backlog',
      title:'需求',assigneeUserId:1};
    state.data.projects=[{id:'p1',name:'项目'}];
    state.data.issues=[base];
    const mrIssue={...base,deliveryState:'merged',deliveryMrState:'merged',
      deliveryMrs:[{url:'https://git.example.com/team/repo/-/merge_requests/17',state:'merged',repository:'repo'}]};
    const closed={...base,status:'closed',closeSource:'manual'};
    return {
      stages:{
        backlog:issueCurrentStage(base).label,
        inProgress:issueCurrentStage({...base,status:'in_progress'}).label,
        blocked:issueCurrentStage({...base,status:'blocked'}).key,
        done:issueCurrentStage({...base,status:'done'}).key,
        closed:issueCurrentStage(closed).key,
        clarifying:issueCurrentStage({...base,clarification:{status:'analyzing'}}).label,
      },
      outcomes:{
        backlog:issueOutcome(base).key,
        closed:issueOutcome(closed).title,
        merged:issueOutcome(mrIssue).key,
      },
      actions:{
        closed:issueActionButtons(closed),
        open:issueActionButtons(base),
      },
      owner:issueOwnerSummary(base),
      mrList:deliveryMRList(mrIssue),mrLabel:deliveryLockedLabel(mrIssue),
      mrButtons:deliveryMRButtons(mrIssue),
      reopen:reopenDeliveredButton(closed),
      removed:['clientIssueLanes','issueActor','issueRunHealth','triggerSOPButton','canRetriggerIssue',
               'normalizeSOPLane','inferSOPLane','projectSOPBindings','normalizePlaybookList','issueLaneDots']
        .filter(name=>typeof window[name]!=='undefined'),
    };
  })()`));
  assert.equal(result.stages.blocked, 'attention');
  assert.equal(result.stages.done, 'closed');
  assert.equal(result.stages.closed, 'closed');
  assert.equal(result.outcomes.closed, '需求已关闭');
  assert.equal(result.outcomes.merged, 'closed');
  assert.equal(result.mrList[0].iid, 17);
  assert.equal(result.mrLabel, 'MR 已合入');
  assert.match(result.mrButtons, /merge_requests\/17/);
  assert.match(result.reopen, /重新打开/);
  assert.equal(result.owner, '未指定');
  assert.deepEqual([...result.removed], []);
});
