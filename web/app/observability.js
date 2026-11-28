// ================= 用量台（外部上报用量） =================
// 只展示外部工作流上报的用量：Token、费用、缓存、执行数、趋势、模型与供应商拆分、执行明细。
// 任何运行质量口径（成功率、P95、人工介入、返工、成员与项目汇总）都不在本页。
let obsUsageRequest=0;

function obsIcon(name,size){
  size=size||14;
  return `<svg class="agent-icon harness-icon" data-icon="${name}" width="${size}" height="${size}" aria-hidden="true" focusable="false"></svg>`;
}
function obsHydrateIcons(root){window.AgentFlowHarnessIcons?.hydrate?.(root||document)}

// ---------- 公共组件 ----------
function obsSeg(attr,items,active,small){
  return `<div class="obs-seg${small?' sm':''}" role="group">${items.map(([id,label,disabled,title,icon])=>`<button type="button" ${attr}="${escapeHTML(id)}" class="${id===active?'active':''}"${disabled?' disabled':''}${title?` title="${escapeHTML(title)}"`:''}>${icon?obsIcon(icon):''}${label}</button>`).join('')}</div>`;
}
function obsCard(key,title,sub,tools,body,opts){
  opts=opts||{};
  const expand=opts.expand?`<button type="button" class="obs-x-btn" data-obs-expand="${escapeHTML(key)}" title="放大" aria-label="放大查看">${obsIcon('expand')}</button>`:'';
  return `<section class="obs-card2 ${opts.klass||''}" data-obs-card="${escapeHTML(key)}"><header class="obs-card-head"><div class="obs-card-heading"><h3>${title}</h3>${sub?`<p>${sub}</p>`:''}</div><div class="obs-card-tools">${tools||''}${expand}</div></header>${body}</section>`;
}
function obsSpark(values){
  const list=(values||[]).map(v=>Number(v)).filter(v=>Number.isFinite(v));
  if(list.length<2)return '<div class="obs-spark is-empty"></div>';
  const w=120,h=24,max=Math.max(...list),min=Math.min(...list);
  const d=list.map((v,i)=>(i?'L':'M')+(i*w/(list.length-1)).toFixed(1)+','+(h-2-(h-4)*(v-min)/((max-min)||1)).toFixed(1)).join('');
  return `<svg class="obs-spark" viewBox="0 0 ${w} ${h}" preserveAspectRatio="none" aria-hidden="true"><path d="${d}" fill="none" stroke="currentColor" stroke-width="1.3" vector-effect="non-scaling-stroke"/></svg>`;
}
function obsMeter(pct){
  const v=Math.max(0,Math.min(100,Number(pct)||0));
  return `<div class="obs-meter" aria-hidden="true"><i style="width:${v.toFixed(1)}%"></i></div>`;
}
function obsKpi(o){
  const tip=o.tip&&o.tip.length?`<div class="obs-kpi-tip" role="tooltip">${o.tip.map(r=>Array.isArray(r)?`<div><span>${r[0]}</span><b>${r[1]}</b></div>`:`<p>${r}</p>`).join('')}</div>`:'';
  const viz=o.meter!=null?obsMeter(o.meter):(o.spark||'<div class="obs-spark is-empty"></div>');
  return `<div class="obs-kpi${tip?' has-tip':''}${o.missing?' is-missing':''}"${tip?' tabindex="0"':''}><div class="obs-kpi-label">${o.label}</div><div class="obs-kpi-value"><b class="num" data-obs-count>${o.value}</b>${o.unit?`<u>${o.unit}</u>`:''}</div>${viz}<div class="obs-kpi-foot">${o.foot||''}</div>${tip}</div>`;
}
function obsKpiGroup(title,items,aside){
  return `<div class="obs-kg"><h4><span>${title}</span>${aside?`<span>${aside}</span>`:''}</h4><div class="obs-ks">${items.map(obsKpi).join('')}</div></div>`;
}
function obsKpiGroups(groups,klass){
  return `<div class="obs-kgroups ${klass||''}">${groups.join('')}</div>`;
}

// ---------- 数字滚动 ----------
function obsCountUp(el){
  if(!el||el.dataset.obsCounted)return;
  el.dataset.obsCounted='1';
  if(obsReducedMotion()||el.children.length)return;
  const text=el.textContent;
  const m=text.match(/^([^\d]*?)(\d[\d,]*(?:\.\d+)?)([\s\S]*)$/);
  if(!m||/\d/.test(m[3]))return;
  const raw=m[2].replace(/,/g,''),to=parseFloat(raw);
  if(!Number.isFinite(to)||to===0)return;
  const dec=(raw.split('.')[1]||'').length,comma=m[2].includes(',');
  const fmt=v=>{const s=comma?v.toLocaleString('en-US',{minimumFractionDigits:dec,maximumFractionDigits:dec}):v.toFixed(dec);return m[1]+s+m[3]};
  const start=performance.now(),dur=700;
  el.textContent=fmt(0);
  const step=now=>{
    if(!el.isConnected){el.textContent=text;return}
    const k=Math.min(1,Math.max(0,(now-start)/dur));
    if(k>=1){el.textContent=text;return}
    el.textContent=fmt(to*(1-Math.pow(1-k,3)));
    requestAnimationFrame(step);
  };
  requestAnimationFrame(step);
}

