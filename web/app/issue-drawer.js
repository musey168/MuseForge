// ================= Issue 详情抽屉 =================
const issueClarifyJobs=new Map();
let renderedIssueClarifyKey='';
let submittedClarifyDraft=null;
function issueClarifying(id){
  if(!!id&&issueClarifyJobs.has(id))return true;
  const issue=(state.data.issues||[]).find(i=>i.id===id)||state.issueDetailData?.issue;
  return issue?.id===id&&issue?.clarification?.status==='analyzing';
}
function closeIssueDrawer(){
  const id=state.issueDetailData?.issue?.id;
  const running=issueClarifying(id);
  state.drawerOpen=false;
  const d=$('#issueDetailDrawer');if(d){d.classList.remove('open');d.setAttribute('aria-hidden','true')}
  if(running)toast('澄清继续在后台进行');
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
    const stage=issueCurrentStage(issue),needsConfirm=clarificationNeedsConfirm(issue);
    state.issueDetailTab=tab||(issueClarifying(issueId)||needsConfirm?'clarify':(stage.key==='attention'?'overview':'overview'));
    $$('#issueDetailTabs .drawer-tab').forEach(b=>b.classList.toggle('active',b.dataset.dtab===state.issueDetailTab));
    renderIssueDetailTab();
    if(issueClarifying(issueId)){
      restoreIssueClarifyThinking(issueId);
      attachIssueClarifyStream(issueId);
    }
  }catch(e){$('#issueDetailBody').innerHTML=`<div class="detail-empty">加载失败：${escapeHTML(e.message)}</div>`}
}
function issueClarifyViewKey(issue,project){
  const c=issue?.clarification||{};
  return JSON.stringify({
    analyzing:!!issueClarifying(issue?.id),
    status:c.status||'',
    lastError:c.lastError||'',
    summary:c.summary||'',
    writeBack:c.writeBackStatus||'',
    writeBackError:c.writeBackError||'',
    questions:(c.questions||[]).map(q=>({question:q.question||'',multi:!!q.multiSelect,options:clarifyQuestionOptions(q)})),
    history:(c.history||[]).map(h=>({role:h.role||'',content:h.content||''})),
    source:issue?.source||'',
    external:issue?.externalRef?.externalUrl||'',
    confirm:!!clarificationNeedsConfirm(issue),
    blocked:''
  });
}
function clarifyQuestionKey(questions){
  return (questions||[]).map(q=>`${q.question||''}\n${clarifyQuestionOptions(q).join('\n')}`).join('\n---\n');
}
function rememberSubmittedClarifyDraft(issue,msg){
  const questions=issue?.clarification?.questions||[];
  const captured=captureIssueClarifyDraft($('#issueDetailBody'));
  const reply=String(msg||'').trim();
  submittedClarifyDraft={
    issueId:issue?.id||'',
    questionKey:clarifyQuestionKey(questions),
    questions:captured?.questions||[],
    extra:captured?.extra||'',
    reply:CLARIFY_AUTO_PROMPTS.includes(reply)?'':reply
  };
}
function clarifyAnswerDraft(issue){
  const saved=submittedClarifyDraft;
  if(!saved||saved.issueId!==issue?.id)return null;
  const status=issue?.clarification?.status||'';
  if(status==='ready'){submittedClarifyDraft=null;return null}
  if(!issueClarifying(issue.id)&&saved.questionKey!==clarifyQuestionKey(issue?.clarification?.questions)){
    submittedClarifyDraft=null;
    return null;
  }
  return saved;
}
function clarifySubmittedHTML(reply){
  const text=String(reply||'').trim();
  if(!text||CLARIFY_AUTO_PROMPTS.includes(text))return '';
  return `<div class="clarify-submitted"><b>本轮回答已提交</b><p>${escapeHTML(text)}</p></div>`;
}
function clarifyQuestionCardsHTML(questions,saved){
  return (questions||[]).map((q,i)=>{
    const picked=saved?.questions?.[i];
    const options=clarifyQuestionOptions(q).map((opt,j)=>`<button type="button" class="issue-clarify-option${picked?.selected?.[j]?' selected':''}" data-multi="${!!q.multiSelect}">${escapeHTML(opt)}</button>`).join('');
    return `<div class="issue-clarify-question"><b>${i+1}. ${escapeHTML(q.question)}</b><div class="issue-clarify-options">${options}</div><input class="issue-clarify-other" data-question="${i}" placeholder="其他：自己填写" value="${escapeHTML(picked?.other||'')}"></div>`;
  }).join('');
}
function captureIssueClarifyDraft(host){
  if(!host?.querySelectorAll)return null;
  const active=document.activeElement;
  const others=[...host.querySelectorAll('.issue-clarify-other')];
  const questions=[...host.querySelectorAll('.issue-clarify-question')].map(row=>({
    text:row.querySelector('b')?.textContent||'',
    selected:[...row.querySelectorAll('.issue-clarify-option')].map(b=>b.classList.contains('selected')),
    other:row.querySelector('.issue-clarify-other')?.value||''
  }));
  const extra=host.querySelector('#issueClarifyInput');
  if(!questions.length&&!extra)return null;
  let focus=null;
  if(active&&host.contains?.(active)){
    const sel=typeof active.selectionStart==='number'?{start:active.selectionStart,end:active.selectionEnd}:null;
    if(active.id==='issueClarifyInput')focus={kind:'extra',sel};
    else if(active.classList?.contains('issue-clarify-other'))focus={kind:'other',index:others.indexOf(active),sel};
  }
  return {questions,extra:extra?.value||'',focus,scroll:host.scrollTop||0};
}
function restoreIssueClarifySelection(el,sel){
  if(!el||!sel||typeof el.setSelectionRange!=='function')return;
  try{el.setSelectionRange(sel.start,sel.end)}catch{}
}
function restoreIssueClarifyDraft(host,draft){
  if(!host||!draft)return;
  const rows=[...host.querySelectorAll('.issue-clarify-question')];
  const same=rows.length===draft.questions.length&&rows.every((row,i)=>(row.querySelector('b')?.textContent||'')===draft.questions[i].text);
  if(!same)return;
  rows.forEach((row,i)=>{
    const saved=draft.questions[i];
    [...row.querySelectorAll('.issue-clarify-option')].forEach((b,j)=>b.classList.toggle('selected',!!saved.selected[j]));
    const other=row.querySelector('.issue-clarify-other');
    if(other)other.value=saved.other||'';
  });
  const extra=host.querySelector('#issueClarifyInput');
  if(extra)extra.value=draft.extra||'';
  if(host.scrollTop!=null)host.scrollTop=draft.scroll||0;
  if(draft.focus?.kind==='extra'&&extra){
    extra.focus?.();
    restoreIssueClarifySelection(extra,draft.focus.sel);
  }else if(draft.focus?.kind==='other'){
    const input=host.querySelectorAll('.issue-clarify-other')[draft.focus.index];
    if(input){input.focus?.();restoreIssueClarifySelection(input,draft.focus.sel)}
  }
}
function renderIssueDetailTab(){
  const data=state.issueDetailData;const host=$('#issueDetailBody');if(!data||!host)return;
  const issue=data.issue;if(!issue){host.innerHTML='<div class="detail-empty">未找到该需求</div>';renderedIssueClarifyKey='';return}
  const project=(state.data.projects||[]).find(p=>p.id===issue.projectId);
  $('#issueDetailKey').textContent=issueKeyLabel(issue);
  $('#issueDetailTitle').textContent=issue.title||'';
  $('#issueDetailSubtitle').textContent=`${project?.name||'—'} · ${issuePriorityLabel(issue.priority)} · ${issueStatusText(issue)} · 更新于 ${new Date(issue.updatedAt).toLocaleString()}`;
  // 答案只存在输入框里。问题没变时跳过重绘，避免轮询和运行推送把正在填的内容清掉。
  const clarifyKey=state.issueDetailTab==='clarify'?issueClarifyViewKey(issue,project):'';
  if(clarifyKey&&clarifyKey===renderedIssueClarifyKey&&host.querySelector?.('.clarify-workspace'))return;
  const clarifyDraft=clarifyKey?captureIssueClarifyDraft(host):null;
  const panes={overview:overviewPane(issue,data),operations:operationsPane(issue),clarify:clarifyPane(issue,project),delivery:deliveryPane(issue,data),activity:activityPane(issue,data)};
  host.innerHTML=panes[state.issueDetailTab]||'<div class="detail-empty">暂无内容</div>';
  renderedIssueClarifyKey=clarifyKey;
  host.querySelectorAll('[data-edit-issue]').forEach(b=>b.onclick=()=>{closeIssueDrawer();openIssue(issue)});
  bindUserSearches(host);
  host.querySelectorAll('[data-push-tapd]').forEach(b=>b.onclick=()=>pushIssueToTAPDFromDetail(b.dataset.pushTapd));
  host.querySelectorAll('[data-resync-tapd]').forEach(b=>b.onclick=()=>resyncTAPDIssueById(b.dataset.resyncTapd));
  host.querySelectorAll('[data-reopen-issue]').forEach(b=>b.onclick=()=>reopenDeliveredIssue(b.dataset.reopenIssue));
  host.querySelectorAll('[data-delete-issue]').forEach(b=>b.onclick=()=>deleteIssueById(b.dataset.deleteIssue||issue.id));
  bindIssueDeletionContext(host);
  host.querySelectorAll('[data-close-issue]').forEach(b=>b.onclick=()=>closeIssueById(b.dataset.closeIssue||issue.id));
  host.querySelectorAll('[data-start-clarify]').forEach(b=>b.onclick=()=>openIssueClarification(true));
  host.querySelectorAll('[data-continue-clarify]').forEach(b=>b.onclick=()=>{
    const current=state.issueDetailData?.issue;
    if(current?.clarification?.lastError){retryIssueClarifyTurn();return;}
    sendIssueClarification();
  });
  host.querySelectorAll('[data-stop-clarify]').forEach(b=>b.onclick=()=>stopIssueClarification(issue.id));
  host.querySelectorAll('[data-retry-clarify]').forEach(b=>b.onclick=()=>retryIssueClarificationWriteBack());
  host.querySelectorAll('[data-retry-clarify-turn]').forEach(b=>b.onclick=()=>retryIssueClarifyTurn());
  host.querySelectorAll('[data-reclarify]').forEach(b=>b.onclick=()=>openIssueClarification(false,true));
  host.querySelectorAll('.issue-clarify-option').forEach(b=>b.onclick=()=>{
    if(b.dataset.multi==='true'){
      b.classList.toggle('selected');
    }else{
      const wasSelected=b.classList.contains('selected');
      b.parentElement.querySelectorAll('.issue-clarify-option').forEach(x=>x.classList.remove('selected'));
      if(!wasSelected)b.classList.add('selected');
    }
  });
  host.querySelectorAll('[data-clarify-model]').forEach(picker=>bindClarifyModelPicker(picker,issue));
  if(state.issueDetailTab==='clarify'&&issueClarifying(issue.id))restoreIssueClarifyThinking(issue.id);
  if(clarifyDraft)restoreIssueClarifyDraft(host,clarifyDraft);
}
function drawerFact(label,value,title){return `<div class="detail-fact"><span>${label}</span><b title="${escapeHTML(title||value||'')}">${value===''||value==null?'—':value}</b></div>`}
function clarificationOverviewCard(issue){
  const c=issue.clarification||{},source=issue.source==='gongfeng'?'工蜂':issue.source==='tapd'?'TAPD':'LoopForge';
  if(issueClarifying(issue.id)||c.status==='analyzing'){
    return `<div class="clarify-overview"><span>…</span><div><b>澄清正在后台分析</b><small>关闭网页也不会中断，点查看可回到分析过程</small></div><button class="button ghost small" data-start-clarify>查看</button></div>`;
  }
  if(c.status==='ready'&&c.summary){
    const failed=c.writeBackStatus==='failed';
    const pending=c.writeBackStatus==='pending';
    const cls=failed||pending?'failed':'ready';
    const title=failed?'澄清已完成，写回失败':pending?'澄清已完成，待确认写回':`已澄清 · ${c.writeBackStatus==='succeeded'?`已写回 ${source}`:'结论已保存'}`;
    return `<div class="clarify-overview ${cls}"><span>${failed||pending?'!':'✓'}</span><div><b>${title}</b><small>${escapeHTML((c.summary||'').slice(0,120))}${c.summary&&c.summary.length>120?'…':''}</small></div><button class="button ghost small" data-start-clarify>查看</button></div>`;
  }
  if(c.status==='in_progress'){
    return `<div class="clarify-overview"><span>…</span><div><b>澄清尚未完成</b><small>继续回答关键问题，进度已经保存；不影响直接触发 LOOP</small></div><button class="button ghost small" data-start-clarify>继续</button></div>`;
  }
  return `<div class="clarify-overview"><span>?</span><div><b>描述不够清楚？</b><small>结合原单和代码问清关键细节</small></div><button class="button ghost small" data-start-clarify>帮我问清楚</button></div>`;
}
function setIssueClarifyTab(){
  state.issueDetailTab='clarify';
  $$('#issueDetailTabs .drawer-tab').forEach(b=>b.classList.toggle('active',b.dataset.dtab==='clarify'));
}
const CLARIFY_AUTO_PROMPTS=['请先检查这张工作项的描述和相关代码，告诉我还缺哪些关键信息。','请基于已有澄清结论继续检查，还有哪些重要信息需要补充？'];
function clarifyKickoffPending(issue){
  const c=issue?.clarification||{};
  if(c.status==='ready'||c.status==='in_progress')return false;
  return !(c.questions||[]).length;
}
function openIssueClarification(autoStart=false,restart=false){
  const issue=state.issueDetailData?.issue;if(!issue)return;
  setIssueClarifyTab();
  if(issueClarifying(issue.id)&&!restart){
    renderIssueDetailTab();
    restoreIssueClarifyThinking(issue.id);
    attachIssueClarifyStream(issue.id);
    return;
  }
  issue.clarification=issue.clarification||{};
  if(restart||!issue.clarification.status||issue.clarification.status==='needs_user'||issue.clarification.status==='analyzing'){
    if(restart)issue.clarification.questions=[];
  }
  renderIssueDetailTab();
  if(autoStart&&issue.clarification.status==='analyzing')attachIssueClarifyStream(issue.id);
  else if(autoStart&&clarifyKickoffPending(issue))void sendIssueClarification(CLARIFY_AUTO_PROMPTS[0]);
  else if(restart)void sendIssueClarification(CLARIFY_AUTO_PROMPTS[1]);
}
function collectIssueClarifyReply(){
  const parts=[];
  $$('.issue-clarify-question').forEach(row=>{
    const question=row.querySelector('b')?.textContent||'';
    const selected=[...row.querySelectorAll('.issue-clarify-option.selected')].map(x=>x.textContent.trim());
    const other=row.querySelector('.issue-clarify-other')?.value.trim();
    const answers=[...selected,...(other?[other]:[])];
    if(answers.length)parts.push(`${question} ${answers.join('、')}`);
  });
  const extra=$('#issueClarifyInput')?.value.trim();
  if(extra)parts.push(extra);
  return parts.join('\n');
}
function retryIssueClarifyTurn(){
  const issue=state.issueDetailData?.issue;if(!issue)return;
  const last=(issue.clarification?.history||[]).filter(h=>h.role==='user').pop();
  sendIssueClarification(last?.content||CLARIFY_AUTO_PROMPTS[0]);
}
function issueClarifyThinking(ev,issueId){
  const job=issueId?issueClarifyJobs.get(issueId):null;
  if(job)job.thinking.push(ev);
  const box=$('#issueClarifyThinking');if(!box)return;
  if(job&&state.issueDetailData?.issue?.id!==issueId)return;
  appendIssueClarifyThought(box,ev);
}
function appendIssueClarifyThought(box,ev){
  if(ev.kind!=='tool'&&clarifyThoughtShouldHide(ev&&ev.text))return;
  box.hidden=false;
  const row=document.createElement('div');
  row.className=`think-row ${ev.kind==='tool'?'think-tool':(ev.kind==='status'?'think-status':'think-idea')}`;
  row.innerHTML=`<span class="think-icon">${ev.kind==='tool'?'🔍':(ev.kind==='status'?'⏳':'💭')}</span><span class="think-text">${escapeHTML(ev.kind==='tool'?`${clarifyToolLabel(ev.tool)}${ev.detail?' · '+ev.detail:''}`:(ev.text||''))}</span>`;
  box.appendChild(row);box.scrollTop=box.scrollHeight;
}
function restoreIssueClarifyThinking(issueId){
  const job=issueClarifyJobs.get(issueId),box=$('#issueClarifyThinking');
  if(!box)return;
  if(job){
    box.innerHTML='';
    job.thinking.forEach(ev=>appendIssueClarifyThought(box,ev));
  }
  const button=$('[data-continue-clarify]'),stop=$('[data-stop-clarify]'),hint=$('#issueClarifyHint');
  if(button){button.disabled=true;button.textContent='分析中…'}
  if(stop)stop.hidden=false;
  if(hint)hint.textContent='正在后台分析，关闭网页也不会中断';
}
function stopIssueClarification(issueId){
  const job=issueClarifyJobs.get(issueId);
  if(job)job.stopping=true;
  api(`/api/issues/${encodeURIComponent(issueId)}/chat-clarify/cancel`,{method:'POST',body:'{}'}).catch(()=>{});
  if(job?.abort)job.abort.abort();
}
async function attachIssueClarifyStream(issueId){
  if(!issueId)return;
  const existing=issueClarifyJobs.get(issueId);
  if(existing?.abort&&!existing.abort.signal.aborted&&existing.attached)return;
  const abort=new AbortController();
  const job={abort,thinking:existing?.thinking||[],attached:true};
  issueClarifyJobs.set(issueId,{...job});
  renderIssues();
  try{
    const resp=await fetch(`/api/issues/${encodeURIComponent(issueId)}/chat-clarify`,{headers:{Accept:'text/event-stream'},signal:abort.signal});
    if(resp.status===404){
      issueClarifyJobs.delete(issueId);
      return;
    }
    if(!resp.ok){let em=`HTTP ${resp.status}`;try{const d=await resp.json();em=d.error||em}catch{}throw new Error(em)}
    const data=await consumeIssueClarifyStream(resp,issueId);
    await applyIssueClarifyDone(issueId,data);
  }catch(err){
    if(err.name==='AbortError'){
      const current=issueClarifyJobs.get(issueId);
      if(current?.stopping)toast('已停止本轮澄清');
      return;
    }
    toast(`澄清失败：${err.message}`,'err');
    setIssueClarifyLastError(issueId,err.message);
  }finally{
    finishIssueClarifyJob(issueId,abort);
  }
}
function finishIssueClarifyJob(issueId,abort){
  const current=issueClarifyJobs.get(issueId);
  if(current?.abort===abort)issueClarifyJobs.delete(issueId);
  if(state.drawerOpen&&state.issueDetailData?.issue?.id===issueId)renderIssueDetailTab();
  renderIssues();
}
function setIssueClarifyLastError(issueId,message){
  const issue=state.issueDetailData?.issue;
  if(issue?.id!==issueId)return;
  issue.clarification=issue.clarification||{};
  issue.clarification.lastError=message||'';
}
async function applyIssueClarifyDone(issueId,data){
  if(!data||data.error)throw new Error(data?.error||'分析未返回结果');
  let updated=data.issue;
  if(!updated){
    try{const detail=await api(`/api/issues/${encodeURIComponent(issueId)}`);updated=detail.issue}catch{}
  }
  if(updated){
    const idx=(state.data.issues||[]).findIndex(x=>x.id===updated.id);if(idx>=0)state.data.issues[idx]=updated;
    if(state.issueDetailData?.issue?.id===issueId)state.issueDetailData.issue=updated;
  }
  if(state.drawerOpen&&state.issueDetailData?.issue?.id===issueId)renderIssueDetailTab();
  else{
    renderIssues();
    if($('#dashboardView')?.classList.contains('active'))renderDashboard();
  }
  const result=parseClarifyPayload(data);
  if(result.ready){
    const wb=updated?.clarification?.writeBackStatus;
    if(wb==='pending')toast('澄清完成，请确认结论后再写回原单','ok');
    else if(wb==='succeeded')toast('澄清完成，已写回原单','ok');
    else toast('澄清完成，结论已保存','ok');
  }
  else toast('澄清已更新，可以回来继续补充','ok');
}
async function consumeIssueClarifyStream(resp,issueId){
  const reader=resp.body.getReader(),decoder=new TextDecoder();let buffer='',done=null;
  const dispatch=(event,dataStr)=>{let payload={};try{payload=JSON.parse(dataStr)}catch{}if(event==='thought')issueClarifyThinking(payload,issueId);else if(event==='done')done=payload;else if(event==='error')done={error:payload.error||'分析失败'}};
  for(;;){
    const next=await reader.read();if(next.done)break;
    buffer+=decoder.decode(next.value,{stream:true});let idx;
    while((idx=buffer.indexOf('\n\n'))>=0){const chunk=buffer.slice(0,idx);buffer=buffer.slice(idx+2);let event='message',data='';chunk.split('\n').forEach(line=>{if(line.startsWith('event:'))event=line.slice(6).trim();else if(line.startsWith('data:'))data+=line.slice(5).trim()});if(data)dispatch(event,data)}
  }
  return done||{error:'分析未返回结果'};
}
async function sendIssueClarification(initialMessage=''){
  const issue=state.issueDetailData?.issue;if(!issue)return;
  const issueId=issue.id;
  if(issueClarifyJobs.has(issueId)&&issueClarifyJobs.get(issueId)?.attached)return toast('澄清正在后台进行');
  if(issue.clarification?.status==='analyzing'&&!initialMessage){
    attachIssueClarifyStream(issueId);
    return;
  }
  const picker=$('#issueDetailBody [data-clarify-model]');
  await picker?.modelPickerReady;
  if(state.issueDetailData?.issue?.id!==issueId||issueClarifyJobs.get(issueId)?.attached)return;
  if(picker&&!picker.isConnected&&(!initialMessage||!state.drawerOpen||state.issueDetailTab!=='clarify'))return;
  const model=picker?.querySelector('[data-feature-model]')?.value||undefined;
  if(!model&&!initialMessage)return toast('暂无可选模型，请检查个人配置中的执行器');
  const msg=initialMessage||collectIssueClarifyReply();
  if(!msg)return toast('请先选择或填写答案');
  const c=issue.clarification||(issue.clarification={});
  rememberSubmittedClarifyDraft(issue,msg);
  c.lastError='';
  const prev=c.history||[];
  const last=prev[prev.length-1];
  const history=(last&&last.role==='user'&&last.content===msg)?prev.slice():[...prev,{role:'user',content:msg}];
  c.status='analyzing';
  c.history=history;
  const idxSeed=(state.data.issues||[]).findIndex(x=>x.id===issueId);if(idxSeed>=0)state.data.issues[idxSeed].clarification=c;
  const abort=new AbortController();
  issueClarifyJobs.set(issueId,{abort,thinking:[],attached:true});
  renderIssueDetailTab();
  renderIssues();
  try{
    const resp=await fetch(`/api/issues/${encodeURIComponent(issueId)}/chat-clarify`,{method:'POST',headers:{'Content-Type':'application/json','Accept':'text/event-stream'},body:JSON.stringify({message:msg,history,model}),signal:abort.signal});
    if(!resp.ok){let em=`HTTP ${resp.status}`;try{const d=await resp.json();em=d.error||em}catch{}throw new Error(em)}
    const data=(resp.headers.get('Content-Type')||'').includes('text/event-stream')?await consumeIssueClarifyStream(resp,issueId):await resp.json();
    await applyIssueClarifyDone(issueId,data);
  }catch(err){
    if(c.status==='analyzing')c.status=(c.questions||[]).length?'in_progress':'needs_user';
    if(err.name==='AbortError'){
      const current=issueClarifyJobs.get(issueId);
      if(current?.stopping)toast('已停止本轮澄清');
      return;
    }
    toast(`澄清失败：${err.message}`,'err');
    setIssueClarifyLastError(issueId,err.message);
  }finally{
    finishIssueClarifyJob(issueId,abort);
  }
}
async function retryIssueClarificationWriteBack(){
  const issue=state.issueDetailData?.issue;if(!issue)return;
  try{
    const updated=await api(`/api/issues/${encodeURIComponent(issue.id)}/clarification`,{method:'POST',body:JSON.stringify({ready:true,retry:true})});
    state.issueDetailData.issue=updated;
    const idx=(state.data.issues||[]).findIndex(x=>x.id===updated.id);
    if(idx>=0)state.data.issues[idx]=updated;
    renderIssueDetailTab();
    renderIssues();
    toast(updated.clarification?.writeBackStatus==='succeeded'?'已写回原单评论，现在可以触发需求了！':updated.clarification?.writeBackStatus==='pending'?'结论已保存，尚未写回原单':`写回仍未成功：${updated.clarification?.writeBackError||'未知错误'}`,updated.clarification?.writeBackStatus==='succeeded'?'ok':'err');
  }catch(e){toast(e.message,'err')}
}
async function closeIssueById(issueId){
  const issue=(state.data.issues||[]).find(i=>i.id===issueId);
  if(!issue)return toast('未找到需求单');
  if(issueManuallyClosed(issue)||issueTrackerClosed(issue))return toast('该需求单已处于关闭状态');
  if(!issueCanClose(issue))return toast('需求正在澄清或执行 LOOP，请先停止后再关闭','err');
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
async function resyncTAPDIssueById(issueId){
  const issue=(state.data.issues||[]).find(i=>i.id===issueId)||state.issueDetailData?.issue;
  if(!issue)return toast('未找到需求单');
  if(issue.source!=='tapd'||!issue.externalRef?.externalId)return toast('仅 TAPD 来源的需求支持重新同步');
  try{
    const updated=await api(`/api/issues/${encodeURIComponent(issueId)}/resync-tapd`,{method:'POST'});
    const idx=(state.data.issues||[]).findIndex(i=>i.id===issueId);
    if(idx>=0)state.data.issues[idx]=updated;
    if(state.issueDetailData?.issue?.id===issueId){
      state.issueDetailData.issue=updated;
      renderIssueDetailTab();
    }
    renderIssues();
    toast('已从 TAPD 同步最新内容','ok');
  }catch(err){
    toast(err.message,'err');
  }
}
function issueProgressActionsHTML(issue,{includeIssueActions=false}={}){
  const issueActions=[];
  if(includeIssueActions){
    issueActions.push('<button class="button ghost small" data-edit-issue>编辑需求</button>');
    if(issueCanPushTAPD(issue))issueActions.push(`<button class="button ghost small" data-push-tapd="${escapeHTML(issue.id)}">同步到 TAPD</button>`);
    if(issueCanClose(issue)&&issue.status!=='done'&&!issueManuallyClosed(issue)&&!issueTrackerClosed(issue)&&!deliveryLocked(issue))issueActions.push(`<button class="button ghost small" data-close-issue="${escapeHTML(issue.id)}">关闭需求</button>`);
    issueActions.push(issueDeleteControlHTML(issue));
    if(issue.externalRef?.externalUrl)issueActions.push(`<a class="button ghost small" href="${escapeHTML(safeURL(issue.externalRef.externalUrl))}" target="_blank" rel="noopener noreferrer">打开外部链接 ↗</a>`);
    if(issue.source==='tapd'&&issue.externalRef?.externalId)issueActions.push(`<button class="button ghost small" data-resync-tapd="${escapeHTML(issue.id)}">从 TAPD 同步</button>`);
  }
  const group=(title,items)=>items.some(Boolean)?`<div class="issue-action-group"><span>${title}</span><div class="issue-action-group-buttons">${items.filter(Boolean).join('')}</div></div>`:'';
  const groups=[group('需求管理',issueActions)].filter(Boolean);
  return groups.length?`<div class="issue-action-groups">${groups.join('')}</div>`:'';
}
function overviewPane(issue,data){
  const project=(state.data.projects||[]).find(p=>p.id===issue.projectId),repo=(state.data.repositories||[]).find(r=>r.id===(issue.repositoryId||project?.defaultRepositoryId));
  const out=issueOutcome(issue);
  let actions='';
  if(issueManuallyClosed(issue)||deliveryLocked(issue))actions=reopenDeliveredButton(issue)+deliveryMRButtons(issue);
  actions+=issueProgressActionsHTML(issue,{includeIssueActions:true});
  const banner=`<div class="issue-outcome ${out.key}"><b>${escapeHTML(out.title)}</b><p>${escapeHTML(out.body)}</p><div class="issue-outcome-actions">${actions}</div></div>`;
  const facts=`<div class="detail-grid">${drawerFact('责任人',issueAssigneeName(issue)||'未指定')}${drawerFact('优先级',issuePriorityLabel(issue.priority))}${drawerFact('来源',issueSourceLabel(issue))}${metricsIssueSource(issue)==='local'?drawerFact('统计关联 TAPD',escapeHTML(issue.tapdId||'未绑定（仍参与统计）')):''}${drawerFact('状态',escapeHTML(issueStatusLabel(issue.status)))}${drawerFact('代码仓库',escapeHTML(repo?.name||'未绑定'))}${drawerFact('继续开发分支',escapeHTML(storedIssueWorkBranch(issue)||'自动新建'))}</div>`;
  const description=issue.description?`<div class="markdown-body">${renderRequirementMarkdown(issue.description)}</div>`:'';
  const images=(data.images||[]).map(image=>{
    const url=`/api/issues/${encodeURIComponent(issue.id)}/images/${encodeURIComponent(image.id)}`;
    const name=escapeHTML(image.name||'需求图片');
    return `<li><a href="${url}" target="_blank" rel="noopener noreferrer"><img src="${url}" alt="${name}" loading="lazy"></a></li>`;
  }).join('');
  const gallery=images?`<ul class="issue-description-images" aria-label="需求图片">${images}</ul>`:'';
  const content=description||gallery?`<div class="detail-desc issue-description">${description}${gallery}</div>`:'';
  return `<div class="detail-section">${banner}${facts}${content}${clarificationOverviewCard(issue)}</div>`;
}
function clarifyHistoryHTML(c){
  const rows=[];
  (c.history||[]).forEach(msg=>{
    const content=(msg.content||'').trim();
    if(!content)return;
    if(msg.role==='assistant'){
      let parsed={};try{parsed=JSON.parse(content)}catch{}
      const questions=(parsed.questions||[]).map(q=>`<li>${escapeHTML(q.question||'')}</li>`).join('');
      if(questions)rows.push(`<div class="clarify-turn ai"><span>问</span><ul>${questions}</ul></div>`);
    }else if(!CLARIFY_AUTO_PROMPTS.includes(content)){
      rows.push(`<div class="clarify-turn me"><span>答</span><p>${escapeHTML(content)}</p></div>`);
    }
  });
  if(!rows.length)return'';
  return `<details class="clarify-history"><summary>澄清过程（${rows.length} 条）</summary><div class="clarify-turns">${rows.join('')}</div></details>`;
}
function clarifyPane(issue,project){
  const c=recoverClarifyView(issue.clarification||{}),source=issue.source==='gongfeng'?'工蜂 Issue':issue.source==='tapd'?'TAPD 需求':'工作项';
  if(c.status==='ready'&&submittedClarifyDraft?.issueId===issue.id)submittedClarifyDraft=null;
  const writeBack=c.writeBackStatus==='succeeded'?`已写回 ${source}`:c.writeBackStatus==='failed'?'写回失败':c.writeBackStatus==='skipped'?'仅保存到 LoopForge':'';
  if(c.status==='ready'&&c.summary){
    const pending=c.writeBackStatus==='pending'&&(issue.source==='tapd'||issue.source==='gongfeng')&&issue.externalRef;
    const retry=c.writeBackStatus==='failed'?`<button class="button primary small" data-retry-clarify>重试写回原单</button>`:'';
    const confirm=pending?`<button class="button primary small" data-retry-clarify title="确认写回原单（追加评论）之后，即可开始触发需求">确认写回原单（追加评论）</button>`:'';
    const external=issue.externalRef?.externalUrl?`<a class="button ghost small" href="${escapeHTML(safeURL(issue.externalRef.externalUrl))}" target="_blank" rel="noopener noreferrer">打开原单 ↗</a>`:'';
    const stepGuide=pending?`<div class="clarify-next-step-callout"><div class="clarify-step-badge active"><span class="step-num">1</span><span>确认写回原单（追加评论）</span></div></div>`:'';
    const head=pending
      ?`<div class="clarify-ready-head"><span class="clarify-check warn">!</span><div><b>请确认澄清结论并写回原单</b><small>结论已追加到平台说明。确认写回原单（追加评论）后即可继续（不会修改原单描述）</small></div></div>${stepGuide}`
      :`<div class="clarify-ready-head"><span class="clarify-check">✓</span><div><b>澄清已完成${c.writeBackStatus==='succeeded'?` · 已写回 ${source} 评论`:''}</b><small class="${c.writeBackStatus==='failed'?'danger':''}">${escapeHTML(writeBack||'结论已保存到平台说明')}</small></div></div>`;
    const actionTip=pending
      ?`<div class="clarify-action-hint">确认写回原单（追加评论）后即可继续</div>`
      :'';
    return `<div class="clarify-workspace">${head}${clarifyVisibleSummary(c.summary)?`<div class="clarify-summary">${escapeHTML(clarifyVisibleSummary(c.summary))}</div>`:''}${c.writeBackError?`<div class="clarify-error">${escapeHTML(c.writeBackError)}</div>`:''}${clarifyHistoryHTML(c)}<div class="clarify-actions">${confirm}${retry}${external}<button class="button ghost small" data-reclarify>再问一轮</button>${actionTip}</div></div>`;
  }
  const questions=c.questions||[];
  const analyzing=issueClarifying(issue.id);
  const saved=clarifyAnswerDraft(issue);
  const questionHTML=analyzing?'':clarifyQuestionCardsHTML(questions,saved);
  const submittedHTML=analyzing?clarifySubmittedHTML(saved?.reply):'';
  if(c.status!=='in_progress'&&!analyzing){
    const intro=`<div class="clarify-orb">?</div><h3>描述不够清楚？一起问明白</h3><p>可选步骤。分析师只围绕这张单本身，结合本项目代码问清阻塞开工的细节。</p>`;
    return `<div class="clarify-workspace clarify-empty">${intro}<div class="clarify-start-row">${clarifyModelPickerHTML(false)}<button class="button primary" data-start-clarify>开始澄清</button></div></div>`;
  }
  const lastError=(!analyzing&&c.lastError)?`<div class="clarify-error req-clarify-retry"><span>${escapeHTML(c.lastError)}</span><button type="button" class="button primary small" data-retry-clarify-turn>重新分析</button></div>`:'';
  const rounds=(c.history||[]).filter(m=>m.role==='assistant').length;
  // Own text node so the sentence matches an English entry; a trailing count does not.
  const roundNote=(!analyzing&&!c.lastError)?`已澄清 ${rounds||0} 轮。`:'';
  const nextHint=analyzing?'正在后台分析，关闭详情不会中断':(c.lastError?'上次分析未完成，可重新分析':'回答后点「继续澄清」。分析师确认够用后会给出结论。');
  const composerInput=analyzing?'':`<textarea id="issueClarifyInput" rows="3" placeholder="${questions.length?'点选上面的答案，或补充说明':'补充你认为重要的信息'}">${escapeHTML(saved?.extra||'')}</textarea>`;
  return `<div class="clarify-workspace"><div class="clarify-intro"><span>AI</span><div><b>${analyzing?'正在分析这张':'继续完善这张'}${issue.source==='gongfeng'?' Issue':'需求'}</b><small>${analyzing?'关闭详情不会中断，再打开可继续看过程':'每轮只问最关键的 1–3 个问题，只围绕这张单本身'}</small></div>${clarifyModelPickerHTML(analyzing)}</div>${clarifyVisibleSummary(c.summary)?`<div class="clarify-summary compact">${escapeHTML(clarifyVisibleSummary(c.summary))}</div>`:''}${clarifyHistoryHTML(c)}${submittedHTML}${questionHTML}${lastError}<div id="issueClarifyThinking" class="req-thinking-stream" hidden></div><div class="issue-clarify-composer">${composerInput}<div><span id="issueClarifyHint">${nextHint}${roundNote?` <span>${roundNote}</span>`:''}</span><span><button class="button ghost small" data-stop-clarify ${analyzing?'':'hidden'}>停止</button> <button class="button primary small" data-continue-clarify${analyzing?' disabled':''}>${analyzing?'分析中…':(c.lastError?'重新分析':'继续澄清')}</button></span></div></div></div>`;
}
function latestClarifyParsed(c){
  const rows=[...(c?.history||[])].reverse();
  for(const msg of rows){
    if(msg.role!=='assistant')continue;
    try{
      const parsed=JSON.parse(msg.content||'');
      if(parsed&&typeof parsed==='object'&&(parsed.questions||parsed.ready))return parsed;
    }catch{}
  }
  return null;
}
function clarifyVisibleSummary(s){
  const t=String(s||'').trim();
  if(!t)return '';
  const proto=clarifyExtractProtocol(t);
  if(proto&&( 'mcpCalls' in proto||'suggestedTitle' in proto||('summary' in proto&&'questions' in proto))){
    const head=t.charAt(0)==='{'||t.charAt(0)==='['?'':t.slice(0,t.indexOf('{')).trim();
    if(head&&!clarifyThoughtShouldHide(head))return head;
    const inner=String(proto.summary||'').trim();
    return inner&&!clarifyThoughtShouldHide(inner)?inner:'';
  }
  if(clarifyThoughtShouldHide(t))return '';
  return t;
}
function clarifyExtractProtocol(s){
  let t=String(s||'').trim();
  if(!t)return null;
  t=t.replace(/^```json\s*/i,'').replace(/^```\s*/,'').replace(/\s*```$/,'');
  const start=t.indexOf('{'),end=t.lastIndexOf('}');
  if(start<0||end<=start)return null;
  try{
    const m=JSON.parse(t.slice(start,end+1));
    return m&&typeof m==='object'?m:null;
  }catch{return null}
}
function recoverClarifyView(src){
  const c={...(src||{})};
  const proto=clarifyExtractProtocol(c.summary);
  if(proto){
    if(!(c.questions||[]).length&&Array.isArray(proto.questions)){
      c.questions=normalizeClarifyParsed(proto).questions;
    }
    c.summary=clarifyVisibleSummary(c.summary);
  }
  return c;
}
function clarifyThoughtShouldHide(s){
  let t=String(s||'').trim();
  if(!t)return false;
  t=t.replace(/^```json\s*/i,'').replace(/^```\s*/,'').replace(/\s*```$/,'');
  if(t.charAt(0)==='{'||t.charAt(0)==='[')return true;
  if(/"questions"\s*:/.test(t)&&(/"summary"\s*:/.test(t)||/"mcpCalls"\s*:/.test(t)))return true;
  const start=t.indexOf('{'),end=t.lastIndexOf('}');
  if(start<0||end<=start)return false;
  try{
    const m=JSON.parse(t.slice(start,end+1));
    if(!m||typeof m!=='object')return false;
    return 'mcpCalls' in m||'suggestedTitle' in m||('summary' in m&&'questions' in m);
  }catch{return false}
}
// 对话需求的「打开会话」：对有权限的项目成员都显示（非创建者只读打开），会话删除后置灰。
// 需求快照没加载到时，从运行列表里的对话工作区取仓库与工作分支。
// 分支名保留斜杠（与 GitHub、工蜂自己的链接一致），其他字符照常编码。
// 仓库网页地址 + 分支 → 分支页；有基线分支时链到对比页，直接看这次改了什么。
const conversationMRBadgeTone={merged:'success',integrated:'success',opened:'info',reopened:'info',closed:'neutral'};
// 对话需求按仓库汇总：工作分支 → 目标分支、PR/MR 与状态放在同一张卡片里，分支和 PR/MR 都能直接跳到代码平台。
// 对话需求的阶段卡片：合入为绿、进行中为蓝；没合入就关闭的保持中性，不当成交付成功。
// 对话里直接改代码的需求没有 LOOP 节点，执行进展展示对话交付：阶段、会话入口、各仓库的分支与 MR。
// 对话需求的代码交付：与执行进展同一套仓库卡片，交付方式指向会话，不链到没有节点的工作区运行。
function deliveryPane(issue,data){
  const repos=(state.data.repositories||[]).filter(r=>r.projectId===issue.projectId);
  const rows=repos.map(r=>`<div class="issue-action-group"><span>${escapeHTML(r.name||r.id)}</span><div class="issue-action-group-buttons"><span class="muted mono">${escapeHTML(r.defaultBranch||'—')}</span></div></div>`).join('');
  return `<div class="detail-section"><h3>代码仓库<span class="sub">需求关联的仓库与分支</span></h3>${rows||'<div class="detail-empty">当前项目还没有绑定代码仓库</div>'}<div class="issue-outcome-actions">${deliveryMRButtons(issue)}</div></div>`;
}
// 多仓库需求按仓库逐组展示；优先用 Run 冻结的仓库上下文（含工作分支），再补需求绑定的仓库。
// LOOP 需求的代码交付。
// Execution order of a LOOP's nodes, same rule as orderNodesByEdges in
// internal/repo/requirement.go: stable topological sort over the edges, array
// order breaks ties, cycles fall back to array order. The editor appends a node
// added in a later LOOP version to the end of the array, so without this a gate
// inserted mid-flow is listed after the last node.
function formatActivityTime(ts){
  const d=new Date(ts);
  if(Number.isNaN(d.getTime()))return '—';
  const p=n=>String(n).padStart(2,'0');
  return `${p(d.getMonth()+1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}
function activityEventLabel(type){
  return ({
    'run.started':'开始执行','run.failed':'执行失败','run.succeeded':'执行完成','run.cancelled':'已取消',
    'node.started':'节点开始','node.failed':'节点失败','node.retry':'准备重试','node.succeeded':'节点完成','node.passed':'人工通过节点',
    'task.created':'创建任务'
  }[type]||String(type||'').replace(/^node\./,'节点 · ').replace(/^run\./,'运行 · '));
}
function issueOperationLabel(action){
  return {create:'创建需求',update:'编辑需求',close:'关闭需求',reopen:'重新打开',dispatch:'触发 LOOP'}[action]||action||'操作';
}
// 聊天 Bot 发起的操作：actor 仍是执行所用的平台账号，这里补出是哪个 IM 用户发起的。
// 渠道报了显示名就用名字，没报（企业微信、飞书只给 ID）就用 ID，后面的平台账号本身带真名。
function issueOperationOrigin(op){
  const sender=(op?.actorSender||'').trim();
  if(!sender)return '';
  const channel={wecom:'企业微信',feishu:'飞书',dingtalk:'钉钉'}[op.actorChannel]||op.actorChannel||'聊天';
  const named=(op.actorSenderName||'').trim();
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
  if(issueManuallyClosed(issue)&&issue.deliveryUpdatedAt&&!stored.some(op=>op.action==='close')){
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

// Association does not expose private conversations to other project members.
