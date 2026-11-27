import test from 'node:test';
import assert from 'node:assert/strict';
import { loadApp, settle } from '../lib/load-app.mjs';

test('project identity, filtering, payload validation, and project form behavior are deterministic', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  app.router.put('/api/projects', (call) => call.body);
  const values = app.evaluate(`({
    identity:generateProjectIdentity('Web Console',[{id:'web-console',key:'WC'}],'seed'),
    chinese:generateProjectIdentity('控制台',[],'a1b2'),
    route:parseProjectAdminRoute('#/projects/project%201/tapd'),
    hash:projectAdminHash('project 1','repos'),
    filtered:filterAdminProjects(
      [{id:'p1',name:'Alpha',ownerId:1},{id:'p2',name:'Beta',ownerId:2}],
      [{id:'r1',projectId:'p1',name:'frontend'}],'front','all').map(p=>p.id)
  })`);
  assert.equal(JSON.stringify(values.identity), JSON.stringify({ id: 'web-console-2', key: 'WCA' }));
  assert.equal(JSON.stringify(values.chinese), JSON.stringify({ id: 'proj-a1b2', key: 'PRJ' }));
  assert.equal(JSON.stringify(values.route), JSON.stringify({ id: 'project 1', tab: 'tapd' }));
  assert.equal(values.hash, '#/projects/project%201/repos');
  assert.equal(JSON.stringify(values.filtered), JSON.stringify(['p1']));

  app.evaluate(`
    state.currentUser={id:1,loginName:'owner'};
    state.data.users=[state.currentUser,{id:2,loginName:'member'}];
    state.data.workflows.push({id:'generic-backend-delivery',name:'Generic',generic:true});
    state.projectAdminDetailId='new';state.projectAdminTab='basic';state.projectDraft=null;
    renderProjectDetail({force:true});
  `);
  const form = app.document.querySelector('#projectAdminForm');
  form.elements.name.value = 'Payments API';
  form.elements.name.dispatchEvent(new app.window.Event('input', { bubbles: true }));
  assert.equal(form.elements.id.value, 'payments-api');
  assert.equal(form.elements.key.value, 'PA');
  form.querySelector('[data-pa-color="#10b981"]').click();
  assert.equal(app.evaluate(`state.projectDraft.color`), '#10b981');
  assert.equal(app.evaluate(`setProjectAdminTab('repos')`), false);
  const payloadResult = app.evaluate(`
    (() => {
      state.data.repositories=[{id:'repo-x',projectId:'payments-api'}];
      const payload=buildProjectPayload({id:'payments-api',name:' Payments ',key:'pay',ownerId:1,
        workflowId:'generic-backend-delivery',defaultRepositoryId:'repo-x'});
      const valid=validateProjectPayload(payload,{creating:true,projects:[],workflows:state.data.workflows});
      const noOwner=validateProjectPayload({...payload,ownerId:0},
        {creating:true,projects:[],workflows:state.data.workflows});
      // The id is a path segment of /api/projects/:id: a slash or an over-long
      // id would create a project nobody can open or delete.
      const slash=validateProjectPayload({...payload,id:'wa bad/id?x'},{creating:true,projects:[],workflows:state.data.workflows});
      const long=validateProjectPayload({...payload,id:'p'.repeat(65)},{creating:true,projects:[],workflows:state.data.workflows});
      const legacy=validateProjectPayload({...payload,id:'old id'},{creating:false,saved:true,projects:[],workflows:state.data.workflows});
      const longName=generateProjectIdentity('Alpha '.repeat(20),[],'seed').id;
      return {payload,valid,noOwner,slash,long,legacy,longName};
    })()
  `);
  assert.equal(payloadResult.payload.name, 'Payments');
  assert.equal(payloadResult.payload.key, 'PAY');
  assert.ok(!('members' in payloadResult.payload), 'a project write carries no roster');
  assert.ok(!('membersVersion' in payloadResult.payload), 'a project write carries no roster token');
  assert.ok(!('businessDomainId' in payloadResult.payload), 'a project write carries no business domain');
  assert.equal(payloadResult.valid, null);
  assert.equal(payloadResult.noOwner.field, 'ownerId');
  assert.equal(payloadResult.slash.field, 'id');
  assert.match(payloadResult.slash.message, /不能包含空格/);
  assert.match(payloadResult.long.message, /64/);
  assert.equal(payloadResult.legacy, null);
  assert.ok(payloadResult.longName.length <= 56 && !payloadResult.longName.endsWith('-'), payloadResult.longName);

  app.evaluate(`
    resetProjectAdminDetail();
    state.data.projects[0].ownerId=1;state.data.projects[0].key='ATL';state.data.projects[0].workflowId='flow-1';
    state.projectAdminDetailId='project-1';state.projectAdminTab='basic';
    renderProjectDetail({force:true});
  `);
  const edit = app.document.querySelector('#projectAdminForm');
  edit.querySelector('[data-pa-ident-edit]').click();
  assert.equal(app.evaluate(`state.projectIdentCustom`), true);
  edit.querySelector('[data-pa-ident-collapse]').click();
  edit.elements.description.value = 'Updated project scope';
  edit.elements.description.dispatchEvent(new app.window.Event('input', { bubbles: true }));
  edit.dispatchEvent(new app.window.Event('submit', { bubbles: true, cancelable: true }));
  await settle(8);
  const update = app.router.calls.find(c => c.method === 'PUT' && c.path === '/api/projects');
  assert.equal(update.body.description, 'Updated project scope');
  assert.equal(app.evaluate(`state.data.projects[0].description`), 'Updated project scope');
});