// ---------- Harness Selector ----------
// Harness Selector 面板挂在 <body> 上，会比 innerHTML 重绘活得久，重绘前先释放。
function obsDisposeSelects(){
  (state.obsSelects||[]).forEach(api=>api.dispose?.());
  state.obsSelects=[];
}
function obsEnhanceSelects(root,panelHost){
  const list=[];
  root.querySelectorAll('select[data-obs-select]').forEach(sel=>{
    const api=window.enhanceSelect?.(sel);
    if(!api)return;
    if(panelHost)panelHost.appendChild(api.panel);
    list.push(api);
  });
  return list;
}

// ---------- 放大窗口 ----------
const OBS_FOCUSABLE='a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])';
let obsExpandState=null;
function obsExpandModal(card){
  const modal=card?.closest?.('.view')?.querySelector?.('.obs-expand')||document.getElementById('obsExpand');
  if(modal&&!modal.dataset.bound){
    modal.dataset.bound='1';
    modal.addEventListener('click',e=>{if(e.target===modal)obsCollapse()});
    document.addEventListener('keydown',e=>{if(e.key==='Escape'&&obsExpandState)obsCollapse()});
    modal.addEventListener('keydown',e=>{if(e.key==='Tab'&&obsExpandState)obsTrapFocus(modal,e)});
  }
  return modal;
}
function obsTrapFocus(modal,e){
  const box=modal.querySelector('.obs-expand-box');
  const list=[...(box?.querySelectorAll(OBS_FOCUSABLE)||[])].filter(el=>!el.hidden&&el.offsetParent!==null);
  if(!list.length){e.preventDefault();box?.focus({preventScroll:true});return}
  const first=list[0],last=list[list.length-1],active=document.activeElement;
  const inside=box.contains(active);
  if(e.shiftKey&&(active===first||!inside||active===box)){e.preventDefault();last.focus()}
  else if(!e.shiftKey&&(active===last||!inside)){e.preventDefault();first.focus()}
}
function obsExpand(card){
  const modal=obsExpandModal(card);
  const box=modal?.querySelector('.obs-expand-box');
  if(!card||!box)return;
  if(obsExpandState)obsCollapse({silent:true});
  const holder=document.createComment('obs-expand');
  card.before(holder);
  box.innerHTML='';
  box.appendChild(card);
  card.classList.add('is-expanded');
  const trigger=card.querySelector('[data-obs-expand]');
  if(trigger){trigger.innerHTML=obsIcon('close');trigger.title='关闭';trigger.setAttribute('aria-label','关闭放大窗口');obsHydrateIcons(trigger)}
  obsExpandState={card,holder,key:card.dataset.obsCard,trigger,modal};
  modal.hidden=false;
  document.body.classList.add('obs-expand-open');
  box.focus({preventScroll:true});
  requestAnimationFrame(()=>{
    for(const el of card.querySelectorAll('[data-obs-chart]')){
      const entry=obsChartEntry(el.dataset.obsChart);
      if(entry&&entry.el===el){entry.chart.resize();obsReplayChart(entry.key)}
    }
    obsMountCharts(card);
  });
}
function obsCollapse(opts){
  opts=opts||{};
  const st=obsExpandState;
  if(!st)return;
  obsExpandState=null;
  const modal=st.modal||document.getElementById('obsExpand');
  const box=modal?.querySelector('.obs-expand-box');
  const card=st.card;
  card.classList.remove('is-expanded');
  const btn=card.querySelector('[data-obs-expand]');
  if(btn){btn.innerHTML=obsIcon('expand');btn.title='放大';btn.setAttribute('aria-label','放大查看');obsHydrateIcons(btn)}
  if(st.holder.isConnected)st.holder.replaceWith(card);
  else{obsDisposeCharts(card);card.remove()}
  if(box)box.innerHTML='';
  if(modal)modal.hidden=true;
  document.body.classList.remove('obs-expand-open');
  if(card.isConnected){
    for(const el of card.querySelectorAll('[data-obs-chart]')){const entry=obsChartEntry(el.dataset.obsChart);if(entry&&entry.el===el)entry.chart.resize()}
    if(!opts.silent)btn?.focus({preventScroll:true});
  }
}

