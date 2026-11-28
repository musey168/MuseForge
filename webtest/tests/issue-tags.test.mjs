import test from 'node:test';
import assert from 'node:assert/strict';
import { loadApp, settle, chooseOption } from '../lib/load-app.mjs';


test('choosing a tag in the list filter narrows the requirement list', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  await settle(3);
  app.evaluate(`state.data.issues=[
    {id:'a',projectId:'project-1',title:'一',status:'backlog',tags:['bug']},
    {id:'b',projectId:'project-1',title:'二',status:'backlog'}
  ];
  state.projectFilter='project-1';state.issueTagFilter='';renderIssues();`);
  chooseOption(app.document.querySelector('#issueTagFilter'), 'bug');
  await settle(2);
  assert.equal(app.evaluate('state.issueTagFilter'), 'bug');
  assert.deepEqual([...app.evaluate(`[...document.querySelectorAll('.issue-table-row[data-id]')].map(r=>r.dataset.id)`)], ['a']);
});


test('the requirement form takes tags and the requirement keeps them', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const tags = app.evaluate(`(() => {
    openIssue(null);
    const form=document.querySelector('#issueForm');
    form.elements.title.value='带标签的需求';
    form.elements.tags.value='bug, ui， bug';
    return issueFromForm().tags;
  })()`);
  assert.deepEqual([...tags], ['bug', 'ui']);
  const filled = app.evaluate(`(() => {
    openIssue({id:'i-1',projectId:'project-1',title:'已有标签',status:'backlog',tags:['ui','ui','前端']});
    return document.querySelector('#issueForm').elements.tags.value;
  })()`);
  assert.equal(filled, 'ui, 前端');
});

test('the requirement list filters by tag and shows the tags', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const result = app.evaluate(`(() => {
    state.data.issues=[
      {id:'a',projectId:'project-1',title:'一',status:'backlog',tags:['bug']},
      {id:'b',projectId:'project-1',title:'二',status:'backlog',tags:['ui','bug']},
      {id:'c',projectId:'project-1',title:'三',status:'backlog'}
    ];
    state.projectFilter='project-1';
    state.issueTagFilter='';
    const all=filteredIssues().map(i=>i.id);
    state.issueTagFilter='bug';
    const bug=filteredIssues().map(i=>i.id);
    state.issueTagFilter='';
    renderIssues();
    const options=[...document.querySelectorAll('#issueTagFilter option')].map(o=>o.value);
    const row=document.querySelector('.issue-table-row[data-id]').innerHTML;
    return {all,bug,options,row};
  })()`);
  assert.deepEqual([...result.all], ['a', 'b', 'c']);
  assert.deepEqual([...result.bug], ['a', 'b']);
  assert.deepEqual([...result.options], ['', 'bug', 'ui']);
  assert.match(result.row, /issue-title-tag[^>]*>bug</);
});

test('the requirement detail shows the tags', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const html = app.evaluate(`(() => {
    const issue={...state.data.issues[0],status:'in_progress',source:'local',tags:['bug','ui']};
    return overviewPane(issue,{issue,comments:[],images:[]});
  })()`);
  assert.match(html, /标签/);
  assert.match(html, /bug、ui/);
});
