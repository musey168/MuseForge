// ================= 统计图表层（ECharts） =================
// option 构造函数均为纯函数：只读入参，不访问 DOM 或 echarts 全局，便于 node 测试。
const OBS_ECHARTS_VERSION='6.1.0';
const OBS_ECHARTS_SRC='/vendor/echarts/echarts.min.js?v='+OBS_ECHARTS_VERSION;
const OBS_ECHARTS_TIMEOUT_MS=8000;
const OBS_THEME_PRESETS={
  dark:{c1:'#5b8cff',c2:'#3d5fae',c3:'#2a3d66',c4:'#4b5568',text:'#e8edf6',muted:'#94a3bd',muted2:'#5b6b85',line:'rgba(148,163,184,.13)',lineStrong:'rgba(148,163,184,.25)',panel:'#131a29',panel2:'#19223a',panel3:'#222d48',red:'#f87171',green:'#34d399',amber:'#fbbf24'},
  light:{c1:'#4f7cf0',c2:'#8fb0f7',c3:'#c8d7fb',c4:'#c4cad6',text:'#0f172a',muted:'#475569',muted2:'#94a3b8',line:'rgba(30,41,59,.10)',lineStrong:'rgba(30,41,59,.18)',panel:'#ffffff',panel2:'#f3f5fa',panel3:'#e8ecf4',red:'#c63c4b',green:'#059669',amber:'#b45309'},
};
const OBS_THEME_VARS={c1:'--c1',c2:'--c2',c3:'--c3',c4:'--c4',text:'--text',muted:'--muted',muted2:'--muted-2',line:'--line',lineStrong:'--line-strong',panel:'--panel',panel2:'--panel-2',panel3:'--panel-3',red:'--red',green:'--green',amber:'--amber'};
let obsEChartsPromise=null;
// 加载失败只在下一次整页挂载（重渲染/再次进入统计页）时重试，同一次渲染内的多张图共用失败结果。
let obsEChartsFailed=false;
const obsCharts=new Map();
let obsChartSpecs={};
let obsChartIO=null,obsChartRO=null,obsThemeMO=null;
let obsVisibleCallbacks=new WeakMap();
const obsChartWaiting=new Set();