test('repository drawer detects Git SSH URLs and persists a default repository', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  app.router.post('/api/repositories', (call) => ({ ...call.body, id: 'repo-new' }));
  app.router.put('/api/projects', (call) => call.body);
  app.router.delete('/api/repositories', { ok: true });
  app.evaluate(`
    state.projectAdminDetailId='project-1';
    state.projectDraftFor='project-1';
    state.projectDraft=projectDraftFrom(state.data.projects[0]);
    state.projectDraftBase=clone(state.projectDraft);
    state.data.repositories=[];
    openRepoDrawer('');
  `);
  const form = app.document.querySelector('#repoDrawerForm');
  form.elements.repoUrl.value = 'git@gitlab.example.org:platform/payments.git';
  form.elements.repoUrl.dispatchEvent(new app.window.Event('input', { bubbles: true }));
  await settle(6);
  assert.equal(app.document.querySelector('#repoDrawerSSHMissing').hidden, false);
  assert.match(app.document.querySelector('#repoDrawerSSHMissing').textContent, /可稍后/);
  assert.equal(app.document.querySelector('#repoDrawerSave').disabled, false, 'a missing key is advisory and does not block saving');
  assert.equal(app.document.querySelector('#repoDrawerDetect').hidden, false);
  assert.equal(form.elements.name.value, 'payments');
  assert.equal(app.document.querySelector('#repoDrawerSSH').hidden, false);
  form.elements.webUrl.value = 'https://gitlab.example.org/platform/payments';
  form.elements.defaultBranch.value = 'main';
  form.elements.integrateBranch.value = 'release';
  form.dispatchEvent(new app.window.Event('submit', { bubbles: true, cancelable: true }));
  await settle(8);
  const create = app.router.calls.find(c => c.method === 'POST' && c.path === '/api/repositories');
  assert.equal(create.body.provider, 'ssh');
  assert.equal(create.body.gongfengProjectId, '');
  assert.equal(create.body.cloneUrl, 'git@gitlab.example.org:platform/payments.git');
  assert.equal(create.body.webUrl, 'https://gitlab.example.org/platform/payments');
  assert.equal(create.body.defaultBranch, 'main');
  assert.equal(app.evaluate(`state.data.repositories[0].id`), 'repo-new');
  assert.equal(app.evaluate(`state.data.projects[0].defaultRepositoryId`), 'repo-new');
  assert.equal(app.document.querySelector('#repoDrawer').classList.contains('open'), false);
  app.evaluate(`window.__deleteRepo=deleteProjectRepository('repo-new')`);
  await settle(2);
  app.document.querySelector('#confirmOkBtn').click();
  await app.evaluate(`window.__deleteRepo`);
  assert.equal(app.evaluate(`state.data.repositories.length`), 0);
  assert.ok(app.router.calls.some(c => c.method === 'DELETE' && c.path === '/api/repositories'));
});


