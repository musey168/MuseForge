function setIssueView(view, rerender=true){
  state.issueView=view==='kanban'?'kanban':'table';
  localStorage.setItem('agentflow.issueView',state.issueView);
  $$('[data-issue-view]').forEach(b=>b.classList.toggle('active',b.dataset.issueView===state.issueView));
  if(rerender&&state.data)renderIssues();
}

function enhanceIssueFilters(){
  ['#issueStatusFilter','#sourceFilter','#issueTagFilter'].forEach(sel=>window.enhanceSelect?.($(sel)));
}

function renderIssueTagFilter(projectIssues=[]){
  const sel=$('#issueTagFilter');if(!sel)return;
  const current=state.issueTagFilter||'';
  const counts=new Map();
  projectIssues.forEach(i=>issueTagList(i).forEach(tag=>counts.set(tag,(counts.get(tag)||0)+1)));
  const tags=[...counts.entries()].sort((a,b)=>b[1]-a[1]||a[0].localeCompare(b[0]));
  const meta=n=>n?` data-meta="${n} 条"`:'';
  let html='<option value="">全部标签</option>';
  for(const [tag,count] of tags)html+=`<option value="${escapeHTML(tag)}"${meta(count)}${current===tag?' selected':''}>${escapeHTML(tag)}</option>`;
  sel.innerHTML=html;
  sel.value=current;
}

function normalizeIssueTitleSearch(value){
  return String(value||'').normalize('NFKC').toLocaleLowerCase().replace(/[\s\p{P}\p{S}]+/gu,'');
}

function issueMatchesQuery(issue,project,query){
  const raw=String(query||'').trim().toLocaleLowerCase();
  if(!raw)return true;
  const title=String(issue?.title||''),normalizedTitle=normalizeIssueTitleSearch(title),normalizedQuery=normalizeIssueTitleSearch(raw);
  const titleTokens=raw.split(/[\s\p{P}\p{S}]+/u).map(normalizeIssueTitleSearch).filter(Boolean);
  if(normalizedQuery&&normalizedTitle.includes(normalizedQuery))return true;
  if(titleTokens.length&&titleTokens.every(token=>normalizedTitle.includes(token)))return true;
  const key=`${project?.key||'ISS'}-${issue?.number||''}`,external=issue?.externalRef?.externalId||issue?.externalRef?.externalIid||'';
  return `${key} ${title} ${issue?.description||''} ${external}`.toLocaleLowerCase().includes(raw);
}

function filteredIssues(){
  const projects=state.data.projects||[];
  let issues=(state.data.issues||[]).filter(i=>!state.projectFilter||i.projectId===state.projectFilter);
  if(state.sourceFilter)issues=issues.filter(i=>(i.source||'local')===state.sourceFilter);
  if(state.issueStatusFilter)issues=issues.filter(i=>issueMatchesStatusFilter(i,state.issueStatusFilter));
  if(state.issueTagFilter)issues=issues.filter(i=>issueTagList(i).includes(state.issueTagFilter));
  const issueFilter=state.issueDeliveryFilter||'';
  if(issueFilter==='open')issues=issues.filter(i=>issueOpen(i));
  else if(issueFilter==='closed')issues=issues.filter(i=>!issueOpen(i));
  else if(issueFilter==='blocked')issues=issues.filter(i=>issueStatusKey(i)==='blocked');
  else if(issueFilter==='progress')issues=issues.filter(i=>issueInProgress(i));
  if(state.issueQuery){
    issues=issues.filter(i=>issueMatchesQuery(i,projects.find(x=>x.id===i.projectId),state.issueQuery));
  }
  const priority={urgent:0,high:1,medium:2,low:3};
  return [...issues].sort((a,b)=>(priority[a.priority]??4)-(priority[b.priority]??4)||(b.number||0)-(a.number||0));
}

function renderIssues(){
  const projectIssues=(state.data.issues||[]).filter(i=>!state.projectFilter||i.projectId===state.projectFilter);
  renderProjectContext(projectIssues);
  renderIssueTagFilter(projectIssues);
  const statusSel=$('#issueStatusFilter');if(statusSel&&statusSel.value!==state.issueStatusFilter)statusSel.value=state.issueStatusFilter||'';
  const issues=filteredIssues();
  syncIssueSelection(issues);
  const paged=state.issueView==='kanban'?null:paginateList('issues',issues);
  const visibleIssues=paged?paged.items:issues;
  renderIssueBulkActions(visibleIssues);
  $$('[data-issue-view]').forEach(b=>b.classList.toggle('active',b.dataset.issueView===state.issueView));
  const board=$('#issueBoard');
  board.classList.toggle('kanban-mode',state.issueView==='kanban');
  if(state.issueView==='kanban'){
    board.innerHTML=renderIssueKanban(issues);
    bindIssueKanban();
    bindIssueSelection(visibleIssues);
    return;
  }
  const emptyTitle='当前项目没有匹配的工作项';
  const emptyHint='从 GitHub 或 GitLab 导入，或在本平台新建需求。';
  const keyWidth=issueKeyColumnWidth(paged.items);
  const pager=pagerHTML('issues',paged)||(paged.total?`<nav class="pager pager-summary" aria-label="分页"><span class="pager-meta">共 ${paged.total} 条</span></nav>`:'');
  board.innerHTML=`<div class="issue-table-card"><div class="issue-table-scroll"><div class="issue-table${state.issueSelectionMode?' issue-table-selectable':''}" data-page-anchor="issues" style="--issue-col-key:${keyWidth}"><div class="issue-table-row issue-table-head" role="row">${state.issueSelectionMode?`<div class="issue-select-cell">${issueSelectAllCheckbox('全选本页可删除的需求')}</div>`:''}<span>编号</span><span>标题</span><span>来源</span><span>仓库</span><span>优先级</span><span>状态</span><span>操作</span></div><div class="issue-table-body">${paged.items.map(issueRow).join('')||`<div class="issue-table-empty">${huiIcon('search')}<b>${emptyTitle}</b><p>${emptyHint}</p></div>`}</div></div></div>${pager}</div>`;
  $$('.issue-table-row[data-id]').forEach(el=>el.onclick=()=>openIssueDetail(el.dataset.id));
  bindIssueRowActions(board);
  bindPager(board);
  bindIssueSelection(visibleIssues);
}