// ---------- 用量数据 ----------
function obsTimeZone(){
  try{return Intl.DateTimeFormat().resolvedOptions().timeZone||''}catch(e){return ''}
}
function obsUsageGrain(){return ['day','week','month'].includes(state.usageGrain)?state.usageGrain:'day'}
function obsUsageGrainName(){return {day:'日',week:'周',month:'月'}[obsUsageGrain()]}
function obsUsageBucket(){return String(state.usageBucket||'')}
function obsUsageBuckets(){return state.usageBuckets||[]}
function obsUsageSummary(){return state.usage?.summary||{}}
function obsUsageIsEmpty(){
  const payload=state.usage;
  if(!payload)return false;
  if(payload.empty===true)return true;
  return Number(payload.summary?.executions||0)===0;
}
// 费用按阅读者货币显示；无法确定的费用不写成 0。
function obsUsageCurrency(){return obsUsageSummary().costCurrency||'CNY'}
// One amount, from whichever shape carries it: an execution row names its own
// cost, the summary names the total.
function obsUsageAmount(source,currency){
  const cur=currency||obsUsageCurrency();
  const value=cur==='USD'?(source?.costUsd??source?.totalCostUsd):(source?.costCny??source?.totalCostCny);
  return Number(value||0);
}
function obsUsageMoney(value,currency){
  const cur=currency||obsUsageCurrency();
  const symbol=cur==='CNY'?'¥':cur==='USD'?'$':cur+' ';
  const amount=Number(value||0);
  return symbol+(amount>0&&amount<.01?amount.toPrecision(3):amount.toFixed(2));
}
function obsUsageCostText(cost,status,currency){
  if(status==='unknown')return '<span class="muted">未识别</span>';
  return obsUsageMoney(cost,currency);
}
function obsUsageStatusText(status){
  return ({succeeded:'成功',failed:'失败',running:'运行中',cancelled:'已取消',queued:'排队中',waiting:'等待中',pending:'待执行',unknown:'未知'}[status])||status||'—';
}
function obsUsageExecTime(value){
  const at=value?new Date(value):null;
  if(!at||Number.isNaN(at.getTime()))return '—';
  return at.toLocaleString('zh-CN',{hour12:false});
}
function obsUsageQuery(){
  const parts=[];
  const base=projectQuery().replace(/^\?/,'');
  if(base)parts.push(base);
  parts.push('grain='+encodeURIComponent(obsUsageGrain()));
  const tz=obsTimeZone();
  if(tz)parts.push('tz='+encodeURIComponent(tz));
  if(obsUsageBucket())parts.push('bucket='+encodeURIComponent(obsUsageBucket()));
  return '?'+parts.join('&');
}
function obsUsageRememberBuckets(payload){
  const dates=(payload?.trend||[]).map(t=>t?.date).filter(Boolean);
  if(!dates.length)return;
  const merged=new Set([...(state.usageBuckets||[]),...dates]);
  state.usageBuckets=[...merged].sort();
}
async function obsLoadUsage(){
  const request=++obsUsageRequest;
  state.usageLoading=true;
  state.usageError='';
  renderObservability();
  try{
    const payload=await api('/api/usage'+obsUsageQuery(),{silent:true});
    if(request!==obsUsageRequest)return;
    state.usage=payload||null;
    obsUsageRememberBuckets(payload);
  }catch(e){
    if(request===obsUsageRequest){state.usageError=e.message||lfT('用量加载失败','Unable to load usage');state.usage=null}
  }finally{
    if(request===obsUsageRequest){state.usageLoading=false;renderObservability()}
  }
}
async function obsLoadStats(){
  try{
    state.stats=await api('/api/stats/overview'+obsPlatformQuery(),{silent:true})||null;
    state.statsError='';
  }catch(e){
    state.statsError=e.message||'';
  }
  renderObservability();
}
async function refreshObs(){return Promise.all([obsLoadUsage(),obsLoadStats()])}
function setObsUsageGrain(grain){
  grain=['day','week','month'].includes(grain)?grain:'day';
  if(obsUsageGrain()===grain)return;
  state.usageGrain=grain;
  state.usageBucket='';
  localStorage.setItem('agentflow.usageGrain',grain);
  obsLoadUsage();
  obsLoadStats();
}
function setObsUsageBucket(key){
  const next=String(key||'');
  if(obsUsageBucket()===next)return;
  state.usageBucket=next;
  obsLoadUsage();
}
// The picker walks the buckets the server reported; it never leaves the range.
function obsUsageStepBucket(buckets,current,step){
  const list=buckets||[];
  if(!list.length)return null;
  const index=current?list.indexOf(current):-1;
  const next=list[index+step];
  return next==null?null:next;
}
function shiftObsUsageBucket(step){
  const next=obsUsageStepBucket(obsUsageBuckets(),obsUsageBucket(),step);
  if(next==null)return;
  setObsUsageBucket(next);
}

// ---------- 作用域栏 ----------
function obsUsageScopeBar(){
  const project=currentProject();
  const projectName=project?.name||project?.id||'当前项目';
  const grain=obsUsageGrain();
  const buckets=obsUsageBuckets();
  const current=obsUsageBucket();
  const choices=buckets.length?buckets:[current].filter(Boolean);
  const picker=choices.length
    ?`<div class="obs-period-picker" role="group" aria-label="选择时段">
        <button type="button" class="obs-icon-btn" data-obs-bucket-step="-1" title="上一段" aria-label="上一段">${obsIcon('chevron-left')}</button>
        <select id="obsUsageBucket" data-obs-select data-icon="calendar" aria-label="选择时段">${choices.map(b=>`<option value="${escapeHTML(b)}" ${b===current?'selected':''}>${escapeHTML(b)}</option>`).join('')}</select>
        <button type="button" class="obs-icon-btn" data-obs-bucket-step="1" title="下一段" aria-label="下一段">${obsIcon('chevron-right')}</button>
      </div>`
    :'';
  const tz=obsTimeZone();
  const summary=obsUsageSummary();
  const notes=[];
  if(summary.malformedReports)notes.push(`${summary.malformedReports} 条上报格式异常`);
  if(summary.skippedExecutions)notes.push(`${summary.skippedExecutions} 条上报已跳过`);
  const chips=[`<span class="obs-chip">${escapeHTML(projectName)}</span>`];
  if(current)chips.push(`<span class="obs-chip">${escapeHTML(current)}</span>`);
  if(tz)chips.push(`<span class="obs-chip">${escapeHTML(tz)}</span>`);
  notes.forEach(n=>chips.push(`<span class="obs-chip">${escapeHTML(n)}</span>`));
  return `<div class="obs-fbar"><div class="obs-fbar-row">${obsSeg('data-obs-usage-grain',[['day','日'],['week','周'],['month','月']],grain)}${picker}</div><div class="obs-scopeline">${chips.join('')}</div></div>`;
}

