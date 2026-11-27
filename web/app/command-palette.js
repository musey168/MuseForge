// 全局命令面板（⌘K）：按页面、需求、运行、LOOP 和角色检索并跳转。
let commandMatches=[],commandSelectedIndex=0;
function commandCatalog(){
  const pages=[['issues','需求列表','查看进度，触发或重试'],['approvals','待我确认','批准或退回卡住的需求'],['observability','用量台','上报用量 · Token 与费用'],['settings','个人配置','账号、模型与执行凭据'],['projects','项目配置','项目、成员和仓库'],['bots','通道管理','企业微信、飞书、钉钉机器人通道'],['sops','流程模板','Issue 修复与需求开发'],['runs','运行记录','查看节点日志和产物'],['aiSchedules','定时任务','提示词任务、LOOP 触发与共享任务'],['agents','角色定义','管理角色和能力资产'],['evolution','RSI工作台','诊断问题、运行记录与修复提案'],['mcp','能力资产 · MCP','添加项目级或个人 MCP'],['rules','能力资产 · 规则','当前项目的行为与执行约束'],['skills','能力资产 · 技能','当前项目的技能和附加文件'],['runners','执行机器','用自己的电脑或 CVM 跑任务'],['sandboxes','沙箱管理','隔离方式、Dockerfile 与环境变量']].map(([view,title,meta])=>({kind:'页面',title,meta,action:()=>switchView(view)}));
  const issues=(state.data.issues||[]).map(i=>({kind:'需求',title:i.title,meta:`${i.priority||'—'} · ${issueStatusLabel(i.status)}`,action:()=>openIssueDetail(i.id)}));
  const runs=sopRuns(state.data.runs).map(r=>({kind:'运行',title:runIssueTitle(r),meta:`${runDisplayStatus(r)} · ${r.workflowSnapshot?.name||r.workflowId}`,action:()=>openRunFromId(r.id)}));
  const sops=catalogWorkflows().map(w=>({kind:'LOOP',title:w.name,meta:`${w.status==='published'?'已发布':'草稿'} · ${w.nodes?.length||0} 个节点`,action:()=>{state.workflow=clone(w);state.selectedNode=null;switchView('flow')}}));
  const agents=(state.data.agents||[]).map(a=>({kind:'Agent',title:a.name,meta:a.description||a.id,action:()=>{switchView('agents');openAgentWizard(a)}}));
  const actions=[{kind:'操作',title:'新建 LOOP',meta:'创建流程草稿',action:openSOPWizard}];
  return [...actions,...pages,...issues,...runs,...sops,...agents];
}
function openCommandPalette(){const host=$('#commandPalette');host.hidden=false;$('#commandSearch').value='';renderCommandResults();requestAnimationFrame(()=>$('#commandSearch').focus())}
function closeCommandPalette(){$('#commandPalette').hidden=true}
function moveCommandSelection(delta){
  if(!commandMatches.length)return;
  commandSelectedIndex=(commandSelectedIndex+delta+commandMatches.length)%commandMatches.length;
  highlightCommandSelection();
}
function highlightCommandSelection(){
  $$('#commandResults [data-command-index]').forEach((el,i)=>{
    el.classList.toggle('selected',i===commandSelectedIndex);
    if(i===commandSelectedIndex)el.scrollIntoView({block:'nearest'});
  });
}
function renderCommandResults(){
  const q=$('#commandSearch').value.trim().toLowerCase();
  commandMatches=commandCatalog().filter(x=>!q||`${x.kind} ${x.title} ${x.meta}`.toLowerCase().includes(q)).slice(0,12);
  commandSelectedIndex=0;
  $('#commandResults').innerHTML=commandMatches.length
    ?commandMatches.map((x,i)=>`<button data-command-index="${i}" class="${i===0?'selected':''}"><span class="command-kind">${escapeHTML(x.kind)}</span><span><b>${escapeHTML(x.title)}</b><small>${escapeHTML(x.meta||'')}</small></span><em>↵</em></button>`).join('')
    :`<div class="command-empty"><b>没有匹配结果</b><span>换一个标题、编号或页面名称试试</span></div>`;
}
function executeCommand(index){const item=commandMatches[index];if(!item)return;closeCommandPalette();item.action()}
