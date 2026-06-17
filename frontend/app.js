const state = { token: localStorage.getItem("token"), refreshToken: localStorage.getItem("refreshToken"), user: null, page: "dashboard", dashboard: null, agentEditorId: null, pptEditorId: null, lastAgentCallId: null, generationPollTimer: null, focusGenerationTaskId: null, selectedReferenceAssetId: "", generationPrompt: "", showGenerationHistory: true, generationCanvasStartedAt: "" };
const pages = [
  ["dashboard","运营总览"],["generate","AI 创作"],["assets","作品中心"],["ppt","AI PPT"],
  ["agents","智能体"],["geo","GEO 优化"],["enterprise","企业管理"],["membership","会员订单"],["channel","代理商"],["admin","运营后台"]
];

const $ = (s) => document.querySelector(s);
const esc = (v="") => String(v).replace(/[&<>"']/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"}[c]));
const money = cents => `¥${(Number(cents || 0)/100).toFixed(2)}`;
const toast = msg => { $("#toast").textContent=msg; $("#toast").classList.add("show"); setTimeout(()=>$("#toast").classList.remove("show"),2200); };

async function api(path, options={}, allowRefresh=true) {
  const res = await fetch(`/api/v1${path}`, { ...options, headers: { "Content-Type":"application/json", ...(state.token?{Authorization:`Bearer ${state.token}`}:{}) }});
  const out = await res.json();
  if (res.status===401 && allowRefresh && state.refreshToken && path!=="/auth/refresh") {
    const refreshed=await api("/auth/refresh",{method:"POST",body:JSON.stringify({refreshToken:state.refreshToken})},false);
    state.token=refreshed.token;state.refreshToken=refreshed.refreshToken;
    localStorage.setItem("token",state.token);localStorage.setItem("refreshToken",state.refreshToken);
    return api(path,options,false);
  }
  if (!res.ok) throw new Error(out.message);
  return out.data;
}

async function download(path, filename) {
  const res = await fetch(`/api/v1${path}`, { headers: { Authorization:`Bearer ${state.token}` } });
  if (!res.ok) throw new Error((await res.json()).message);
  const blob = await res.blob();
  const link = document.createElement("a");
  link.href = URL.createObjectURL(blob); link.download = filename; link.click();
  URL.revokeObjectURL(link.href);
}

$("#login-form").addEventListener("submit", async e => {
  e.preventDefault();
  try {
    const data = await api("/auth/login",{method:"POST",body:JSON.stringify({email:$("#email").value,password:$("#password").value})});
    state.token=data.token;state.refreshToken=data.refreshToken;localStorage.setItem("token",data.token);localStorage.setItem("refreshToken",data.refreshToken);await boot();
  } catch(e){toast(e.message)}
});
$("#logout").onclick=async()=>{try{await api("/auth/logout",{method:"POST"})}catch{}localStorage.removeItem("token");localStorage.removeItem("refreshToken");location.reload()};

async function boot(){
  try{state.dashboard=await api("/dashboard");state.user=state.dashboard.user}
  catch{state.token=null;state.refreshToken=null;localStorage.removeItem("token");localStorage.removeItem("refreshToken");return}
  $("#login").classList.add("hidden");$("#app").classList.remove("hidden");
  $("#nav").innerHTML=pages.filter(([id])=>(id!=="admin"||state.user.role==="SUPER_ADMIN")&&(id!=="channel"||["SUPER_ADMIN","AGENT_L1","AGENT_L2"].includes(state.user.role))).map(([id,label])=>`<button class="nav-item" data-page="${id}">${label}</button>`).join("");
  $("#nav").onclick=e=>{if(e.target.dataset.page){state.page=e.target.dataset.page;render()}};
  render();
}

async function refresh(){state.dashboard=await api("/dashboard");state.user=state.dashboard.user;render()}
function scheduleGenerationPoll() {
  if (state.generationPollTimer) return;
  state.generationPollTimer = setTimeout(async () => {
    state.generationPollTimer = null;
    try {
      const tasks = await api("/generation-tasks");
      const running = tasks.some(t => ["QUEUED","PROCESSING","RETRYING"].includes(t.status));
      if (state.page === "generate" || state.page === "assets") await refresh();
      if (running) scheduleGenerationPoll();
      else toast("生成任务已完成，可在作品中心查看");
    } catch (e) {
      toast(e.message);
    }
  }, 4000);
}
const list = (items, renderer) => items.length ? `<div class="list">${items.map(renderer).join("")}</div>` : `<div class="empty">暂无数据，先创建一条试试。</div>`;
const status = v => `<span class="status ${v}">${esc(v)}</span>`;

async function render(){
  document.querySelectorAll(".nav-item").forEach(el=>el.classList.toggle("active",el.dataset.page===state.page));
  document.body.classList.toggle("generate-page",state.page==="generate");
  $("#page-title").textContent=pages.find(p=>p[0]===state.page)?.[1]||"工作台";
  $("#user-name").textContent=state.user.name;
  $("#points").textContent=`${state.dashboard.account.available} 可用积分`;
  const renderers={dashboard:renderDashboard,generate:renderGenerateCanvas,assets:renderAssets,ppt:renderPpt,agents:renderAgentBuilder,geo:renderGeoOperations,enterprise:renderEnterprise,membership:renderMembershipInvoices,channel:renderChannel,admin:renderAdmin};
  try{$("#content").innerHTML=await renderers[state.page]();bindPage()}
  catch(e){$("#content").innerHTML=`<div class="card">${esc(e.message)}</div>`}
}

async function renderDashboard(){
  const d=await api("/dashboard"); state.dashboard=d;
  return `<div class="hero"><span class="eyebrow">WELCOME BACK</span><h2>${esc(d.user.name)}，开始创造今天的新成果</h2><p>当前为 ${esc(d.plan.name)}。统一工作台已经连接内容生产、商业化和增长分析模块。</p></div>
  <div class="grid">${Object.entries(d.metrics).map(([k,v])=>`<div class="card metric"><span>${({tasks:"生成任务",assets:"作品资产",presentations:"PPT 项目",agents:"智能体",geoTasks:"GEO 监测",orders:"订单",enterpriseMembers:"企业成员"}[k])}</span><strong>${v}</strong></div>`).join("")}</div>`;
}