// ---------- 面板 ----------
function obsUsageTokenPanel(){
  const s=obsUsageSummary();
  const input=Number(s.inputTokens||0),output=Number(s.outputTokens||0);
  const read=Number(s.cacheReadTokens||0),write=Number(s.cacheWriteTokens||0);
  const total=Number(s.totalTokens||0);
  return obsKpiGroup('Token 用量',[
    {label:'总 Token',value:formatTokens(total),foot:`输入 ${formatTokens(input)} · 输出 ${formatTokens(output)} · 缓存读 ${formatTokens(read)} · 缓存写 ${formatTokens(write)}`,tip:[['输入',formatTokens(input)],['输出',formatTokens(output)],['缓存读',formatTokens(read)],['缓存写',formatTokens(write)]]},
    {label:'输入',value:formatTokens(input),foot:`占总量 ${total?Math.round(input*100/total):0}%`},
    {label:'输出',value:formatTokens(output),foot:`占总量 ${total?Math.round(output*100/total):0}%`},
  ]);
}
function obsUsageCostPanel(){
  const s=obsUsageSummary();
  const currency=obsUsageCurrency();
  const rate=Number(s.usdToCny||0);
  const reported=Number(s.costReportedUsd||0),estimated=Number(s.costEstimatedUsd||0);
  // The split arrives in USD; the reader's currency decides the conversion.
  const inReader=usd=>currency==='CNY'?(rate>0?usd*rate:null):usd;
  const unknown=Number(s.unknownCostExecutions||0);
  const coverage=s.costCoveragePct!=null?Number(s.costCoveragePct):null;
  const total=obsUsageAmount(s,currency);
  return obsKpiGroup('费用',[
    {label:'总费用',value:obsUsageMoney(total,currency),foot:`${currency==='CNY'?'人民币':'美元'}${rate>0?` · 1 USD = ${rate} CNY`:''}`,tip:['费用优先使用上报值，缺失部分按估算补齐。','无法确定的费用显示为「未识别」，不写成 0。']},
    {label:'已上报',value:inReader(reported)==null?'—':obsUsageMoney(inReader(reported),currency),foot:`${Number(s.reportedExecutions||0)} 次执行`},
    {label:'估算',value:inReader(estimated)==null?'—':obsUsageMoney(inReader(estimated),currency),foot:`${Number(s.estimatedExecutions||0)} 次执行`},
    {label:'未知成本',value:String(unknown),foot:unknown?'无法确定费用':'无',missing:unknown>0},
    {label:'费用覆盖率',value:coverage==null?'—':coverage.toFixed(0),unit:coverage==null?'':'%',meter:coverage==null?0:coverage},
  ]);
}
function obsUsageCachePanel(){
  const s=obsUsageSummary();
  const rate=s.cacheHitRate!=null?Number(s.cacheHitRate):null;
  return obsKpiGroup('缓存与执行',[
    {label:'缓存命中率',value:rate==null?'—':rate.toFixed(0),unit:rate==null?'':'%',meter:rate==null?0:rate,foot:'缓存读 ÷ 输入（输入含缓存 Token）'},
    {label:'缓存读',value:formatTokens(s.cacheReadTokens||0),foot:'命中缓存的输入 Token'},
    {label:'缓存写',value:formatTokens(s.cacheWriteTokens||0),foot:'写入缓存的输入 Token'},
    {label:'执行数',value:String(Number(s.executions||0)),foot:`${Number(s.reportedExecutions||0)} 次有上报费用 · ${Number(s.estimatedExecutions||0)} 次估算`},
  ]);
}
function obsUsageExecutionPanel(){
  const s=obsUsageSummary();
  const executions=Number(s.executions||0);
  const reported=Number(s.reportedExecutions||0),estimated=Number(s.estimatedExecutions||0);
  const unknown=Number(s.unknownCostExecutions||0);
  const malformed=Number(s.malformedReports||0),skipped=Number(s.skippedExecutions||0);
  return obsKpiGroup('上报执行',[
    {label:'上报执行数',value:String(executions),foot:`${reported} 次有上报费用 · ${estimated} 次按价格表估算`},
    {label:'无法确定费用',value:String(unknown),foot:unknown?'这些执行不计入已上报':'无',missing:unknown>0},
    {label:'被拒绝的上报',value:String(malformed),foot:'格式非法，已跳过',missing:malformed>0},
    {label:'无法读取的上报',value:String(skipped),foot:'已跳过，不影响其余统计',missing:skipped>0},
  ]);
}
function obsUsageTrendPoints(){
  const currency=obsUsageCurrency();
  return (state.usage?.trend||[]).map(p=>({...p,cost:obsUsageAmount(p,currency)}));
}
function obsUsageTrendCard(){
  const points=obsUsageTrendPoints();
  const metric=['tokens','cost','executions'].includes(state.usageMetric)?state.usageMetric:'tokens';
  const labels={tokens:'Token',cost:'费用',executions:'执行数'};
  const chart=obsChartSlot('usage-trend',{
    empty:!points.length,emptyText:'暂无趋势',
    option:(theme,reduced)=>obsUsageTrendOption(points,metric,{theme,reduced,fmtTokens,fmtCost:v=>obsUsageMoney(v,currency)}),
    fallback:()=>`<div class="obs-bd-bars">${points.map(p=>`<div><span>${escapeHTML(p.date)}</span>${obsMeter(obsUsageTrendValue(p,metric))}</div>`).join('')}</div>`,
  },240);
  return obsCard('trend','用量趋势',`按${obsUsageGrainName()}统计外部上报的执行`,obsSeg('data-obs-usage-metric',[['tokens','Token'],['cost','费用'],['executions','执行数']],metric,true),chart,{expand:true,klass:'obs-trend-card'});
}
function obsUsageBreakdownRows(rows){
  const currency=obsUsageCurrency();
  return (rows||[]).map(r=>({
    key:String(r.key||r.name||''),
    name:String(r.name||r.key||'—'),
    executions:Number(r.executions||0),
    tokens:Number(r.tokens||0),
    inputTokens:Number(r.inputTokens||0),
    outputTokens:Number(r.outputTokens||0),
    cacheReadTokens:Number(r.cacheReadTokens||0),
    cacheWriteTokens:Number(r.cacheWriteTokens||0),
    cost:obsUsageAmount(r,currency),
    tokenSharePct:r.tokenSharePct!=null?Number(r.tokenSharePct):null,
    costSharePct:r.costSharePct!=null?Number(r.costSharePct):null,
  }));
}
function obsUsageBreakdownCard(key,title,rows){
  const list=obsUsageBreakdownRows(rows);
  const currency=obsUsageCurrency();
  const tokenTotal=list.reduce((n,r)=>n+r.tokens,0);
  const costTotal=list.reduce((n,r)=>n+r.cost,0);
  const chart=obsChartSlot(`usage-${key}`,{
    empty:!list.length,emptyText:'暂无用量',
    option:(theme,reduced)=>obsBreakdownOption(list,'tokens',{theme,reduced,fmtTokens,fmtCost:v=>obsUsageMoney(v,currency)}),
    fallback:()=>`<div class="obs-bd-bars">${list.map(r=>`<div><span>${escapeHTML(r.name)}</span>${obsMeter(tokenTotal?r.tokens*100/tokenTotal:0)}</div>`).join('')}</div>`,
  },190);
  const table=list.length
    ?`<div class="obs-tw"><table class="obs-table"><thead><tr><th>名称</th><th>执行</th><th>Token</th><th>Token 占比</th><th>费用</th><th>费用占比</th></tr></thead><tbody>${list.map(r=>`<tr><td><b>${escapeHTML(r.name)}</b></td><td>${r.executions}</td><td>${formatTokens(r.tokens)}</td><td>${obsMeter(r.tokenSharePct!=null?r.tokenSharePct:tokenTotal?r.tokens*100/tokenTotal:0)}<em class="obs-pct">${(r.tokenSharePct!=null?r.tokenSharePct:(tokenTotal?r.tokens*100/tokenTotal:0)).toFixed(1)}%</em></td><td>${obsUsageMoney(r.cost,currency)}</td><td>${obsMeter(r.costSharePct!=null?r.costSharePct:costTotal?r.cost*100/costTotal:0)}<em class="obs-pct">${(r.costSharePct!=null?r.costSharePct:(costTotal?r.cost*100/costTotal:0)).toFixed(1)}%</em></td></tr>`).join('')}</tbody></table></div>`
    :'';
  return obsCard(key,title,'按名称聚合 Token 与费用','',`${chart}${table}`,{expand:true});
}
function obsUsageExecutionRows(){
  return (state.usage?.executions||[]).slice().sort((a,b)=>{
    const at=a?.startedAt?new Date(a.startedAt).getTime():0;
    const bt=b?.startedAt?new Date(b.startedAt).getTime():0;
    return bt-at;
  });
}
function obsUsageExecutionTable(list){
  const currency=obsUsageCurrency();
  const paged=paginateList('obs-executions',list);
  const body=paged.items.map(exec=>{
    const expanded=state.usageExpanded===String(exec.id||'');
    const usage=exec.usage||[];
    const detail=expanded&&usage.length
      ?`<tr class="obs-exec-detail"><td colspan="7"><div class="obs-tw"><table class="obs-table"><thead><tr><th>供应商</th><th>模型</th><th>输入</th><th>输出</th><th>缓存读</th><th>缓存写</th><th>费用</th></tr></thead><tbody>${usage.map(u=>`<tr><td>${escapeHTML(u.provider||'—')}</td><td>${escapeHTML(u.model||'—')}</td><td>${formatTokens(u.inputTokens||0)}</td><td>${formatTokens(u.outputTokens||0)}</td><td>${formatTokens(u.cacheReadTokens||0)}</td><td>${formatTokens(u.cacheWriteTokens||0)}</td><td>${obsUsageCostText(obsUsageAmount(u,currency),u.costStatus,currency)}</td></tr>`).join('')}</tbody></table></div></td></tr>`
      :'';
    return `<tr class="run-row" data-obs-exec="${escapeHTML(String(exec.id||''))}"><td><b>${escapeHTML(exec.workflowName||exec.workflowId||'—')}</b><br><code>${escapeHTML(exec.externalExecutionId||exec.id||'')}</code></td><td>${escapeHTML(obsUsageStatusText(exec.status))}</td><td>${escapeHTML(obsUsageExecTime(exec.startedAt))}</td><td>${exec.durationMs?duration(exec.durationMs):'—'}</td><td>${formatTokens(exec.tokens||0)}</td><td>${obsUsageCostText(obsUsageAmount(exec,currency),exec.costStatus,currency)}</td><td>${usage.length?`<button type="button" class="obs-btn ghost" data-obs-exec-toggle="${escapeHTML(String(exec.id||''))}">${expanded?'收起':'明细'} ${usage.length}</button>`:'—'}</td></tr>${detail}`;
  }).join('');
  const empty=paged.items.length?'':`<tr><td colspan="7" class="detail-empty">暂无上报的执行</td></tr>`;
  return `<div class="obs-tw"><table class="obs-table"><thead><tr><th>执行</th><th>状态</th><th>开始时间</th><th>耗时</th><th>Token</th><th>费用</th><th>模型明细</th></tr></thead><tbody>${body}${empty}</tbody></table></div>${pagerHTML('obs-executions',paged)}`;
}
function obsUsageExecutionCard(){
  return obsCard('executions','执行明细','最新在上；点「明细」展开该次执行的模型用量','',obsUsageExecutionTable(obsUsageExecutionRows()));
}
function obsUsageEmptyPanel(){
  return `<section class="obs-card2" data-obs-card="empty"><header class="obs-card-head"><div class="obs-card-heading"><h3>还没有 Agent 用量</h3><p>上面的平台统计是 MuseForge 自己的数据；这里统计接入的 Agent 花了多少 Token 和费用。</p></div></header><div class="obs-empty-usage"><ol><li>在「数据源」里为当前项目创建上报凭据（Token 只显示一次）。</li><li>把凭据填进 Agent 的连接命令：omp 用扩展，Codex / pi / Claude Code 用 <code>museforge-usage</code> 回填。</li><li>运行后回到本页刷新，就能看到 Token、费用、缓存和模型拆分。</li></ol></div></section>`;
}

