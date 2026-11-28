// ================= Issue 详情抽屉 =================
function closeIssueDrawer(){
  state.drawerOpen=false;
  const d=$('#issueDetailDrawer');if(d){d.classList.remove('open');d.setAttribute('aria-hidden','true')}
}
async function openIssueDetail(id,tab){
  if(!id)return;
  const issueId=String(id);
  state.drawerOpen=true;state.selectedIssue=issueId;state.issueDetailData=null;
  const drawer=$('#issueDetailDrawer');
  if(drawer&&drawer.parentElement!==document.body)document.body.appendChild(drawer);
  drawer.classList.add('open');
  drawer.setAttribute('aria-hidden','false');
  $('#issueDetailBody').innerHTML='<div class="detail-empty">正在加载需求详情…</div>';
  try{
    const detail=await api(`/api/issues/${encodeURIComponent(issueId)}`);const issue=detail.issue;const comments=detail.comments||[];
    const localIssue=(state.data.issues||[]).find(i=>i.id===issueId);
    // Keep local view state, but deletion feedback must come from this fresh detail response.
    const permissions={deletionAllowed:issue.deletionAllowed,deletionRequiresForce:issue.deletionRequiresForce,deletionBlock:issue.deletionBlock,closureAllowed:issue.closureAllowed};
    if(localIssue){Object.assign(issue,localIssue,permissions);Object.assign(localIssue,permissions)}
    let requirement=null;
    try{requirement=await api(`/api/issues/${encodeURIComponent(issueId)}/requirement`,{silent:true})}catch(e){}
    state.issueDetailData={issue,comments,requirement,images:detail.images||[]};
    state.issueDetailTab=tab||'overview';
    $$('#issueDetailTabs .drawer-tab').forEach(b=>b.classList.toggle('active',b.dataset.dtab===state.issueDetailTab));
    renderIssueDetailTab();
  }catch(e){$('#issueDetailBody').innerHTML=`<div class="detail-empty">加载失败：${escapeHTML(e.message)}</div>`}
}
function renderIssueDetailTab(){
  const data=state.issueDetailData;const host=$('#issueDetailBody');if(!data||!host)return;
  const issue=data.issue;if(!issue){host.innerHTML='<div class="detail-empty">未找到该需求</div>';return}
  const project=(state.data.projects||[]).find(p=>p.id===issue.projectId);
  $('#issueDetailKey').textContent=issueKeyLabel(issue);
  $('#issueDetailTitle').textContent=issue.title||'';
  $('#issueDetailSubtitle').textContent=`${project?.name||'—'} · ${issuePriorityLabel(issue.priority)} · ${issueStatusText(issue)} · 更新于 ${new Date(issue.updatedAt).toLocaleString()}`;
  const panes={overview:overviewPane(issue,data),operations:operationsPane(issue),delivery:deliveryPane(issue,data),activity:activityPane(issue,data)};
  host.innerHTML=panes[state.issueDetailTab]||'<div class="detail-empty">暂无内容</div>';
  host.querySelectorAll('[data-edit-issue]').forEach(b=>b.onclick=()=>{closeIssueDrawer();openIssue(issue)});
  host.querySelectorAll('[data-reopen-issue]').forEach(b=>b.onclick=()=>reopenDeliveredIssue(b.dataset.reopenIssue));
  host.querySelectorAll('[data-delete-issue]').forEach(b=>b.onclick=()=>deleteIssueById(b.dataset.deleteIssue||issue.id));
  bindIssueDeletionContext(host);
  host.querySelectorAll('[data-close-issue]').forEach(b=>b.onclick=()=>closeIssueById(b.dataset.closeIssue||issue.id));
}
function drawerFact(label,value,title){return `<div class="detail-fact"><span>${label}</span><b title="${escapeHTML(title||value||'')}">${value===''||value==null?'—':value}</b></div>`}
function issueProgressActionsHTML(issue,{includeIssueActions=false}={}){
  const issueActions=[];
  if(includeIssueActions){
    issueActions.push('<button class="button ghost small" data-edit-issue>编辑需求</button>');
    if(issueCanClose(issue)&&issue.status!=='done'&&!issueClosed(issue))issueActions.push(`<button class="button ghost small" data-close-issue="${escapeHTML(issue.id)}">关闭需求</button>`);
    issueActions.push(issueDeleteControlHTML(issue));
    if(issue.externalRef?.externalUrl)issueActions.push(`<a class="button ghost small" href="${escapeHTML(safeURL(issue.externalRef.externalUrl))}" target="_blank" rel="noopener noreferrer">打开外部链接 ↗</a>`);
  }
  const group=(title,items)=>items.some(Boolean)?`<div class="issue-action-group"><span>${title}</span><div class="issue-action-group-buttons">${items.filter(Boolean).join('')}</div></div>`:'';
  const groups=[group('需求管理',issueActions)].filter(Boolean);
  return groups.length?`<div class="issue-action-groups">${groups.join('')}</div>`:'';
}
function overviewPane(issue,data){
  const project=(state.data.projects||[]).find(p=>p.id===issue.projectId),repo=(state.data.repositories||[]).find(r=>r.id===(issue.repositoryId||project?.defaultRepositoryId));
  const out=issueOutcome(issue);
  let actions='';
  if(issueReopenable(issue))actions=reopenDeliveredButton(issue)+deliveryMRButtons(issue);
  actions+=issueProgressActionsHTML(issue,{includeIssueActions:true});
  const banner=`<div class="issue-outcome ${out.key}"><b>${escapeHTML(out.title)}</b><p>${escapeHTML(out.body)}</p><div class="issue-outcome-actions">${actions}</div></div>`;
  const facts=`<div class="detail-grid">${drawerFact('责任人',issueAssigneeName(issue)||'未指定')}${drawerFact('优先级',issuePriorityLabel(issue.priority))}${drawerFact('来源',issueSourceLabel(issue))}${metricsIssueSource(issue)==='local'?drawerFact('统计关联 TAPD',escapeHTML(issue.tapdId||'未绑定（仍参与统计）')):''}${drawerFact('状态',escapeHTML(issueStatusLabel(issue.status)))}${drawerFact('标签',issueTagList(issue).map(t=>escapeHTML(t)).join('、'))}${drawerFact('代码仓库',escapeHTML(repo?.name||'未绑定'))}${drawerFact('继续开发分支',escapeHTML(storedIssueWorkBranch(issue)||'自动新建'))}</div>`;
  const description=issue.description?`<div class="markdown-body">${renderRequirementMarkdown(issue.description)}</div>`:'';
  const images=(data.images||[]).map(image=>{
    const url=`/api/issues/${encodeURIComponent(issue.id)}/images/${encodeURIComponent(image.id)}`;
    const name=escapeHTML(image.name||'需求图片');
    return `<li><a href="${url}" target="_blank" rel="noopener noreferrer"><img src="${url}" alt="${name}" loading="lazy"></a></li>`;
  }).join('');
  const gallery=images?`<ul class="issue-description-images" aria-label="需求图片">${images}</ul>`:'';
  const content=description||gallery?`<div class="detail-desc issue-description">${description}${gallery}</div>`:'';
  return `<div class="detail-section">${banner}${facts}${content}</div>`;
}
async function closeIssueById(issueId){
  const issue=(state.data.issues||[]).find(i=>i.id===issueId);
  if(!issue)return toast('未找到需求单');
  if(issueClosed(issue))return toast('该需求单已处于关闭状态');
  if(!issueCanClose(issue))return toast('这张需求当前不能关闭，请稍后再试','err');
  const ok=await afConfirm({
    title:'手动关闭需求',
    message:`确定手动关闭需求「${issue.title||issue.id}」？关闭后可在状态里筛「已关闭」。`,
    confirmText:'确认关闭'
  });
  if(!ok)return;
  try{
    const updated=await api(`/api/issues/${encodeURIComponent(issueId)}/close`,{method:'POST'});
    const idx=(state.data.issues||[]).findIndex(i=>i.id===issueId);
    if(idx>=0)state.data.issues[idx]=updated;
    if(state.issueDetailData?.issue?.id===issueId){
      state.issueDetailData.issue=updated;
      renderIssueDetailTab();
    }
    renderIssues();
    toast('需求已关闭','ok');
  }catch(err){
    toast(err.message,'err');
  }
}
// 需求绑定的代码仓库与分支，以及已经提过的 MR。
function deliveryPane(issue,data){
  const repos=(state.data.repositories||[]).filter(r=>r.projectId===issue.projectId);
  const rows=repos.map(r=>`<div class="issue-action-group"><span>${escapeHTML(r.name||r.id)}</span><div class="issue-action-group-buttons"><span class="muted mono">${escapeHTML(r.defaultBranch||'—')}</span></div></div>`).join('');
  return `<div class="detail-section"><h3>代码仓库<span class="sub">需求关联的仓库与分支</span></h3>${rows||'<div class="detail-empty">当前项目还没有绑定代码仓库</div>'}<div class="issue-outcome-actions">${deliveryMRButtons(issue)}</div></div>`;
}
function formatActivityTime(ts){
  const d=new Date(ts);
  if(Number.isNaN(d.getTime()))return '—';
  const p=n=>String(n).padStart(2,'0');
  return `${p(d.getMonth()+1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}
function issueOperationLabel(action){
  return {create:'创建需求',update:'编辑需求',close:'关闭需求',reopen:'重新打开'}[action]||action||'操作';
}
// 聊天 Bot 发起的操作：actor 仍是执行所用的平台账号，这里补出是哪个 IM 用户发起的。
// 渠道报了显示名就用名字，没报（企业微信、飞书只给 ID）就用 ID，后面的平台账号本身带真名。
function issueOperationOrigin(op){
  const sender=(op?.actorSender||'').trim();
  if(!sender)return '';
  const channel={wecom:'企业微信',feishu:'飞书',dingtalk:'钉钉'}[op.actorChannel]||op.actorChannel||'聊天';
  const named=(op?.actorSenderName||'').trim();
  return `${channel} ${named?`${named}（${sender}）`:sender}`;
}
function issueOperationEntries(issue){
  const stored=(issue?.operations||[]).map(op=>({
    t:new Date(op.at).getTime(),
    action:op.action||'',
    actor:op.actor||'',
    origin:issueOperationOrigin(op),
    label:issueOperationLabel(op.action),
    detail:op.detail||''
  })).filter(op=>!Number.isNaN(op.t));
  const inferred=[];
  if(issue?.createdAt&&!stored.some(op=>op.action==='create')){
    inferred.push({t:new Date(issue.createdAt).getTime(),action:'create',actor:'',origin:'',label:'创建需求',detail:'按创建时间补记'});
  }
  if(issueClosed(issue)&&issue.deliveryUpdatedAt&&!stored.some(op=>op.action==='close')){
    inferred.push({t:new Date(issue.deliveryUpdatedAt).getTime(),action:'close',actor:'',origin:'',label:'关闭需求',detail:'手动关闭'});
  }
  return stored.concat(inferred).filter(op=>!Number.isNaN(op.t)).sort((a,b)=>b.t-a.t);
}
function operationsPane(issue){
  const rows=issueOperationEntries(issue).map(e=>{
    const who=e.origin?(e.actor?`${e.origin}（平台账号 ${e.actor}）`:e.origin):e.actor;
    const detail=[who,e.detail].filter(Boolean).join(' · ');
    return `<div class="activity-row"><time>${formatActivityTime(e.t)}</time><b>${escapeHTML(e.label)}</b><p>${escapeHTML(detail||'—')}</p></div>`;
  }).join('');
  return `<div class="detail-section"><h3>操作日志<span class="sub">关闭、重新打开和编辑都会记在这里</span></h3>${rows?`<div class="activity-list">${rows}</div>`:'<div class="detail-empty compact">还没有操作记录</div>'}</div>`;
}
function activityPane(issue,data){
  const comments=data.comments||[];
  const commentHtml=comments.slice().reverse().map(c=>`<div class="comment-row"><div class="muted">${c.authorType==='agent'?'Agent':c.authorId||'成员'} · ${new Date(c.createdAt).toLocaleString()}</div><p>${escapeHTML(c.body)}</p></div>`).join('')||'<div class="detail-empty compact">暂无讨论</div>';
  return `<div class="detail-section"><h3>讨论与评论</h3>${commentHtml}</div>`;
}


async function deleteIssueById(issueId){
  if(state.issueBulkDeleting)return toast('正在批量删除需求，请稍候','info');
  const issue=(state.data.issues||[]).find(i=>i.id===issueId)||state.issueDetailData?.issue;
  if(!issue||issue.id!==issueId)return toast('未找到需求单');
  if(!issueCanDelete(issue))return toast(issueDeletionBlock(issue).reason,'err');
  const force=issue.deletionRequiresForce===true;
  const title=force?'强制删除需求':'删除需求';
  const external=issue.source==='tapd'||issue.source==='gongfeng';
  if(!await afConfirm({title,message:`确定${title}「${issue.title||issue.id}」？${issueDeletionWarning(force,external)}`,danger:true,confirmText:force?'确认强制删除':'确认删除'}))return;
  try{
    await api(issueDeletePath(issueId,force),{method:'DELETE'});
    state.data.issues=(state.data.issues||[]).filter(i=>i.id!==issueId);
    state.issueSelection?.delete(issueId);
    if(state.issueDetailData?.issue?.id===issueId){closeIssueDrawer();state.issueDetailData=null;state.selectedIssue=null;}
    state.data.tasks=(state.data.tasks||[]).filter(t=>t.issueId!==issueId);
    renderIssues();
    toast('需求已删除','ok');
  }catch(err){
    toast(err.message,'err');
    await refreshIssues();
  }
}