async function renderGenerate(){
  const [tasks,assets,models]=await Promise.all([api("/generation-tasks"),api("/assets"),api("/models")]);
  if (tasks.some(t => ["QUEUED","PROCESSING","RETRYING"].includes(t.status))) scheduleGenerationPoll();
  const imageAssets=assets.filter(a=>a.mediaType==="image");
  const recentTasks=tasks.slice().reverse();
  const latestTask=recentTasks[0];
  if (!state.focusGenerationTaskId && latestTask) state.focusGenerationTaskId=latestTask.id;
  const runningTasks=recentTasks.filter(t=>["QUEUED","PROCESSING","RETRYING"].includes(t.status));
  const running=runningTasks[0];
  const timelineTasks=tasks.slice().sort((a,b)=>new Date(a.createdAt||0)-new Date(b.createdAt||0));
  const taskAssets=timelineTasks.flatMap(t=>(t.resultIds||[]).map((assetId,resultIndex)=>({task:t,asset:assets.find(a=>a.id===assetId),resultIndex}))).filter(item=>item.asset?.mediaType==="image");
  const canvasItems=taskAssets.slice(0,36);
  const focusItem=canvasItems.find(item=>item.task?.id===state.focusGenerationTaskId)||canvasItems[0];
  const focusTask=focusItem?.task||tasks.find(t=>t.id===state.focusGenerationTaskId)||latestTask;
  const focusAsset=focusItem?.asset;
  const boardWidth=1040;
  const boardHeight=Math.max(1100,(canvasItems.length+runningTasks.length)*430+260);
  const positionFor=(i)=>({x:70,y:120+i*430});
  const roundLabel=(i)=>`第 ${i+1} 轮`;
  const typeLabel=(t)=>({TEXT_TO_IMAGE:"画图",IMAGE_TO_IMAGE:"编辑图",TEXT_TO_VIDEO:"视频",IMAGE_TO_VIDEO:"图生视频"}[t?.type]||"画图");
  const taskStateLabel=(t)=>({SUCCEEDED:"已完成",FAILED:"失败",CANCELLED:"已取消",QUEUED:"排队中",PROCESSING:"生成中",RETRYING:"重试中"}[t?.status]||t?.status||"");
  const timeLabel=(t)=>{const d=new Date(t?.workerFinishedAt||t?.updatedAt||t?.createdAt||Date.now());return `${String(d.getMonth()+1).padStart(2,"0")}/${String(d.getDate()).padStart(2,"0")} ${String(d.getHours()).padStart(2,"0")}:${String(d.getMinutes()).padStart(2,"0")}`};
  const modelOnline=models.length>0;
  const preferredModel=models.find(m=>m.code==="gpt-image-2")?.code||models.find(m=>m.capabilities?.includes("TEXT_TO_IMAGE"))?.code||models[0]?.code;
  return `<div class="creation-shell">
    <div class="creation-topbar">
      <div class="creation-brand"><strong>先知 AI</strong><span>创作画布</span></div>
      <div class="tabs creation-tabs" id="generation-tabs">${[["TEXT_TO_IMAGE","画图"],["IMAGE_TO_IMAGE","参考图"],["TEXT_TO_VIDEO","视频"],["IMAGE_TO_VIDEO","图生视频"]].map(([v,l],i)=>`<button data-type="${v}" class="${i===0?"active":""}">${l}</button>`).join("")}</div>
      <div class="creation-badges"><span class="${modelOnline?"online":"offline"}">${modelOnline?"ONLINE":"OFFLINE"}</span><span>余额 ${state.dashboard.account.available}</span><span>运行 ${tasks.filter(t=>["QUEUED","PROCESSING","RETRYING"].includes(t.status)).length}</span></div>
    </div>
    <section class="creation-canvas">
      <div class="canvas-grid"></div>
      <div class="canvas-viewport">
        <div class="canvas-board" data-board-width="${boardWidth}" data-board-height="${boardHeight}">
          ${canvasItems.length?canvasItems.map((item,i)=>{const pos=positionFor(i);const isFocus=item.asset.id===focusAsset?.id;return `<article class="canvas-node canvas-record ${isFocus?"active":""}" data-x="${pos.x}" data-y="${pos.y}">
            <div class="canvas-node-media ${item.asset.mediaType==="video"?"is-video":""}">${item.asset.mediaType==="image"?`<img src="${item.asset.url}" alt="${esc(item.asset.name)}" onerror="this.closest('.canvas-node').remove()">`:`<div class="video-result">视频资源已生成</div>`}</div>
            <div class="canvas-record-meta">
              <div class="record-head"><span>${roundLabel(i)}</span><span>${typeLabel(item.task)}</span><strong class="${item.task.status==="FAILED"?"failed":"done"}">${taskStateLabel(item.task)}</strong><time>${timeLabel(item.task)}</time></div>
              <p>${esc(item.task.prompt||"")}</p>
              <div class="record-actions"><button data-reuse-task="${item.task.id}">复用配置</button><button class="delete" data-delete-asset="${item.asset.id}">删除</button></div>
            </div>
          </article>`}).join(""):`<div class="canvas-empty board-empty"><strong>${running?"正在生成中":"输入提示词开始生成"}</strong><span>${running?"真实模型可能需要几十秒，完成后会自动追加到画布。":"生成图片会直接追加到当前画布，不会覆盖旧图。"}</span></div>`}
          ${runningTasks.map((t,i)=>{const pos=positionFor(canvasItems.length+i);return `<article class="canvas-node canvas-record canvas-node-pending" data-x="${pos.x}" data-y="${pos.y}"><div class="canvas-empty"><strong>正在生成中</strong><span>${esc(t.prompt)}</span></div><div class="canvas-record-meta"><div class="record-head"><span>${roundLabel(canvasItems.length+i)}</span><span>${typeLabel(t)}</span><strong>生成中</strong><time>${timeLabel(t)}</time></div><p>${esc(t.prompt||"")}</p></div></article>`}).join("")}
        </div>
      </div>
    </section>
      <textarea id="prompt" rows="3" placeholder="输入你想要生成的画面，也可先上传参考图" required></textarea>
      <div class="composer-controls">
        <button type="button" id="upload-reference">上传</button>
        <input id="quick-upload-file" class="hidden" type="file" accept="image/png,image/jpeg,image/webp,image/svg+xml">
        <label>参考图<select id="reference-asset"><option value="">不使用</option>${imageAssets.map(a=>`<option value="${a.id}">${esc(a.name)}</option>`).join("")}</select></label>
        <label>模型<select id="model">${models.map(m=>`<option value="${m.code}" ${m.code===preferredModel?"selected":""}>${esc(m.name)}</option>`).join("")}</select></label>
        <label>张数<input id="count" type="number" min="1" max="4" value="1"></label>
        <label>比例<select id="image-ratio"><option value="4:3">4:3</option><option value="1:1">1:1</option><option value="3:4">3:4</option><option value="16:9">16:9</option><option value="9:16">9:16</option></select></label>
        <label>计费<select id="billing-source"><option value="AUTO">自动</option><option value="ENTERPRISE">企业</option><option value="PERSONAL">个人</option></select></label>
        <div class="composer-estimate"><span>预计</span><strong id="generation-estimate">计算中</strong></div>
        <button class="send-button" title="开始生成">生成</button>
      </div>
    </form>
  </div>`;
}

async function renderGenerateCanvas(){
  const [tasks,assets,models]=await Promise.all([api("/generation-tasks"),api("/assets"),api("/models")]);
  if (tasks.some(t => ["QUEUED","PROCESSING","RETRYING"].includes(t.status))) scheduleGenerationPoll();
  const imageAssets=assets.filter(a=>a.mediaType==="image");
  const recentTasks=tasks.slice().reverse();
  const latestTask=recentTasks[0];
  if (!state.focusGenerationTaskId && latestTask) state.focusGenerationTaskId=latestTask.id;
  const runningTasks=recentTasks.filter(t=>["QUEUED","PROCESSING","RETRYING"].includes(t.status));
  const running=runningTasks[0];
  const timelineTasks=tasks.slice().sort((a,b)=>new Date(a.createdAt||0)-new Date(b.createdAt||0));
  const taskAssets=timelineTasks.flatMap(task=>(task.resultIds||[]).map((assetId,resultIndex)=>({task,asset:assets.find(a=>a.id===assetId),resultIndex}))).filter(item=>item.asset?.mediaType==="image");
  const startedAt=state.generationCanvasStartedAt?new Date(state.generationCanvasStartedAt).getTime():0;
  const visibleTaskAssets=state.showGenerationHistory?taskAssets:taskAssets.filter(item=>new Date(item.task?.createdAt||0).getTime()>=startedAt);
  const canvasItems=visibleTaskAssets.slice(-36);
  const focusItem=canvasItems.find(item=>item.task?.id===state.focusGenerationTaskId)||canvasItems[canvasItems.length-1];
  const focusAsset=focusItem?.asset;
  const isSmallScreen=window.matchMedia?.("(max-width: 760px)").matches;
  const boardWidth=isSmallScreen?Math.max(360,window.innerWidth):1040;
  const rowGap=isSmallScreen?680:430;
  const boardHeight=Math.max(1100,(canvasItems.length+runningTasks.length)*rowGap+260);
  const positionFor=(i)=>({x:isSmallScreen?16:70,y:120+i*rowGap});
  const roundLabel=(i)=>`第 ${i+1} 轮`;
  const typeLabel=(task)=>({TEXT_TO_IMAGE:"画图",IMAGE_TO_IMAGE:"编辑图",TEXT_TO_VIDEO:"视频",IMAGE_TO_VIDEO:"图生视频"}[task?.type]||"画图");
  const taskStateLabel=(task)=>({SUCCEEDED:"已完成",FAILED:"失败",CANCELLED:"已取消",QUEUED:"排队中",PROCESSING:"生成中",RETRYING:"重试中"}[task?.status]||task?.status||"");
  const timeLabel=(task)=>{const d=new Date(task?.workerFinishedAt||task?.updatedAt||task?.createdAt||Date.now());return `${String(d.getMonth()+1).padStart(2,"0")}/${String(d.getDate()).padStart(2,"0")} ${String(d.getHours()).padStart(2,"0")}:${String(d.getMinutes()).padStart(2,"0")}`};
  const modelOnline=models.length>0;
  const preferredModel=models.find(m=>m.code==="gpt-image-2")?.code||models.find(m=>m.capabilities?.includes("TEXT_TO_IMAGE"))?.code||models[0]?.code||"";
  const rowHtml=(item,i)=>{
    const pos=positionFor(i);
    const isFocus=item.asset?.id===focusAsset?.id;
    const failed=item.task?.status==="FAILED";
    return `<article class="canvas-node canvas-record ${isFocus?"active":""}" data-x="${pos.x}" data-y="${pos.y}">
      <div class="canvas-node-media"><img src="${item.asset.url}" alt="${esc(item.asset.name)}" onerror="this.closest('.canvas-node').remove()"></div>
      <div class="canvas-record-meta">
        <div class="record-head"><span>${roundLabel(i)}</span><span>${typeLabel(item.task)}</span><strong class="${failed?"failed":"done"}">${taskStateLabel(item.task)}</strong><time>${timeLabel(item.task)}</time></div>
        <p>${esc(item.task.prompt||"")}</p>
        <div class="record-actions"><button data-reuse-task="${item.task.id}">复用配置</button><button class="delete" data-delete-asset="${item.asset.id}" title="删除">删除</button></div>
      </div>
    </article>`;
  };
  const pendingHtml=(task,i)=>{
    const pos=positionFor(canvasItems.length+i);
    return `<article class="canvas-node canvas-record canvas-node-pending" data-x="${pos.x}" data-y="${pos.y}">
      <div class="canvas-empty"><strong>正在生成中</strong><span>${esc(task.prompt||"")}</span></div>
      <div class="canvas-record-meta">
        <div class="record-head"><span>${roundLabel(canvasItems.length+i)}</span><span>${typeLabel(task)}</span><strong>生成中</strong><time>${timeLabel(task)}</time></div>
        <p>${esc(task.prompt||"")}</p>
      </div>
    </article>`;
  };
  return `<div class="creation-shell">
    <div class="creation-topbar">
      <div class="creation-brand"><strong>先知 AI</strong><span>创作画布</span></div>
      <div class="tabs creation-tabs" id="generation-tabs">${[["TEXT_TO_IMAGE","画图"],["IMAGE_TO_IMAGE","参考图"],["TEXT_TO_VIDEO","视频"],["IMAGE_TO_VIDEO","图生视频"]].map(([v,l],i)=>`<button data-type="${v}" class="${i===0?"active":""}">${l}</button>`).join("")}</div>
      <div class="creation-badges"><span class="${modelOnline?"online":"offline"}">${modelOnline?"ONLINE":"OFFLINE"}</span><span>余额 ${state.dashboard.account.available}</span><span>运行 ${runningTasks.length}</span></div>
    </div>
    <section class="creation-canvas">
      <div class="canvas-grid"></div>
      <div class="canvas-toolbar">
        <button type="button" class="history-button" data-open-history><span>↺</span> 历史对话 <strong>${taskAssets.length}</strong></button>
        <button type="button" class="new-button" data-new-canvas><span>＋</span> 新建</button>
        <button type="button" class="icon-button" data-clear-canvas title="清空当前画布">⌫</button>
      </div>
      <div class="canvas-viewport">
        <div class="canvas-board" data-board-width="${boardWidth}" data-board-height="${boardHeight}">
          ${canvasItems.length?canvasItems.map(rowHtml).join(""):`<div class="canvas-empty board-empty"><div class="empty-rule-row"><span></span><em>GENERATIVE · ATELIER</em><span></span></div><strong>Turn ideas into images</strong><small>${state.showGenerationHistory?"输入提示词开始生成，作品会追加在当前画布里。":"这是一个全新画布，可从已有结果之外继续发起新的无状态编辑。"}</small><div class="empty-flow-row"><span>01</span><i></i><em>SKETCH → RENDER</em><i></i><span>02</span></div></div>`}
          ${runningTasks.map(pendingHtml).join("")}
        </div>
      </div>
    </section>
    <form id="generation-form" class="creation-composer">
      <textarea id="prompt" rows="3" placeholder="输入你想要生成的画面，也可先上传参考图" required>${esc(state.generationPrompt||"")}</textarea>
      <div class="composer-controls">
        <button type="button" id="upload-reference">上传</button>
        <input id="quick-upload-file" class="hidden" type="file" accept="image/png,image/jpeg,image/webp,image/svg+xml">
        <label>参考图<select id="reference-asset"><option value="">不使用</option>${imageAssets.map(a=>`<option value="${a.id}" ${a.id===state.selectedReferenceAssetId?"selected":""}>${esc(a.name)}</option>`).join("")}</select></label>
        <label>模型<select id="model">${models.map(m=>`<option value="${m.code}" ${m.code===preferredModel?"selected":""}>${esc(m.name)}</option>`).join("")}</select></label>
        <label>张数<input id="count" type="number" min="1" max="4" value="1"></label>
        <label>比例<select id="image-ratio"><option value="4:3">4:3</option><option value="1:1">1:1</option><option value="3:4">3:4</option><option value="16:9">16:9</option><option value="9:16">9:16</option></select></label>
        <label>计费<select id="billing-source"><option value="AUTO">自动</option><option value="ENTERPRISE">企业</option><option value="PERSONAL">个人</option></select></label>
        <div class="composer-estimate"><span>预计</span><strong id="generation-estimate">计算中</strong></div>
        <button class="send-button" title="开始生成">生成</button>
      </div>
    </form>
  </div>`;
}