function issueKanbanCard(i){
  const stage=issueCurrentStage(i);
  const key=issueKeyLabel(i);
  const cardTone=stage.key==='attention'?' needs-attention':'';
  return `<article class="kanban-card${cardTone}" draggable="true" data-id="${escapeHTML(i.id)}">
    <div class="kanban-card-top"><b>${escapeHTML(key)}</b>${issueSelectionCheckbox(i)}</div>
    <h4 title="${escapeHTML(i.title)}">${issueTagsMarkup(i)}${issueTitleMarkup(i.title)}</h4>
    <div class="kanban-card-meta">
      ${issuePriorityMarkup(i.priority)}
      <span class="kanban-stage ${stage.key}" title="${escapeHTML(stage.hint||'')}">${issueStageBadge(stage)}</span>
    </div>
    <div class="kanban-card-actions">${issueActionButtons(i)}</div>
  </article>`;
}

function renderIssueKanban(issues){
  return `<div class="kanban-board">${issueStatuses.map(([status])=>{
    const col=issues.filter(i=>(i.status||'backlog')===status);
    return `<section class="kanban-column" data-status="${status}">
      <header>${huiIcon(issueStatusIcon(status))}<b>${issueStatusLabel(status)}</b><span>${col.length}</span></header>
      <div class="kanban-column-body">${col.map(issueKanbanCard).join('')||`<div class="kanban-empty">${huiIcon('check')}暂无卡片</div>`}</div>
    </section>`;
  }).join('')}</div>`;
}

