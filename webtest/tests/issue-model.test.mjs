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
    const closed={...base,status:'closed',closeSource:'manual'};
    return {
      stages:{
        backlog:issueCurrentStage(base).key,
        backlogLabel:issueCurrentStage(base).label,
        todo:issueCurrentStage({...base,status:'todo'}).key,
        inProgress:issueCurrentStage({...base,status:'in_progress'}).key,
        inProgressLabel:issueCurrentStage({...base,status:'in_progress'}).label,
        inReview:issueCurrentStage({...base,status:'in_review'}).key,
        blocked:issueCurrentStage({...base,status:'blocked'}).key,
        done:issueCurrentStage({...base,status:'done'}).key,
        closed:issueCurrentStage(closed).key,
      },
      outcomes:{
        backlog:issueOutcome(base).key,
        closed:issueOutcome(closed).title,
        done:issueOutcome({...base,status:'done'}).title,
      },
      actions:{
        closed:issueActionButtons(closed),
        open:issueActionButtons(base),
      },
      owner:issueOwnerSummary(base),
      reopen:reopenIssueButton(closed),
      reopenOpen:reopenIssueButton(base),
      removed:['clientIssueLanes','issueActor','issueRunHealth','triggerSOPButton','canRetriggerIssue',
               'normalizeSOPLane','inferSOPLane','projectSOPBindings','normalizePlaybookList','issueLaneDots',
               'issueClarifying','clarificationNeedsConfirm','issueTrackerClosed','issueMergedDelivery',
               'issueManuallyClosed','deliveryLocked','gongfengIssueClosed',
               'deliveryMRLinked','pendingDeliveryMRCount','deliveryMRList','deliveryMRButtons',
               'deliveryLockedLabel','reopenDeliveredButton','deliveryPane']
        .filter(name=>typeof window[name]!=='undefined'),
    };
  })()`));
  assert.equal(result.stages.backlog, 'not_started');
  assert.equal(result.stages.backlogLabel, '待规划');
  assert.equal(result.stages.todo, 'not_started');
  assert.equal(result.stages.inProgress, 'active');
  assert.equal(result.stages.inProgressLabel, '进行中');
  assert.equal(result.stages.inReview, 'active');
  assert.equal(result.stages.blocked, 'attention');
  assert.equal(result.stages.done, 'closed');
  assert.equal(result.stages.closed, 'closed');
  assert.equal(result.outcomes.backlog, 'idle');
  assert.equal(result.outcomes.closed, '需求已关闭');
  assert.equal(result.outcomes.done, '需求已完成');
  assert.match(result.actions.open, /查看/);
  assert.equal(result.actions.closed.includes('查看'), false, 'a closed requirement shows only its menu');
  assert.match(result.reopen, /重新打开/);
  assert.equal(result.reopenOpen, '', 'an open requirement has nothing to reopen');
  assert.equal(result.owner, '未指定');
  assert.deepEqual([...result.removed], []);
});