async function renderAssets(){
  const items=await api("/assets");
  return `<div class="card"><form id="upload-form" class="toolbar"><label>上传参考文件<input id="upload-file" type="file" accept="image/png,image/jpeg,image/webp,image/svg+xml,text/plain,application/pdf" required></label><button>上传至资产库</button></form></div><div class="card panel"><h2>作品中心</h2>${items.length?`<div class="asset-grid">${items.slice().reverse().map(a=>`<article class="card asset">${a.mediaType==="image"?`<img src="${a.url}" alt="${esc(a.name)}">`:`<div class="empty">${a.mediaType==="video"?"视频占位资源":"文档资产"}</div>`}<div><strong>${a.favorite?"★ ":""}${esc(a.name)}</strong><p class="muted">${esc(a.metadata.prompt||a.metadata.contentType||"")}</p><div class="toolbar"><button data-favorite-asset="${a.id}" data-next="${!a.favorite}">${a.favorite?"取消收藏":"收藏"}</button><button data-download-asset="${a.id}" data-name="${esc(a.name)}">下载</button>${a.taskId?`<button data-regenerate-asset="${a.id}">再次生成</button>`:""}<button data-delete-asset="${a.id}">删除</button></div></div></article>`).join("")}</div>`:`<div class="empty">生成成功或上传的内容会出现在这里。</div>`}</div>`;
}

async function renderPpt(){
  const items=await api("/presentations");
  const selected=items.find(p=>p.id===state.pptEditorId)||items[0];if(selected)state.pptEditorId=selected.id;
  return `<div class="card"><form id="ppt-form" class="toolbar"><label>PPT 主题<input id="ppt-topic" placeholder="例如：2026 年 AI 营销增长方案" required></label><label>视觉主题<select id="ppt-theme"><option>科技蓝</option><option>商务黑金</option><option>清新绿色</option></select></label><button>生成大纲</button></form></div><div class="split panel"><div class="card"><h2>PPT 项目</h2>${list(items,p=>`<div class="list-item"><div><h3>${esc(p.topic)}</h3><p>${p.slides.length} 页 · ${esc(p.theme)}</p></div><button data-edit-ppt="${p.id}">编辑</button></div>`)}</div><div class="card"><h2>页面级编辑${selected?` · ${esc(selected.topic)}`:""}</h2>${selected?`<div class="toolbar"><button data-regenerate-ppt="${selected.id}">重新生成大纲</button><button data-pptx="${selected.id}">导出 PPTX</button><button data-pdf="${selected.id}">导出 PDF</button></div>${list(selected.slides,s=>`<div class="list-item"><div><h3>${s.index}. ${esc(s.title)}</h3><p>${esc(s.content)}</p>${s.notes?`<small>备注：${esc(s.notes)}</small>`:""}</div><div><button data-edit-slide="${s.index-1}">编辑</button><button data-move-slide="${s.index-1}" data-direction="-1">上移</button><button data-move-slide="${s.index-1}" data-direction="1">下移</button></div></div>`)}`:`<div class="empty">创建 PPT 后即可编辑页面。</div>`}</div></div>`;
}

async function renderAgents(){
  const items=await api("/agents");
  return `<div class="split"><div class="card"><h2>创建智能体</h2><form id="agent-form"><label>名称<input id="agent-name" required placeholder="企业知识助手"></label><label>说明<textarea id="agent-desc" rows="4"></textarea></label><button>创建默认工作流</button></form></div><div class="card"><h2>智能体列表</h2>${list(items,a=>`<div class="list-item"><div><h3>${esc(a.name)}</h3><p>V${a.version} · ${a.workflow.length} 个节点 · 调用 ${a.callCount} 次</p></div><div>${a.status==="DRAFT"?`<button data-publish="${a.id}">发布</button>`:`<button data-call="${a.id}">调用</button>`}</div></div>`)}</div></div>`;
}

async function renderAgentBuilder(){
  const [items,kbs]=await Promise.all([api("/agents"),api("/knowledge-bases")]);
  const selected=items.find(a=>a.id===state.agentEditorId)||items[0];
  if(selected)state.agentEditorId=selected.id;
  const versions=selected?await api(`/agents/${selected.id}/versions`):[];
  const stats=selected?await api(`/agents/${selected.id}/stats`):null;
  return `<div class="split"><div class="card"><h2>创建智能体</h2><form id="agent-form"><label>名称<input id="agent-name" required placeholder="企业知识助手"></label><label>说明<textarea id="agent-desc" rows="3"></textarea></label><button>创建默认工作流</button></form><h2 class="panel">智能体列表</h2>${list(items,a=>`<div class="list-item"><div><h3>${esc(a.name)}</h3><p>V${a.version} · ${a.workflow.length} 个节点 · ${a.status}</p></div><button data-edit-agent="${a.id}">编辑</button></div>`)}</div>
  <div class="card"><h2>可视化工作流${selected?` · ${esc(selected.name)}`:""}</h2>${selected?`<div id="workflow-canvas" class="workflow-canvas">${selected.workflow.map((n,i)=>`<div class="workflow-step"><span>${i+1}</span><strong>${n.type}</strong><small>${esc(n.label||n.id)}</small>${!["START","END"].includes(n.type)?`<button data-remove-node="${i}">移除</button>`:""}</div>`).join('<div class="workflow-arrow">↓</div>')}</div>
  <div class="toolbar panel"><label>新增节点<select id="workflow-node-type"><option>LLM</option><option>KNOWLEDGE</option><option>TOOL</option><option>CONDITION</option><option>OUTPUT</option></select></label><button id="add-workflow-node">添加节点</button><button id="save-workflow">保存新版本</button>${selected.status==="DRAFT"?`<button data-publish="${selected.id}">发布</button>`:`<button data-call="${selected.id}">调用</button><button data-share-agent="${selected.id}">生成分享链接</button>`}${state.lastAgentCallId?`<button id="agent-feedback">评价上次调用</button>`:""}</div>
  <div class="grid panel"><div class="metric"><span>调用次数</span><strong>${stats.calls}</strong></div><div class="metric"><span>公开调用</span><strong>${stats.publicCalls}</strong></div><div class="metric"><span>平均响应</span><strong>${stats.averageLatencyMs}ms</strong></div><div class="metric"><span>平均评分</span><strong>${stats.averageRating??"-"}</strong></div></div>${stats.share?`<p class="muted">公开调用地址：<code>/api/v1/public/agents/${stats.share.token}/call</code></p>`:""}
  <div class="panel"><h3>绑定知识库</h3>${kbs.length?kbs.map(k=>`<label class="check-row"><input type="checkbox" data-kb-id="${k.id}" ${selected.knowledgeBaseIds.includes(k.id)?"checked":""}>${esc(k.name)}</label>`).join(""):`<p class="muted">暂无知识库</p>`}</div>
  <div class="panel"><h3>版本记录</h3>${list(versions.slice().reverse(),v=>`<div class="list-item"><div><h3>V${v.version}</h3><p>${v.reason}</p></div><button data-rollback-agent="${selected.id}" data-version="${v.version}">回滚</button></div>`)}</div>`:`<div class="empty">创建智能体后即可编辑工作流</div>`}</div></div>`;
}

