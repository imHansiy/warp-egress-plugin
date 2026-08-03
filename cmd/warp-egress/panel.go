package main

const panelHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light">
<title>WARP 出口管理</title>
<style>
:root{
  color-scheme:light;
  --bg:#ffffff;--soft:#f7f9fc;--soft-blue:#f2f7ff;--line:#e3e9f1;--line-strong:#cfd9e7;
  --text:#17243a;--muted:#6f7f95;--subtle:#9aa7b9;--blue:#1672e8;--blue-dark:#0e5fc5;
  --blue-soft:#eaf3ff;--red:#c23d4d;--red-soft:#fff2f4;--amber:#8a5b00;--amber-soft:#fff8e8;
  --radius:12px;--shadow:0 18px 50px rgba(20,44,80,.16);
}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--text);font:14px/1.5 Inter,ui-sans-serif,system-ui,-apple-system,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif;-webkit-font-smoothing:antialiased}
button,input,select{font:inherit}button{border:0}button:focus-visible,input:focus-visible,select:focus-visible{outline:3px solid rgba(22,114,232,.16);outline-offset:2px}
.page{max-width:1240px;margin:0 auto;padding:24px 28px 48px}
.header{display:flex;align-items:center;justify-content:space-between;gap:20px;margin-bottom:14px}.compact-header{min-height:36px}
.title-row{display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.status-chip{display:inline-flex;align-items:center;gap:7px;border:1px solid var(--line);border-radius:999px;padding:5px 9px;color:var(--muted);font-size:12px;background:#fff}.status-dot{width:7px;height:7px;border-radius:50%;background:#aab5c4}.status-dot.ok{background:var(--blue);box-shadow:0 0 0 4px rgba(22,114,232,.1)}
.header-actions{display:flex;gap:8px;align-items:center;flex-wrap:wrap}.btn{height:36px;padding:0 13px;border-radius:9px;background:var(--blue);color:#fff;cursor:pointer;font-weight:700;display:inline-flex;align-items:center;justify-content:center;gap:7px;white-space:nowrap;transition:.15s ease}.btn:hover{background:var(--blue-dark)}.btn:disabled{opacity:.5;cursor:not-allowed}.btn.secondary{background:#fff;color:#354963;border:1px solid var(--line-strong)}.btn.secondary:hover{background:var(--soft);border-color:#aebdd1}.btn.soft{background:var(--blue-soft);color:var(--blue)}.btn.soft:hover{background:#dfeeff}.btn.danger{background:#fff;color:var(--red);border:1px solid #edc5cb}.btn.danger:hover{background:var(--red-soft)}.btn.small{height:30px;padding:0 9px;font-size:12px}.btn.icon{width:36px;padding:0}.spinner{width:13px;height:13px;border:2px solid currentColor;border-right-color:transparent;border-radius:50%;animation:spin .65s linear infinite}@keyframes spin{to{transform:rotate(360deg)}}
.notice{display:none;margin-bottom:16px;padding:11px 13px;border:1px solid #bfd9fa;background:var(--soft-blue);border-radius:10px;color:#28517f;font-size:12px}.notice.show{display:flex;align-items:center;justify-content:space-between;gap:14px}.notice.error{border-color:#efc9cf;background:var(--red-soft);color:#8d2f3b}.notice code{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;color:#164f96}
.current{border:1px solid var(--line);border-radius:var(--radius);padding:18px 20px;display:grid;grid-template-columns:minmax(0,1fr) auto;gap:24px;align-items:center;background:#fff;margin-bottom:22px}
.current-main{min-width:0}.eyebrow{color:var(--muted);font-size:12px;font-weight:700;margin-bottom:6px}.current-name{display:flex;align-items:center;gap:9px;flex-wrap:wrap}.current-name strong{font-size:18px}.current-ip{margin-top:5px;font:700 15px/1.45 ui-monospace,SFMono-Regular,Menlo,monospace;word-break:break-all}.current-ip-alt{margin-top:3px;font:600 12px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;color:var(--muted);word-break:break-all}.current-meta{margin-top:6px;color:var(--muted);font-size:12px;display:flex;gap:14px;flex-wrap:wrap}.current-meta code{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;color:#51637a}
.current-switch{display:flex;align-items:flex-end;gap:8px;min-width:380px}.field{display:grid;gap:6px}.field.grow{flex:1}.field label{font-size:12px;font-weight:700;color:#4a5b72}.input,.select{height:36px;border:1px solid var(--line-strong);background:#fff;color:var(--text);border-radius:9px;padding:0 10px;min-width:0}.input:focus,.select:focus{border-color:#7facdf;outline:none;box-shadow:0 0 0 3px rgba(22,114,232,.08)}
.section{margin-top:24px}.section-head{display:flex;align-items:center;justify-content:space-between;gap:18px;margin-bottom:10px}.section-title h2{font-size:16px;margin:0}.section-title p{margin:3px 0 0;color:var(--muted);font-size:12px}.section-actions{display:flex;gap:8px;align-items:center;flex-wrap:wrap}
.table-wrap{border:1px solid var(--line);border-radius:var(--radius);overflow:hidden;background:#fff}.table-scroll{overflow:auto}table{width:100%;border-collapse:collapse;min-width:850px}th,td{padding:12px 14px;border-bottom:1px solid var(--line);text-align:left;vertical-align:middle}th{background:#fafbfd;color:#66758a;font-size:11px;text-transform:none;font-weight:750;letter-spacing:.01em}tbody tr:last-child td{border-bottom:0}tbody tr:hover{background:#fbfdff}.name-cell strong{display:block;font-size:13px}.name-cell span{display:block;margin-top:2px;color:var(--muted);font-size:11px}.mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace}.muted{color:var(--muted)}.subtle{color:var(--subtle)}
.badge{display:inline-flex;align-items:center;gap:5px;min-height:23px;padding:2px 8px;border-radius:999px;background:#f1f4f8;color:#53657c;font-size:11px;font-weight:700;white-space:nowrap}.badge.blue{background:var(--blue-soft);color:var(--blue)}.badge.red{background:var(--red-soft);color:var(--red)}.badge.amber{background:var(--amber-soft);color:var(--amber)}.badge.outline{background:#fff;border:1px solid var(--line)}
.actions{display:flex;align-items:center;gap:6px;white-space:nowrap}.menu{position:relative}.menu-pop{display:none;position:absolute;right:0;top:34px;z-index:10;width:150px;border:1px solid var(--line);border-radius:10px;background:#fff;box-shadow:0 12px 32px rgba(23,42,70,.14);padding:5px}.menu.open .menu-pop{display:grid}.menu-pop button{height:32px;border-radius:7px;background:transparent;color:#354963;text-align:left;padding:0 9px;cursor:pointer}.menu-pop button:hover{background:var(--soft)}.menu-pop button.danger-text{color:var(--red)}
.empty{padding:34px 20px;text-align:center;color:var(--muted)}
.routing-summary{border:1px solid var(--line);border-radius:var(--radius);padding:16px 18px;display:flex;align-items:center;justify-content:space-between;gap:18px;background:#fff}.summary-left{min-width:0}.summary-left strong{font-size:14px}.summary-left p{margin:5px 0 0;color:var(--muted);font-size:12px}.summary-chips{display:flex;gap:7px;flex-wrap:wrap;margin-top:9px}.summary-actions{display:flex;gap:8px;align-items:center;white-space:nowrap}
.overlay{position:fixed;inset:0;background:rgba(14,27,45,.36);display:none;align-items:center;justify-content:center;padding:20px;z-index:100}.overlay.show{display:flex}.modal{width:min(520px,100%);max-height:calc(100vh - 40px);overflow:auto;background:#fff;border-radius:14px;box-shadow:var(--shadow);border:1px solid rgba(255,255,255,.7)}.modal.wide{width:min(820px,100%)}.modal-head{padding:18px 20px 14px;border-bottom:1px solid var(--line);display:flex;align-items:flex-start;justify-content:space-between;gap:14px}.modal-head h3{margin:0;font-size:17px}.modal-head p{margin:4px 0 0;color:var(--muted);font-size:12px}.modal-body{padding:18px 20px;display:grid;gap:14px}.modal-foot{padding:13px 20px;border-top:1px solid var(--line);display:flex;justify-content:flex-end;gap:8px;background:#fbfcfe}.close{width:32px;height:32px;border-radius:8px;background:transparent;color:var(--muted);font-size:20px;cursor:pointer}.close:hover{background:var(--soft);color:var(--text)}
.drawer-overlay{position:fixed;inset:0;background:rgba(14,27,45,.28);display:none;z-index:90}.drawer-overlay.show{display:block}.drawer{position:absolute;right:0;top:0;height:100%;width:min(760px,92vw);background:#fff;box-shadow:-20px 0 55px rgba(20,44,80,.18);display:flex;flex-direction:column}.drawer-head{padding:18px 20px;border-bottom:1px solid var(--line);display:flex;align-items:flex-start;justify-content:space-between;gap:14px}.drawer-head h3{margin:0;font-size:17px}.drawer-head p{margin:4px 0 0;color:var(--muted);font-size:12px}.tabs{display:flex;gap:4px;padding:10px 20px 0;border-bottom:1px solid var(--line)}.tab{height:38px;padding:0 14px;background:transparent;color:var(--muted);font-weight:700;cursor:pointer;border-bottom:2px solid transparent}.tab.active{color:var(--blue);border-color:var(--blue)}.drawer-body{flex:1;overflow:auto;padding:18px 20px}.drawer-foot{padding:12px 20px;border-top:1px solid var(--line);display:flex;align-items:center;justify-content:space-between;gap:12px;background:#fbfcfe}.drawer-foot small{color:var(--muted)}
.rule-block{border:1px solid var(--line);border-radius:11px;margin-bottom:14px;overflow:hidden}.rule-head{padding:12px 14px;background:#fafbfd;display:flex;align-items:center;justify-content:space-between;gap:10px;border-bottom:1px solid var(--line)}.rule-head strong{font-size:13px}.rule-body{padding:12px 14px}.rule-row{display:grid;grid-template-columns:32px minmax(120px,.8fr) minmax(160px,1.2fr) 34px;gap:8px;align-items:center;margin-bottom:8px}.rule-row.regex{grid-template-columns:32px 120px minmax(180px,1fr) minmax(150px,.8fr) 34px}.rule-row:last-child{margin-bottom:0}.toggle{width:34px;height:20px;position:relative;display:inline-block}.toggle input{display:none}.toggle span{position:absolute;inset:0;border-radius:999px;background:#cdd6e2;cursor:pointer;transition:.15s}.toggle span:after{content:"";position:absolute;width:14px;height:14px;border-radius:50%;background:#fff;top:3px;left:3px;box-shadow:0 1px 3px rgba(0,0,0,.18);transition:.15s}.toggle input:checked+span{background:var(--blue)}.toggle input:checked+span:after{transform:translateX(14px)}
.auth-toolbar{display:flex;gap:8px;align-items:center;justify-content:space-between;flex-wrap:wrap;margin-bottom:10px}.auth-toolbar .input{min-width:250px}.auth-table{border:1px solid var(--line);border-radius:10px;overflow:auto}.auth-table table{min-width:700px}.route-note{font-size:11px;color:var(--muted)}
.auto-grid{display:grid;gap:12px}.setting{border:1px solid var(--line);border-radius:10px;padding:12px 13px}.setting-row{display:flex;align-items:center;justify-content:space-between;gap:16px}.setting strong{display:block;font-size:13px}.setting p{margin:3px 0 0;color:var(--muted);font-size:11px}.setting .field{margin-top:10px}
.toast-area{position:fixed;right:20px;bottom:20px;display:grid;gap:8px;z-index:150}.toast{width:min(340px,calc(100vw - 40px));padding:11px 13px;background:#17243a;color:#fff;border-radius:10px;box-shadow:var(--shadow)}.toast.error{background:#8e2f3b}.toast strong{display:block;font-size:12px}.toast p{margin:3px 0 0;font-size:11px;opacity:.82}.hidden{display:none!important}
@media(max-width:820px){.page{padding:18px 16px 36px}.header{align-items:center}.current{grid-template-columns:1fr}.current-switch{min-width:0;width:100%}.routing-summary{align-items:flex-start;flex-direction:column}.summary-actions{width:100%}.summary-actions .btn{flex:1}.rule-row,.rule-row.regex{grid-template-columns:28px 1fr 34px}.rule-row select:nth-of-type(2),.rule-row.regex .rule-target{grid-column:2}.rule-row.regex .rule-profile{grid-column:2}.drawer{width:100vw}.header-actions .btn span.label{display:none}}
</style>
</head>
<body>
<div class="page">
  <header class="header compact-header">
    <div class="title-row">
      <span class="status-chip"><span id="connectionDot" class="status-dot"></span><span id="connectionText">未连接</span></span>
      <span id="versionChip" class="badge outline">插件 -</span>
    </div>
    <div class="header-actions">
      <button class="btn secondary icon" data-action="refresh" title="刷新">↻</button>
      <button class="btn secondary" data-action="open-auto"><span class="label">自动切换</span></button>
      <button class="btn secondary" data-action="open-connect"><span class="label">连接设置</span></button>
    </div>
  </header>

  <div id="errorNotice" class="notice error"><span id="errorText"></span></div>

  <section class="current">
    <div class="current-main">
      <div class="eyebrow">当前全局出口</div>
      <div class="current-name"><strong id="currentName">未选择</strong><span id="currentHealth" class="badge outline">等待配置</span></div>
      <div id="currentIP" class="current-ip">-</div>
      <div id="currentIPAlt" class="current-ip-alt"></div>
      <div class="current-meta">
        <span id="currentLocation">国家 -</span>
        <span id="currentLatency">延迟 -</span>
        <span>中继 <code id="relayURL">-</code> <button class="btn soft small" data-action="copy-required">复制</button></span>
      </div>
    </div>
    <div class="current-switch">
      <div class="field grow"><label>切换到</label><select id="globalProfileSelect" class="select"></select></div>
      <button id="switchButton" class="btn" data-action="switch-global">切换</button>
    </div>
  </section>

  <section class="section">
    <div class="section-head">
      <div class="section-title"><h2>出口配置</h2><p>每个配置对应一个独立 SOCKS5 出口。</p></div>
      <div class="section-actions">
        <button class="btn secondary" data-action="check-all">检测全部</button>
        <button class="btn" data-action="open-create">新增出口</button>
      </div>
    </div>
    <div class="table-wrap">
      <div id="profilesEmpty" class="empty hidden">尚未创建出口配置。</div>
      <div id="profilesTableWrap" class="table-scroll">
        <table>
          <thead><tr><th>名称</th><th>状态</th><th>公网 IP</th><th>国家</th><th>延迟</th><th>本地代理</th><th style="width:190px">操作</th></tr></thead>
          <tbody id="profilesBody"></tbody>
        </table>
      </div>
    </div>
  </section>

  <section class="section">
    <div class="routing-summary">
      <div class="summary-left">
        <strong>认证文件分流</strong>
        <p>未设置例外时，认证文件继承上方的全局出口。</p>
        <div class="summary-chips">
          <span id="typeRuleCount" class="badge outline">类型规则 0</span>
          <span id="regexRuleCount" class="badge outline">正则规则 0</span>
          <span id="exactRuleCount" class="badge outline">单文件 0</span>
        </div>
      </div>
      <div class="summary-actions">
        <button class="btn secondary" data-action="apply-rules">应用到认证文件</button>
        <button class="btn" data-action="open-routing">管理分流</button>
      </div>
    </div>
  </section>
</div>

<div id="connectOverlay" class="overlay">
  <div class="modal">
    <div class="modal-head"><div><h3>连接 CLIProxyAPI</h3><p>请输入 remote-management.secret-key。</p></div><button class="close" data-action="close-connect">×</button></div>
    <div class="modal-body"><div class="field"><label>管理密钥</label><input id="managementKey" class="input" type="password" autocomplete="current-password" placeholder="输入管理密钥"></div><div class="notice show">密钥只保存在当前标签页，不会写入插件配置。</div></div>
    <div class="modal-foot"><button class="btn secondary" data-action="disconnect">清除密钥</button><button id="connectButton" class="btn" data-action="connect">连接</button></div>
  </div>
</div>

<div id="createOverlay" class="overlay">
  <div class="modal">
    <div class="modal-head"><div><h3>新增出口</h3><p>创建托管 WARP，或接入已有 SOCKS5。</p></div><button class="close" data-action="close-create">×</button></div>
    <div class="modal-body">
      <div class="field"><label>名称</label><input id="createName" class="input" placeholder="例如：新加坡 01"></div>
      <div class="field"><label>类型</label><select id="createMode" class="select"><option value="managed">托管 WARP</option><option value="external">外部 SOCKS5</option></select></div>
      <div id="externalField" class="field hidden"><label>SOCKS5 地址</label><input id="createProxy" class="input mono" placeholder="socks5://127.0.0.1:42000"></div>
      <label style="display:flex;align-items:center;gap:8px"><input id="createAutoStart" type="checkbox" checked> 创建后立即启动并检测</label>
    </div>
    <div class="modal-foot"><button class="btn secondary" data-action="close-create">取消</button><button id="createButton" class="btn" data-action="create-profile">创建</button></div>
  </div>
</div>

<div id="autoOverlay" class="overlay">
  <div class="modal">
    <div class="modal-head"><div><h3>自动切换</h3><p>低频设置默认收起，不占用主页面。</p></div><button class="close" data-action="close-auto">×</button></div>
    <div class="modal-body auto-grid">
      <div class="setting"><div class="setting-row"><div><strong>定时轮换</strong><p>按固定间隔更换全局出口。</p></div><label class="toggle"><input id="autoEnabled" type="checkbox"><span></span></label></div><div class="field"><label>间隔（分钟）</label><input id="autoInterval" class="input" type="number" min="0" step="1"></div></div>
      <div class="setting"><div class="setting-row"><div><strong>异常故障转移</strong><p>当前出口检测失败时自动选择其他健康出口。</p></div><label class="toggle"><input id="autoFailover" type="checkbox"><span></span></label></div></div>
      <div class="setting"><div class="setting-row"><div><strong>要求不同公网 IP</strong><p>跳过与当前出口公网 IP 相同的候选配置。</p></div><label class="toggle"><input id="autoDifferent" type="checkbox"><span></span></label></div></div>
    </div>
    <div class="modal-foot"><button class="btn secondary" data-action="run-auto">立即执行一次</button><button id="saveAutoButton" class="btn" data-action="save-auto">保存</button></div>
  </div>
</div>

<div id="confirmOverlay" class="overlay">
  <div class="modal">
    <div class="modal-head"><div><h3 id="confirmTitle">确认操作</h3><p id="confirmText"></p></div></div>
    <div class="modal-foot"><button class="btn secondary" data-action="confirm-cancel">取消</button><button id="confirmButton" class="btn danger" data-action="confirm-ok">确认</button></div>
  </div>
</div>

<div id="routingOverlay" class="drawer-overlay">
  <aside class="drawer">
    <div class="drawer-head"><div><h3>管理认证文件分流</h3><p>优先级：单文件 → 正则 → 类型 → 全局。</p></div><button class="close" data-action="close-routing">×</button></div>
    <div class="tabs"><button class="tab active" data-tab="rules">规则</button><button class="tab" data-tab="auth">认证文件</button></div>
    <div id="routingRules" class="drawer-body">
      <div class="rule-block">
        <div class="rule-head"><strong>默认出口</strong></div>
        <div class="rule-body"><div class="field"><label>未命中例外规则时使用</label><select id="ruleGlobalProfile" class="select"></select></div></div>
      </div>
      <div class="rule-block">
        <div class="rule-head"><strong>认证类型 / 服务商</strong><button class="btn secondary small" data-action="add-type-rule">添加</button></div>
        <div class="rule-body"><div id="typeRules"></div><div id="typeRulesEmpty" class="empty">尚无类型规则。</div></div>
      </div>
      <div class="rule-block">
        <div class="rule-head"><strong>正则表达式</strong><button class="btn secondary small" data-action="add-regex-rule">添加</button></div>
        <div class="rule-body"><div id="regexRules"></div><div id="regexRulesEmpty" class="empty">尚无正则规则。</div></div>
      </div>
    </div>
    <div id="routingAuth" class="drawer-body hidden">
      <div class="auth-toolbar"><input id="authSearch" class="input" placeholder="搜索文件、邮箱或标签"><select id="authProviderFilter" class="select"><option value="all">全部类型</option></select></div>
      <div class="auth-table"><table><thead><tr><th>认证文件</th><th>命中规则</th><th>单文件出口</th></tr></thead><tbody id="authBody"></tbody></table><div id="authEmpty" class="empty hidden">没有匹配的认证文件。</div></div>
    </div>
    <div class="drawer-foot"><small id="rulesDirtyText">没有未保存修改</small><div><button class="btn secondary" data-action="close-routing">关闭</button> <button id="saveRulesButton" class="btn" data-action="save-rules">保存规则</button></div></div>
  </aside>
</div>

<div id="toastArea" class="toast-area"></div>
<script>
(function(){
'use strict';
var state={status:null,profiles:[],rules:emptyRules(),savedRules:emptyRules(),auto:{enabled:false,failover_enabled:true,rotate_interval_seconds:0,require_different_ip:true},files:[],connected:false,confirmResolve:null,authLoaded:false,demo:new URLSearchParams(location.search).get('demo')==='1'};
function el(id){return document.getElementById(id)}
function emptyRules(){return {global_profile_id:'',type_rules:[],regex_rules:[],exact_rules:{}}}
function clone(v){return JSON.parse(JSON.stringify(v))}
function esc(v){return String(v==null?'':v).replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]})}
function key(){return sessionStorage.getItem('warp-egress-key')||''}
function profile(id){for(var i=0;i<state.profiles.length;i++){if(state.profiles[i].id===id)return state.profiles[i]}return null}
function normalizeRules(r){r=r||{};return {global_profile_id:r.global_profile_id||'',type_rules:Array.isArray(r.type_rules)?r.type_rules:[],regex_rules:Array.isArray(r.regex_rules)?r.regex_rules:[],exact_rules:r.exact_rules||{}}}
function setBusy(button,busy,label){if(!button)return;if(busy){button.dataset.old=button.innerHTML;button.disabled=true;button.innerHTML='<span class="spinner"></span>'+(label||'处理中')}else{button.disabled=false;if(button.dataset.old)button.innerHTML=button.dataset.old}}
function overlay(id,show){el(id).classList.toggle('show',show)}
function toast(title,message,type){var box=document.createElement('div');box.className='toast'+(type==='error'?' error':'');box.innerHTML='<strong>'+esc(title)+'</strong><p>'+esc(message||'')+'</p>';el('toastArea').appendChild(box);setTimeout(function(){box.remove()},3500)}
function showError(message){el('errorText').textContent=message||'';el('errorNotice').classList.toggle('show',!!message)}
function updateConnection(){el('connectionDot').classList.toggle('ok',state.connected);el('connectionText').textContent=state.connected?'已连接':'未连接';el('versionChip').textContent='插件 '+((state.status&&state.status.version)||'-')}
function options(selected,includeEmpty){var html=includeEmpty?'<option value="">继承全局</option>':'';state.profiles.forEach(function(p){html+='<option value="'+esc(p.id)+'"'+(p.id===selected?' selected':'')+'>'+esc(p.name)+(p.healthy?'':'（异常）')+'</option>'});return html}
function healthBadge(p){if(!p)return '<span class="badge outline">未选择</span>';return p.healthy?'<span class="badge blue">健康</span>':'<span class="badge red">异常</span>'}
function routeLabel(type){return {exact:'单文件',regex:'正则',type:'类型',global:'全局',inherit:'继承全局',none:'未分配'}[type]||type||'未分配'}
function formatTime(value){if(!value||String(value).indexOf('0001-01-01')===0)return '从未';var d=new Date(value);return isNaN(d.getTime())?'从未':d.toLocaleString('zh-CN',{hour12:false})}
async function api(path,method,body){
  if(state.demo)return demoAPI(path,method,body);
  if(!key())throw new Error('请先输入管理密钥');
  var headers={'Authorization':'Bearer '+key()};if(body!==undefined)headers['Content-Type']='application/json';
  var response=await fetch('/v0/management/warp-egress'+path,{method:method||'GET',headers:headers,body:body===undefined?undefined:JSON.stringify(body)});
  var text=await response.text();var data;try{data=JSON.parse(text)}catch(e){data={error:text||('HTTP '+response.status)}}
  if(response.status===401||response.status===403){state.connected=false;updateConnection();overlay('connectOverlay',true)}
  if(!response.ok)throw new Error(data.error||('HTTP '+response.status));state.connected=true;return data
}
async function loadAll(){
  showError('');
  var result=await Promise.all([api('/status'),api('/rules'),api('/auto')]);
  state.status=result[0];state.profiles=(state.status&&state.status.profiles)||[];state.rules=normalizeRules(result[1]);state.savedRules=clone(state.rules);state.auto=result[2]||state.auto;state.connected=true;renderAll();
}
async function loadAuth(){var result=await api('/auth-files');state.files=(result&&result.files)||[];state.authLoaded=true;renderAuth()}
function renderAll(){updateConnection();renderCurrent();renderProfiles();renderSummary();renderRules();renderAuto()}
function renderCurrent(){
  var s=state.status||{};var p=s.global_profile||profile(s.global_profile_id);
  el('currentName').textContent=p?p.name:'未选择';var mainIp=p?(p.exit_ip||''):'';var v4=p?(p.exit_ip_v4||(mainIp&&mainIp.indexOf(':')<0?mainIp:'')):'';var v6=p?(p.exit_ip_v6||(mainIp&&mainIp.indexOf(':')>=0?mainIp:'')):'';el('currentIP').textContent=v4?('IPv4 '+v4):(v6?('IPv6 '+v6):'等待检测');el('currentIPAlt').textContent=(v6&&v4)?('IPv6 '+v6):'';el('currentHealth').outerHTML=healthBadge(p).replace('<span','<span id="currentHealth"');el('currentLocation').textContent='国家 '+coloName(p&&p.colo?p.colo:'-');el('currentLatency').textContent='延迟 '+(p&&p.latency_ms?p.latency_ms+' ms':'-');el('relayURL').textContent=s.global_relay_url||'-';
  el('globalProfileSelect').innerHTML=options(s.global_profile_id,false);el('switchButton').disabled=!state.profiles.length||el('globalProfileSelect').value===s.global_profile_id;
}
function renderProfiles(){
  var body=el('profilesBody');var empty=!state.profiles.length;el('profilesEmpty').classList.toggle('hidden',!empty);el('profilesTableWrap').classList.toggle('hidden',empty);
  var duplicates=(state.status&&state.status.duplicate_exit_ips)||{};
  body.innerHTML=state.profiles.map(function(p){
    var current=state.status&&p.id===state.status.global_profile_id;var duplicate=!!(p.exit_ip&&duplicates[p.exit_ip]&&duplicates[p.exit_ip].length>1);var mode=p.mode==='managed'?'托管 WARP':'外部 SOCKS5';
    return '<tr><td class="name-cell"><strong>'+esc(p.name)+(current?' <span class="badge blue">当前</span>':'')+'</strong><span>'+esc(mode)+'</span></td><td>'+healthBadge(p)+'</td><td class="mono">'+esc(p.exit_ip_v4||(p.exit_ip&&p.exit_ip.indexOf(':')<0?p.exit_ip:'')||'-')+(p.exit_ip_v6||(p.exit_ip&&p.exit_ip.indexOf(':')>=0?p.exit_ip:'')?'<span class="muted"> / '+esc(p.exit_ip_v6||(p.exit_ip&&p.exit_ip.indexOf(':')>=0?p.exit_ip:''))+'</span>':'')+(duplicate?' <span class="badge amber">重复</span>':'')+'</td><td>'+esc(coloName(p.colo)||'-')+'</td><td>'+((p.latency_ms!=null)?p.latency_ms+' ms':'-')+'</td><td class="mono muted">'+esc(p.proxy_url||'-')+'</td><td><div class="actions">'+(current?'':'<button class="btn soft small" data-action="set-global" data-id="'+esc(p.id)+'">设为全局</button>')+'<button class="btn secondary small" data-action="profile-check" data-id="'+esc(p.id)+'">检测</button><div class="menu"><button class="btn secondary small" data-action="toggle-menu">更多</button><div class="menu-pop">'+(p.mode==='managed'?'<button data-action="profile-'+(p.running?'stop':'start')+'" data-id="'+esc(p.id)+'">'+(p.running?'停止':'启动')+'</button><button data-action="profile-recreate" data-id="'+esc(p.id)+'">重新注册</button>':'')+'<button class="danger-text" data-action="profile-delete" data-id="'+esc(p.id)+'">删除</button></div></div></div></td></tr>'
  }).join('')
}
function renderSummary(){el('typeRuleCount').textContent='类型规则 '+state.rules.type_rules.length;el('regexRuleCount').textContent='正则规则 '+state.rules.regex_rules.length;el('exactRuleCount').textContent='单文件 '+Object.keys(state.rules.exact_rules||{}).length}
function renderRules(){
  el('ruleGlobalProfile').innerHTML=options(state.rules.global_profile_id,false);
  el('typeRulesEmpty').classList.toggle('hidden',state.rules.type_rules.length>0);
  el('typeRules').innerHTML=state.rules.type_rules.map(function(r,i){return '<div class="rule-row"><label class="toggle"><input type="checkbox" data-kind="type" data-index="'+i+'" data-field="enabled" '+(r.enabled?'checked':'')+'><span></span></label><input class="input" data-kind="type" data-index="'+i+'" data-field="key" value="'+esc(r.key)+'" placeholder="codex"><select class="select" data-kind="type" data-index="'+i+'" data-field="profile_id">'+options(r.profile_id,false)+'</select><button class="btn danger small icon" data-action="remove-type-rule" data-index="'+i+'">×</button></div>'}).join('');
  el('regexRulesEmpty').classList.toggle('hidden',state.rules.regex_rules.length>0);
  el('regexRules').innerHTML=state.rules.regex_rules.map(function(r,i){return '<div class="rule-row regex"><label class="toggle"><input type="checkbox" data-kind="regex" data-index="'+i+'" data-field="enabled" '+(r.enabled?'checked':'')+'><span></span></label><select class="select rule-target" data-kind="regex" data-index="'+i+'" data-field="target"><option value="all" '+(r.target==='all'?'selected':'')+'>全部字段</option><option value="name" '+(r.target==='name'?'selected':'')+'>文件名</option><option value="email" '+(r.target==='email'?'selected':'')+'>邮箱</option><option value="label" '+(r.target==='label'?'selected':'')+'>标签</option><option value="provider" '+(r.target==='provider'?'selected':'')+'>服务商</option><option value="type" '+(r.target==='type'?'selected':'')+'>类型</option></select><input class="input mono" data-kind="regex" data-index="'+i+'" data-field="pattern" value="'+esc(r.pattern)+'" placeholder="@example\\.com$"><select class="select rule-profile" data-kind="regex" data-index="'+i+'" data-field="profile_id">'+options(r.profile_id,false)+'</select><button class="btn danger small icon" data-action="remove-regex-rule" data-index="'+i+'">×</button></div>'}).join('');
  updateDirty()
}
function updateDirty(){var dirty=JSON.stringify(state.rules)!==JSON.stringify(state.savedRules);el('rulesDirtyText').textContent=dirty?'有未保存修改':'没有未保存修改';el('saveRulesButton').disabled=!dirty}
function renderAuth(){
  var q=(el('authSearch').value||'').toLowerCase();var providerValue=el('authProviderFilter').value||'all';var providers={};state.files.forEach(function(f){var v=f.provider||f.type||'unknown';providers[v]=true});var current=providerValue;el('authProviderFilter').innerHTML='<option value="all">全部类型</option>'+Object.keys(providers).sort().map(function(v){return '<option value="'+esc(v)+'"'+(v===current?' selected':'')+'>'+esc(v)+'</option>'}).join('');
  var list=state.files.filter(function(f){var hay=[f.name,f.email,f.label,f.provider,f.type,f.auth_index].join(' ').toLowerCase();var pv=f.provider||f.type||'unknown';return (!q||hay.indexOf(q)>=0)&&(providerValue==='all'||pv===providerValue)});
  el('authEmpty').classList.toggle('hidden',list.length>0);el('authBody').innerHTML=list.map(function(f){var effective=f.effective||{};var disabled=f.runtime_only?' disabled':'';return '<tr><td class="name-cell"><strong>'+esc(f.name||f.auth_index)+'</strong><span>'+esc(f.email||f.label||f.provider||'')+(f.runtime_only?' · 运行时只读':'')+'</span></td><td><span class="badge outline">'+esc(routeLabel(effective.rule_type))+'</span><div class="route-note">'+esc(effective.rule_key||'')+'</div></td><td><select class="select exact-select" data-auth="'+esc(f.auth_index)+'"'+disabled+'>'+options((state.rules.exact_rules||{})[f.auth_index]||'',true)+'</select></td></tr>'}).join('')
}
function renderAuto(){var a=state.auto||{};el('autoEnabled').checked=!!a.enabled;el('autoFailover').checked=!!a.failover_enabled;el('autoDifferent').checked=!!a.require_different_ip;el('autoInterval').value=Math.round(Number(a.rotate_interval_seconds||0)/60)}
async function connect(){var value=el('managementKey').value.trim();if(!value&&!state.demo){toast('缺少密钥','请输入管理密钥','error');return}if(!state.demo)sessionStorage.setItem('warp-egress-key',value);setBusy(el('connectButton'),true,'连接中');try{await loadAll();overlay('connectOverlay',false);toast('连接成功','插件状态已加载')}catch(e){state.connected=false;updateConnection();showError(e.message);toast('连接失败',e.message,'error')}finally{setBusy(el('connectButton'),false)}}
function disconnect(){if(!state.demo)sessionStorage.removeItem('warp-egress-key');state.connected=false;updateConnection();overlay('connectOverlay',true)}
async function refresh(){try{await loadAll();toast('已刷新','出口状态已更新')}catch(e){showError(e.message);toast('刷新失败',e.message,'error')}}
async function switchGlobal(id){var p=profile(id);if(!p)return;setBusy(el('switchButton'),true,'切换中');try{await api('/global/switch','POST',{profile_id:id});await loadAll();toast('已切换',p.name)}catch(e){toast('切换失败',e.message,'error')}finally{setBusy(el('switchButton'),false)}}
async function createProfile(){var name=el('createName').value.trim();var mode=el('createMode').value;var proxy=el('createProxy').value.trim();if(!name){toast('缺少名称','请输入出口名称','error');return}if(mode==='external'&&!proxy){toast('缺少代理地址','请输入 SOCKS5 地址','error');return}setBusy(el('createButton'),true,'创建中');try{await api('/profiles/create','POST',{name:name,mode:mode,proxy_url:proxy,auto_start:el('createAutoStart').checked});overlay('createOverlay',false);el('createName').value='';el('createProxy').value='';await loadAll();toast('出口已创建',name)}catch(e){toast('创建失败',e.message,'error')}finally{setBusy(el('createButton'),false)}}
async function profileAction(id,action){var p=profile(id);if(!p)return;if(action==='recreate'){var ok=await confirmBox('重新注册出口','将重新生成 '+p.name+' 的 WARP 注册信息，公网 IP 可能变化。','重新注册');if(!ok)return}try{await api('/profiles/action','POST',{id:id,action:action});await loadAll();toast('操作完成',p.name)}catch(e){toast('操作失败',e.message,'error')}}
async function deleteProfile(id){var p=profile(id);if(!p)return;var ok=await confirmBox('删除出口','将删除 '+p.name+' 及引用它的规则。','删除');if(!ok)return;try{await api('/profiles/delete','POST',{id:id});await loadAll();toast('已删除',p.name)}catch(e){toast('删除失败',e.message,'error')}}
async function checkAll(button){if(!state.profiles.length){toast('没有出口','请先新增出口','error');return}setBusy(button,true,'检测中');var failed=0;for(var i=0;i<state.profiles.length;i++){try{await api('/profiles/action','POST',{id:state.profiles[i].id,action:'check'})}catch(e){failed++}}await loadAll();setBusy(button,false);toast('检测完成',failed?'失败 '+failed+' 个':'全部正常',failed?'error':undefined)}
async function saveRules(){setBusy(el('saveRulesButton'),true,'保存中');try{state.rules.global_profile_id=el('ruleGlobalProfile').value;var saved=await api('/rules/save','POST',state.rules);state.rules=normalizeRules(saved);state.savedRules=clone(state.rules);renderAll();toast('规则已保存','尚未写入认证文件')}catch(e){toast('保存失败',e.message,'error')}finally{setBusy(el('saveRulesButton'),false)}}
async function applyRules(){try{var result=await api('/rules/apply','POST',{});toast('应用完成','修改 '+(result.changed||0)+'，失败 '+(result.failed||0),result.failed?'error':undefined);if(state.authLoaded)await loadAuth()}catch(e){toast('应用失败',e.message,'error')}}
async function assignExact(authIndex,profileID,select){if(select)select.disabled=true;try{await api('/auth-files/assign','POST',{auth_index:authIndex,profile_id:profileID,apply_now:true});state.rules.exact_rules[authIndex]=profileID;if(!profileID)delete state.rules.exact_rules[authIndex];state.savedRules=clone(state.rules);renderSummary();await loadAuth();toast('单文件出口已更新','已立即写入认证文件')}catch(e){toast('设置失败',e.message,'error');await loadAuth()}finally{if(select)select.disabled=false}}
async function saveAuto(){setBusy(el('saveAutoButton'),true,'保存中');try{state.auto=await api('/auto/save','POST',{enabled:el('autoEnabled').checked,failover_enabled:el('autoFailover').checked,rotate_interval_seconds:Math.max(0,Number(el('autoInterval').value||0))*60,require_different_ip:el('autoDifferent').checked});renderAuto();overlay('autoOverlay',false);toast('自动切换已保存','设置已生效')}catch(e){toast('保存失败',e.message,'error')}finally{setBusy(el('saveAutoButton'),false)}}
async function runAuto(){try{var result=await api('/auto/run','POST',{});await loadAll();toast('执行完成',result.profile?('已切换到 '+result.profile.name):'当前无需切换')}catch(e){toast('执行失败',e.message,'error')}}
function confirmBox(title,text,label){el('confirmTitle').textContent=title;el('confirmText').textContent=text;el('confirmButton').textContent=label||'确认';overlay('confirmOverlay',true);return new Promise(function(resolve){state.confirmResolve=resolve})}
function resolveConfirm(value){overlay('confirmOverlay',false);if(state.confirmResolve){state.confirmResolve(value);state.confirmResolve=null}}
function copy(text){if(!text)return;navigator.clipboard.writeText(text).then(function(){toast('已复制',text)}).catch(function(){var ta=document.createElement('textarea');ta.value=text;document.body.appendChild(ta);ta.select();document.execCommand('copy');ta.remove();toast('已复制',text)})}
function openRouting(){renderRules();overlay('routingOverlay',true)}
function closeRouting(){if(JSON.stringify(state.rules)!==JSON.stringify(state.savedRules)){confirmBox('放弃未保存修改','关闭后将恢复到上次保存的规则。','放弃修改').then(function(ok){if(ok){state.rules=clone(state.savedRules);renderRules();overlay('routingOverlay',false)}})}else{overlay('routingOverlay',false)}}
function switchTab(name){document.querySelectorAll('.tab').forEach(function(t){t.classList.toggle('active',t.dataset.tab===name)});el('routingRules').classList.toggle('hidden',name!=='rules');el('routingAuth').classList.toggle('hidden',name!=='auth');if(name==='auth'&&!state.authLoaded){loadAuth().catch(function(e){toast('读取失败',e.message,'error')})}else if(name==='auth')renderAuth()}
function handle(action,node){var id=node.dataset.id;switch(action){case'refresh':refresh();break;case'open-connect':el('managementKey').value=key();overlay('connectOverlay',true);break;case'close-connect':overlay('connectOverlay',false);break;case'connect':connect();break;case'disconnect':disconnect();break;case'copy-required':copy((state.status&&state.status.required_host_proxy_url)||'');break;case'switch-global':switchGlobal(el('globalProfileSelect').value);break;case'set-global':switchGlobal(id);break;case'open-create':overlay('createOverlay',true);break;case'close-create':overlay('createOverlay',false);break;case'create-profile':createProfile();break;case'profile-check':profileAction(id,'check');break;case'profile-start':profileAction(id,'start');break;case'profile-stop':profileAction(id,'stop');break;case'profile-recreate':profileAction(id,'recreate');break;case'profile-delete':deleteProfile(id);break;case'toggle-menu':document.querySelectorAll('.menu.open').forEach(function(m){if(m!==node.parentElement)m.classList.remove('open')});node.parentElement.classList.toggle('open');break;case'check-all':checkAll(node);break;case'open-routing':openRouting();break;case'close-routing':closeRouting();break;case'add-type-rule':if(!state.profiles.length){toast('没有出口','请先新增出口','error');break}state.rules.type_rules.push({key:'',profile_id:state.profiles[0].id,enabled:true});renderRules();break;case'remove-type-rule':state.rules.type_rules.splice(Number(node.dataset.index),1);renderRules();break;case'add-regex-rule':if(!state.profiles.length){toast('没有出口','请先新增出口','error');break}state.rules.regex_rules.push({id:'rule-'+Date.now(),pattern:'',target:'all',profile_id:state.profiles[0].id,enabled:true});renderRules();break;case'remove-regex-rule':state.rules.regex_rules.splice(Number(node.dataset.index),1);renderRules();break;case'save-rules':saveRules();break;case'apply-rules':applyRules();break;case'open-auto':renderAuto();overlay('autoOverlay',true);break;case'close-auto':overlay('autoOverlay',false);break;case'save-auto':saveAuto();break;case'run-auto':runAuto();break;case'confirm-ok':resolveConfirm(true);break;case'confirm-cancel':resolveConfirm(false);break}}
document.addEventListener('click',function(e){var tab=e.target.closest('[data-tab]');if(tab){switchTab(tab.dataset.tab);return}var action=e.target.closest('[data-action]');if(action){handle(action.dataset.action,action);return}if(!e.target.closest('.menu'))document.querySelectorAll('.menu.open').forEach(function(m){m.classList.remove('open')})});
document.addEventListener('change',function(e){if(e.target.id==='globalProfileSelect')el('switchButton').disabled=e.target.value===(state.status&&state.status.global_profile_id);if(e.target.id==='createMode')el('externalField').classList.toggle('hidden',e.target.value!=='external');if(e.target.id==='ruleGlobalProfile'){state.rules.global_profile_id=e.target.value;updateDirty()}if(e.target.dataset.kind){var arr=e.target.dataset.kind==='type'?state.rules.type_rules:state.rules.regex_rules;var row=arr[Number(e.target.dataset.index)];if(row){row[e.target.dataset.field]=e.target.type==='checkbox'?e.target.checked:e.target.value;updateDirty()}}if(e.target.classList.contains('exact-select'))assignExact(e.target.dataset.auth,e.target.value,e.target);if(e.target.id==='authProviderFilter')renderAuth()});
document.addEventListener('input',function(e){if(e.target.id==='authSearch')renderAuth();if(e.target.dataset.kind){var arr=e.target.dataset.kind==='type'?state.rules.type_rules:state.rules.regex_rules;var row=arr[Number(e.target.dataset.index)];if(row){row[e.target.dataset.field]=e.target.value;updateDirty()}}});
document.addEventListener('keydown',function(e){if(e.key==='Escape'){if(el('confirmOverlay').classList.contains('show'))resolveConfirm(false);else if(el('routingOverlay').classList.contains('show'))closeRouting();else document.querySelectorAll('.overlay.show').forEach(function(o){o.classList.remove('show')})}if(e.key==='Enter'&&el('connectOverlay').classList.contains('show'))connect()});
function coloName(c){var m={SIN:'新加坡',NRT:'日本',KIX:'日本',HKG:'中国香港',TPE:'中国台湾',ICN:'韩国',BKK:'泰国',KUL:'马来西亚',SGN:'越南',MNL:'菲律宾',CGK:'印度尼西亚',LAX:'美国',SJC:'美国',SEA:'美国',PDX:'美国',SFO:'美国',DFW:'美国',AUS:'美国',DEN:'美国',ORD:'美国',MIA:'美国',ATL:'美国',IAD:'美国',EWR:'美国',BOS:'美国',PHX:'美国',LAS:'美国',MSP:'美国',YVR:'加拿大',YYZ:'加拿大',YUL:'加拿大',GRU:'巴西',EZE:'阿根廷',SCL:'智利',LIM:'秘鲁',BOG:'哥伦比亚',MEX:'墨西哥',AMS:'荷兰',FRA:'德国',LHR:'英国',MAN:'英国',CDG:'法国',PAR:'法国',MAD:'西班牙',BCN:'西班牙',MXP:'意大利',ZRH:'瑞士',ARN:'瑞典',OSL:'挪威',CPH:'丹麦',HEL:'芬兰',WAW:'波兰',PRG:'捷克',VIE:'奥地利',BUD:'匈牙利',ATH:'希腊',IST:'土耳其',SOF:'保加利亚',DXB:'阿联酋',TLV:'以色列',JNB:'南非',CPT:'南非',LOS:'尼日利亚',NBO:'肯尼亚',CAI:'埃及',BOM:'印度',DEL:'印度',BLR:'印度',MAA:'印度',KHI:'巴基斯坦',DAC:'孟加拉',CMB:'斯里兰卡',SYD:'澳大利亚',MEL:'澳大利亚',BNE:'澳大利亚',PER:'澳大利亚',AKL:'新西兰'};return m[c]||c;}
function demoSeed(){state.status={version:'0.3.0',global_relay_url:'socks5://127.0.0.1:40000',required_host_proxy_url:'socks5://127.0.0.1:40000',global_profile_id:'sin-main',global_relay_running:true,duplicate_exit_ips:{},profiles:[{id:'sin-main',name:'新加坡主出口',mode:'managed',proxy_url:'socks5://127.0.0.1:41000',running:true,healthy:true,exit_ip:'2a09:bac1:6540:8::',exit_ip_v4:'104.28.210.10',exit_ip_v6:'2a09:bac1:6540:8::',colo:'SIN',latency_ms:28,last_checked:new Date().toISOString()},{id:'nrt-backup',name:'东京备用出口',mode:'managed',proxy_url:'socks5://127.0.0.1:41001',running:true,healthy:true,exit_ip:'104.28.210.12',exit_ip_v4:'104.28.210.12',exit_ip_v6:'2a09:bac0:1:2::',colo:'NRT',latency_ms:61,last_checked:new Date().toISOString()}]};state.status.global_profile=state.status.profiles[0];state.profiles=state.status.profiles;state.rules={global_profile_id:'sin-main',type_rules:[{key:'codex',profile_id:'nrt-backup',enabled:true}],regex_rules:[],exact_rules:{}};state.savedRules=clone(state.rules);state.auto={enabled:false,failover_enabled:true,rotate_interval_seconds:0,require_different_ip:true};state.connected=true;renderAll()}
function demoAPI(path,method,body){return new Promise(function(resolve){setTimeout(function(){if(path==='/status')resolve(state.status);else if(path==='/rules')resolve(state.rules);else if(path==='/auto')resolve(state.auto);else if(path==='/auth-files')resolve({files:[{auth_index:'codex-a.json',name:'codex-a.json',provider:'codex',email:'a@example.com',runtime_only:false,effective:{rule_type:'type',profile_id:'nrt-backup',rule_key:'codex'}},{auth_index:'claude-b.json',name:'claude-b.json',provider:'claude',email:'b@example.com',runtime_only:false,effective:{rule_type:'global',profile_id:'sin-main'}}]});else if(path==='/global/switch'){state.status.global_profile_id=body.profile_id;state.status.global_profile=profile(body.profile_id);resolve(state.status)}else if(path==='/rules/save'){state.rules=clone(body);resolve(state.rules)}else if(path==='/auto/save'){state.auto=clone(body);resolve(state.auto)}else resolve({status:'ok',changed:2,failed:0})},180)})}
if(state.demo){demoSeed()}else{el('managementKey').value=key();updateConnection();if(key()){loadAll().catch(function(e){showError(e.message);overlay('connectOverlay',true)})}else{overlay('connectOverlay',true)}}
})();
</script>
</body>
</html>
`