// ---------- 平台统计（MuseForge 自己的数据） ----------
function obsPlatform(){return state.stats||null}
function obsPlatformQuery(){
  const tz=obsTimeZone();
  return `?grain=${encodeURIComponent(obsUsageGrain())}${tz?`&tz=${encodeURIComponent(tz)}`:''}`;
}
const OBS_STATUS_LABELS={backlog:'待规划',todo:'待处理',in_progress:'进行中',in_review:'评审中',blocked:'受阻',done:'已完成',closed:'已关闭',cancelled:'已取消'};
const OBS_SOURCE_LABELS={local:'平台新建',github:'GitHub',gitlab:'GitLab'};
const OBS_PRIORITY_LABELS={urgent:'紧急',high:'高',medium:'中',low:'低'};
function obsPlatLabel(labels,key){return labels[key]||key||'—'}
function obsPlatPairs(map,labels,order){
  const entries=Object.entries(map||{}).filter(([,v])=>Number(v)>0);
  const rank=k=>{const i=order.indexOf(k);return i<0?order.length:i};
  return entries.sort((a,b)=>rank(a[0])-rank(b[0])||Number(b[1])-Number(a[1])).map(([k,v])=>[obsPlatLabel(labels,k),Number(v)]);
}
function obsPlatDist(title,pairs){
  if(!pairs.length)return `<div class="obs-plat-dist"><h5>${title}</h5><p class="muted">暂无</p></div>`;
  const max=Math.max(...pairs.map(p=>p[1]));
  return `<div class="obs-plat-dist"><h5>${title}</h5>${pairs.map(([k,v])=>`<div class="obs-plat-row"><span>${escapeHTML(k)}</span>${obsMeter(v*100/max)}<b>${v}</b></div>`).join('')}</div>`;
}
function obsPlatSparkRow(label,values,unit){
  const total=values.reduce((sum,v)=>sum+v,0);
  return `<div class="obs-plat-spark"><span>${label}</span>${obsSpark(values)}<b>${formatTokens(total)}${unit||''}</b></div>`;
}
function obsPlatformCard(){
  const s=obsPlatform();
  if(!s)return '';
  const requirements=s.requirements||{};
  const total=Number(requirements.total||0);
  const kpis=obsKpiGroup('平台概览',[
    {label:'项目',value:String(Number(s.projects||0)),foot:'分组容器'},
    {label:'需求',value:String(total),foot:`未完成 ${Number(requirements.open||0)} · 已完成 ${Number(requirements.closed||0)}`},
    {label:'未完成',value:String(Number(requirements.open||0)),foot:'还没关掉的需求',meter:total?Number(requirements.open||0)*100/total:0},
    {label:'已完成',value:String(Number(requirements.closed||0)),foot:'完成 · 关闭 · 取消',meter:total?Number(requirements.closed||0)*100/total:0},
    {label:'仓库',value:String(Number(s.repositories?.total||0)),foot:'已连接的代码仓库'},
    {label:'机器',value:String(Number(s.machines?.total||0)),foot:`在线 ${Number(s.machines?.connected||0)}`},
  ]);
  if(s.empty){
    return `<section class="obs-card2" data-obs-card="platform"><header class="obs-card-head"><div class="obs-card-heading"><h3>平台统计</h3><p>还没有数据：先在需求列表里建一个项目，数据会出现在这里。</p></div></header>${kpis}</section>`;
  }
  const trend=s.trend||[];
  const dists=`<div class="obs-grid2">${obsPlatDist('状态分布',obsPlatPairs(requirements.byStatus,OBS_STATUS_LABELS,['backlog','todo','in_progress','in_review','blocked','done','closed','cancelled']))}${obsPlatDist('来源分布',obsPlatPairs(requirements.bySource,OBS_SOURCE_LABELS,['local','github','gitlab']))}${obsPlatDist('优先级分布',obsPlatPairs(requirements.byPriority,OBS_PRIORITY_LABELS,['urgent','high','medium','low']))}${obsPlatDist('标签',(requirements.tags||[]).map(t=>[t.tag,Number(t.count||0)]))}</div>`;
  const sparks=`<div class="obs-plat-spark-wrap">${obsPlatSparkRow(`近 ${trend.length} ${obsUsageGrainName()}创建`,trend.map(p=>Number(p.created||0)))}${obsPlatSparkRow(`近 ${trend.length} ${obsUsageGrainName()}关闭`,trend.map(p=>Number(p.closed||0)))}</div>`;
  return obsCard('platform','平台统计','项目、需求、仓库与机器，按'+obsUsageGrainName()+'看创建与关闭', '', `${kpis}${dists}${sparks}`);
}
// ---------- 渲染 ----------
function renderObservability(){
  const host=$('#obsBody');
  if(!host)return;
  const manageButton=$('#obsManageSources');
  if(manageButton)manageButton.hidden=!obsCanManageSources();
  const reopen=obsExpandState?.key||'';
  if(reopen)obsCollapse({silent:true});
  obsDisposeCharts(host);
  obsDisposeSelects();
  const payload=state.usage;
  let body='';
  if(state.usageError){
    body=`<div class="detail-empty" role="alert">${escapeHTML(state.usageError)}</div>`;
  }else if(state.usageLoading&&!payload){
    body=`<div class="detail-empty">${escapeHTML(lfT('正在加载用量…','Loading usage…'))}</div>`;
  }else if(obsUsageIsEmpty()){
    body=obsUsageEmptyPanel();
  }else if(payload){
    body=`${obsKpiGroups([obsUsageTokenPanel(),obsUsageCachePanel(),obsUsageCostPanel(),obsUsageExecutionPanel()])}<div class="obs-grid2">${obsUsageTrendCard()}${obsUsageBreakdownCard('models','模型用量',payload.models||[])}</div>${obsUsageBreakdownCard('providers','供应商用量',payload.providers||[])}${obsUsageExecutionCard()}`;
  }else{
    body=`<div class="detail-empty">${escapeHTML(lfT('暂无统计数据','No usage data yet'))}</div>`;
  }
  host.innerHTML=`<div class="obs-pane active">${obsUsageScopeBar()}${obsPlatformCard()}${body}</div>`
  state.obsSelects=obsEnhanceSelects(host);
  host.querySelectorAll('[data-obs-usage-grain]').forEach(b=>b.onclick=()=>setObsUsageGrain(b.dataset.obsUsageGrain));
  host.querySelectorAll('[data-obs-usage-metric]').forEach(b=>b.onclick=()=>{state.usageMetric=b.dataset.obsUsageMetric;renderObservability()});
  host.querySelectorAll('[data-obs-bucket-step]').forEach(b=>b.onclick=()=>shiftObsUsageBucket(Number(b.dataset.obsBucketStep)));
  const bucketSelect=host.querySelector('#obsUsageBucket');
  if(bucketSelect)bucketSelect.onchange=()=>setObsUsageBucket(bucketSelect.value);
  host.querySelectorAll('[data-obs-exec-toggle]').forEach(b=>b.onclick=e=>{
    e.stopPropagation();
    const id=b.dataset.obsExecToggle;
    state.usageExpanded=state.usageExpanded===id?'':id;
    obsRefreshPaged();
  });
  host.querySelectorAll('[data-obs-expand]').forEach(b=>b.onclick=e=>{
    e.stopPropagation();
    const card=b.closest('[data-obs-card]');
    if(obsExpandState&&obsExpandState.card===card)obsCollapse();
    else obsExpand(card);
  });
  host.setAttribute('aria-busy',String(!!state.usageLoading));
  bindPager(host);
  obsHydrateIcons(host);
  obsMountCharts(host);
  if(reopen){const card=host.querySelector(`[data-obs-card="${reopen}"]`);if(card)obsExpand(card)}
}
function obsRefreshPaged(){
  const card=document.querySelector('[data-obs-card="executions"]');
  if(!card){renderObservability();return}
  const next=document.createElement('template');
  next.innerHTML=obsUsageExecutionCard();
  const replacement=next.content.firstElementChild;
  if(!replacement){card.remove();return}
  card.replaceWith(replacement);
  replacement.querySelectorAll('[data-obs-exec-toggle]').forEach(b=>b.onclick=e=>{
    e.stopPropagation();
    const id=b.dataset.obsExecToggle;
    state.usageExpanded=state.usageExpanded===id?'':id;
    obsRefreshPaged();
  });
  bindPager(replacement);
  obsHydrateIcons(replacement);
}