async function renderGeo(){
  const [brands,tasks]=await Promise.all([api("/geo/brands"),api("/geo/monitor-tasks")]);
  return `<div class="split"><div class="card"><h2>品牌资产</h2><form id="brand-form"><label>品牌名称<input id="brand-name" required></label><label>关键词（逗号分隔）<input id="brand-keywords"></label><button>创建品牌</button></form>${list(brands,b=>`<div class="list-item"><div><h3>${esc(b.name)}</h3><p>${esc(b.keywords.join("、"))}</p></div><button data-monitor="${b.id}">立即监测</button></div>`)}</div><div class="card"><h2>监测结果</h2>${list(tasks,t=>`<div class="list-item"><div><h3>${esc(t.question)}</h3><p>提及率 ${Math.round(t.result.mentionRate*100)}% · 排名 ${t.result.rank}</p></div>${status(t.status)}</div>`)}</div></div>`;
}

async function renderGeoOperations(){
  const data=await api("/geo/overview");
  const brandOptions=data.brands.map(b=>`<option value="${b.id}">${esc(b.name)}</option>`).join("");
  const latest=data.tasks.slice(-12).reverse();
  return `<div class="grid"><div class="card metric"><span>监测品牌</span><strong>${data.brands.length}</strong></div><div class="card metric"><span>监测次数</span><strong>${data.tasks.length}</strong></div><div class="card metric"><span>运行计划</span><strong>${data.schedules.filter(s=>s.status==="ACTIVE").length}</strong></div><div class="card metric"><span>内容发布</span><strong>${data.publications.length}</strong></div></div>
  <div class="split panel"><div class="card"><h2>品牌资产</h2><form id="brand-form"><label>品牌名称<input id="brand-name" required></label><label>关键词（逗号分隔）<input id="brand-keywords"></label><label>竞品（逗号分隔）<input id="brand-competitors"></label><button>创建品牌</button></form>${list(data.brands,b=>`<div class="list-item"><div><h3>${esc(b.name)}</h3><p>${esc((b.keywords||[]).join("、"))}</p></div><button data-monitor="${b.id}">立即监测</button></div>`)}</div>
  <div class="card"><h2>定时监测计划</h2>${data.brands.length?`<form id="geo-schedule-form"><label>品牌<select id="geo-schedule-brand">${brandOptions}</select></label><label>监测问题<input id="geo-schedule-question" placeholder="推荐相关服务"></label><label>频率<select id="geo-schedule-frequency"><option>DAILY</option><option>WEEKLY</option><option>MONTHLY</option></select></label><button>创建计划</button></form>`:"请先创建品牌"}${list(data.schedules,s=>`<div class="list-item"><div><h3>${esc(s.question)}</h3><p>${s.frequency} · 下次 ${new Date(s.nextRunAt).toLocaleString()}</p></div><button data-run-schedule="${s.id}">立即执行</button></div>`)}</div></div>
  <div class="split panel"><div class="card"><h2>监测趋势</h2>${list(latest,t=>`<div class="list-item"><div><h3>${esc(t.question)}</h3><p>提及率 ${Math.round(t.result.mentionRate*100)}% · 引用率 ${Math.round(t.result.citationRate*100)}% · 排名 ${t.result.rank}</p></div>${status(t.status)}</div>`)}</div>
  <div class="card"><h2>报告与优化内容</h2>${data.brands.length?`<div class="toolbar"><button id="geo-report" data-brand="${data.brands[0].id}">生成周报</button><button id="geo-content" data-brand="${data.brands[0].id}">生成优化文章</button></div>`:""}${list(data.reports.slice(-5).reverse(),r=>`<div class="list-item"><div><h3>${r.period} 报告</h3><p>提及率 ${Math.round(r.metrics.mentionRate*100)}% · 趋势 ${r.metrics.mentionTrend>=0?"+":""}${Math.round(r.metrics.mentionTrend*100)}%</p></div><strong>${r.taskCount} 次</strong></div>`)}${list(data.contents.slice(-5).reverse(),c=>`<div class="list-item"><div><h3>${esc(c.title)}</h3><p>${esc(c.content)}</p></div><div>${status(c.status)}<button data-publish-geo="${c.id}">记录发布</button></div></div>`)}</div></div>
  <div class="card panel"><h2>内容发布与效果</h2>${list(data.publications.slice().reverse(),p=>`<div class="list-item"><div><h3>${esc(p.platform)} · ${esc(p.url)}</h3><p>曝光 ${p.effect.latest.impressions} · 引用 ${p.effect.latest.citations} · 品牌提及 ${p.effect.latest.brandMentions} · 点击 ${p.effect.latest.clicks}</p><small>引用率 ${Math.round(p.effect.citationRate*10000)/100}% · 提及增长 ${p.effect.mentionGrowth>=0?"+":""}${p.effect.mentionGrowth}</small></div><button data-geo-metrics="${p.id}">录入效果</button></div>`)}</div>`;
}

async function renderEnterprise(){
  const enterprise=await api("/enterprises/current");
  if(!enterprise) return `<div class="card"><h2>创建企业空间</h2><p class="muted">创建后可邀请成员并统一分配 AI 使用额度。</p><form id="enterprise-form" class="toolbar"><label>企业名称<input id="enterprise-name" required></label><label>企业总额度<input id="enterprise-quota" type="number" value="10000" min="1" required></label><button>创建企业</button></form></div>`;
  const admin=enterprise.membership.role==="ENTERPRISE_ADMIN";
  return `<div class="grid"><div class="card metric"><span>企业总额度</span><strong>${enterprise.totalQuota}</strong></div><div class="card metric"><span>可分配额度</span><strong>${enterprise.availableQuota}</strong></div><div class="card metric"><span>成员数量</span><strong>${enterprise.members.length}</strong></div></div>
  ${admin?`<div class="card panel"><form id="enterprise-member-form" class="toolbar"><label>成员邮箱<input id="enterprise-member-email" type="email" required></label><label>初始额度<input id="enterprise-member-quota" type="number" value="1000" min="0"></label><button>添加成员</button></form></div>`:""}
  <div class="card panel"><h2>${esc(enterprise.name)}成员</h2>${list(enterprise.members,m=>`<div class="list-item"><div><h3>${esc(m.user.name)}</h3><p>${esc(m.user.email)} · ${m.role}</p></div><div><strong>${m.quotaUsed}/${m.quotaLimit}</strong>${admin?` <button data-quota-member="${m.id}">追加额度</button>`:""}</div></div>`)}</div>
  <div class="card panel"><h2>额度流水</h2>${list((enterprise.quotaTransactions||[]).slice(-10).reverse(),t=>`<div class="list-item"><div><h3>${t.type}</h3><p>${t.referenceType||"ENTERPRISE"} · ${t.referenceId||t.memberId}</p></div><strong>${t.amount}</strong></div>`)}</div>`;
}

async function renderMembership(){
  const [plans,orders,points]=await Promise.all([api("/plans"),api("/orders"),api("/points/account")]);
  return `<div class="grid">${plans.map(p=>`<div class="card"><span class="eyebrow">${p.durationDays>365?"BASE":p.durationDays>30?"BEST VALUE":"POPULAR"}</span><h2>${esc(p.name)}</h2><h3>${money(p.price)}</h3><p class="muted">${p.points} 积分 · 并发 ${p.concurrency}</p>${p.price?`<button data-plan="${p.id}">创建订单</button>`:""}</div>`).join("")}</div><div class="split panel"><div class="card"><h2>订单</h2>${list(orders,o=>`<div class="list-item"><div><h3>${o.id}</h3><p>${money(o.amount)} · ${o.status}</p></div><div>${o.status==="PENDING"?`<button data-pay="${o.id}">模拟支付</button>`:""}${state.user.role==="SUPER_ADMIN"&&o.status==="PAID"?`<button data-release="${o.id}">释放佣金</button> <button data-refund="${o.id}">退款</button>`:""}</div></div>`)}</div><div class="card"><h2>积分流水</h2>${list(points.transactions.slice(-8).reverse(),t=>`<div class="list-item"><div><h3>${t.type}</h3><p>${t.referenceType} · ${t.referenceId}</p></div><strong>${t.amount}</strong></div>`)}</div></div>`;
}