test('project owner deletes a project from its danger zone and the list moves on', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  app.router.delete('/api/projects/okr-system', (call) => ({ id: 'okr-system', archivedAt: '2026-11-01T00:00:00Z' }));
  app.evaluate(`
    afConfirm=async()=>true;
    state.authRequired=true;
    state.currentUser={id:7,loginName:'owner'};
    state.data.users=[state.currentUser];
    state.data.projects=[...state.data.projects,{id:'okr-system',name:'OKR系统',ownerId:7}];
    state.projectFilter='okr-system';
  `);
  const values = app.evaluate(`({
    owner:canDeleteProject({id:'okr-system',ownerId:7}),
    other:canDeleteProject({id:'x',ownerId:8}),
    unsaved:canDeleteProject(null),
    row:projectDeleteRow({id:'okr-system',ownerId:7}).includes('data-project-delete="okr-system"'),
    hidden:projectDeleteRow({id:'x',ownerId:8})
  })`);
  assert.deepEqual({ ...values }, { owner: true, other: false, unsaved: false, row: true, hidden: '' });
  await app.evaluate(`deleteProjectAdmin('okr-system')`);
  await settle(4);
  assert.ok(app.router.calls.some(c => c.method === 'DELETE' && c.path === '/api/projects/okr-system'));
  assert.equal(app.evaluate(`state.data.projects.some(p=>p.id==='okr-system')`), false);
  assert.notEqual(app.evaluate(`state.projectFilter`), 'okr-system');
  assert.equal(app.evaluate(`localStorage.getItem('agentflow.activeProject')`), app.evaluate(`state.projectFilter`));
  assert.equal(app.evaluate(`state.projectAdminDetailId||''`), '');
});

test('the project view exposes no membership controls', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  app.router.put('/api/projects', (call) => call.body);
  app.evaluate(`
    state.authRequired=true;
    state.currentUser={id:1,loginName:'owner'};
    state.data.users=[state.currentUser,{id:2,loginName:'member'}];
    state.data.projects=[{...state.data.projects[0],key:'ATL',workflowId:'flow-1',ownerId:1}];
    state.projectAdminDetailId='project-1';state.projectAdminTab='basic';state.projectDraft=null;
    renderProjectDetail({force:true});
    renderProjectList();
  `);
  const form = app.document.querySelector('#projectAdminForm');
  assert.ok(form, 'the project settings form is rendered');
  for (const marker of ['data-member-row', 'member-role', 'data-remove-member', 'data-pa-join-box', '加入申请', '项目成员']) {
    assert.equal(form.innerHTML.includes(marker), false, `the settings page has no ${marker}`);
  }
  assert.equal(form.querySelector('[data-pa-panel="members"]'), null, 'there is no members panel');
  assert.deepEqual(
    [...app.document.querySelectorAll('#projectAdminTabs [data-pa-tab]')].map((b) => b.dataset.paTab),
    ['basic', 'repos', 'tapd'],
    'the settings page has no members tab',
  );
  const list = app.document.querySelector('#projectAdminList');
  assert.equal(list.innerHTML.includes('data-project-join'), false, 'the project card offers no join request');
  assert.equal(list.innerHTML.includes('pa-av'), false, 'the project card shows no member avatars');
  assert.equal(list.innerHTML.includes('位成员'), false, 'the project card shows no member count');
  const filters = [...app.document.querySelectorAll('#projectAdminFilters [data-project-filter]')].map((b) => b.dataset.projectFilter);
  assert.deepEqual(filters, ['all', 'setup'], 'the toolbar has no membership filter');
});
