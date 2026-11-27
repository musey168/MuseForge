// --- Sync Panel (embedded in Issues view) ---
function sourceBadge(issue){if(!issue.source||issue.source==='local')return '<span class="sync-badge local">本地</span>';if(issue.source==='github')return '<span class="sync-badge local">GitHub</span>';if(issue.source==='gongfeng')return '<span class="sync-badge gongfeng">工蜂</span>';if(issue.source==='tapd')return '<span class="sync-badge tapd">TAPD</span>';return ''}
function externalLink(issue){if(!issue.externalRef)return '';return `<a href="${escapeHTML(safeURL(issue.externalRef.externalUrl))}" target="_blank" rel="noopener noreferrer" class="ext-link" title="${escapeHTML(issue.externalRef.externalUrl)}">↗</a>`}
function renderSync(){
  const cfg=state.data.syncConfig||{};const logs=state.data.syncLogs||[];
  const syncing=state.data.syncing;
  const projectIssues=(state.data.issues||[]).filter(i=>!state.projectFilter||i.projectId===state.projectFilter);
  const gfCount=projectIssues.filter(i=>i.source==='gongfeng').length;
  const tapdCount=projectIssues.filter(i=>i.source==='tapd').length;
  const githubCount=projectIssues.filter(i=>i.source==='github').length;
  const lastSync=knownTime(cfg.lastSyncAt)?.toLocaleString()||'从未同步';
  // Scope logs to the current project (legacy entries without projectId stay
  // visible everywhere); diagnostics only reflect what the user can act on.
  const scopedLogs=logs.filter(l=>!state.projectFilter||!l.projectId||l.projectId===state.projectFilter);
  // Only the most recent sync round counts for status: stale failures from
  // older rounds (or legacy unscoped entries) must not keep the badge red.
  const recentLogs=scopedLogs.filter(l=>!knownTime(cfg.lastSyncAt)||new Date(l.startedAt).getTime()>=new Date(cfg.lastSyncAt).getTime()-15*60*1000);
  const recentBySource=['github','gongfeng','tapd'].map(source=>recentLogs.find(l=>l.source===source)).filter(Boolean);
  const scopedFails=recentBySource.filter(l=>l.status==='failed');
  const badge=$('#syncBadge');
  if(syncing){badge.className='sync-badge live';badge.textContent='同步中'}
  else if(scopedFails.length){badge.className='sync-badge failed';badge.textContent='同步失败'}
  else if(scopedLogs.some(l=>l.status==='success')||cfg.lastSyncStatus==='success'){badge.className='sync-badge success';badge.textContent='已同步'}
  else{badge.className='sync-badge live';badge.textContent='未配置'}
  const more=$('#issueHeadingMore');
  if(more){const failed=!syncing&&scopedFails.length>0;more.classList.toggle('has-alert',failed);more.title=failed?'外部同步失败':''}
  const latestBySource=['github','gongfeng','tapd'].map(source=>scopedLogs.find(l=>l.source===source)).filter(Boolean);
  const diag=[...new Set(scopedFails.flatMap(l=>String(l.message||'').split(/[；;]/)).map(s=>s.trim()).filter(Boolean))];
  const diagText=diag.join('；');
  $('#syncPanelSummary').innerHTML=`<span class="sync-stat"><b>${githubCount}</b>GitHub</span>${gfCount?`<span class="sync-stat"><b>${gfCount}</b>工蜂</span>`:''}${tapdCount?`<span class="sync-stat"><b>${tapdCount}</b>TAPD</span>`:''}<span class="sync-stat">最近：${lastSync}</span>${latestBySource.map(l=>`<span class="sync-stat ${l.status==='failed'?'danger':'success'}"><b>${l.status==='failed'?'!':'✓'}</b>${l.source==='github'?'GitHub':l.source==='gongfeng'?'工蜂':'TAPD'} ${l.status==='failed'?'异常':'正常'}</span>`).join('')}${diagText?`<span class="sync-stat danger sync-error-detail" title="${escapeHTML(diagText)}"><b>诊断</b>${escapeHTML(diagText)}</span>`:''}`;
  const project=currentProject();
  // POST /api/sync/now and PUT /api/sync/config are platform-wide (every
  // project's repositories), so only platform admins may use them once login
  // is required; without login the server lets everyone through.
  const canSync=!state.authRequired||isPlatformAdmin(),adminOnly=canSync?'':'只有平台管理员可以同步外部工作项或修改同步配置';
  const githubButton=document.querySelector("#importGitHubIssues");if(githubButton){githubButton.disabled=!!syncing||!canSync;githubButton.title=adminOnly}
  for(const id of ['#syncAutoEnabledInline','#syncAutoEnabled','#syncInterval','#saveSyncConfig']){const el=$(id);if(el){el.disabled=!canSync;el.title=adminOnly}}
  const repos=(state.data.repositories||[]).filter(r=>r.projectId===state.projectFilter&&r.provider==='github');
  const repoHost=$('#syncGongfengRepos');
  if(repoHost){
    repoHost.innerHTML=repos.length
      ? `<div class="sync-repo-head">当前项目 GitHub 仓库（${escapeHTML(project?.name||state.projectFilter||'—')}）</div>`+repos.map(r=>`<div class="sync-repo-item"><b>${escapeHTML(r.name)}</b><code>${escapeHTML(r.gongfengProjectId||'—')}</code>${project?.defaultRepositoryId===r.id?'<em>默认</em>':''}</div>`).join('')
      : `<div class="sync-empty">当前项目还没有绑定 GitHub 仓库，去「项目配置」添加。</div>`;
  }
  const configPanel=$('#syncPanelConfig'),editing=configPanel&&configPanel.contains(document.activeElement);
  if(!editing){
    $('#syncGongfengURL').value=cfg.gongfengMcpUrl||'';
    $('#syncTAPDWorkspace').value=project?.tapdWorkspaceId||'';
    $('#syncTAPDIteration').value=project?.tapdIterationId||'';
    $('#syncTAPDURL').value=cfg.tapdMcpUrl||'';
    $('#syncInterval').value=cfg.autoSyncIntervalSec||300;
    $('#syncAutoEnabled').checked=cfg.autoSyncEnabled||false;
  }
  $('#syncAutoEnabledInline').checked=cfg.autoSyncEnabled||false;
  const repoNameOf=id=>{const r=(state.data.repositories||[]).find(x=>x.id===id);return r?r.name:''};
  $('#syncPushIssue').innerHTML=projectIssues.filter(i=>!i.externalRef||i.source==='local').map(i=>{const rn=repoNameOf(i.repositoryId||(i.repositoryIds||[])[0]);return `<option value="${i.id}">${rn?escapeHTML(rn)+' · ':''}${escapeHTML((i.title||'').slice(0,40))}</option>`}).join('')||'<option value="">当前项目暂无可推送项</option>';
  if(syncing){$('#syncNowBtn').textContent='⟳ 同步中…';$('#syncNowBtn').disabled=true}else{$('#syncNowBtn').textContent='⟳ 立即同步';$('#syncNowBtn').disabled=!canSync}
  $('#syncNowBtn').title=adminOnly;
  $('#syncLogList').innerHTML=scopedLogs.length?scopedLogs.slice(0,20).map(l=>`<div class="sync-log-item"><span class="sync-log-direction ${l.direction}">${l.direction==='pull'?'拉取':'推送'}</span><span class="sync-log-source">${l.source}</span><span class="sync-log-message">${escapeHTML(l.message||l.action)}</span><span class="sync-log-status ${l.status}">${l.status==='success'?'成功':'失败'}</span><span class="sync-log-time">${new Date(l.startedAt).toLocaleTimeString()}</span></div>`).join(''):'<div class="sync-empty">暂无同步记录</div>';
  renderTrackerImport();
}
// trackerImportLabel 是最近一次导入的结果：时间、结论和原因。
function trackerImportLabel(run){
  if(!run)return '尚未导入';
  const when=knownTime(run.finishedAt||run.startedAt)?.toLocaleString()||'—';
  const label=run.status==='success'?'成功':run.status==='partial'?'部分成功':'失败';
  return `最近导入：${when} · ${label}${run.reason?` · ${run.reason}`:''}`;
}
// renderTrackerImport 画出最近一次导入的结论和每个仓库的结果。
function renderTrackerImport(){
  const run=state.data.trackerImport;
  const status=$('#trackerImportStatus');
  if(status){status.textContent=trackerImportLabel(run);status.className=`sync-import-status ${run&&run.status?run.status:''}`}
  const host=$('#trackerImportRepos');
  if(!host)return;
  const projectName=id=>{const p=(state.data.projects||[]).find(x=>x.id===id);return p?p.name:''};
  const outcomes=(run&&run.repositories)||[];
  host.innerHTML=outcomes.length?outcomes.map(o=>{
    const elsewhere=o.projectId&&state.projectFilter&&o.projectId!==state.projectFilter?`（${projectName(o.projectId)||o.projectId}）`:'';
    return `<div class="sync-repo-item"><b>${escapeHTML(o.name||o.repositoryId||'')}</b>${elsewhere?`<span class="sync-stat"><b>·</b>${escapeHTML(elsewhere)}</span>`:''}<span class="sync-stat ${o.status==='failed'?'danger':'success'}"><b>${o.status==='failed'?'!':'✓'}</b>${o.status==='failed'?'失败':'成功'}</span>${o.reason?`<span class="sync-import-reason">${escapeHTML(o.reason)}</span>`:''}</div>`;
  }).join(''):'';
}
// loadTrackerImport 读出最近一次导入的结果；它存在服务端，重启后仍在。
async function loadTrackerImport(){
  try{
    const answer=await api('/api/tracker/import?provider=github',{silent:true});
    state.data.trackerImport=(answer&&answer.run)||null;
  }catch(e){
    state.data.trackerImport=null;
  }
  renderTrackerImport();
}
// triggerTrackerImport 立即导入 GitHub 需求，并刷新需求列表和导入结论。
async function triggerTrackerImport(){
  const button=$('#trackerImportBtn');
  if(button){button.disabled=true;button.textContent='⟳ 导入中…'}
  try{
    const answer=await api('/api/tracker/import',{method:'POST',body:JSON.stringify({provider:'github',projectId:state.projectFilter||''})});
    state.data.trackerImport=(answer&&answer.run)||null;
    renderTrackerImport();
    await refreshIssues();
    const run=state.data.trackerImport;
    if(run&&run.status==='success')toast(`已导入 ${run.imported} 条需求`);
    else if(run&&run.status==='partial')toast(`部分仓库导入失败：${run.reason||''}`);
    else toast(`导入失败：${run&&run.reason?run.reason:'没有可导入的仓库'}`);
  }catch(e){
    toast(e.message||'导入失败');
  }finally{
    if(button){button.disabled=false;button.textContent='⟳ 立即导入 GitHub 需求'}
  }
}
function scheduleModelAPI(){return window.AgentFlowAssistantModels}
function clarifyModelPickerHTML(disabled){
  return `<div class="feature-model-picker clarify-model-picker" data-clarify-model ${disabled?'data-picker-readonly':''}>${scheduleModelAPI()?.featurePickerHTML()||''}</div>`;
}
let clarifyModelChoice;
function bindClarifyModelPicker(host, issue){
  const models=scheduleModelAPI();
  return models?.bindFeaturePicker(host,{
    model:clarifyModelChoice?.issueId===issue.id?clarifyModelChoice.model:'',
    legacyModel:localStorage.getItem('agentflow.clarifyModel'),
    readOnly:host.hasAttribute('data-picker-readonly'),
    onChange:model=>{clarifyModelChoice={issueId:issue.id,model}},
    loadCatalog:()=>models.fetchCatalog(`/api/issues/${encodeURIComponent(issue.id)}/clarify-models`)
  });
}
// The AI 定时任务 model picker: a CLI select narrows the model list to that
// CLI, like the chat composer's CLI switch. The menu is fixed-positioned so the
// modal's scroll area cannot clip it, and opens toward the side with room.
function scheduleModelCatalog(){
  return window.AgentFlowAssistantModels?.catalog?.()||{};
}
// Call with the menu shown: a short list must sit against its toggle, so the
// placement uses the rendered height, not the maximum.
// The pinned conversation is chosen by the server on the first fixed run.
// Only jobs that open a conversation per execution choose how many to keep.
// The schedule is started and stopped on the row, not in the editor.
// AI job rows keep 立即运行 on the row; the rest sits in a ⋯ menu. Items keep
// the data-* hooks the row buttons used, so the list binds them as before.
// AI jobs sharing a name fold into one collapsed group, placed where the first
// of them sits; expanded groups stay open across poll re-renders.
// Saving an AI job keeps whatever the row switch last set; new jobs start enabled.
async function refreshSyncStatus(){try{const wasSyncing=!!state.data.syncing,status=await api('/api/sync/status',{silent:true});state.data.syncLogs=status.logs||[];state.data.syncing=status.syncing||false;state.data.syncConfig=status.config||state.data.syncConfig;renderSync();if(wasSyncing&&!state.data.syncing)await refreshIssues()}catch(e){if(!state.data.syncing)$('#syncBadge').textContent='连接异常'}}
async function syncNow(){try{$('#syncNowBtn').textContent='⟳ 同步中…';$('#syncNowBtn').disabled=true;state.data.syncing=true;renderSync();await api('/api/sync/now',{method:'POST',body:'{}'});toast('同步已启动…');let attempts=0,checking=false;const poll=setInterval(async()=>{if(checking)return;checking=true;attempts++;try{const status=await api('/api/sync/status');state.data.syncLogs=status.logs||[];state.data.syncing=status.syncing||false;state.data.syncConfig=status.config||state.data.syncConfig;if(!state.data.syncing){clearInterval(poll);const issues=await api('/api/issues');state.data.issues=issues;renderIssues();renderSync();const scoped=state.data.syncLogs.filter(l=>!state.projectFilter||!l.projectId||l.projectId===state.projectFilter);const failed=['github','gongfeng','tapd'].map(source=>scoped.find(l=>l.source===source)).filter(l=>l&&l.status==='failed');toast(failed.length?`同步未完全成功：${failed.map(l=>l.source==='github'?'GitHub':l.source==='gongfeng'?'工蜂':'TAPD').join('、')}，详见诊断`:`同步完成：已刷新外部工作项`)}else if(attempts>=80){clearInterval(poll);renderSync();toast('同步仍在后台执行，可稍后查看结果')}}catch(e){if(attempts>=80){clearInterval(poll);state.data.syncing=false;renderSync();toast(`同步状态检查失败：${e.message}`)}}finally{checking=false}},1500)}catch(e){toast(e.message);state.data.syncing=false;renderSync()}}
async function saveSyncConfig(){
  const project=currentProject();
  const tapdWs=$('#syncTAPDWorkspace').value.trim(),tapdIt=$('#syncTAPDIteration').value.trim();
  const cfg={
    gongfengProjectId:state.data.syncConfig?.gongfengProjectId||'',
    gongfengMcpUrl:$('#syncGongfengURL').value.trim(),
    tapdMcpUrl:$('#syncTAPDURL').value.trim(),
    autoSyncIntervalSec:Number($('#syncInterval').value)||300,
    autoSyncEnabled:$('#syncAutoEnabled').checked
  };
  try{
    const saved=await api('/api/sync/config',{method:'PUT',body:JSON.stringify(cfg)});
    state.data.syncConfig=saved;
    $('#syncAutoEnabledInline').checked=saved.autoSyncEnabled;
    if(project){
      const updated=await api('/api/projects',{method:'PUT',body:JSON.stringify({...project,tapdWorkspaceId:tapdWs,tapdIterationId:tapdIt})});
      const idx=(state.data.projects||[]).findIndex(p=>p.id===updated.id);
      if(idx>=0)state.data.projects[idx]=updated;
    }
    renderSync();
    toast('同步配置已保存');
  }catch(e){toast(e.message)}
}
async function pushIssue(target){const issueId=$('#syncPushIssue').value;if(!issueId)return toast('请先选择一个本地工作项');try{const updated=await api('/api/sync/create',{method:'POST',body:JSON.stringify({issueId,target})});const idx=state.data.issues.findIndex(i=>i.id===issueId);if(idx>=0)state.data.issues[idx]=updated;renderIssues();toast(target==='gongfeng'?`已在工蜂创建 Issue #${updated.externalRef?.externalIid}`:`已在 TAPD 创建需求 ${updated.externalRef?.externalId}`)}catch(e){toast(e.message)}}