function obsEsc(v){return String(v??'').replace(/[&<>'"]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[c]))}
function obsReducedMotion(){return typeof matchMedia==='function'&&matchMedia('(prefers-reduced-motion: reduce)').matches}
function obsEnsureECharts(){
  if(typeof window!=='undefined'&&window.echarts&&typeof window.echarts.init==='function')return Promise.resolve(window.echarts);
  if(obsEChartsPromise)return obsEChartsPromise;
  obsEChartsPromise=new Promise(resolve=>{
    if(typeof document==='undefined'){resolve(null);return}
    let done=false,timer=0;
    const finish=value=>{if(done)return;done=true;clearTimeout(timer);resolve(value)};
    const script=document.createElement('script');
    script.src=OBS_ECHARTS_SRC;
    script.async=true;
    script.dataset.obsEcharts=OBS_ECHARTS_VERSION;
    script.onload=()=>{const ec=window.echarts&&typeof window.echarts.init==='function'?window.echarts:null;if(!ec)obsEChartsFailed=true;finish(ec)};
    script.onerror=()=>{obsEChartsFailed=true;script.remove();finish(null)};
    timer=setTimeout(()=>{obsEChartsFailed=true;finish(null)},OBS_ECHARTS_TIMEOUT_MS);
    document.head.appendChild(script);
  });
  return obsEChartsPromise;
}
function obsEChartsTheme(mode){
  const doc=typeof document!=='undefined'?document.documentElement:null;
  mode=mode||(doc&&doc.dataset&&doc.dataset.theme==='dark'?'dark':'light');
  const theme={...(OBS_THEME_PRESETS[mode]||OBS_THEME_PRESETS.light),mode};
  if(doc&&typeof getComputedStyle==='function'){
    const css=getComputedStyle(doc);
    for(const [key,name] of Object.entries(OBS_THEME_VARS)){
      const value=String(css.getPropertyValue(name)||'').trim();
      if(value)theme[key]=value;
    }
  }
  return theme;
}

// ---------- option 公共片段 ----------
function obsChartOpts(opts){
  opts=opts||{};
  return {theme:opts.theme||OBS_THEME_PRESETS.light,reduced:!!opts.reduced,fmtTokens:opts.fmtTokens||(v=>String(Math.round(Number(v)||0))),fmtCost:opts.fmtCost||(v=>'¥'+Number(v||0).toFixed(2)),label:opts.label||(p=>String(p?.label||p?.date||''))};
}
function obsChartBase(o,extra){
  const t=o.theme;
  return Object.assign({
    backgroundColor:'transparent',
    textStyle:{color:t.muted,fontSize:11},
    aria:{enabled:true},
    animation:!o.reduced,
    animationDuration:o.reduced?0:800,
    animationEasing:'cubicOut',
    animationDurationUpdate:o.reduced?0:500,
    animationEasingUpdate:'cubicOut',
    tooltip:obsTooltipBase(t),
  },extra||{});
}
function obsTooltipBase(t){
  return {confine:true,backgroundColor:t.panel3,borderColor:t.lineStrong,borderWidth:1,padding:[8,10],textStyle:{color:t.text,fontSize:11.5},extraCssText:'box-shadow:none;border-radius:9px;'};
}
function obsCategoryAxis(t,labels,extra){
  return Object.assign({type:'category',data:labels,axisLine:{lineStyle:{color:t.line}},axisTick:{show:false},axisLabel:{color:t.muted2,fontSize:10.5,hideOverlap:true}},extra||{});
}
function obsValueAxis(t,formatter,extra){
  return Object.assign({type:'value',splitNumber:4,axisLine:{show:false},axisTick:{show:false},axisLabel:{color:t.muted2,fontSize:10.5,formatter},splitLine:{lineStyle:{color:t.line,type:[3,4]}}},extra||{});
}
function obsBarDelay(o,step){return o.reduced?0:(idx=>idx*(step||35))}
function obsTooltipRows(title,rows,note){
  return `<div class="obs-tt"><b>${obsEsc(title)}</b>${rows.map(([k,v])=>`<div><span>${obsEsc(k)}</span><em>${obsEsc(v)}</em></div>`).join('')}${note?`<small>${obsEsc(note)}</small>`:''}</div>`;
}

// ---------- 用量趋势 ----------
// 上报用量趋势只有三类口径：Token 构成、费用、执行次数。
const OBS_USAGE_TREND_MODES=['tokens','cost','executions'];
function obsUsageTrendValue(p,mode){
  if(mode==='cost')return Number((p?.cost??p?.costCny)||0);
  if(mode==='executions')return Number(p?.executions||0);
  return Number(p?.tokens||0);
}
function obsUsageTrendLabel(p){return String(p?.label||p?.date||'')}
function obsUsageTrendFormat(mode,o){
  if(mode==='cost')return v=>o.fmtCost(v);
  if(mode==='executions')return v=>String(Math.round(Number(v)||0));
  return v=>o.fmtTokens(v);
}
function obsUsageTrendOption(points,mode,opts){
  const o=obsChartOpts(opts),t=o.theme;
  points=Array.isArray(points)?points:[];
  mode=OBS_USAGE_TREND_MODES.includes(mode)?mode:'tokens';
  const labels=points.map(p=>o.label(p));
  const fmt=obsUsageTrendFormat(mode,o);
  const tooltip={...obsTooltipBase(t),trigger:'axis',axisPointer:{type:mode==='cost'?'line':{type:'shadow',shadowStyle:{color:t.text,opacity:.04}},lineStyle:{color:t.lineStrong}},formatter:params=>{
    const list=Array.isArray(params)?params:[params];
    const idx=list[0]?list[0].dataIndex:-1;
    const p=points[idx];
    if(!p)return '';
    const rows=[['执行',String(p.executions||0)],['Token',o.fmtTokens(p.tokens)]];
    if(mode==='tokens')rows.push(['输入',o.fmtTokens(p.inputTokens)],['输出',o.fmtTokens(p.outputTokens)],['缓存读',o.fmtTokens(p.cacheReadTokens)],['缓存写',o.fmtTokens(p.cacheWriteTokens)]);
    rows.push(['费用',o.fmtCost(p.costCny)]);
    return obsTooltipRows(labels[idx],rows);
  }};
  let series;
  if(mode==='tokens'){
    const parts=[['输入','inputTokens',t.c1],['输出','outputTokens',t.c2],['缓存读','cacheReadTokens',t.c3],['缓存写','cacheWriteTokens',t.c4]];
    series=parts.map(([name,field,color],i)=>({name,type:'line',stack:'tokens',symbol:'none',smooth:false,lineStyle:{width:0},areaStyle:{color,opacity:1},itemStyle:{color},data:points.map(p=>Number(p?.[field]||0)),animationDelay:o.reduced?0:i*90}));
  }else if(mode==='cost'){
    series=[{name:'费用',type:'line',symbol:'circle',symbolSize:points.length>16?0:5,showSymbol:points.length<=16,lineStyle:{color:t.c1,width:1.8},itemStyle:{color:t.c1,borderColor:t.panel,borderWidth:1.5},areaStyle:{color:t.c1,opacity:.08},data:points.map(p=>Number(p?.costCny||0)),connectNulls:true,animationDuration:o.reduced?0:900}];
  }else{
    series=[{name:'执行',type:'bar',barMaxWidth:28,itemStyle:{color:t.c1,borderRadius:[2,2,0,0]},data:points.map(p=>Number(p?.executions||0)),animationDelay:obsBarDelay(o,points.length>60?4:30)}];
  }
  return obsChartBase(o,{
    grid:{left:6,right:12,top:14,bottom:4,containLabel:true},
    tooltip,
    xAxis:obsCategoryAxis(t,labels,{boundaryGap:mode==='executions'}),
    yAxis:obsValueAxis(t,v=>fmt(v)),
    series,
  });
}

// ---------- Token / 费用去向 ----------
function obsBreakdownPalette(t){return [t.c1,t.c2,t.c3,t.muted2,t.c4,t.panel3]}
function obsBreakdownValue(row,metric){return Number(metric==='cost'?(row?.cost??row?.costCny):row?.tokens)||0}
function obsBreakdownTotal(rows,metric){return (rows||[]).reduce((n,r)=>n+obsBreakdownValue(r,metric),0)}
function obsBreakdownOption(rows,metric,opts){
  const o=obsChartOpts(opts),t=o.theme,pal=obsBreakdownPalette(t);
  rows=Array.isArray(rows)?rows:[];
  metric=metric==='cost'?'cost':'tokens';
  const total=obsBreakdownTotal(rows,metric);
  const fmt=metric==='cost'?o.fmtCost:o.fmtTokens;
  return obsChartBase(o,{
    tooltip:{...obsTooltipBase(t),trigger:'item',formatter:p=>obsTooltipRows(p.name,[[metric==='cost'?'费用':'Token',fmt(p.value)],['占比',total?(p.value*100/total).toFixed(1)+'%':'—']],'点击查看明细')},
    series:[{type:'pie',radius:['66%','80%'],center:['50%','50%'],avoidLabelOverlap:true,minAngle:2,padAngle:1,label:{show:false},labelLine:{show:false},itemStyle:{borderColor:t.panel,borderWidth:2},emphasis:{scale:true,scaleSize:4,label:{show:false}},
      animationType:'expansion',animationDuration:o.reduced?0:700,
      data:rows.map((r,i)=>({name:String(r.name||r.key||'—'),value:obsBreakdownValue(r,metric),itemStyle:{color:pal[Math.min(i,pal.length-1)]}}))}],
  });
}

// ---------- 挂载、释放与重播 ----------
// spec: {option(theme,reduced), fallback()->html, empty, on:{event:fn}, axisClick(idx), mounted(entry)}
function obsChartSlot(key,spec,height){
  obsChartSpecs[key]=spec||{};
  const empty=!spec||spec.empty;
  return `<div class="obs-chart" data-obs-chart="${obsEsc(key)}" style="height:${Number(height)||240}px"${empty?' data-empty="1"':''}>${empty?`<div class="obs-chart-empty">${obsEsc(spec?.emptyText||'暂无数据')}</div>`:'<div class="obs-chart-skeleton" aria-hidden="true"></div>'}</div>`;
}
function obsChartEntry(key){return obsCharts.get(key)||null}
function obsChartCount(){return obsCharts.size}
function obsObserveVisible(el,fn){
  if(typeof IntersectionObserver!=='function'){fn();return}
  if(!obsChartIO)obsChartIO=new IntersectionObserver(entries=>{
    for(const e of entries){
      if(!e.isIntersecting)continue;
      const cb=obsVisibleCallbacks.get(e.target);
      obsVisibleCallbacks.delete(e.target);
      obsChartIO.unobserve(e.target);
      if(cb)cb();
    }
  },{threshold:.15});
  obsVisibleCallbacks.set(el,fn);
  obsChartIO.observe(el);
}
function obsResizeObserver(){
  if(obsChartRO||typeof ResizeObserver!=='function')return obsChartRO;
  obsChartRO=new ResizeObserver(entries=>{
    for(const e of entries){
      const el=e.target;
      if(obsChartWaiting.has(el)){
        if(el.clientWidth>0){obsChartWaiting.delete(el);obsChartRO.unobserve(el);obsInitChart(el)}
        continue;
      }
      for(const entry of obsCharts.values())if(entry.el===el&&!entry.chart.isDisposed?.())entry.chart.resize();
    }
  });
  return obsChartRO;
}
function obsWatchTheme(){
  if(obsThemeMO||typeof MutationObserver!=='function'||typeof document==='undefined')return;
  obsThemeMO=new MutationObserver(()=>obsRebuildCharts());
  obsThemeMO.observe(document.documentElement,{attributes:true,attributeFilter:['data-theme']});
}
function obsMountCharts(root){
  if(!root||typeof root.querySelectorAll!=='function')return;
  if(obsEChartsFailed){obsEChartsFailed=false;obsEChartsPromise=null}
  obsWatchTheme();
  for(const [key,entry] of obsCharts)if(!entry.el.isConnected)obsDisposeChart(key);
  root.querySelectorAll('[data-obs-chart]').forEach(el=>{
    const key=el.dataset.obsChart;
    if(el.dataset.empty||el.dataset.obsChartPending)return;
    const entry=obsCharts.get(key);
    if(entry&&entry.el===el)return;
    if(entry)obsDisposeChart(key);
    el.dataset.obsChartPending='1';
    obsObserveVisible(el,()=>obsInitChart(el));
  });
  if(typeof obsCountUp==='function')root.querySelectorAll('[data-obs-count]').forEach(el=>{if(!el.dataset.obsCounted)obsObserveVisible(el,()=>obsCountUp(el))});
}
async function obsInitChart(el){
  const key=el.dataset.obsChart;
  if(!el.isConnected||!obsChartSpecs[key])return;
  if(!el.clientWidth){
    const ro=obsResizeObserver();
    if(ro){obsChartWaiting.add(el);ro.observe(el);return}
  }
  const ec=await obsEnsureECharts();
  const spec=obsChartSpecs[key];
  if(!el.isConnected||el.dataset.obsChart!==key||!spec)return;
  delete el.dataset.obsChartPending;
  if(!ec){obsChartFallback(el,spec);return}
  const existing=obsCharts.get(key);
  if(existing){if(existing.el===el)return;obsDisposeChart(key)}
  el.querySelector('.obs-chart-skeleton')?.remove();
  const chart=ec.init(el,null,{renderer:'svg'});
  const entry={key,el,chart,spec};
  obsCharts.set(key,entry);
  el.classList.add('is-ready');
  chart.setOption(spec.option(obsEChartsTheme(),obsReducedMotion()),true);
  obsBindChartEvents(entry);
  obsResizeObserver()?.observe(el);
  if(typeof spec.mounted==='function')spec.mounted(entry);
}
function obsBindChartEvents(entry){
  const {chart,spec}=entry;
  for(const [name,fn] of Object.entries(spec.on||{}))chart.on(name,params=>fn(params,entry));
  if(typeof spec.axisClick==='function'){
    chart.getZr().on('click',e=>{
      const point=[e.offsetX,e.offsetY];
      if(!chart.containPixel({gridIndex:0},point))return;
      const pos=chart.convertFromPixel({gridIndex:0},point);
      const idx=Math.round(Array.isArray(pos)?pos[0]:pos);
      if(Number.isFinite(idx))spec.axisClick(idx,entry);
    });
  }
}
function obsChartFallback(el,spec){
  let html='';
  try{html=typeof spec.fallback==='function'?spec.fallback():''}catch(e){html=''}
  el.innerHTML=html||'<div class="obs-chart-empty">图表暂不可用</div>';
  el.classList.add('is-fallback');
  el.style.height='auto';
  const head=el.closest('[data-obs-card]')?.querySelector('.obs-card-head');
  if(head&&!head.querySelector('.obs-fallback-badge'))head.querySelector('.obs-card-tools')?.insertAdjacentHTML('afterbegin','<span class="obs-fallback-badge" title="图表库未加载，显示简版图表">简版图表</span>');
}
function obsDisposeChart(key){
  const entry=obsCharts.get(key);
  if(!entry)return;
  obsCharts.delete(key);
  obsChartRO?.unobserve(entry.el);
  try{entry.chart.dispose()}catch(e){}
}
function obsDisposeCharts(root){
  for(const [key,entry] of [...obsCharts]){
    if(!root||!entry.el.isConnected||root.contains(entry.el))obsDisposeChart(key);
  }
  const scope=root&&typeof root.querySelectorAll==='function'?root:(typeof document!=='undefined'?document:null);
  scope?.querySelectorAll?.('[data-obs-chart],[data-obs-count]').forEach(el=>{
    obsVisibleCallbacks.delete(el);
    obsChartIO?.unobserve(el);
    obsChartWaiting.delete(el);
    delete el.dataset.obsChartPending;
  });
}
function obsReplayChart(key){
  const entry=obsCharts.get(key);
  if(!entry)return false;
  entry.chart.clear();
  entry.chart.setOption(entry.spec.option(obsEChartsTheme(),obsReducedMotion()),true);
  return true;
}
function obsRebuildCharts(){
  for(const [key,entry] of [...obsCharts]){
    const el=entry.el;
    obsDisposeChart(key);
    if(el.isConnected)obsInitChart(el);
  }
}