// ---------- 外部数据源（上报凭据） ----------
function obsCanManageSources(){
  const p=currentProject(),u=state.currentUser;
  return !!(p&&u&&!u.isGuest);
}
async function openObsSources(){
  const project=currentProject();if(!project)return toast('请先选择项目');
  const dialog=document.createElement('dialog');dialog.className='obs-source-dialog';
  const memberIds=new Set(project.ownerId?[Number(project.ownerId)]:[]);
  const users=(state.data.users||[]).filter(u=>memberIds.has(Number(u.id)));
  const options=users.map(u=>`<option value="${Number(u.id)}">${escapeHTML(userDisplay(u))}</option>`).join('');
  dialog.setAttribute('aria-label',`${project.name} · 外部数据源`);
  dialog.innerHTML=`<header class="obs-source-head">${obsIcon('integration',16)}<h3>${escapeHTML(project.name)} · 外部数据源</h3><button class="obs-icon-btn" data-close type="button" title="关闭" aria-label="关闭">${obsIcon('close')}</button></header><div class="obs-source-body"><p>为外部工作流创建专用上报凭据。Token 仅在创建时显示，可随时撤销。</p><div data-sources>正在加载…</div><form data-create><label class="field">数据源名称<input name="name" required maxlength="100" placeholder="例如：团队代码评审流水线"></label><label class="field">有效期（天）<input name="days" type="number" min="1" max="365" value="90" required></label><p>责任人映射（可选）：外部账号对应本项目成员；不填责任人的记录归入“未分配”。</p><div data-mappings></div><div class="obs-source-actions"><button type="button" class="button ghost" data-add>${obsIcon('plus')}添加责任人映射</button><button class="button primary" type="submit">${obsIcon('plus')}创建数据源</button></div></form><div data-created hidden><p>请保存 Token，关闭后无法再次查看。</p><textarea data-token readonly aria-label="上报 Token"></textarea><button type="button" class="button ghost" data-copy>${obsIcon('copy')}复制 Token</button><p>上报地址</p><code>${escapeHTML(location.origin+'/api/insights/v1/ingest')}</code><p>完成或失败时上报一次执行汇总；相同 ID 重试不会重复计量。协议及脚本见仓库 docs/external-workflow-usage.md。</p></div></div>`;
  const selects=new Set();
  document.body.appendChild(dialog);
  dialog.onclose=()=>{selects.forEach(picker=>picker.dispose());selects.clear();dialog.remove()};
  dialog.querySelector('[data-close]').onclick=()=>dialog.close();
  obsHydrateIcons(dialog);
  dialog.showModal();
  async function refresh(){
    try{
      const items=await api('/api/insights/v1/sources?projectId='+encodeURIComponent(project.id),{silent:true});
      if(!dialog.isConnected)return;
      dialog.querySelector('[data-sources]').innerHTML=items.length?items.map(s=>`<div class="obs-source-row obs-source-item">${obsIcon('key')}<span><b>${escapeHTML(s.name)}</b><code>${escapeHTML(s.tokenPrefix)}</code><em>${s.revokedAt?'已撤销':new Date(s.expiresAt)<=new Date()?'已过期':'有效至 '+new Date(s.expiresAt).toLocaleDateString()}</em></span>${s.revokedAt?'':`<button type="button" class="obs-icon-btn" data-revoke="${escapeHTML(s.id)}" title="撤销" aria-label="撤销 ${escapeHTML(s.name)}">${obsIcon('trash')}</button>`}</div>`).join(''):'<p class="muted">尚无外部数据源</p>';
      obsHydrateIcons(dialog.querySelector('[data-sources]'));
      dialog.querySelectorAll('[data-revoke]').forEach(b=>b.onclick=async()=>{b.disabled=true;try{await api('/api/insights/v1/sources/'+encodeURIComponent(b.dataset.revoke),{method:'DELETE'});await refresh()}catch(e){toast(e.message);b.disabled=false}});
    }catch(e){dialog.querySelector('[data-sources]').textContent=e.message}
  }
  dialog.querySelector('[data-add]').onclick=()=>{
    const row=document.createElement('div');row.className='obs-source-row';row.dataset.mapping='';
    row.innerHTML=`<input required maxlength="128" placeholder="外部账号" aria-label="外部账号"><span class="obs-source-pick"><select required data-obs-select data-icon="user" aria-label="平台责任人"><option value="">选择责任人</option>${options}</select></span><button type="button" class="obs-icon-btn" title="移除" aria-label="移除责任人映射">${obsIcon('trash')}</button>`;
    dialog.querySelector('[data-mappings]').appendChild(row);
    // 原生 <dialog> 位于 top layer，挂在 body 上的面板会被它盖住。
    const [picker]=obsEnhanceSelects(row,dialog);
    if(picker){
      selects.add(picker);
      const select=picker.select;
      select.addEventListener('invalid',()=>picker.trigger.classList.add('is-invalid'));
      select.addEventListener('change',()=>picker.trigger.classList.toggle('is-invalid',!select.value));
    }
    row.querySelector('.obs-icon-btn').onclick=()=>{if(picker){picker.dispose();selects.delete(picker)}row.remove()};
    obsHydrateIcons(row);
  };
  dialog.querySelector('[data-copy]').onclick=async()=>{try{await navigator.clipboard.writeText(dialog.querySelector('[data-token]').value);toast('Token 已复制')}catch(e){toast('请手动复制 Token')}};
  dialog.querySelector('[data-create]').onsubmit=async e=>{
    e.preventDefault();const form=e.currentTarget,button=form.querySelector('[type=submit]'),userMappings=Object.create(null);
    for(const row of form.querySelectorAll('[data-mapping]')){const key=row.querySelector('input').value.trim();if(!key||Object.hasOwn(userMappings,key))return toast('外部账号不能为空或重复');userMappings[key]=Number(row.querySelector('select').value)}
    button.disabled=true;
    try{
      const result=await api('/api/insights/v1/sources',{method:'POST',body:JSON.stringify({projectId:project.id,name:form.elements.name.value,expiresInDays:Number(form.elements.days.value),userMappings})});
      dialog.querySelector('[data-created]').hidden=false;dialog.querySelector('[data-token]').value=result.token;form.reset();selects.forEach(picker=>picker.refresh());await refresh();
    }catch(e){toast(e.message)}finally{button.disabled=false}
  };
  await refresh();
}