async function renderMembershipInvoices(){
  const [plans,orders,points,invoices,coupons]=await Promise.all([api("/plans"),api("/orders"),api("/points/account"),api("/invoices"),api("/coupons")]);
  return `<div class="grid">${plans.map(p=>`<div class="card"><span class="eyebrow">${p.durationDays>365?"BASE":p.durationDays>30?"BEST VALUE":"POPULAR"}</span><h2>${esc(p.name)}</h2><h3>${money(p.price)}</h3><p class="muted">${p.points} 积分 · 并发 ${p.concurrency}</p>${p.price?`<button data-plan="${p.id}">创建订单</button>`:""}</div>`).join("")}</div>
  <div class="card panel"><div class="toolbar"><form id="coupon-claim-form" class="toolbar"><label>领取优惠券<input id="coupon-claim-code" placeholder="输入优惠券码" required></label><button>领取</button></form><form id="redeem-code-form" class="toolbar"><label>兑换权益<input id="redeem-code" placeholder="输入兑换码" required></label><button>立即兑换</button></form><label>下单使用优惠券<select id="order-coupon"><option value="">不使用优惠券</option>${coupons.filter(c=>c.status==="AVAILABLE").map(c=>`<option value="${c.coupon.code}">${esc(c.coupon.name)} · ${c.coupon.type==="PERCENT"?c.coupon.value+"%":money(c.coupon.value)}</option>`).join("")}</select></label></div>${state.user.role==="SUPER_ADMIN"?`<form id="coupon-create-form" class="toolbar panel"><label>优惠券码<input id="coupon-code" required></label><label>名称<input id="coupon-name" required></label><label>固定减免（分）<input id="coupon-value" type="number" min="1" value="1000" required></label><button>创建优惠券</button></form>`:""}</div>
  <div class="split panel"><div class="card"><h2>订单</h2>${list(orders,o=>`<div class="list-item"><div><h3>${o.id}</h3><p>${money(o.amount)} · ${o.status}</p></div><div>${o.status==="PENDING"?`<button data-pay="${o.id}">模拟支付</button>`:""}${state.user.role!=="SUPER_ADMIN"&&o.status==="PAID"?`<button data-request-invoice="${o.id}">申请发票</button>`:""}${state.user.role==="SUPER_ADMIN"&&o.status==="PAID"?`<button data-release="${o.id}">释放佣金</button> <button data-refund="${o.id}">退款</button>`:""}</div></div>`)}</div><div class="card"><h2>积分流水</h2>${list(points.transactions.slice(-8).reverse(),t=>`<div class="list-item"><div><h3>${t.type}</h3><p>${t.referenceType} · ${t.referenceId}</p></div><strong>${t.amount}</strong></div>`)}</div></div>
  <div class="split panel"><div class="card"><h2>我的优惠券</h2>${list(coupons,c=>`<div class="list-item"><div><h3>${esc(c.coupon.name)}</h3><p>${c.coupon.code} · ${c.coupon.type==="PERCENT"?c.coupon.value+"%":money(c.coupon.value)}</p></div>${status(c.status)}</div>`)}</div><div class="card"><h2>发票</h2>${list(invoices,i=>`<div class="list-item"><div><h3>${esc(i.title)}</h3><p>${i.orderId} · ${money(i.amount)} · ${i.invoiceNumber||"待开票"}</p></div>${state.user.role==="SUPER_ADMIN"&&i.status==="PENDING"?`<button data-issue-invoice="${i.id}">确认开票</button>`:status(i.status)}</div>`)}</div></div>`;
}

