function issueCanClose(issue){
  return issue?.closureAllowed===true;
}
function issueCanDelete(issue){
  return issue?.deletionAllowed===true;
}
function issueDeletionBlock(issue){
  if(issueCanDelete(issue))return null;
  return issue?.deletionBlock?.reason?issue.deletionBlock:{code:'unknown',reason:'暂时无法确认删除条件，请刷新后重试'};
}
function issueDeletePath(id,force){
  return `/api/issues/${encodeURIComponent(id)}${force?'?force=true':''}`;
}
function issueDeletionWarning(force,external){
  return `需求及其评论将被删除。${force?'关联的执行记录将一并清理。':''}删除后无法恢复。${external?'外部原单会保留，后续同步可能重新导入。':''}`;
}
function issueDeletionContextHTML(issue,{menu=false}={}){
  const block=issueDeletionBlock(issue);
  if(!block)return '';
  const label='查看需求详情';
  return `<button type="button" class="${menu?'issue-popover-item':'button ghost small'}" data-issue-deletion-context="${escapeHTML(issue.id)}" aria-label="${escapeHTML(`${label}：${issue.title||issue.id}，${block.reason}`)}">${menu?huiIcon('arrow-right','popover-icon'):''}${label}</button>`;
}
function issueDeleteControlHTML(issue,{menu=false}={}){
  const block=issueDeletionBlock(issue),cls=menu?'issue-popover-item':'button ghost small';
  const reasonId=`issue-delete-reason-${menu?'menu':'detail'}-${issue.id}`;
  const label=!block&&issue.deletionRequiresForce?'强制删除':'删除需求';
  const button=`<button type="button" class="${cls}${block?'':' danger'}" ${menu?'data-action="delete"':`data-delete-issue="${escapeHTML(issue.id)}"`}${block?` disabled aria-describedby="${escapeHTML(reasonId)}"`:''}>${menu?huiIcon('trash','popover-icon'):''}${label}</button>`;
  if(!block)return button;
  return `<div class="issue-delete-control">${button}<p class="issue-delete-reason" id="${escapeHTML(reasonId)}">${escapeHTML(block.reason)}</p>${issueDeletionContextHTML(issue,{menu})}</div>`;
}
function bindIssueDeletionContext(root){
  root.querySelectorAll('[data-issue-deletion-context]').forEach(button=>button.onclick=e=>{
    e.stopPropagation();
    const id=button.dataset.issueDeletionContext;
    const detailIssue=state.issueDetailData?.issue?.id===id?state.issueDetailData.issue:null;
    const issue=(root.id==='issueDetailBody'?detailIssue:null)||(state.data.issues||[]).find(i=>i.id===id)||detailIssue;
    if(!issue)return;
    const block=issueDeletionBlock(issue);
    if(!block)return;
    hideIssueActionMenu();
    openIssueDetail(id,'overview');
  });
}
const issueStatuses=[['backlog','Backlog'],['todo','Todo'],['in_progress','In progress'],['in_review','In review'],['done','Done'],['closed','Closed'],['blocked','Blocked'],['cancelled','Cancelled']];
function isRequirementWorkItem(issue){
  const kind=String(issue?.kind||'').toLowerCase();
  const source=String(issue?.source||'').toLowerCase();
  if(source==='gongfeng'||kind==='issue')return false;
  return kind==='requirement'||source==='tapd'||source==='local'||source==='';
}
function issueOwnerSummary(i){
  return issueAssigneeName(i)||'未指定';
}
// A requirement is closed when its own status says so; nothing else can close it.
function issueClosed(i){return String(i?.status||'').toLowerCase()==='closed'}
function issueOutcome(i){
  const stage=issueCurrentStage(i);
  if(issueClosed(i))return{key:'closed',short:'已关闭',title:'需求已关闭',body:'该需求已关闭。如需继续修改，请先重新打开。'};
  const delivery=deliveryLockedLabel(i);
  if(delivery==='MR 已合入'||delivery==='已合入基线')return{key:'closed',short:'已交付',title:delivery,body:'交付已经闭环。如需继续修改，请先重新打开。'};
  if(String(i?.status||'').toLowerCase()==='done')return{key:'closed',short:'已完成',title:'需求已完成',body:'需求已完成。'};
  if(stage.key==='attention')return{key:'attention',short:'需处理',title:stage.label,body:stage.hint};
  if(stage.key==='active')return{key:'active',short:'进行中',title:stage.label,body:stage.hint};
  return{key:'idle',short:stage.label,title:stage.label,body:stage.hint};
}
function deliveryMRLinked(i){return deliveryMRList(i).length>0}
// 待处理 MR：已创建但还没合入的 MR（opened/reopened）。状态未知的历史 MR 不计入，避免数字虚高。
function pendingDeliveryMRCount(i){return deliveryMRList(i).filter(m=>['opened','reopened'].includes(String(m.state||'').toLowerCase())).length}
// 多仓库交付每个仓库一个 MR；旧数据只有 deliveryMrUrl。
function deliveryMRList(i){
  const withRef=m=>{const hit=String(m.url).match(/^https?:\/\/[^/]+\/(.+?)(?:\/-)?\/merge_requests\/(\d+)/);return{...m,project:m.project||hit?.[1]||'',iid:m.iid||Number(hit?.[2])||0}};
  const list=(i?.deliveryMrs||[]).filter(m=>String(m?.url||'').trim());
  if(list.length)return list.map(withRef);
  const url=String(i?.deliveryMrUrl||'').trim();
  return url?[withRef({url,state:i?.deliveryMrState||''})]:[];
}
const deliveryMRStateText={merged:'已合入',integrated:'已合入基线',opened:'待合入',reopened:'待合入',closed:'已关闭'};
function deliveryMRButtons(issue,{cls='button ghost small'}={}){
  const list=deliveryMRList(issue);
  return list.map(mr=>{
    const iid=mr.iid||String(mr.url).match(/merge_requests\/(\d+)/)?.[1]||'';
    const repo=mr.repository||String(mr.project||'').split('/').pop();
    const label=list.length>1?`打开 MR · ${repo||'仓库'}${iid?` !${iid}`:''}`:'打开 MR';
    const st=deliveryMRStateText[String(mr.state||'').toLowerCase()];
    return `<a class="${cls}" href="${escapeHTML(safeURL(mr.url))}" target="_blank" rel="noopener noreferrer" title="${escapeHTML([mr.project,st].filter(Boolean).join(' · ')||mr.url)}">${escapeHTML(label)} ↗</a>`;
  }).join('');
}
function deliveryLockedLabel(i){
  if(issueClosed(i))return '关闭';
  const state=String(i?.deliveryMrState||'').toLowerCase();
  // 旧版手动关闭会写入 deliveryState=merged、deliveryMrState=closed，但没有 MR 链接。
  if(!deliveryMRLinked(i))return state==='closed'?'已关闭':'交付闭环';
  switch(state){
    case 'merged':return 'MR 已合入';
    case 'integrated':return '已合入基线';
    case 'closed':return 'MR 已关闭';
    default:return '交付闭环';
  }
}
// 交付已经闭环（MR 合入）或需求已关闭时，用户可以把它重新打开。
function issueReopenable(i){
  if(issueClosed(i))return true;
  const label=deliveryLockedLabel(i);
  return label==='MR 已合入'||label==='已合入基线';
}
function reopenDeliveredButton(issue,{cls='button primary small'}={}){if(!issueReopenable(issue))return'';return `<button type="button" class="${cls}" data-reopen-issue="${escapeHTML(issue.id)}">重新打开</button>`}
function issueActionButtons(i){
  const more=`<button type="button" class="issue-act-more" data-issue-menu="${escapeHTML(i.id)}" title="更多操作" aria-label="更多操作"><svg class="agent-icon harness-icon" data-icon="more-horizontal" width="16" height="16" aria-hidden="true" focusable="false"></svg></button>`;
  if(issueClosed(i))return more;
  return `<button type="button" class="issue-act" data-open-issue="${escapeHTML(i.id)}" data-dtab="overview">查看</button>${more}`;
}
let issueMenuPopover=null,issueMenuTrigger=null;
function ensureIssueMenuPopover(){
  if(issueMenuPopover)return issueMenuPopover;
  issueMenuPopover=document.createElement('div');
  issueMenuPopover.id='issueActionPopover';
  issueMenuPopover.className='issue-action-popover';
  issueMenuPopover.hidden=true;
  document.body.appendChild(issueMenuPopover);
  document.addEventListener('click',e=>{
    if(issueMenuPopover.hidden)return;
    if(issueMenuPopover.contains(e.target)||e.target.closest('[data-issue-menu]'))return;
    hideIssueActionMenu();
  });
  window.addEventListener('resize',hideIssueActionMenu,{passive:true});
  window.addEventListener('scroll',hideIssueActionMenu,{passive:true});
  document.addEventListener('keydown',e=>{
    if(issueMenuPopover.hidden)return;
    if(e.key==='Escape'){
      e.preventDefault();
      hideIssueActionMenu();
      issueMenuTrigger?.focus({preventScroll:true});
    }else if(['ArrowDown','ArrowUp','Home','End'].includes(e.key)){
      e.preventDefault();
      const items=[...issueMenuPopover.querySelectorAll('button:not(:disabled)')];
      const current=items.indexOf(document.activeElement);
      const next=e.key==='Home'?0:e.key==='End'?items.length-1:(current+(e.key==='ArrowUp'?-1:1)+items.length)%items.length;
      items[next]?.focus();
    }else if(e.key==='Tab'){
      hideIssueActionMenu();
      issueMenuTrigger?.focus({preventScroll:true});
    }
  });
  return issueMenuPopover;
}
function hideIssueActionMenu(){
  issueMenuTrigger?.setAttribute('aria-expanded','false');
  if(issueMenuPopover&&!issueMenuPopover.hidden){
    issueMenuPopover.hidden=true;
    issueMenuPopover.innerHTML='';
  }
}
function showIssueActionMenu(e,issueId){
  e.preventDefault();
  e.stopPropagation();
  const issue=(state.data?.issues||[]).find(i=>i.id===issueId);
  if(!issue)return;
  issueMenuTrigger?.setAttribute('aria-expanded','false');
  issueMenuTrigger=e.currentTarget;
  issueMenuTrigger.setAttribute('aria-expanded','true');
  issueMenuTrigger.setAttribute('aria-controls','issueActionPopover');
  const menu=ensureIssueMenuPopover();
  const isClosed=issue.status==='done'||issueClosed(issue);
  const stateAction=isClosed
    ?`<button type="button" class="issue-popover-item" data-action="reopen">${huiIcon('undo','popover-icon')}重新打开</button>`
    :(issueCanClose(issue)?`<button type="button" class="issue-popover-item" data-action="close">${huiIcon('close','popover-icon')}关闭需求</button>`:'');
  menu.innerHTML=`
    <button type="button" class="issue-popover-item" data-action="edit">${huiIcon('edit','popover-icon')}编辑需求</button>
    ${stateAction}
    ${issueDeleteControlHTML(issue,{menu:true})}
  `;
  window.AgentFlowHarnessIcons?.hydrate?.(menu);
  menu.querySelector('[data-action="edit"]').onclick=ev=>{
    ev.stopPropagation();
    hideIssueActionMenu();
    openIssue(issue);
  };
  const deleteBtn=menu.querySelector('[data-action="delete"]');
  if(deleteBtn&&!deleteBtn.disabled)deleteBtn.onclick=ev=>{
    ev.stopPropagation();
    hideIssueActionMenu();
    deleteIssueById(issue.id);
  };
  bindIssueDeletionContext(menu);
  const closeBtn=menu.querySelector('[data-action="close"]');
  if(closeBtn){
    closeBtn.onclick=ev=>{
      ev.stopPropagation();
      hideIssueActionMenu();
      closeIssueById(issue.id);
    };
  }
  const reopenBtn=menu.querySelector('[data-action="reopen"]');
  if(reopenBtn){
    reopenBtn.onclick=ev=>{
      ev.stopPropagation();
      hideIssueActionMenu();
      reopenDeliveredIssue(issue.id);
    };
  }
  menu.hidden=false;
  menu.querySelector('button:not(:disabled)')?.focus();
  const btn=e.currentTarget;
  const rect=btn.getBoundingClientRect();
  const menuWidth=menu.offsetWidth||136;
  const menuHeight=menu.offsetHeight||72;
  let left=rect.right-menuWidth;
  left=Math.max(8,Math.min(left,window.innerWidth-menuWidth-8));
  let top=rect.bottom+4;
  if(top+menuHeight>window.innerHeight-8){
    top=rect.top-menuHeight-4;
  }
  menu.style.position='fixed';
  menu.style.left=`${Math.round(left)}px`;
  menu.style.top=`${Math.round(Math.max(8,top))}px`;
}
function bindIssueRowActions(root=document){
  root.querySelectorAll('[data-issue-menu]').forEach(button=>button.onclick=e=>showIssueActionMenu(e,button.dataset.issueMenu));
  root.querySelectorAll('[data-edit-issue]').forEach(button=>button.onclick=e=>{e.stopPropagation();const id=button.dataset.editIssue;openIssue((state.data.issues||[]).find(i=>i.id===id)||(id?{id}:null))});
  root.querySelectorAll('[data-open-issue]').forEach(button=>button.onclick=e=>{e.stopPropagation();openIssueDetail(button.dataset.openIssue,button.dataset.dtab||'overview')});
  root.querySelectorAll('[data-reopen-issue]').forEach(button=>button.onclick=e=>{e.stopPropagation();reopenDeliveredIssue(button.dataset.reopenIssue)});
  root.querySelectorAll('[data-close-issue]').forEach(button=>button.onclick=e=>{e.stopPropagation();closeIssueById(button.dataset.closeIssue)});
}