let kanbanDragMoved=false;
function bindIssueKanban(){
  $$('.kanban-card').forEach(card=>{
    card.ondragstart=e=>{
      kanbanDragMoved=false;
      e.dataTransfer.setData('text/plain',card.dataset.id);
      e.dataTransfer.effectAllowed='move';
      card.classList.add('dragging');
    };
    card.ondrag=()=>{kanbanDragMoved=true};
    card.ondragend=()=>card.classList.remove('dragging');
    card.onclick=()=>{if(kanbanDragMoved){kanbanDragMoved=false;return}openIssueDetail(card.dataset.id)};
  });
  bindIssueRowActions($('#issueBoard'));
  $$('.kanban-column').forEach(col=>{
    col.ondragover=e=>{e.preventDefault();e.dataTransfer.dropEffect='move';col.classList.add('drop-target')};
    col.ondragleave=e=>{if(!col.contains(e.relatedTarget))col.classList.remove('drop-target')};
    col.ondrop=async e=>{
      e.preventDefault();
      col.classList.remove('drop-target');
      const id=e.dataTransfer.getData('text/plain');
      const issue=(state.data.issues||[]).find(i=>i.id===id);
      const next=col.dataset.status;
      if(!issue||(issue.status||'backlog')===next)return;
      const prev=issue.status;
      issue.status=next;
      renderIssues();
      try{
        await persistIssue(issue);
        toast('状态已更新','ok');
      }catch(err){
        issue.status=prev;
        renderIssues();
        toast(err.message,'err');
      }
    };
  });
}
function issueStatusLabel(value){return ({backlog:'待规划',todo:'待处理',in_progress:'进行中',in_review:'评审中',done:'已完成',blocked:'已阻塞',cancelled:'已取消',closed:'已关闭'}[value]||value||'—')}
// A requirement's own status drives its stage: no run, no flow and no tracker
// state can move it.
function issueCurrentStage(i){
  const status=String(i?.status||'').toLowerCase();
  if(status==='blocked')return{key:'attention',label:'已阻塞',hint:'需求被标记为阻塞，需要处理'};
  if(status==='done'||status==='closed')return{key:'closed',label:issueStatusLabel(status),hint:status==='done'?'需求已完成':'需求已关闭'};
  if(status==='in_progress'||status==='in_review')return{key:'active',label:issueStatusLabel(status),hint:'需求正在推进'};
  return{key:'not_started',label:issueStatusLabel(status),hint:'需求尚未开始'};
}
// A requirement stays open until it is done, closed or cancelled.
function issueOpen(i){return !['done','closed','cancelled'].includes(String(i?.status||'').toLowerCase())}
// A requirement is in progress while it is being worked on or reviewed.
function issueInProgress(i){return ['in_progress','in_review'].includes(String(i?.status||'').toLowerCase())}
function issueStatusKey(i){return String(i?.status||'').toLowerCase()}
function issueStatusIcon(value){return ({backlog:'circle',todo:'clock',in_progress:'user-hold',in_review:'pending-review',done:'check',closed:'close',blocked:'alert',cancelled:'close'}[value]||'circle')}
function huiIcon(name,cls=''){return `<svg class="agent-icon harness-icon${cls?` ${cls}`:''}" data-icon="${escapeHTML(name)}" width="14" height="14" aria-hidden="true" focusable="false"></svg>`}
// stage.key alone is not enough: 「已关闭」 shares closed with a finished requirement.
function issueStageTone(stage){
  const key=stage?.key||'',label=String(stage?.label||'');
  if(key==='active')return{tone:'info',icon:'lamp'};
  if(key==='attention')return{tone:'error',icon:'alert'};
  if(key==='closed')return label==='已完成'?{tone:'success',icon:'check'}:{tone:'neutral',icon:'close'};
  return {tone:'neutral',icon:'circle'};
}
function issueStageBadge(stage){
  const {tone,icon}=issueStageTone(stage);
  const mark=icon==='lamp'?'<span class="hui-lamp" aria-hidden="true"><i></i></span>':huiIcon(icon);
  return `<span class="hui-badge" data-tone="${tone}" data-stage-icon="${icon}">${mark}<span class="hui-badge-label">${escapeHTML(stage?.label||'')}</span></span>`;
}
function issuePriorityMarkup(value){
  const pri=value||'medium';
  const icon={urgent:'priority-bolt',high:'priority-high',medium:'priority-medium',low:'priority-low'}[pri]||'priority-medium';
  return `<span class="issue-priority ${escapeHTML(pri)}">${huiIcon(icon)}<span>${issuePriorityLabel(pri)}</span></span>`;
}
// 来源：GitHub、工蜂与 TAPD 由外部同步，其余都算平台新建；取值与来源筛选器保持一致。
function issueSourceLabel(i){
  const s=metricsIssueSource(i);
  if(s==='github')return 'GitHub';
  if(s==='gitlab')return 'GitLab';
  return '平台新建';
}
function issueSourceMarkup(i){
  const s=metricsIssueSource(i)||'local';
  const tone=s==='github'||s==='gitlab'?'github':'local';
  const label=issueSourceLabel(i);
  return `<span class="issue-source-chip tone-${tone}" title="需求来源：${escapeHTML(label)}">${escapeHTML(label)}</span>`;
}
function issueStatusText(issue){
  return issueStatusLabel(issueStatusBucket(issue));
}
function issueStatusBucket(issue){
  if(!issue)return 'backlog';
  return issue.status||'backlog';
}
function issueMatchesStatusFilter(issue,filter){
  return !filter||issueStatusBucket(issue)===filter;
}
function issuePriorityLabel(value){return ({urgent:'紧急',high:'高',medium:'中',low:'低'}[value]||value||'—')}
function issueKeyLabel(i){
  if(i.source==='gongfeng'&&i.externalRef?.externalIid) return `#${i.externalRef.externalIid}`;
  if(i.source==='tapd'&&i.externalRef?.externalId) return String(i.externalRef.externalId);
  const p=(state.data.projects||[]).find(x=>x.id===i.projectId);
  return `${p?.key||'ISS'}-${i.number||''}`;
}
function issueKeyMarkup(i){
  const raw=String(issueKeyLabel(i)||'');
  const m=raw.match(/^(.*?)(\d+)$/);
  if(!m) return `<span class="issue-key-num">${escapeHTML(raw)}</span>`;
  return `<span class="issue-key-prefix">${escapeHTML(m[1])}</span><span class="issue-key-num">${escapeHTML(m[2])}</span>`;
}
function issueKeyColumnWidth(issues){
  let maxLen=0;
  for(const i of (issues||[])){
    const len=String(issueKeyLabel(i)||'').length;
    if(len>maxLen)maxLen=len;
  }
  if(maxLen>=10)return '120px';
  if(maxLen>=7)return '96px';
  return '76px';
}
function issueTagTone(tag){
  const t=String(tag||'').toLowerCase();
  if(/bug|缺陷|故障/.test(t)) return 'danger';
  if(/巡检|监控|告警/.test(t)) return 'warn';
  if(/mr|审查|review/.test(t)) return 'accent';
  if(/前端|ui|交互/.test(t)) return 'cyan';
  return 'neutral';
}
// 需求标签：一条需求可以带多个标签，列表按标签筛选，详情里也显示它们。
function issueTagList(i){
  const raw=Array.isArray(i?.tags)?i.tags:(typeof i?.tags==='string'?i.tags.split(/[,，;；\s]+/):[]);
  return [...new Set(raw.map(t=>String(t||'').trim()).filter(Boolean))];
}
function issueTagsMarkup(i){
  return issueTagList(i).map(t=>`<span class="issue-title-tag tone-${issueTagTone(t)}" title="${escapeHTML(t)}">${escapeHTML(t)}</span>`).join('');
}
function parseIssueTags(value){
  const raw=Array.isArray(value)?value:String(value||'').split(/[,，;；\s]+/);
  return [...new Set(raw.map(t=>String(t||'').trim()).filter(Boolean))].slice(0,12);
}
function issueTitleMarkup(title){
  const tags=[];
  let rest=String(title||'');
  const re=/^\s*(?:\[([^[\]【】]{1,24})\]|【([^[\]【】]{1,24})】)\s*/;
  let m;
  while((m=rest.match(re))){tags.push(m[1]||m[2]);rest=rest.slice(m[0].length)}
  rest=rest.trim()||title||'';
  const pills=tags.map(t=>`<span class="issue-title-tag tone-${issueTagTone(t)}" title="${escapeHTML(t)}">${escapeHTML(t)}</span>`).join('');
  return `${pills}<b title="${escapeHTML(title||'')}">${escapeHTML(rest)}</b>`;
}
function issueRow(i){
  const p=(state.data.projects||[]).find(x=>x.id===i.projectId);
  const boundRepos=(state.data.repositories||[]).filter(x=>(i.repositoryIds||[]).includes(x.id)||x.id===i.repositoryId||(!((i.repositoryIds||[]).length||i.repositoryId)&&x.id===p?.defaultRepositoryId));
  const issueRepos=[boundRepos.find(x=>x.id===i.repositoryId),...boundRepos.filter(x=>x.id!==i.repositoryId)].filter(Boolean);
  const repo=issueRepos[0],repoLabel=issueRepos.length>1?`${repo?.name||'未绑定'} 等${issueRepos.length}个`:repo?.name||'未绑定';
  const stage=issueCurrentStage(i);
  const rowTone=stage.key==='attention'?' needs-attention':'';
  const pri=i.priority||'medium';
  const stageCell=`<div class="issue-stage-cell"><span class="issue-stage-pill ${stage.key}" title="${escapeHTML(stage.hint||'')}">${escapeHTML(stage.label)}</span></div>`;
  return `<div class="issue-table-row${rowTone}" role="row" data-id="${escapeHTML(i.id)}">${state.issueSelectionMode?`<div class="issue-select-cell">${issueSelectionCheckbox(i)}</div>`:''}<div class="issue-key-cell" title="${escapeHTML(String(issueKeyLabel(i)||''))}">${issueKeyMarkup(i)}</div><div class="issue-title-cell">${issueTagsMarkup(i)}${issueTitleMarkup(i.title)}</div><div class="issue-source-cell">${issueSourceMarkup(i)}</div><div class="issue-repository-cell" title="${escapeHTML(issueRepos.map(r=>r.name).join('、')||'未绑定')} · ${escapeHTML(i.baseBranch||repo?.defaultBranch||'')}">${escapeHTML(repoLabel)}</div><div class="issue-priority-cell">${issuePriorityMarkup(i.priority)}</div>${stageCell}<div class="issue-actions-cell">${issueActionButtons(i)}</div></div>`;
}
let refreshIssuesPending=null;
// One fetch at a time, but never latched: the refresh button, a view change and
// a write all have to see the current list.
async function refreshIssues(){
  if(refreshIssuesPending)return refreshIssuesPending;
  refreshIssuesPending=(async()=>{
    try{
      const issues=await api('/api/issues',{silent:true});
      state.data.issues=issues;
      renderIssues();
      if(state.drawerOpen&&state.issueDetailData?.issue?.id){
        const curId=state.issueDetailData.issue.id,updatedIssue=issues.find(i=>i.id===curId);
        if(updatedIssue){state.issueDetailData.issue=Object.assign({},state.issueDetailData.issue,updatedIssue);renderIssueDetailTab()}
      }
    }catch(e){}
    finally{refreshIssuesPending=null}
  })();
  return refreshIssuesPending;
}
function fillIssueRepositories(projectId,selected){
  const f=$('#issueForm'),project=(state.data.projects||[]).find(p=>p.id===projectId),repos=(state.data.repositories||[]).filter(r=>r.projectId===projectId),box=$('#issueRepoChecks');
  if(!box)return;
  const def=project?.defaultRepositoryId;
  const sel=Array.isArray(selected)?selected:(selected?[selected]:repos.map(r=>r.id));
  box.innerHTML=repos.map(r=>{
    const checked=sel.includes(r.id)?' checked':'';
    return `<label class="repo-check"><input type="checkbox" value="${escapeHTML(r.id)}"${checked}><span>${escapeHTML(r.name)} · ${escapeHTML(r.defaultBranch)}${r.id===def?'（默认）':''}</span></label>`;
  }).join('');
}
function selectedIssueRepositoryIds(){
  return [...document.querySelectorAll('#issueRepoChecks input[type="checkbox"]:checked')].map(i=>i.value).filter(Boolean);
}
// Older records stored the configured main branch in baseBranch automatically.
// Apply compatibility only when reading saved issues, never to new form input.
function storedIssueWorkBranch(issue){
  const project=(state.data.projects||[]).find(p=>p.id===issue.projectId);
  const ids=[...new Set([...(issue.repositoryIds||[]),issue.repositoryId].filter(Boolean))];
  if(!ids.length&&project?.defaultRepositoryId)ids.push(project.defaultRepositoryId);
  if(ids.length!==1)return '';
  const repo=(state.data.repositories||[]).find(r=>r.id===ids[0]);
  if(issue.repositoryBranches)return String(issue.repositoryBranches[ids[0]]||'').trim();
  const branch=String(issue.baseBranch||'').trim();
  return branch===String(repo?.defaultBranch||'').trim()?'':branch;
}
function updateIssueBaseBranchInput(input,hint,repositoryIds){
  if(!input)return;
  const repos=(state.data.repositories||[]).filter(r=>(repositoryIds||[]).includes(r.id));
  const ids=JSON.stringify(repos.map(r=>r.id).sort());
  if(repos.length!==1||(input.dataset.repositoryIds&&input.dataset.repositoryIds!==ids))input.value='';
  input.dataset.repositoryIds=ids;
  input.disabled=repos.length!==1;
  const mainBranch=repos.length===1?repos[0].defaultBranch:'';
  input.placeholder=mainBranch?`留空从主分支 ${mainBranch} 新建`:'留空从各仓库配置的主分支新建';
  if(hint)hint.textContent=repos.length!==1?'指定继续开发分支时请选择一个仓库；留空从各仓库配置的主分支分别新建工作分支。':'留空从配置主分支新建工作分支；填写时直接继续该已有分支，不新建。';
}
function refreshIssueBaseBranchInput(){
  renderIssueRepositoryBranchInputs($('#issueForm')?.elements.baseBranch,$('#issueBaseBranchHint'),$('#issueRepositoryBranches'),selectedIssueRepositoryIds());
}
function selectedIssueRepositoryBranches(input,host){
  const branches={};
  let ids=[];try{ids=JSON.parse(input?.dataset.repositoryIds||'[]')}catch{}
  if(ids.length===1)branches[ids[0]]=input.value.trim();
  host?.querySelectorAll('[data-repository-branch]').forEach(el=>{branches[el.dataset.repositoryBranch]=el.value.trim()});
  return branches;
}
function renderIssueRepositoryBranchInputs(input,hint,host,repositoryIds,branches){
  if(!input||!host)return;
  branches=branches||selectedIssueRepositoryBranches(input,host);
  const repos=repositoryIds.map(id=>(state.data.repositories||[]).find(r=>r.id===id)).filter(Boolean);
  updateIssueBaseBranchInput(input,hint,repositoryIds);
  input.hidden=repos.length>1;
  if(repos.length===1)input.value=branches[repos[0].id]||'';
  host.hidden=repos.length<2;
  host.innerHTML=repos.length<2?'':repos.map(repo=>`<label class="repository-branch-row"><span>${escapeHTML(repo.name)}</span><input class="input" data-repository-branch="${escapeHTML(repo.id)}" aria-label="${escapeHTML(repo.name)} 的继续开发分支" value="${escapeHTML(branches[repo.id]||'')}" placeholder="${escapeHTML(repo.defaultBranch?`留空从主分支 ${repo.defaultBranch} 新建`:'留空从配置主分支新建')}" autocomplete="off"></label>`).join('');
  if(repos.length>1&&hint)hint.textContent='分别填写各仓库的已有功能分支，直接继续开发；留空的仓库从各自配置的主分支新建工作分支。';
}
function fillIssueAssignees(projectId,value=''){
  const f=$('#issueForm'),picker=$('#issueAssigneePicker');if(!f||!picker)return;
  const project=(state.data.projects||[]).find(p=>p.id===projectId),selected=Number(value)||Number(state.currentUser?.id)||Number(project?.ownerId)||0;
  picker.innerHTML=userCombobox('assigneeUserId',selected,'使用项目 Owner');
  bindUserSearches(f);
}
function openIssue(issue=null){
  state.selectedIssue=issue?.id||null;
  const f=$('#issueForm'),projects=state.data.projects||[];
  f.elements.projectId.innerHTML=projects.map(p=>`<option value="${p.id}">${escapeHTML(p.name)}</option>`).join('');
  const v=issue||{id:'',projectId:state.projectFilter||projects[0]?.id||'',title:'',description:'',priority:'medium',status:'backlog',repositoryId:'',repositoryIds:[],baseBranch:''};
  ['id','projectId','title','description','priority','status','baseBranch','tapdId'].forEach(k=>f.elements[k].value=v[k]||'');
  if(f.elements.tags)f.elements.tags.value=issueTagList(v).join(', ');
  f.elements.baseBranch.value=storedIssueWorkBranch(v);
  fillIssueRepositories(v.projectId,(v.repositoryIds&&v.repositoryIds.length)?v.repositoryIds:(v.repositoryId?[v.repositoryId]:null));
  fillIssueAssignees(v.projectId,v.assigneeUserId||'');
  delete f.elements.baseBranch.dataset.repositoryIds;
  renderIssueRepositoryBranchInputs(f.elements.baseBranch,$('#issueBaseBranchHint'),$('#issueRepositoryBranches'),selectedIssueRepositoryIds(),v.repositoryBranches||{[v.repositoryId]:storedIssueWorkBranch(v)});
  f.elements.projectId.onchange=()=>{fillIssueRepositories(f.elements.projectId.value);fillIssueAssignees(f.elements.projectId.value,'');renderIssueRepositoryBranchInputs(f.elements.baseBranch,$('#issueBaseBranchHint'),$('#issueRepositoryBranches'),selectedIssueRepositoryIds(),v.repositoryBranches||{});};
  $('#issueRepoChecks').onchange=refreshIssueBaseBranchInput;
  $('#issueModalTitle').textContent=issue?`${(projects.find(p=>p.id===v.projectId)?.key||'ISS')}-${v.number} ${v.title}`:'新建工作项';
  let stateHtml='';
  if(v.externalRef){stateHtml+=`<br>外部来源: <b>${escapeHTML(v.externalRef.source)}</b> · <a href="${escapeHTML(safeURL(v.externalRef.externalUrl))}" target="_blank" rel="noopener noreferrer">${escapeHTML(v.externalRef.externalId||v.externalRef.externalUrl)}</a>`;}
  const reopenBtn=$('#reopenIssue');
  if(reopenBtn){
    const locked=issue&&issueReopenable(v);
    reopenBtn.hidden=!locked;
    reopenBtn.onclick=locked?()=>reopenIssue(issue.id):null;
  }
  const isNew=!issue?.id,note=$('#issueBoundNote');
  const manualBinding=$('#issueManualTapdBinding');
  const showManualBinding=()=>{if(manualBinding)manualBinding.hidden=true};
  showManualBinding();
  if(note){note.hidden=isNew||!v.externalRef;const text=$('#issueBoundNoteText');if(text&&v.externalRef){const src=v.externalRef.source==='github'?'GitHub Issue':v.externalRef.source==='tapd'?'TAPD':v.externalRef.source==='gongfeng'?'工蜂 Issue':'外部';const id=v.externalRef.externalIid||v.externalRef.externalId||'';text.innerHTML=`已关联 ${src} <b>${escapeHTML(String(id))}</b>。保存只改本平台记录，不会回写外部单据。`}}
  const submit=document.querySelector('#issueForm button[type="submit"]');
  if(submit)submit.textContent=isNew?'创建需求':'保存';
  $('#issueModal').classList.add('open');
}
function issueFromForm(){
  const f=$('#issueForm'),id=f.elements.id.value,old=canonicalIssue(id);
  const assigneeUserId=Number(f.elements.assigneeUserId?.value)||0;
  const source=old.source||'local';
  const repositoryIds=selectedIssueRepositoryIds(),status=f.elements.status.value;
  const closeSource=status==='closed'?(old.status==='closed'?old.closeSource||'manual':'manual'):'';
  return {...old,id,projectId:f.elements.projectId.value,title:f.elements.title.value.trim(),description:f.elements.description.value,priority:f.elements.priority.value,status,closeSource,source,tags:parseIssueTags(f.elements.tags?.value),repositoryId:repositoryIds[0]||'',repositoryIds,baseBranch:f.elements.baseBranch.value.trim(),repositoryBranches:selectedIssueRepositoryBranches(f.elements.baseBranch,$('#issueRepositoryBranches')),assigneeUserId,tapdId:source==='local'?f.elements.tapdId.value.trim():'',externalRef:trackerRefOf(old)||old.externalRef||undefined};
}

