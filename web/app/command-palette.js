// 全局命令面板（⌘K）：按页面和需求检索并跳转。
let commandMatches=[],commandSelectedIndex=0;
function commandCatalog(){
  const pages=[['issues','需求列表','我的需求与进度'],['observability','用量台','上报用量 · Token 与费用'],['projects','项目配置','项目与代码仓库'],['runners','执行机器','我的开发机器清单'],['settings','个人配置','账号、模型与执行凭据']];
  const issues=(state.data.issues||[]).map(i=>({kind:'需求',title:i.title,meta:`${i.priority||'—'} · ${issueStatusLabel(i.status)}`,action:()=>openIssueDetail(i.id)}));
  return [...pages,...issues];
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
