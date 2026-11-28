// --- 外部同步面板（嵌入 Issues 视图）---
// 面板现在只做一件事：把当前项目已绑定的 GitHub / GitLab 仓库里的需求导入进来。
// 平台级的同步（状态、立即同步、自动同步、推送到工蜂 / TAPD）随多账号一起退役：
// 控制台只有一个账号，没有需要平台管理员代办的同步动作。
// trackerImportProvider 是导入面板选中的来源：GitHub 或 GitLab。
function trackerImportProvider(){
  const picker=$('#trackerImportProvider');
  return picker&&picker.value?picker.value:'github';
}
function trackerImportProviderLabel(provider){
  return provider==='gitlab'?'GitLab':'GitHub';
}
// trackerImportLabel 是最近一次导入的结果：时间、结论和原因。
function trackerImportLabel(run){
  if(!run)return '尚未导入';
  const when=knownTime(run.finishedAt||run.startedAt)?.toLocaleString()||'—';
  const label=run.status==='success'?'成功':run.status==='partial'?'部分成功':'失败';
  return `最近导入：${when} · ${label}${run.reason?` · ${run.reason}`:''}`;
}
// renderSync 画出外部同步面板：来源选择、导入结论和每个仓库的结果。
function renderSync(){
  const provider=trackerImportProvider(),providerLabel=trackerImportProviderLabel(provider);
  const picker=$('#trackerImportProvider');
  if(picker&&!picker.dataset.bound){
    picker.dataset.bound='1';
    picker.onchange=()=>{state.data.trackerImport=null;renderSync();loadTrackerImport()};
  }
  const button=$('#trackerImportBtn');
  if(button)button.textContent=`⟳ 立即导入 ${providerLabel} 需求`;
  const hint=$('#trackerImportHint');
  if(hint)hint.textContent=`按当前项目已绑定的 ${providerLabel} 仓库导入需求`;
  const run=state.data.trackerImport;
  const status=$('#trackerImportStatus');
  if(status){status.textContent=trackerImportLabel(run);status.className=`sync-import-status ${run&&run.status?run.status:''}`}
  const project=currentProject();
  const scope=$('#trackerImportScope');
  if(scope){
    const repos=(state.data.repositories||[]).filter(r=>r.projectId===state.projectFilter&&(r.provider==='github'||r.provider==='gitlab'));
    scope.innerHTML=repos.length
      ? `<div class="sync-repo-head">当前项目 ${providerLabel} 仓库（${escapeHTML(project?.name||state.projectFilter||'—')}）</div>`+repos.map(r=>`<div class="sync-repo-item"><b>${escapeHTML(r.name)}</b><code>${escapeHTML(r.gongfengProjectId||'—')}</code>${project?.defaultRepositoryId===r.id?'<em>默认</em>':''}</div>`).join('')
      : `<div class="sync-empty">当前项目还没有绑定仓库，去「项目配置」添加。</div>`;
  }
  const host=$('#trackerImportRepos');
  if(!host)return;
  const projectName=id=>{const p=(state.data.projects||[]).find(x=>x.id===id);return p?p.name:''};
  const outcomes=(run&&run.repositories)||[];
  host.innerHTML=outcomes.length?outcomes.map(o=>{
    const elsewhere=o.projectId&&state.projectFilter&&o.projectId!==state.projectFilter?`（${projectName(o.projectId)||o.projectId}）`:'';
    return `<div class="sync-repo-item"><b>${escapeHTML(o.name||o.repositoryId||'')}</b>${elsewhere?`<span class="sync-stat"><b>·</b>${escapeHTML(elsewhere)}</span>`:''}<span class="sync-stat ${o.status==='failed'?'danger':'success'}"><b>${o.status==='failed'?'!':'✓'}</b>${o.status==='failed'?'失败':'成功'}</span>${o.reason?`<span class="sync-import-reason">${escapeHTML(o.reason)}</span>`:''}</div>`;
  }).join(''):'';
}
// loadTrackerImport 读出选中来源最近一次导入的结果；它存在服务端，重启后仍在。
async function loadTrackerImport(){
  const provider=trackerImportProvider();
  try{
    const answer=await api(`/api/tracker/import?provider=${encodeURIComponent(provider)}`,{silent:true});
    if(provider===trackerImportProvider())state.data.trackerImport=(answer&&answer.run)||null;
  }catch(e){
    state.data.trackerImport=null;
  }
  renderSync();
}
// triggerTrackerImport 立即导入选中来源的需求，并刷新需求列表和导入结论。
async function triggerTrackerImport(){
  const provider=trackerImportProvider(),providerLabel=trackerImportProviderLabel(provider);
  const button=$('#trackerImportBtn');
  if(button){button.disabled=true;button.textContent='⟳ 导入中…'}
  try{
    const answer=await api('/api/tracker/import',{method:'POST',body:JSON.stringify({provider,projectId:state.projectFilter||''})});
    state.data.trackerImport=(answer&&answer.run)||null;
    renderSync();
    await refreshIssues();
    const run=state.data.trackerImport;
    if(run&&run.status==='success')toast(`已从 ${providerLabel} 导入 ${run.imported} 条需求`);
    else if(run&&run.status==='partial')toast(`${providerLabel} 部分仓库导入失败：${run.reason||''}`);
    else toast(`${providerLabel} 导入失败：${run&&run.reason?run.reason:'没有可导入的仓库'}`);
  }catch(e){
    toast(e.message||'导入失败');
  }finally{
    if(button){button.disabled=false;button.textContent=`⟳ 立即导入 ${providerLabel} 需求`}
  }
}