async function renderChannel(){
  const [agents,commissions,withdrawals,codes,performance,statements]=await Promise.all([api("/channel-agents"),api("/commissions"),api("/withdrawals"),api("/redemption-codes"),api("/channel-agents/performance"),api("/settlement-statements")]);
  const canCreate=["SUPER_ADMIN","AGENT_L1"].includes(state.user.role);
  const canBind=["AGENT_L1","AGENT_L2"].includes(state.user.role);
  return `${canCreate||canBind?`<div class="card"><div class="toolbar">${canCreate?`<form id="channel-form" class="toolbar"><label>下级代理名称<input id="channel-name" required></label><label>邮箱<input id="channel-email" type="email" required></label><label>初始密码<input id="channel-password" value="Agent123!" required></label><button>创建代理商</button></form>`:""}${canBind?`<form id="bind-form" class="toolbar"><label>绑定客户邮箱<input id="customer-email" type="email" required></label><button>绑定客户</button></form>`:""}${["SUPER_ADMIN","AGENT_L1","AGENT_L2"].includes(state.user.role)?`<form id="redemption-create-form" class="toolbar"><label>积分兑换码额度<input id="redemption-points" type="number" min="1" value="100" required></label><label>可兑换次数<input id="redemption-max-uses" type="number" min="1" value="1" required></label><button>创建积分兑换码</button></form>`:""}</div></div>`:""}
  <div class="grid panel">${performance.slice(0,4).map(p=>`<div class="card metric"><span>#${p.rank} ${esc(p.name)}</span><strong>${money(p.revenue)}</strong><small>${p.customers} 客户 · ${p.paidOrders} 已支付订单</small></div>`).join("")}</div>
  <div class="card panel"><h2>业绩排行榜</h2>${state.user.role==="SUPER_ADMIN"?`<button id="snapshot-performance">保存当前业绩快照</button>`:""}${list(performance,p=>`<div class="list-item"><div><h3>#${p.rank} ${esc(p.name)}</h3><p>${p.level} 级 · 客户 ${p.customers} · 下级 ${p.directAgents} · 订单 ${p.paidOrders}/${p.orders} · 退款 ${p.refundedOrders}</p></div><div><strong>${money(p.revenue)}</strong><p class="muted">可提现佣金 ${money(p.commissionAvailable)}</p></div></div>`)}</div>
  <div class="split panel"><div class="card"><h2>代理商</h2>${list(agents,a=>`<div class="list-item"><div><h3>${a.id}</h3><p>${a.level} 级 · 邀请码 ${a.inviteCode}</p></div>${state.user.role==="SUPER_ADMIN"&&a.status==="PENDING"?`<button data-approve-channel="${a.id}">审核通过</button>`:status(a.status)}</div>`)}</div><div class="card"><h2>佣金与提现</h2>${list(commissions,c=>`<div class="list-item"><div><h3>${c.orderId}</h3><p>比例 ${c.rate*100}% · ${c.status}</p></div><strong>${money(c.amount)}</strong></div>`)}${commissions.some(c=>c.status==="AVAILABLE")?`<button id="withdraw-all">申请提现可用佣金</button>`:""}${list(withdrawals,w=>`<div class="list-item"><div><h3>${w.id}</h3><p>${w.status}</p></div>${state.user.role==="SUPER_ADMIN"&&w.status==="PENDING"?`<button data-approve-withdraw="${w.id}">审核提现</button>`:`<strong>${money(w.amount)}</strong>`}</div>`)}</div></div><div class="split panel"><div class="card"><h2>结算单</h2>${list(statements,s=>`<div class="list-item"><div><h3>${s.statementNumber}</h3><p>${s.commissionIds.length} 条佣金 · ${s.status}</p></div><strong>${money(s.amount)}</strong></div>`)}</div><div class="card"><h2>兑换码</h2>${list(codes,c=>`<div class="list-item"><div><h3>${c.code}</h3><p>${c.type} · 已兑换 ${c.usesCount}/${c.maxUses}</p></div>${status(c.status)}</div>`)}</div></div>`;
}

async function renderAdmin(){
  const [data,http,modelConfig]=await Promise.all([api("/admin/overview"),api("/admin/metrics"),api("/admin/model-config")]);
  return `<div class="grid">${Object.entries(data.metrics).map(([k,v])=>`<div class="card metric"><span>${({users:"用户总数",activeUsers:"活跃用户",paidRevenue:"已支付收入",generationTasks:"生成任务",generationSuccessRate:"生成成功率",pendingWithdrawals:"待审提现",rejectedModeration:"审核拒绝"}[k]||k)}</span><strong>${k==="paidRevenue"?money(v):k==="generationSuccessRate"?Math.round(v*100)+"%":v}</strong></div>`).join("")}</div>
  <div class="split panel"><div class="card"><h2>用户管理</h2>${list(data.users,u=>`<div class="list-item"><div><h3>${esc(u.name)}</h3><p>${esc(u.email)} · ${u.role} · ${u.status}</p></div>${u.id!==state.user.id?`<button data-user-status="${u.id}" data-next-status="${u.status==="ACTIVE"?"SUSPENDED":"ACTIVE"}">${u.status==="ACTIVE"?"冻结":"恢复"}</button>`:""}</div>`)}</div>
  <div class="card"><h2>模型成本与成功率</h2>${list(data.modelUsage,m=>`<div class="list-item"><div><h3>${esc(m.model)}</h3><p>${m.tasks} 个任务 · 成功 ${m.succeeded}</p></div><strong>${m.points} 积分</strong></div>`)}<h3 class="panel">供应商调用</h3>${list(data.providerUsage,p=>`<div class="list-item"><div><h3>${esc(p.providerCode)}</h3><p>${p.calls} 次 · 成功 ${p.succeeded} · 平均 ${p.averageLatencyMs}ms</p></div><strong>${money(p.costCents)}</strong></div>`)}</div></div>
  <div class="split panel"><div class="card"><h2>HTTP 监控与告警</h2><div class="grid"><div class="metric"><span>请求数</span><strong>${http.requests}</strong></div><div class="metric"><span>错误率</span><strong>${Math.round(http.errorRate*100)}%</strong></div></div>${list(http.alerts,a=>`<div class="list-item"><div><h3>${a.code}</h3><p>${esc(a.message)}</p></div>${status(a.level)}</div>`)}${list(http.routes.slice(0,8),r=>`<div class="list-item"><div><h3>${esc(r.route)}</h3><p>${r.requests} 次 · 错误 ${r.errors}</p></div><strong>${r.averageLatencyMs}ms</strong></div>`)}</div><div class="card"><h2>内容审核记录</h2>${list(data.moderationLogs,m=>`<div class="list-item"><div><h3>${m.contentType}</h3><p>${(m.matchedTerms||[]).join("、")||"通过"}</p></div>${status(m.status)}</div>`)}</div></div>
  <div class="split panel"><div class="card"><h2>模型供应商配置</h2><form id="model-provider-form" class="toolbar"><label>供应商编码<input id="model-provider-code" required placeholder="provider-code"></label><label>供应商名称<input id="model-provider-name" required></label><label>服务地址<input id="model-provider-url" type="url" placeholder="https://api.example.com"></label><button>创建供应商</button></form>${list(modelConfig.providers,p=>`<div class="list-item"><div><h3>${esc(p.name)}</h3><p>${p.code} · ${p.baseUrl||"未配置地址"}</p></div>${status(p.status)}</div>`)}</div><div class="card"><h2>模型与计费规则</h2><form id="model-definition-form" class="toolbar"><label>供应商<select id="model-definition-provider">${modelConfig.providers.filter(p=>p.status==="ACTIVE").map(p=>`<option value="${p.code}">${esc(p.name)}</option>`).join("")}</select></label><label>模型编码<input id="model-definition-code" required></label><label>模型名称<input id="model-definition-name" required></label><label>能力<select id="model-definition-capability"><option>TEXT_TO_IMAGE</option><option>IMAGE_TO_IMAGE</option><option>TEXT_TO_VIDEO</option><option>IMAGE_TO_VIDEO</option></select></label><label>会员等级<select id="model-definition-tier"><option>PAID</option><option>FREE</option></select></label><label>积分单价<input id="model-definition-cost" type="number" min="1" value="10"></label><button>创建模型</button></form>${list(modelConfig.models,m=>`<div class="list-item"><div><h3>${esc(m.name)}</h3><p>${m.code} · ${m.providerCode} · ${m.capabilities.join("、")} · ${m.tier}</p></div><button data-model-status="${m.id}" data-next="${m.status==="ACTIVE"?"DISABLED":"ACTIVE"}">${m.status==="ACTIVE"?"停用":"启用"}</button></div>`)}</div></div>
  <div class="card panel"><h2>最近审计日志</h2>${list(data.recentAuditLogs,a=>`<div class="list-item"><div><h3>${a.action}</h3><p>${a.targetType} · ${a.targetId}</p></div><small>${new Date(a.createdAt).toLocaleString()}</small></div>`)}</div>`;
}

function bindPage(){
  let genType="TEXT_TO_IMAGE";
  document.querySelectorAll("[data-request-invoice]").forEach(b=>b.onclick=async()=>{const title=window.prompt("请输入发票抬头",state.user.name);if(!title)return;await api("/invoices",{method:"POST",body:JSON.stringify({orderId:b.dataset.requestInvoice,title,email:state.user.email})});toast("发票申请已提交");render()});
  document.querySelectorAll("[data-issue-invoice]").forEach(b=>b.onclick=async()=>{await api(`/invoices/${b.dataset.issueInvoice}/issue`,{method:"POST"});toast("发票已开具");render()});
  document.querySelectorAll("[data-user-status]").forEach(b=>b.onclick=async()=>{await api(`/admin/users/${b.dataset.userStatus}/status`,{method:"POST",body:JSON.stringify({status:b.dataset.nextStatus})});toast("用户状态已更新");render()});
  $("#model-provider-form")?.addEventListener("submit",async e=>{e.preventDefault();await api("/admin/model-providers",{method:"POST",body:JSON.stringify({code:$("#model-provider-code").value,name:$("#model-provider-name").value,baseUrl:$("#model-provider-url").value||null})});toast("模型供应商已创建");render()});
  $("#model-definition-form")?.addEventListener("submit",async e=>{e.preventDefault();const capability=$("#model-definition-capability").value;await api("/admin/model-definitions",{method:"POST",body:JSON.stringify({providerCode:$("#model-definition-provider").value,code:$("#model-definition-code").value,name:$("#model-definition-name").value,capabilities:[capability],tier:$("#model-definition-tier").value,pointCosts:{[capability]:Number($("#model-definition-cost").value)}})});toast("模型与计费规则已创建");render()});
  document.querySelectorAll("[data-model-status]").forEach(b=>b.onclick=async()=>{await api(`/admin/model-definitions/${b.dataset.modelStatus}/status`,{method:"POST",body:JSON.stringify({status:b.dataset.next})});toast("模型状态已更新");render()});
  document.querySelectorAll("[data-edit-agent]").forEach(b=>b.onclick=()=>{state.agentEditorId=b.dataset.editAgent;render()});
  document.querySelectorAll("[data-edit-ppt]").forEach(b=>b.onclick=()=>{state.pptEditorId=b.dataset.editPpt;render()});
  document.querySelectorAll("[data-edit-slide]").forEach(b=>b.onclick=async()=>{const items=await api("/presentations");const ppt=items.find(p=>p.id===state.pptEditorId);const i=Number(b.dataset.editSlide);const title=window.prompt("页面标题",ppt.slides[i].title);if(title===null)return;const content=window.prompt("页面内容",ppt.slides[i].content);if(content===null)return;const notes=window.prompt("演讲备注",ppt.slides[i].notes||"");const slides=structuredClone(ppt.slides);slides[i]={...slides[i],title,content,notes:notes||""};await api(`/presentations/${ppt.id}`,{method:"PUT",body:JSON.stringify({slides})});toast("页面内容已保存");render()});
  document.querySelectorAll("[data-move-slide]").forEach(b=>b.onclick=async()=>{const items=await api("/presentations");const ppt=items.find(p=>p.id===state.pptEditorId);const from=Number(b.dataset.moveSlide),to=from+Number(b.dataset.direction);if(to<0||to>=ppt.slides.length)return;const slides=structuredClone(ppt.slides);[slides[from],slides[to]]=[slides[to],slides[from]];await api(`/presentations/${ppt.id}`,{method:"PUT",body:JSON.stringify({slides})});toast("页面顺序已更新");render()});
  document.querySelectorAll("[data-regenerate-ppt]").forEach(b=>b.onclick=async()=>{await api(`/presentations/${b.dataset.regeneratePpt}/regenerate-outline`,{method:"POST",body:"{}"});toast("大纲已重新生成");render()});
  $("#add-workflow-node")?.addEventListener("click",async()=>{const agents=await api("/agents");const agent=agents.find(a=>a.id===state.agentEditorId);const type=$("#workflow-node-type").value;const workflow=structuredClone(agent.workflow);workflow.splice(-1,0,{id:`${type.toLowerCase()}_${Date.now()}`,type,label:type});await api(`/agents/${agent.id}/workflow`,{method:"PUT",body:JSON.stringify({workflow,knowledgeBaseIds:agent.knowledgeBaseIds})});toast("节点已添加并保存新版本");render()});
  document.querySelectorAll("[data-remove-node]").forEach(b=>b.onclick=async()=>{const agents=await api("/agents");const agent=agents.find(a=>a.id===state.agentEditorId);const workflow=agent.workflow.filter((_,i)=>i!==Number(b.dataset.removeNode));await api(`/agents/${agent.id}/workflow`,{method:"PUT",body:JSON.stringify({workflow,knowledgeBaseIds:agent.knowledgeBaseIds})});toast("节点已移除并保存新版本");render()});
  $("#save-workflow")?.addEventListener("click",async()=>{const agents=await api("/agents");const agent=agents.find(a=>a.id===state.agentEditorId);const knowledgeBaseIds=[...document.querySelectorAll("[data-kb-id]:checked")].map(el=>el.dataset.kbId);await api(`/agents/${agent.id}/workflow`,{method:"PUT",body:JSON.stringify({workflow:agent.workflow,knowledgeBaseIds})});toast("工作流新版本已保存");render()});
  document.querySelectorAll("[data-rollback-agent]").forEach(b=>b.onclick=async()=>{await api(`/agents/${b.dataset.rollbackAgent}/rollback`,{method:"POST",body:JSON.stringify({version:Number(b.dataset.version)})});toast("工作流已回滚为新草稿版本");render()});
  const updateEstimate=async()=>{if(!$("#generation-estimate"))return;try{const quote=await api("/generations/estimate",{method:"POST",body:JSON.stringify({type:genType,model:$("#model").value,count:Number($("#count").value)})});$("#generation-estimate").textContent=`${quote.pointCost} 积分`}catch(e){$("#generation-estimate").textContent=e.message}};
  $("#generation-tabs")?.addEventListener("click",e=>{if(e.target.dataset.type){genType=e.target.dataset.type;document.querySelectorAll("#generation-tabs button").forEach(b=>b.classList.toggle("active",b===e.target));updateEstimate()}});
  $("#count")?.addEventListener("change",updateEstimate);$("#model")?.addEventListener("change",updateEstimate);updateEstimate();
  $("#new-generation")?.addEventListener("click",()=>{$("#prompt")?.focus();if($("#prompt"))$("#prompt").value=""});
  document.querySelector("[data-open-history]")?.addEventListener("click",()=>{state.showGenerationHistory=true;render()});
  document.querySelector("[data-new-canvas]")?.addEventListener("click",()=>{state.showGenerationHistory=false;state.generationCanvasStartedAt=new Date().toISOString();state.focusGenerationTaskId=null;state.generationPrompt="";if($("#prompt"))$("#prompt").value="";render()});
  document.querySelector("[data-clear-canvas]")?.addEventListener("click",()=>{state.showGenerationHistory=false;state.generationCanvasStartedAt=new Date().toISOString();state.focusGenerationTaskId=null;render()});
  document.querySelector("[data-open-assets]")?.addEventListener("click",()=>{state.page="assets";render()});
  document.querySelectorAll("[data-history-task]").forEach(b=>b.onclick=()=>{if(b.dataset.historyTask)state.focusGenerationTaskId=b.dataset.historyTask;render()});
  const canvasViewport=$(".canvas-viewport");
  if(canvasViewport){
    const board=canvasViewport.querySelector(".canvas-board");
    if(board){
      board.style.width=`${board.dataset.boardWidth}px`;
      board.style.height=`${board.dataset.boardHeight}px`;
      board.querySelectorAll(".canvas-node").forEach(node=>{node.style.left=`${node.dataset.x}px`;node.style.top=`${node.dataset.y}px`});
    }
    canvasViewport.scrollLeft=0;canvasViewport.scrollTop=canvasViewport.scrollHeight;
    let dragging=false,startX=0,startY=0,startLeft=0,startTop=0;
    canvasViewport.addEventListener("pointerdown",e=>{if(e.target.closest("button"))return;dragging=true;startX=e.clientX;startY=e.clientY;startLeft=canvasViewport.scrollLeft;startTop=canvasViewport.scrollTop;canvasViewport.classList.add("dragging");canvasViewport.setPointerCapture?.(e.pointerId)});
    canvasViewport.addEventListener("pointermove",e=>{if(!dragging)return;canvasViewport.scrollLeft=startLeft-(e.clientX-startX);canvasViewport.scrollTop=startTop-(e.clientY-startY)});
    ["pointerup","pointercancel","pointerleave"].forEach(type=>canvasViewport.addEventListener(type,e=>{dragging=false;canvasViewport.classList.remove("dragging");try{canvasViewport.releasePointerCapture?.(e.pointerId)}catch{}}));
  }
  document.querySelectorAll("[data-reuse-task]").forEach(b=>b.onclick=async()=>{const tasks=await api("/generation-tasks");const t=tasks.find(item=>item.id===b.dataset.reuseTask);if(!t)return;const p=t.params||{};$("#prompt").value=t.prompt||p.prompt||"";if($("#model"))$("#model").value=t.model||p.model||$("#model").value;if($("#count"))$("#count").value=p.count||1;if($("#image-ratio")&&p.ratio)$("#image-ratio").value=p.ratio;if($("#reference-asset")&&p.referenceAssetId)$("#reference-asset").value=p.referenceAssetId;if($("#billing-source"))$("#billing-source").value=t.billingSource||p.billingSource||"AUTO";toast("已复用该轮配置")});
  $("#prompt")?.addEventListener("input",e=>{state.generationPrompt=e.target.value});
  $("#reference-asset")?.addEventListener("change",e=>{state.selectedReferenceAssetId=e.target.value});
  $("#upload-reference")?.addEventListener("click",()=>$("#quick-upload-file")?.click());
  $("#quick-upload-file")?.addEventListener("change",async e=>{const file=e.target.files[0];if(!file)return;try{state.generationPrompt=$("#prompt")?.value||state.generationPrompt;const dataBase64=await new Promise((resolve,reject)=>{const reader=new FileReader();reader.onload=()=>resolve(String(reader.result).split(",")[1]);reader.onerror=reject;reader.readAsDataURL(file)});const asset=await api("/assets/upload",{method:"POST",body:JSON.stringify({name:file.name,contentType:file.type,dataBase64})});state.selectedReferenceAssetId=asset.id;toast("参考图已上传并选中");await refresh()}catch(e){toast(e.message)}});
  $("#prompt")?.addEventListener("keydown",e=>{if(e.key==="Enter"&&!e.shiftKey){e.preventDefault();$("#generation-form")?.requestSubmit()}});
  $("#generation-form")?.addEventListener("submit",async e=>{e.preventDefault();const map={TEXT_TO_IMAGE:"/generations/images/text-to-image",IMAGE_TO_IMAGE:"/generations/images/image-to-image",TEXT_TO_VIDEO:"/generations/videos/text-to-video",IMAGE_TO_VIDEO:"/generations/videos/image-to-video"};const sizeMap={"4:3":"1536x1024","3:4":"1024x1536","1:1":"1024x1024","16:9":"1536x864","9:16":"864x1536"};try{await api(map[genType],{method:"POST",body:JSON.stringify({prompt:$("#prompt").value,referenceAssetId:$("#reference-asset").value||null,model:$("#model").value,billingSource:$("#billing-source").value,count:Number($("#count").value),ratio:$("#image-ratio")?.value||"4:3",size:sizeMap[$("#image-ratio")?.value]||"1536x1024",idempotencyKey:crypto.randomUUID()})});toast("任务已进入队列，完成后会自动显示");await refresh();scheduleGenerationPoll()}catch(e){toast(e.message)}});
  document.querySelectorAll("[data-retry-task]").forEach(b=>b.onclick=async()=>{try{await api(`/generation-tasks/${b.dataset.retryTask}/retry`,{method:"POST",body:JSON.stringify({idempotencyKey:crypto.randomUUID()})});toast("失败任务已重新进入队列");render()}catch(e){toast(e.message)}});
  document.querySelectorAll("[data-cancel-task]").forEach(b=>b.onclick=async()=>{try{await api(`/generation-tasks/${b.dataset.cancelTask}/cancel`,{method:"POST"});toast("任务已取消并退还积分");refresh()}catch(e){toast(e.message)}});
  document.querySelectorAll("[data-favorite-asset]").forEach(b=>b.onclick=async()=>{await api(`/assets/${b.dataset.favoriteAsset}/favorite`,{method:"POST",body:JSON.stringify({favorite:b.dataset.next==="true"})});toast("收藏状态已更新");render()});
  document.querySelectorAll("[data-download-asset]").forEach(b=>b.onclick=async()=>{try{await download(`/assets/${b.dataset.downloadAsset}/download`,b.dataset.name);toast("作品已下载")}catch(e){toast(e.message)}});
  document.querySelectorAll("[data-regenerate-asset]").forEach(b=>b.onclick=async()=>{try{await api(`/assets/${b.dataset.regenerateAsset}/regenerate`,{method:"POST",body:JSON.stringify({idempotencyKey:crypto.randomUUID()})});toast("再次生成任务已进入队列");render()}catch(e){toast(e.message)}});
  document.querySelectorAll("[data-delete-asset]").forEach(b=>b.onclick=async()=>{if(!window.confirm("确认从作品中心删除该资产？"))return;await api(`/assets/${b.dataset.deleteAsset}`,{method:"DELETE"});toast("资产已删除");render()});
  $("#upload-form")?.addEventListener("submit",async e=>{e.preventDefault();const file=$("#upload-file").files[0];if(!file)return;try{const dataBase64=await new Promise((resolve,reject)=>{const reader=new FileReader();reader.onload=()=>resolve(String(reader.result).split(",")[1]);reader.onerror=reject;reader.readAsDataURL(file)});await api("/assets/upload",{method:"POST",body:JSON.stringify({name:file.name,contentType:file.type,dataBase64})});toast("文件已上传至资产库");render()}catch(e){toast(e.message)}});
  $("#ppt-form")?.addEventListener("submit",async e=>{e.preventDefault();await api("/presentations",{method:"POST",body:JSON.stringify({topic:$("#ppt-topic").value,theme:$("#ppt-theme").value})});toast("PPT 大纲已生成");render()});
  $("#agent-form")?.addEventListener("submit",async e=>{e.preventDefault();await api("/agents",{method:"POST",body:JSON.stringify({name:$("#agent-name").value,description:$("#agent-desc").value})});toast("智能体已创建");render()});
  document.querySelectorAll("[data-pptx]").forEach(b=>b.onclick=async()=>{try{await download(`/presentations/${b.dataset.pptx}/export-pptx`,`${b.dataset.pptx}.pptx`);toast("PPTX 已导出")}catch(e){toast(e.message)}});
  document.querySelectorAll("[data-pdf]").forEach(b=>b.onclick=async()=>{try{await download(`/presentations/${b.dataset.pdf}/export-pdf`,`${b.dataset.pdf}.pdf`);toast("PDF 已导出")}catch(e){toast(e.message)}});
  document.querySelectorAll("[data-publish]").forEach(b=>b.onclick=async()=>{await api(`/agents/${b.dataset.publish}/publish`,{method:"POST"});toast("智能体已发布");render()});
  document.querySelectorAll("[data-call]").forEach(b=>b.onclick=async()=>{const input=window.prompt("请输入调用内容","请介绍先知 AI");if(input===null)return;const call=await api(`/agents/${b.dataset.call}/call`,{method:"POST",body:JSON.stringify({input})});state.lastAgentCallId=call.id;toast(call.output);render()});
  document.querySelectorAll("[data-share-agent]").forEach(b=>b.onclick=async()=>{const share=await api(`/agents/${b.dataset.shareAgent}/share`,{method:"POST"});toast(`分享令牌：${share.token}`);render()});
  $("#agent-feedback")?.addEventListener("click",async()=>{const rating=Number(window.prompt("请为上次调用评分（1-5）","5"));if(!rating)return;const comment=window.prompt("评价内容","回答有帮助")||"";await api(`/agent-calls/${state.lastAgentCallId}/feedback`,{method:"POST",body:JSON.stringify({rating,comment})});toast("评价已提交");render()});
  $("#brand-form")?.addEventListener("submit",async e=>{e.preventDefault();await api("/geo/brands",{method:"POST",body:JSON.stringify({name:$("#brand-name").value,keywords:$("#brand-keywords").value.split(",").filter(Boolean),competitors:$("#brand-competitors")?.value.split(",").filter(Boolean)||[]})});toast("品牌已创建");render()});
  $("#geo-schedule-form")?.addEventListener("submit",async e=>{e.preventDefault();await api("/geo/schedules",{method:"POST",body:JSON.stringify({brandId:$("#geo-schedule-brand").value,question:$("#geo-schedule-question").value,frequency:$("#geo-schedule-frequency").value})});toast("监测计划已创建");render()});
  document.querySelectorAll("[data-run-schedule]").forEach(b=>b.onclick=async()=>{await api(`/geo/schedules/${b.dataset.runSchedule}/run`,{method:"POST"});toast("计划已执行");render()});
  $("#geo-report")?.addEventListener("click",async e=>{await api("/geo/reports",{method:"POST",body:JSON.stringify({brandId:e.target.dataset.brand,period:"WEEKLY"})});toast("GEO 周报已生成");render()});
  $("#geo-content")?.addEventListener("click",async e=>{await api("/geo/contents",{method:"POST",body:JSON.stringify({brandId:e.target.dataset.brand,type:"ARTICLE"})});toast("优化文章已生成");render()});
  document.querySelectorAll("[data-publish-geo]").forEach(b=>b.onclick=async()=>{const platform=window.prompt("发布平台","官网");if(!platform)return;const url=window.prompt("内容 URL","https://example.com/article");if(!url)return;try{await api(`/geo/contents/${b.dataset.publishGeo}/publish`,{method:"POST",body:JSON.stringify({platform,url})});toast("发布记录已保存");render()}catch(e){toast(e.message)}});
  document.querySelectorAll("[data-geo-metrics]").forEach(b=>b.onclick=async()=>{const impressions=Number(window.prompt("曝光量","1000"));const citations=Number(window.prompt("引用次数","20"));const brandMentions=Number(window.prompt("品牌提及次数","30"));const clicks=Number(window.prompt("点击次数","80"));await api(`/geo/publications/${b.dataset.geoMetrics}/metrics`,{method:"POST",body:JSON.stringify({impressions,citations,brandMentions,clicks})});toast("内容效果已更新");render()});
  $("#enterprise-form")?.addEventListener("submit",async e=>{e.preventDefault();await api("/enterprises",{method:"POST",body:JSON.stringify({name:$("#enterprise-name").value,totalQuota:Number($("#enterprise-quota").value)})});toast("企业空间已创建");refresh()});
  $("#enterprise-member-form")?.addEventListener("submit",async e=>{e.preventDefault();try{await api("/enterprise-members",{method:"POST",body:JSON.stringify({email:$("#enterprise-member-email").value,quotaLimit:Number($("#enterprise-member-quota").value)})});toast("企业成员已添加");render()}catch(e){toast(e.message)}});
  document.querySelectorAll("[data-quota-member]").forEach(b=>b.onclick=async()=>{const amount=Number(window.prompt("追加额度","1000"));if(!amount)return;try{await api(`/enterprise-members/${b.dataset.quotaMember}/allocate-quota`,{method:"POST",body:JSON.stringify({amount})});toast("额度已分配");render()}catch(e){toast(e.message)}});
  document.querySelectorAll("[data-monitor]").forEach(b=>b.onclick=async()=>{await api("/geo/monitor-tasks",{method:"POST",body:JSON.stringify({brandId:b.dataset.monitor})});toast("监测已完成");render()});
  $("#coupon-claim-form")?.addEventListener("submit",async e=>{e.preventDefault();try{await api("/coupons/claim",{method:"POST",body:JSON.stringify({code:$("#coupon-claim-code").value})});toast("优惠券已领取");render()}catch(e){toast(e.message)}});
  $("#coupon-create-form")?.addEventListener("submit",async e=>{e.preventDefault();try{await api("/coupons",{method:"POST",body:JSON.stringify({code:$("#coupon-code").value,name:$("#coupon-name").value,type:"FIXED",value:Number($("#coupon-value").value),maxUses:100})});toast("优惠券已创建");render()}catch(e){toast(e.message)}});
  $("#redeem-code-form")?.addEventListener("submit",async e=>{e.preventDefault();try{await api("/redemption-codes/redeem",{method:"POST",body:JSON.stringify({code:$("#redeem-code").value})});toast("权益兑换成功");refresh()}catch(e){toast(e.message)}});
  $("#redemption-create-form")?.addEventListener("submit",async e=>{e.preventDefault();try{const code=await api("/redemption-codes",{method:"POST",body:JSON.stringify({type:"POINTS",points:Number($("#redemption-points").value),maxUses:Number($("#redemption-max-uses").value)})});toast(`兑换码已创建：${code.code}`);render()}catch(e){toast(e.message)}});
  $("#snapshot-performance")?.addEventListener("click",async()=>{await api("/channel-agents/performance-snapshots",{method:"POST",body:JSON.stringify({period:"ALL"})});toast("业绩快照已保存");render()});
  document.querySelectorAll("[data-plan]").forEach(b=>b.onclick=async()=>{await api("/orders",{method:"POST",body:JSON.stringify({planId:b.dataset.plan,couponCode:$("#order-coupon")?.value||null})});toast("订单已创建");render()});
  document.querySelectorAll("[data-pay]").forEach(b=>b.onclick=async()=>{await api(`/orders/${b.dataset.pay}/pay`,{method:"POST",body:JSON.stringify({eventId:`demo_${Date.now()}`})});toast("支付成功，权益已发放");refresh()});
  document.querySelectorAll("[data-release]").forEach(b=>b.onclick=async()=>{await api(`/orders/${b.dataset.release}/release-commissions`,{method:"POST"});toast("佣金已释放");render()});
  document.querySelectorAll("[data-refund]").forEach(b=>b.onclick=async()=>{try{await api(`/orders/${b.dataset.refund}/refund`,{method:"POST",body:JSON.stringify({eventId:`refund_${Date.now()}`})});toast("退款和佣金回退已完成");refresh()}catch(e){toast(e.message)}});
  $("#channel-form")?.addEventListener("submit",async e=>{e.preventDefault();await api("/channel-agents",{method:"POST",body:JSON.stringify({name:$("#channel-name").value,email:$("#channel-email").value,password:$("#channel-password").value})});toast("代理商已创建");render()});
  $("#bind-form")?.addEventListener("submit",async e=>{e.preventDefault();try{await api("/channel-customers/bind",{method:"POST",body:JSON.stringify({email:$("#customer-email").value})});toast("客户绑定成功")}catch(e){toast(e.message)}});
  document.querySelectorAll("[data-approve-channel]").forEach(b=>b.onclick=async()=>{await api(`/channel-agents/${b.dataset.approveChannel}/approve`,{method:"POST"});toast("代理商已启用");render()});
  $("#withdraw-all")?.addEventListener("click",async()=>{const available=(await api("/commissions")).filter(c=>c.status==="AVAILABLE");for(const c of available)await api("/withdrawals",{method:"POST",body:JSON.stringify({amount:c.amount})});toast("提现申请已创建");render()});
  document.querySelectorAll("[data-approve-withdraw]").forEach(b=>b.onclick=async()=>{await api(`/withdrawals/${b.dataset.approveWithdraw}/approve`,{method:"POST"});toast("提现已审核");render()});
}

if(state.token) boot();