// ParseGongfengRepoURL 与后端 domain.ParseGongfengRepoURL 保持一致：把粘贴的
// 将完整的仓库链接和克隆地址解析成统一的仓库信息。
// repoDisplayPath shows host/path for a parsed repository, e.g. github.com/owner/name.
function repoDisplayPath(info,fallback=''){return info?.provider==='ssh'?(info.project||info.cloneUrl||fallback):info?.webUrl?String(info.webUrl).replace(/^https?:\/\//,''):String(fallback||'')}
function repoProviderLabel(info){
  const labels={ssh:'Git SSH',http:'Git HTTP',https:'Git HTTPS',github:'GitHub',gitlab:'GitLab',gongfeng:'工蜂'};
  return labels[info?.provider]||'Git';
}
// A GitLab project is bound by its project key: "gitlab:<host>/<group>/<repo>".
// The group path may carry subgroups, and the host is the one the URL names.
function parseGitLabProject(host,path){
  const parts=path.replace(/^\/+|\/+$/g,'').split('/').filter(Boolean).map(part=>part.replace(/\.git$/,''));
  if(parts.length<2)return null;
  if(parts.some(part=>!part||part==='.'||part==='..'||!/^[A-Za-z0-9_.-]+$/.test(part)))return null;
  const projectPath=parts.join('/'),name=parts[parts.length-1];
  return {project:`gitlab:${host}/${projectPath}`,name,webUrl:`https://${host}/${projectPath}`,cloneUrl:`https://${host}/${projectPath}.git`,provider:'gitlab'};
}
// GitLab hosts the console recognises by name. A GitLab on any other host is
// still bindable: paste its project key ("gitlab:host/group/repo").
function gitLabHost(hostname){
  const host=String(hostname||'').toLowerCase();
  return host==='gitlab.com'||host.startsWith('gitlab.')||host.includes('.gitlab.')||host.includes('-gitlab.')?host:'';
}
function parseGongfengRepoURL(raw){
  const projectKey=String(raw||'').trim().match(/^gitlab:(\S+)$/i);
  if(projectKey){const slash=projectKey[1].indexOf('/');return slash>0?parseGitLabProject(projectKey[1].slice(0,slash),projectKey[1].slice(slash+1)):null}
  const sshInfo=globalThis.LoopForgeGitSSH?.(raw);if(sshInfo)return sshInfo;
  if(globalThis.LoopForgeExplicitHTTPClone(raw))return globalThis.LoopForgeGitHTTPRepository(raw);
  const managed=globalThis.LoopForgeGongfengRepository?.(raw,state.data?.gongfengHost);if(managed)return managed;
  let publicURL=String(raw||'').trim().replace(/^git clone /,'').replace(/^github:/,'https://github.com/').replace(/^(?:git@github\.com:|ssh:\/\/git@github\.com(?::22)?\/)/,'https://github.com/');
  if(publicURL.startsWith('github.com/')||publicURL.startsWith('gitlab.com/'))publicURL='https://'+publicURL;
  try{
    const u=new URL(publicURL);
    if(!u.username&&!u.password&&['http:','https:'].includes(u.protocol)){
      if(u.hostname==='github.com'){
        const parts=u.pathname.split('/').filter(Boolean),owner=parts[0],name=(parts[1]||'').replace(/\.git$/,'');
        if(parts.length>=2&&/^[A-Za-z0-9_.-]+$/.test(owner)&&/^[A-Za-z0-9_.-]+$/.test(name)&&!['.','..'].includes(owner)&&!['.','..'].includes(name))return {project:'github:'+owner+'/'+name,name,webUrl:'https://github.com/'+owner+'/'+name,cloneUrl:'https://github.com/'+owner+'/'+name+'.git',provider:'github'};
      }else{
        const host=gitLabHost(u.hostname);
        if(host){const info=parseGitLabProject(u.host,u.pathname);if(info)return info}
      }
    }
  }catch(_){}

  return globalThis.LoopForgeGitHTTPRepository(raw);
}
// fillRepositoryFromLink 把解析结果填进空缺字段；显式填过的字段不覆盖。
function fillRepositoryFromLink(url,fill){
  const info=parseGongfengRepoURL(url);
  if(!info)return false;
  fill.gongfengProjectId=['ssh','http','https'].includes(info.provider)?'':info.project;
  if(!String(fill.name||'').trim())fill.name=info.name;
  fill.webUrl=info.provider==='ssh'&&fill.cloneUrl===info.cloneUrl?fill.webUrl:info.webUrl;
  fill.cloneUrl=info.cloneUrl;
  return true;
}
function renderRepositoryCredentialHint(){
  const host=$('#repositoryCredentialHint');if(!host)return;
  const remote=parseGongfengRepoURL($('#repositoryForm')?.elements.repoUrl?.value),ssh=remote?.provider==='ssh',user=userById(state.currentUser?.id)||{};
  host.classList.toggle('git-ssh-missing',ssh);
  host.classList.toggle('repository-credential-hint',!ssh);
  if(ssh){host.setAttribute('role','alert');return renderGitSSHMissingHint(host,remote)}
  host.sshHintVersion=(host.sshHintVersion||0)+1;
  host.hidden=false;
  host.removeAttribute('role');
  const configured=!!user.gitHttpCredentialsConfigured;
  const title=configured?'已配置 Git 访问令牌':'仓库可以先保存，导入需求还需要 Token';
  // The platform reads the repository and fetches imports with this one token;
  // it runs no work of its own, so there is no second identity to fall back on.
  const message=configured?'请确认 Token 已获得目标仓库的访问和写入权限。导入与刷新需求都使用这一个 Token。':'导入需求、读取私有代码前，请先配置 Git 访问令牌。导入与刷新需求都使用这一个 Token。';
  host.dataset.configured=String(configured);
  host.innerHTML=`<div><b>${escapeHTML(title)}</b><p>${escapeHTML(message)}</p></div><button type="button" class="button ghost small" data-repository-configure-token>${configured?'个人设置':'配置 Git 访问令牌'}</button>`;
  host.querySelector('[data-repository-configure-token]').onclick=()=>{
    $('#repositoryModal').classList.remove('open');gotoCredential('gitHttpCredentials');
  };
}
function renderRepositoryList(){renderRepositoryCredentialHint();const project=(state.data.projects||[]).find(p=>p.id===state.projectFilter),repos=(state.data.repositories||[]).filter(r=>r.projectId===state.projectFilter);$('#repositoryProjectHint').textContent=`当前项目：${project?.name||'—'}。粘贴 GitHub 链接或 Git SSH 地址；集成分支用于检查多个 LOOP 执行后是否存在合并冲突，可留空。`;$('#repositoryList').innerHTML=repos.map(r=>`<button type="button" data-edit-repo="${escapeHTML(r.id)}"><b>${escapeHTML(r.name)}</b><span>${escapeHTML(r.gongfengProjectId||r.cloneUrl)} · 主分支 ${escapeHTML(r.defaultBranch||'master')} · 集成分支 ${escapeHTML(r.integrateBranch||'未配置')}</span><em>${project?.defaultRepositoryId===r.id?'默认仓库':'编辑'}</em></button>`).join('')||'<p>当前项目还没有绑定代码仓库。</p>';$('#repositoryList').querySelectorAll('[data-edit-repo]').forEach(b=>b.onclick=()=>fillRepositoryForm((state.data.repositories||[]).find(r=>r.id===b.dataset.editRepo)))}
function fillRepositoryForm(repo={}){const f=$('#repositoryForm');['id','name','gongfengProjectId','webUrl','cloneUrl','localPath'].forEach(k=>{if(f.elements[k])f.elements[k].value=repo[k]||''});if(f.elements.repoUrl)f.elements.repoUrl.value=globalThis.LoopForgeGitRepositoryReference(repo);
  const nameField=$('#repositoryNameField');if(nameField)nameField.hidden=!repo.id;f.elements.defaultBranch.value=repo.defaultBranch||'master';f.elements.integrateBranch.value=repo.integrateBranch||'';const project=(state.data.projects||[]).find(p=>p.id===state.projectFilter);f.elements.makeDefault.checked=!project?.defaultRepositoryId||project.defaultRepositoryId===repo.id;if(f.elements.repoUrl){f.elements.repoUrl.oninput=renderRepositoryCredentialHint;f.elements.repoUrl.onchange=()=>{renderRepositoryCredentialHint();const form=f.elements,fill={name:form.name.value,gongfengProjectId:form.gongfengProjectId.value,webUrl:form.webUrl.value,cloneUrl:form.cloneUrl.value};if(fillRepositoryFromLink(form.repoUrl.value,fill)){form.name.value=fill.name;form.gongfengProjectId.value=fill.gongfengProjectId;form.webUrl.value=fill.webUrl;form.cloneUrl.value=fill.cloneUrl}}};renderRepositoryCredentialHint()}
function openRepositoryModal(){if(!state.projectFilter)return toast('请先选择项目');gitSSHKeyCache=null;fillRepositoryForm();renderRepositoryList();$('#repositoryModal').classList.add('open')}
async function saveRepository(e){e.preventDefault();const f=e.currentTarget,project=(state.data.projects||[]).find(p=>p.id===state.projectFilter);if(f.elements.repoUrl){const fill={name:f.elements.name.value,gongfengProjectId:f.elements.gongfengProjectId.value,webUrl:f.elements.webUrl.value,cloneUrl:f.elements.cloneUrl.value};if(fillRepositoryFromLink(f.elements.repoUrl.value,fill)){f.elements.name.value=fill.name;f.elements.gongfengProjectId.value=fill.gongfengProjectId;f.elements.webUrl.value=fill.webUrl;f.elements.cloneUrl.value=fill.cloneUrl}}const payload={id:f.elements.id.value,projectId:state.projectFilter,provider:(parseGongfengRepoURL(f.elements.repoUrl?.value)?.provider||'gongfeng'),name:f.elements.name.value.trim(),gongfengProjectId:f.elements.gongfengProjectId.value.trim(),webUrl:f.elements.webUrl.value.trim(),cloneUrl:f.elements.cloneUrl.value.trim(),defaultBranch:f.elements.defaultBranch.value.trim(),integrateBranch:f.elements.integrateBranch.value.trim(),localPath:f.elements.localPath.value.trim(),repoUrl:f.elements.repoUrl?f.elements.repoUrl.value.trim():''};try{const repo=await api('/api/repositories',{method:payload.id?'PUT':'POST',body:JSON.stringify(payload)}),idx=(state.data.repositories||[]).findIndex(r=>r.id===repo.id);if(idx>=0)state.data.repositories[idx]=repo;else(state.data.repositories||(state.data.repositories=[])).push(repo);if(f.elements.makeDefault.checked&&project){const saved=await api('/api/projects',{method:'PUT',body:JSON.stringify({...project,defaultRepositoryId:repo.id})});if(saved&&saved.id){const pi=(state.data.projects||[]).findIndex(p=>p.id===saved.id);if(pi>=0)state.data.projects[pi]=saved;project.defaultRepositoryId=repo.id}else project.defaultRepositoryId=repo.id}fillRepositoryForm(repo);renderRepositoryList();renderIssues();toast(repo.provider==='ssh'?'仓库已保存；检出和推送使用 Git SSH 密钥。':(userById(state.currentUser?.id)||{}).gitHubCredentialConfigured?'Git 仓库绑定已保存':'仓库已保存；配置 Git 访问令牌 后才能进行代码交付。')}catch(err){toast(err.message)}}
function canonicalIssue(id){
  id=String(id||'').trim();
  if(!id)return {};
  return (state.data.issues||[]).find(i=>i.id===id)||(state.issueDetailData?.issue?.id===id?state.issueDetailData.issue:{})||{};
}
function trackerRefOf(issue){
  const ref=issue?.externalRef;
  if(ref&&(ref.externalId||ref.externalIid))return ref;
  return undefined;
}
async function persistIssue(issue){
  const id=String(issue.id||'').trim();
  const prior=canonicalIssue(id);
  const payload={...prior,...issue,id};
  if(prior.source)payload.source=prior.source;
  const bound=trackerRefOf(prior)||trackerRefOf(payload);
  if(bound)payload.externalRef=bound;
  else if(payload.externalRef&&!(payload.externalRef.externalId||payload.externalRef.externalIid))delete payload.externalRef;
  delete payload.syncTarget;
  const saved=await api('/api/issues',{method:id?'PUT':'POST',body:JSON.stringify(payload)});
  rememberIssue(saved, id);
  return saved;
}
async function saveIssue(e){e.preventDefault();const button=e.currentTarget.querySelector('button[type="submit"]');try{button.disabled=true;button.textContent='正在保存…';const saved=await persistIssue(issueFromForm());state.selectedIssue=saved.id;$('#issueModal').classList.remove('open');renderIssues();toast('需求已保存')}catch(err){toast(err.message)}finally{button.disabled=false;button.textContent='保存'}}
async function reopenIssueSilent(issueId){
  const issue=await api(`/api/issues/${encodeURIComponent(issueId)}/reopen`,{method:'POST',body:'{}'});
  const idx=(state.data.issues||[]).findIndex(i=>i.id===issue.id);
  if(idx>=0)state.data.issues[idx]=issue;
  return issue;
}
async function reopenIssue(issueId){
  if(!await afConfirm({title:'重新打开确认',message:'重新打开后，这条需求回到待处理状态。确定继续？',confirmText:'确认重新打开'}))return;
  try{
    const issue=await reopenIssueSilent(issueId);
    renderIssues();
    $('#issueModal')?.classList.remove('open');
    if(state.drawerOpen&&state.selectedIssue===issue.id)openIssueDetail(issue.id);
    toast('需求已重新打开');
  }catch(err){toast(err.message)}
}
