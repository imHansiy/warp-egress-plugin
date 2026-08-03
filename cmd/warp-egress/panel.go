package main

const panelHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light">
<title>WARP 出口路由</title>
<style>
:root{
  color-scheme:light;
  --bg:#f4f7fb;--surface:#fff;--surface-soft:#f8faff;--surface-blue:#eef5ff;
  --line:#dce5f1;--line-strong:#c9d7e8;--text:#152238;--muted:#687890;--subtle:#91a0b5;
  --blue:#176ee8;--blue-hover:#0f5dcb;--blue-soft:#eaf3ff;--blue-deep:#0b3d87;
  --red:#c73d4d;--red-soft:#fff0f2;--amber:#946000;--amber-soft:#fff7e3;
  --shadow:0 12px 36px rgba(42,72,111,.08);--shadow-float:0 20px 60px rgba(29,55,91,.18);
  --radius:16px;--radius-sm:10px;--sidebar:252px;
}
*{box-sizing:border-box}
html{scroll-behavior:smooth}
body{margin:0;background:var(--bg);color:var(--text);font:14px/1.55 Inter,ui-sans-serif,system-ui,-apple-system,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif;-webkit-font-smoothing:antialiased}
button,input,select,textarea{font:inherit}
button{border:0}
button:focus-visible,input:focus-visible,select:focus-visible,textarea:focus-visible{outline:3px solid rgba(23,110,232,.18);outline-offset:1px}
.shell{min-height:100vh;display:grid;grid-template-columns:var(--sidebar) minmax(0,1fr)}
.sidebar{position:sticky;top:0;height:100vh;background:#fff;border-right:1px solid var(--line);padding:20px 14px;display:flex;flex-direction:column;z-index:20}
.brand{display:flex;align-items:center;gap:11px;padding:4px 10px 20px}
.brand-mark{width:38px;height:38px;border-radius:12px;background:linear-gradient(145deg,#2381f3,#0f5fcf);display:grid;place-items:center;color:#fff;font-weight:800;box-shadow:0 10px 22px rgba(23,110,232,.24)}
.brand strong{display:block;font-size:15px}.brand small{display:block;color:var(--muted);font-size:11px;margin-top:1px}
.nav-label{padding:8px 12px;color:var(--subtle);font-size:11px;font-weight:700;letter-spacing:.08em}
.nav{display:grid;gap:5px}
.nav button{width:100%;display:flex;align-items:center;gap:10px;padding:10px 12px;border-radius:10px;background:transparent;color:#41516a;cursor:pointer;text-align:left;font-weight:650}
.nav button:hover{background:#f3f7fd;color:var(--blue-deep)}
.nav button.active{background:var(--blue-soft);color:var(--blue);box-shadow:inset 3px 0 0 var(--blue)}
.nav-icon{width:20px;text-align:center;color:currentColor;font-size:12px;font-weight:800}
.sidebar-bottom{margin-top:auto;padding:12px 8px 2px}
.connection-card{border:1px solid var(--line);background:var(--surface-soft);border-radius:12px;padding:11px}
.connection-row{display:flex;align-items:center;gap:8px}.dot{width:8px;height:8px;border-radius:50%;background:#9aa8ba;box-shadow:0 0 0 4px rgba(145,160,181,.12)}.dot.connected{background:var(--blue);box-shadow:0 0 0 4px rgba(23,110,232,.12)}
.connection-card strong{font-size:12px}.connection-card p{margin:5px 0 0;color:var(--muted);font-size:11px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.main{min-width:0}
.topbar{height:76px;position:sticky;top:0;z-index:15;background:rgba(244,247,251,.9);backdrop-filter:blur(14px);border-bottom:1px solid rgba(220,229,241,.8);display:flex;align-items:center;justify-content:space-between;gap:16px;padding:0 26px}
.page-title h1{font-size:20px;margin:0;letter-spacing:-.02em}.page-title p{margin:2px 0 0;color:var(--muted);font-size:12px}
.top-actions{display:flex;align-items:center;gap:8px}
.content{max-width:1500px;margin:0 auto;padding:24px 26px 44px}
.view{display:none}.view.active{display:block;animation:fade .18s ease-out}@keyframes fade{from{opacity:.4;transform:translateY(3px)}to{opacity:1;transform:none}}
.btn{height:36px;padding:0 13px;border-radius:9px;background:var(--blue);color:#fff;cursor:pointer;font-weight:700;display:inline-flex;align-items:center;justify-content:center;gap:7px;white-space:nowrap;transition:.15s ease}
.btn:hover{background:var(--blue-hover);transform:translateY(-1px)}.btn:active{transform:none}.btn:disabled{opacity:.5;cursor:not-allowed;transform:none}
.btn.secondary{background:#fff;color:#36506f;border:1px solid var(--line-strong)}.btn.secondary:hover{background:#f7faff;color:var(--blue);border-color:#a9c6ed}
.btn.soft{background:var(--blue-soft);color:var(--blue)}.btn.soft:hover{background:#dcecff}
.btn.danger{background:#fff;color:var(--red);border:1px solid #efc8ce}.btn.danger:hover{background:var(--red-soft);border-color:#e8aab4}
.btn.ghost{background:transparent;color:var(--muted)}.btn.ghost:hover{background:#edf3fb;color:var(--text)}
.btn.small{height:30px;padding:0 9px;border-radius:8px;font-size:12px}.btn.icon{width:36px;padding:0}
.spinner{width:14px;height:14px;border:2px solid currentColor;border-right-color:transparent;border-radius:50%;display:inline-block;animation:spin .65s linear infinite}@keyframes spin{to{transform:rotate(360deg)}}
.card{background:var(--surface);border:1px solid var(--line);border-radius:var(--radius);box-shadow:var(--shadow)}
.card-head{display:flex;align-items:flex-start;justify-content:space-between;gap:16px;padding:18px 20px;border-bottom:1px solid var(--line)}
.card-head h2{font-size:16px;margin:0}.card-head p{margin:3px 0 0;color:var(--muted);font-size:12px}.card-body{padding:20px}
.metrics{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:14px;margin-bottom:18px}
.metric{padding:17px;border-radius:14px;border:1px solid var(--line);background:linear-gradient(180deg,#fff,#fbfdff);min-width:0}
.metric-label{color:var(--muted);font-size:12px;display:flex;align-items:center;justify-content:space-between;gap:8px}.metric-value{font-size:20px;font-weight:800;margin-top:9px;letter-spacing:-.03em;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.metric-meta{color:var(--subtle);font-size:11px;margin-top:4px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.overview-grid{display:grid;grid-template-columns:minmax(0,1.35fr) minmax(320px,.65fr);gap:16px}.stack{display:grid;gap:16px}
.topology{display:grid;grid-template-columns:1fr 36px 1fr 36px 1fr;align-items:center;gap:4px}.node{border:1px solid var(--line);background:var(--surface-soft);border-radius:13px;padding:14px;min-width:0}.node.active{border-color:#9fc5f5;background:var(--surface-blue)}.node span{display:block;color:var(--muted);font-size:11px}.node strong{display:block;margin-top:5px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.node small{display:block;color:var(--subtle);margin-top:2px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.arrow{text-align:center;color:#9aabc0;font-weight:800}
.quick-grid{display:grid;grid-template-columns:1fr 1fr;gap:10px}.quick{border:1px solid var(--line);border-radius:12px;background:#fff;padding:13px;cursor:pointer;text-align:left;color:var(--text)}.quick:hover{border-color:#9fc5f5;background:#f8fbff}.quick strong{display:block;font-size:13px}.quick span{display:block;color:var(--muted);font-size:11px;margin-top:2px}
.banner{border:1px solid #f0d59a;background:var(--amber-soft);color:#684700;border-radius:12px;padding:12px 14px;display:flex;align-items:flex-start;justify-content:space-between;gap:14px;margin-bottom:16px}.banner.blue{border-color:#bfd8f7;background:#eef6ff;color:#174a83}.banner.danger{border-color:#efc8ce;background:var(--red-soft);color:#8b2935}.banner strong{display:block;font-size:12px}.banner p{margin:3px 0 0;font-size:12px;opacity:.86}.banner.hidden{display:none}
.toolbar{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;margin-bottom:14px}.toolbar-group{display:flex;align-items:center;gap:8px;flex-wrap:wrap}.input,.select,.textarea{border:1px solid var(--line-strong);background:#fff;color:var(--text);border-radius:9px;min-height:36px;padding:7px 10px;transition:.15s}.input:hover,.select:hover,.textarea:hover{border-color:#b3c8e1}.input:focus,.select:focus,.textarea:focus{border-color:#7eace7;box-shadow:0 0 0 3px rgba(23,110,232,.09);outline:none}.textarea{min-height:86px;resize:vertical}.search{min-width:240px}.select{cursor:pointer}.field{display:grid;gap:6px}.field label{font-size:12px;font-weight:700;color:#43536a}.field-hint{font-size:11px;color:var(--muted)}
.profile-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:14px}.profile-card{border:1px solid var(--line);border-radius:14px;background:#fff;overflow:hidden;transition:.16s}.profile-card:hover{border-color:#b7cee9;box-shadow:0 12px 26px rgba(44,75,114,.08);transform:translateY(-1px)}.profile-card.current{border-color:#80afea;box-shadow:0 0 0 3px rgba(23,110,232,.08)}
.profile-top{padding:15px 15px 12px;display:flex;align-items:flex-start;justify-content:space-between;gap:10px}.profile-name{min-width:0}.profile-name strong{font-size:14px;display:block;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.profile-name small{display:block;color:var(--muted);margin-top:2px}.profile-body{padding:0 15px 14px}.profile-ip{font-size:17px;font-weight:800;letter-spacing:-.02em;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.profile-meta{display:grid;grid-template-columns:1fr 1fr;gap:8px;margin-top:12px}.meta-box{border:1px solid var(--line);border-radius:9px;padding:8px;background:var(--surface-soft);min-width:0}.meta-box span{display:block;color:var(--subtle);font-size:10px}.meta-box b{display:block;font-size:12px;margin-top:2px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.profile-error{margin-top:10px;color:var(--red);font-size:11px;word-break:break-word}.profile-actions{border-top:1px solid var(--line);background:#fbfcfe;padding:10px 12px;display:flex;gap:6px;flex-wrap:wrap}
.badge{display:inline-flex;align-items:center;gap:5px;min-height:24px;padding:2px 8px;border-radius:999px;background:#edf2f8;color:#496078;font-size:11px;font-weight:700;white-space:nowrap}.badge.blue{background:var(--blue-soft);color:var(--blue)}.badge.red{background:var(--red-soft);color:var(--red)}.badge.amber{background:var(--amber-soft);color:var(--amber)}.badge.outline{background:#fff;border:1px solid var(--line)}
.table-wrap{border:1px solid var(--line);border-radius:12px;overflow:auto;background:#fff}.table-wrap.tall{max-height:600px}table{width:100%;border-collapse:separate;border-spacing:0;min-width:760px}th,td{text-align:left;padding:11px 12px;border-bottom:1px solid var(--line);vertical-align:middle}th{position:sticky;top:0;background:#f8fafd;color:#65768d;font-size:11px;font-weight:800;z-index:2;letter-spacing:.02em}tbody tr:hover td{background:#fbfdff}tbody tr:last-child td{border-bottom:0}.cell-title{font-weight:700}.cell-sub{font-size:11px;color:var(--muted);margin-top:2px;max-width:440px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.mono{font-family:ui-monospace,SFMono-Regular,Consolas,monospace;font-size:11px}.empty{padding:44px 18px;text-align:center;color:var(--muted)}
.rule-layout{display:grid;grid-template-columns:minmax(260px,.36fr) minmax(0,.64fr);gap:16px}.priority-list{display:grid;gap:7px}.priority-item{border:1px solid var(--line);border-radius:10px;padding:10px;background:var(--surface-soft);display:flex;align-items:center;gap:10px}.priority-number{width:24px;height:24px;border-radius:8px;display:grid;place-items:center;background:var(--blue-soft);color:var(--blue);font-weight:800;font-size:11px}.priority-item strong{display:block;font-size:12px}.priority-item span{display:block;color:var(--muted);font-size:10px}.rules-stack{display:grid;gap:16px}.rule-row{display:grid;grid-template-columns:auto minmax(120px,.8fr) minmax(150px,1fr) minmax(180px,1fr) auto;gap:8px;align-items:center;margin-bottom:8px}.rule-row.regex{grid-template-columns:auto minmax(130px,.65fr) minmax(170px,1fr) minmax(160px,.8fr) auto}.rule-row.invalid .input{border-color:#df7c88;background:#fff8f9}.switch{position:relative;display:inline-flex;width:42px;height:24px;flex:none}.switch input{opacity:0;width:0;height:0}.switch span{position:absolute;inset:0;background:#cbd5e2;border-radius:999px;cursor:pointer;transition:.2s}.switch span:before{content:"";position:absolute;width:18px;height:18px;left:3px;top:3px;background:#fff;border-radius:50%;box-shadow:0 2px 6px rgba(0,0,0,.16);transition:.2s}.switch input:checked+span{background:var(--blue)}.switch input:checked+span:before{transform:translateX(18px)}
.dirty-bar{position:sticky;bottom:16px;margin-top:14px;border:1px solid #a9c9f1;background:rgba(239,247,255,.96);backdrop-filter:blur(10px);border-radius:12px;padding:10px 12px;display:none;align-items:center;justify-content:space-between;gap:10px;box-shadow:0 10px 28px rgba(23,80,148,.14);z-index:8}.dirty-bar.show{display:flex}.dirty-bar strong{font-size:12px}.dirty-bar span{font-size:11px;color:var(--muted)}
.auto-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px}.setting{border:1px solid var(--line);border-radius:13px;padding:15px;background:var(--surface-soft)}.setting-head{display:flex;align-items:flex-start;justify-content:space-between;gap:10px}.setting strong{font-size:13px}.setting p{margin:4px 0 0;color:var(--muted);font-size:11px}.setting .field{margin-top:12px}
.activity{display:grid;gap:8px}.activity-item{border:1px solid var(--line);border-radius:10px;padding:10px 12px;background:#fff;display:grid;grid-template-columns:74px 1fr;gap:10px}.activity-time{font-size:10px;color:var(--subtle);font-family:ui-monospace,SFMono-Regular,Consolas,monospace}.activity-text{font-size:12px;word-break:break-word}.activity-text.error{color:var(--red)}.activity-text.success{color:var(--blue-deep)}
.modal-backdrop{position:fixed;inset:0;background:rgba(16,31,51,.38);backdrop-filter:blur(4px);z-index:100;display:none;align-items:center;justify-content:center;padding:20px}.modal-backdrop.show{display:flex}.modal{width:min(520px,100%);max-height:min(760px,92vh);overflow:auto;background:#fff;border:1px solid rgba(255,255,255,.7);border-radius:18px;box-shadow:var(--shadow-float)}.modal-head{padding:18px 20px;border-bottom:1px solid var(--line);display:flex;align-items:flex-start;justify-content:space-between;gap:12px}.modal-head h3{margin:0;font-size:17px}.modal-head p{margin:3px 0 0;color:var(--muted);font-size:12px}.modal-body{padding:20px;display:grid;gap:14px}.modal-foot{padding:14px 20px;border-top:1px solid var(--line);display:flex;justify-content:flex-end;gap:8px;background:#fbfcfe}
.lock{width:min(440px,100%)}.lock-logo{width:48px;height:48px;border-radius:15px;background:var(--blue);color:#fff;display:grid;place-items:center;font-size:18px;font-weight:900;margin-bottom:8px}.lock-note{border:1px solid #bed7f5;background:#f0f7ff;color:#31577f;border-radius:10px;padding:10px 11px;font-size:11px}
.toast-area{position:fixed;right:18px;bottom:18px;z-index:150;display:grid;gap:8px;width:min(360px,calc(100vw - 36px))}.toast{background:#fff;border:1px solid var(--line);border-left:4px solid var(--blue);border-radius:11px;padding:11px 13px;box-shadow:0 14px 36px rgba(25,52,84,.18);animation:toastIn .2s ease-out}.toast.error{border-left-color:var(--red)}.toast strong{display:block;font-size:12px}.toast p{margin:2px 0 0;color:var(--muted);font-size:11px;word-break:break-word}@keyframes toastIn{from{opacity:0;transform:translateX(10px)}to{opacity:1;transform:none}}
.progress{position:fixed;left:0;top:0;height:3px;background:var(--blue);z-index:200;width:0;opacity:0;transition:width .25s,opacity .25s}.progress.show{opacity:1}.progress.done{width:100%!important;opacity:0}
.mobile-menu{display:none}
.hidden{display:none!important}
@media(max-width:1180px){.profile-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.metrics{grid-template-columns:repeat(2,minmax(0,1fr))}.overview-grid{grid-template-columns:1fr}.rule-layout{grid-template-columns:1fr}.auto-grid{grid-template-columns:1fr 1fr}}
@media(max-width:820px){.shell{display:block}.sidebar{position:fixed;left:-280px;width:252px;transition:.2s;box-shadow:var(--shadow-float)}.sidebar.open{left:0}.mobile-menu{display:inline-flex}.topbar{padding:0 14px}.content{padding:16px 14px 36px}.profile-grid{grid-template-columns:1fr}.auto-grid{grid-template-columns:1fr}.topology{grid-template-columns:1fr}.arrow{transform:rotate(90deg)}.rule-row,.rule-row.regex{grid-template-columns:auto 1fr}.rule-row .select,.rule-row .input{grid-column:2}.rule-row .btn{grid-column:2;justify-self:start}.page-title p{display:none}.search{min-width:0;width:100%}.toolbar{align-items:stretch}.toolbar-group{width:100%}.toolbar-group .input,.toolbar-group .select{flex:1;min-width:0}.metrics{grid-template-columns:1fr 1fr}.top-actions .btn span.label{display:none}}
@media(max-width:520px){.metrics{grid-template-columns:1fr}.quick-grid{grid-template-columns:1fr}.top-actions{gap:4px}.metric-value{font-size:18px}.card-head,.card-body{padding-left:14px;padding-right:14px}}
</style>
</head>
<body>
<div id="progress" class="progress"></div>
<div class="shell">
  <aside id="sidebar" class="sidebar">
    <div class="brand"><div class="brand-mark">W</div><div><strong>WARP Egress</strong><small>CLIProxyAPI Plugin</small></div></div>
    <div class="nav-label">出口管理</div>
    <nav class="nav">
      <button class="active" data-view="overview"><span class="nav-icon">01</span>总览</button>
      <button data-view="profiles"><span class="nav-icon">02</span>WARP 配置</button>
      <button data-view="routing"><span class="nav-icon">03</span>路由规则</button>
      <button data-view="auth"><span class="nav-icon">04</span>认证文件</button>
      <button data-view="automation"><span class="nav-icon">05</span>自动切换</button>
      <button data-view="activity"><span class="nav-icon">06</span>执行记录</button>
    </nav>
    <div class="sidebar-bottom">
      <div class="connection-card">
        <div class="connection-row"><span id="connectionDot" class="dot"></span><strong id="connectionState">未连接</strong></div>
        <p id="connectionDetail">需要管理密钥</p>
      </div>
    </div>
  </aside>

  <main class="main">
    <header class="topbar">
      <div style="display:flex;align-items:center;gap:10px;min-width:0">
        <button class="btn secondary icon mobile-menu" data-action="toggle-sidebar">☰</button>
        <div class="page-title"><h1 id="pageHeading">出口总览</h1><p id="pageDescription">查看当前出口、健康状态与快速操作</p></div>
      </div>
      <div class="top-actions">
        <button class="btn secondary icon" title="刷新" data-action="refresh">↻</button>
        <button class="btn secondary" data-action="copy-proxy"><span class="label">复制全局代理</span></button>
        <button class="btn" data-action="open-connect"><span class="label">连接设置</span></button>
      </div>
    </header>

    <div class="content">
      <div id="configBanner" class="banner blue hidden"><div><strong>CLIProxyAPI 全局代理配置要求</strong><p>请将配置文件中的 proxy-url 设置为 <span id="requiredProxy" class="mono">-</span>，全局出口切换才会生效。</p></div><button class="btn soft small" data-action="copy-required-proxy">复制</button></div>
      <div id="errorBanner" class="banner danger hidden"><div><strong>插件运行异常</strong><p id="errorBannerText"></p></div><button class="btn danger small" data-action="refresh">重试</button></div>

      <section id="view-overview" class="view active">
        <div class="metrics">
          <div class="metric"><div class="metric-label">当前全局出口 <span id="globalHealthBadge" class="badge outline">未选择</span></div><div id="metricGlobal" class="metric-value">-</div><div id="metricGlobalMeta" class="metric-meta">尚未选择 WARP 配置</div></div>
          <div class="metric"><div class="metric-label">固定中继地址</div><div id="metricRelay" class="metric-value mono">-</div><div id="metricRelayMeta" class="metric-meta">CLIProxyAPI 应连接到此地址</div></div>
          <div class="metric"><div class="metric-label">健康配置</div><div id="metricHealthy" class="metric-value">0 / 0</div><div id="metricHealthyMeta" class="metric-meta">等待健康检测</div></div>
          <div class="metric"><div class="metric-label">重复出口</div><div id="metricDuplicates" class="metric-value">0</div><div id="metricDuplicatesMeta" class="metric-meta">没有检测到重复 IP</div></div>
        </div>
        <div class="overview-grid">
          <div class="stack">
            <div class="card">
              <div class="card-head"><div><h2>当前流量路径</h2><p>全局请求通过固定 SOCKS5 中继转发到当前 WARP 配置</p></div><button class="btn secondary small" data-action="copy-proxy">复制代理地址</button></div>
              <div class="card-body"><div class="topology"><div class="node"><span>CLIProxyAPI</span><strong>全局 proxy-url</strong><small>固定配置，不随出口变化</small></div><div class="arrow">→</div><div class="node active"><span>WARP 中继</span><strong id="topologyRelay">-</strong><small id="topologyProfile">未选择配置</small></div><div class="arrow">→</div><div class="node"><span>公网出口</span><strong id="topologyIP">-</strong><small id="topologyColo">等待检测</small></div></div></div>
            </div>
            <div class="card">
              <div class="card-head"><div><h2>配置健康概览</h2><p>按当前检测结果展示出口状态</p></div><button class="btn secondary small" data-action="goto-profiles">管理配置</button></div>
              <div class="card-body"><div id="overviewProfiles" class="profile-grid"></div></div>
            </div>
          </div>
          <div class="stack">
            <div class="card">
              <div class="card-head"><div><h2>快速操作</h2><p>常用操作集中在此处</p></div></div>
              <div class="card-body"><div class="quick-grid">
                <button class="quick" data-action="open-switch"><strong>切换全局出口</strong><span>从健康配置中选择新的出口</span></button>
                <button class="quick" data-action="check-all"><strong>检查全部出口</strong><span>更新 IP、节点与延迟</span></button>
                <button class="quick" data-action="run-auto"><strong>立即自动切换</strong><span>按自动策略选择下一个出口</span></button>
                <button class="quick" data-action="apply-rules"><strong>应用全部规则</strong><span>同步认证文件 proxy_url</span></button>
              </div></div>
            </div>
            <div class="card">
              <div class="card-head"><div><h2>自动切换状态</h2><p>当前策略与最近一次切换</p></div><button class="btn secondary small" data-action="goto-automation">设置</button></div>
              <div class="card-body" id="overviewAuto"></div>
            </div>
          </div>
        </div>
      </section>

      <section id="view-profiles" class="view">
        <div class="card">
          <div class="card-head"><div><h2>WARP 配置池</h2><p>托管 WARP 使用独立 wireproxy 端口，也可接入已有外部 SOCKS5</p></div><button class="btn" data-action="open-create">新增配置</button></div>
          <div class="card-body">
            <div class="toolbar"><div class="toolbar-group"><input id="profileSearch" class="input search" placeholder="搜索名称、IP、节点或代理地址"><select id="profileFilter" class="select"><option value="all">全部状态</option><option value="healthy">仅健康</option><option value="unhealthy">仅异常</option><option value="managed">托管 WARP</option><option value="external">外部 SOCKS5</option></select></div><div class="toolbar-group"><button class="btn secondary" data-action="check-all">检查全部</button><button class="btn secondary" data-action="refresh">重新加载</button></div></div>
            <div id="profilesGrid" class="profile-grid"></div>
          </div>
        </div>
      </section>

      <section id="view-routing" class="view">
        <div class="rule-layout">
          <div class="stack">
            <div class="card"><div class="card-head"><div><h2>规则优先级</h2><p>从上到下首次命中即停止</p></div></div><div class="card-body"><div class="priority-list"><div class="priority-item"><div class="priority-number">1</div><div><strong>单个认证文件</strong><span>按 auth_index 精确绑定</span></div></div><div class="priority-item"><div class="priority-number">2</div><div><strong>正则表达式</strong><span>匹配文件、邮箱、标签等字段</span></div></div><div class="priority-item"><div class="priority-number">3</div><div><strong>认证类型 / 服务商</strong><span>匹配 codex、claude、gemini 等类型</span></div></div><div class="priority-item"><div class="priority-number">4</div><div><strong>全局出口</strong><span>所有未命中认证文件的默认出口</span></div></div></div></div></div>
            <div class="card"><div class="card-head"><div><h2>默认全局出口</h2><p>同时控制固定全局中继的后端</p></div></div><div class="card-body"><select id="globalSelect" class="select" style="width:100%"></select><p class="field-hint" style="margin:8px 0 0">修改后需要保存规则；切换实时全局出口请使用“立即切换”。</p><button class="btn soft" style="margin-top:12px" data-action="switch-selected-global">立即切换到所选配置</button></div></div>
          </div>
          <div class="rules-stack">
            <div class="card"><div class="card-head"><div><h2>认证类型 / 服务商规则</h2><p>例如 codex、claude、gemini、xai</p></div><button class="btn secondary small" data-action="add-type-rule">添加规则</button></div><div class="card-body"><div id="typeRules"></div><div id="typeRulesEmpty" class="empty">尚未添加类型规则</div></div></div>
            <div class="card"><div class="card-head"><div><h2>正则表达式规则</h2><p>可匹配文件名、邮箱、标签、类型、服务商或 auth_index</p></div><button class="btn secondary small" data-action="add-regex-rule">添加规则</button></div><div class="card-body"><div id="regexRules"></div><div id="regexRulesEmpty" class="empty">尚未添加正则规则</div></div></div>
          </div>
        </div>
        <div id="rulesDirty" class="dirty-bar"><div><strong>规则有未保存的修改</strong><br><span>“保存”只写入规则，“保存并应用”会同时改写认证文件。</span></div><div class="toolbar-group"><button class="btn secondary" data-action="discard-rules">放弃修改</button><button class="btn secondary" data-action="save-rules">保存</button><button class="btn" data-action="apply-rules">保存并应用</button></div></div>
      </section>

      <section id="view-auth" class="view">
        <div class="card">
          <div class="card-head"><div><h2>认证文件出口</h2><p>单文件绑定优先级最高；运行时虚拟认证只能查看，不能独立修改</p></div><button class="btn secondary" data-action="load-auth">重新读取</button></div>
          <div class="card-body">
            <div class="toolbar"><div class="toolbar-group"><input id="authSearch" class="input search" placeholder="搜索文件、邮箱、标签、auth_index"><select id="authProviderFilter" class="select"><option value="all">全部类型</option></select><select id="authRouteFilter" class="select"><option value="all">全部规则</option><option value="exact">单文件</option><option value="regex">正则</option><option value="type">类型</option><option value="global">全局</option><option value="inherit">继承全局</option><option value="none">未分配</option></select></div><div class="toolbar-group"><span id="authSelectionCount" class="badge outline">已选 0</span><select id="bulkProfile" class="select"><option value="">继承规则</option></select><button class="btn" data-action="bulk-assign">批量设置</button></div></div>
            <div class="table-wrap tall"><table><thead><tr><th style="width:42px"><input id="selectAllAuth" type="checkbox"></th><th>认证文件</th><th>类型 / 标签</th><th>当前代理</th><th>命中规则</th><th>单文件出口</th></tr></thead><tbody id="authFiles"></tbody></table></div>
          </div>
        </div>
      </section>

      <section id="view-automation" class="view">
        <div class="card">
          <div class="card-head"><div><h2>自动切换策略</h2><p>定时轮换健康出口，并在当前全局出口失效时执行故障转移</p></div><div class="toolbar-group"><button class="btn secondary" data-action="run-auto">立即执行一次</button><button class="btn" data-action="save-auto">保存设置</button></div></div>
          <div class="card-body">
            <div class="auto-grid">
              <div class="setting"><div class="setting-head"><div><strong>启用定时轮换</strong><p>按照设置的时间间隔切换全局出口</p></div><label class="switch"><input id="autoEnabled" type="checkbox"><span></span></label></div><div class="field"><label>轮换间隔（分钟）</label><input id="autoInterval" class="input" type="number" min="0" step="1" value="0"><div class="field-hint">0 表示不执行定时轮换。</div></div></div>
              <div class="setting"><div class="setting-head"><div><strong>出口异常故障转移</strong><p>当前出口健康检测失败时自动切换</p></div><label class="switch"><input id="autoFailover" type="checkbox"><span></span></label></div></div>
              <div class="setting"><div class="setting-head"><div><strong>必须更换公网 IP</strong><p>候选配置与当前出口 IP 相同时跳过</p></div><label class="switch"><input id="autoDifferent" type="checkbox"><span></span></label></div></div>
            </div>
            <div id="autoSummary" class="banner blue" style="margin-top:16px;margin-bottom:0"></div>
          </div>
        </div>
      </section>

      <section id="view-activity" class="view">
        <div class="card"><div class="card-head"><div><h2>执行记录</h2><p>当前标签页内的操作结果与错误记录</p></div><button class="btn secondary" data-action="clear-activity">清空记录</button></div><div class="card-body"><div id="activityList" class="activity"></div><div id="activityEmpty" class="empty">尚未执行任何操作</div></div></div>
      </section>
    </div>
  </main>
</div>

<div id="connectModal" class="modal-backdrop">
  <div class="modal lock"><div class="modal-head"><div><div class="lock-logo">W</div><h3>连接 CLIProxyAPI</h3><p>请输入 remote-management.secret-key</p></div><button class="btn ghost icon" data-action="close-connect">×</button></div><div class="modal-body"><div class="field"><label>管理密钥</label><input id="managementKey" class="input" type="password" autocomplete="current-password" placeholder="输入管理密钥"><div class="field-hint">密钥仅保存在当前浏览器 localStorage，不会写入插件配置。</div></div><div class="lock-note">管理接口必须配置 secret-key。资源面板本身可打开，但所有读取和修改操作都需要认证。</div></div><div class="modal-foot"><button class="btn secondary" data-action="disconnect">清除密钥</button><button id="connectButton" class="btn" data-action="connect">连接并加载</button></div></div>
</div>

<div id="createModal" class="modal-backdrop">
  <div class="modal"><div class="modal-head"><div><h3>新增 WARP 配置</h3><p>创建托管实例或接入已有 SOCKS5</p></div><button class="btn ghost icon" data-action="close-create">×</button></div><div class="modal-body"><div class="field"><label>配置名称</label><input id="createName" class="input" placeholder="例如：新加坡 01"><div class="field-hint">用于面板识别，可随时通过重新创建调整。</div></div><div class="field"><label>配置模式</label><select id="createMode" class="select"><option value="managed">托管 WARP（wgcf + wireproxy）</option><option value="external">外部 SOCKS5</option></select></div><div id="externalProxyField" class="field hidden"><label>SOCKS5 地址</label><input id="createProxy" class="input mono" placeholder="socks5://127.0.0.1:42000"><div class="field-hint">支持 socks5://host:port，也可包含用户名和密码。</div></div><label style="display:flex;align-items:center;gap:8px"><input id="createAutoStart" type="checkbox" checked> 创建后立即启动并检测</label></div><div class="modal-foot"><button class="btn secondary" data-action="close-create">取消</button><button id="createButton" class="btn" data-action="create-profile">创建配置</button></div></div>
</div>

<div id="switchModal" class="modal-backdrop">
  <div class="modal"><div class="modal-head"><div><h3>切换全局出口</h3><p>新连接会立即使用所选配置，已有长连接可能继续使用旧出口</p></div><button class="btn ghost icon" data-action="close-switch">×</button></div><div class="modal-body"><div class="field"><label>目标配置</label><select id="switchProfile" class="select"></select></div><div id="switchPreview" class="banner blue" style="margin:0"></div></div><div class="modal-foot"><button class="btn secondary" data-action="close-switch">取消</button><button id="switchButton" class="btn" data-action="confirm-switch">确认切换</button></div></div>
</div>

<div id="confirmModal" class="modal-backdrop">
  <div class="modal"><div class="modal-head"><div><h3 id="confirmTitle">确认操作</h3><p id="confirmText"></p></div></div><div class="modal-foot"><button class="btn secondary" data-action="confirm-cancel">取消</button><button id="confirmOK" class="btn danger" data-action="confirm-ok">确认</button></div></div>
</div>

<div id="toastArea" class="toast-area"></div>
<script>
(function(){
'use strict';
var state={
  status:null,profiles:[],rules:{global_profile_id:'',type_rules:[],regex_rules:[],exact_rules:{}},
  savedRules:null,auto:{enabled:false,failover_enabled:true,rotate_interval_seconds:0,require_different_ip:true},
  files:[],selectedAuth:{},activity:[],view:'overview',connected:false,pending:0,confirmResolve:null
};
var viewMeta={
  overview:['出口总览','查看当前出口、健康状态与快速操作'],profiles:['WARP 配置','创建、检测并管理独立出口'],
  routing:['路由规则','配置全局、类型与正则分流策略'],auth:['认证文件','查看规则命中并设置单文件出口'],
  automation:['自动切换','配置定时轮换与异常故障转移'],activity:['执行记录','查看当前标签页内的操作结果']
};
function el(id){return document.getElementById(id)}
var WARP_ENC_PREFIX='enc::v1::';var WARP_SECRET_SALT='cli-proxy-api-webui::secure-storage';var _warpKeyCache=null;
function warpPanelKeyBytes(){try{return new TextEncoder().encode(WARP_SECRET_SALT+'|'+window.location.host+'|'+navigator.userAgent)}catch(_){return new TextEncoder().encode(WARP_SECRET_SALT)}}
function warpDeobfuscate(payload){var raw=String(payload==null?'':payload);if(!raw||raw.indexOf(WARP_ENC_PREFIX)!==0)return raw;try{var b64=raw.slice(WARP_ENC_PREFIX.length);var binary=atob(b64);var encrypted=new Uint8Array(binary.length);for(var i=0;i<binary.length;i++)encrypted[i]=binary.charCodeAt(i);var kb=warpPanelKeyBytes();var out=new Uint8Array(encrypted.length);for(var j=0;j<encrypted.length;j++)out[j]=encrypted[j]^kb[j%kb.length];return new TextDecoder().decode(out)}catch(_){return raw}}
function warpTryParse(text){try{return JSON.parse(text)}catch(_){return null}}
function warpStores(){var list=[];try{list.push(window.localStorage)}catch(_){};try{list.push(window.sessionStorage)}catch(_){};try{if(window.parent&&window.parent!==window){try{list.push(window.parent.localStorage)}catch(_){};try{list.push(window.parent.sessionStorage)}catch(_){}}}catch(_){};return list}
function warpRead(store,name){try{return store&&store.getItem?store.getItem(name):null}catch(_){return null}}
function warpExtractFromPanel(){try{var stores=warpStores();for(var s=0;s<stores.length;s++){var store=stores[s];var authRaw=warpRead(store,'cli-proxy-auth');if(authRaw){var parsed=warpTryParse(warpDeobfuscate(authRaw));var k=(parsed&&parsed.state&&parsed.state.managementKey)||(parsed&&parsed.managementKey)||'';if(String(k).trim())return String(k).trim()}var legacy=['managementKey','cli-proxy-management-key','CPA_MANAGEMENT_KEY','management_password'];for(var n=0;n<legacy.length;n++){var raw=warpRead(store,legacy[n]);if(!raw)continue;var plain=warpDeobfuscate(raw);var pp=warpTryParse(plain);if(typeof pp==='string'&&pp.trim())return pp.trim();if(pp&&typeof pp==='object'){var kk=pp.managementKey||pp.password||pp.key||'';if(String(kk).trim())return String(kk).trim()}if(plain&&plain.indexOf(WARP_ENC_PREFIX)!==0&&plain.trim())return plain.trim()}}}catch(_){};return ''}
function key(){if(_warpKeyCache===null){_warpKeyCache=warpExtractFromPanel()||localStorage.getItem('warp-egress-key')||''}return _warpKeyCache}
function resetKeyCache(){_warpKeyCache=null}
function clone(v){return JSON.parse(JSON.stringify(v))}
function esc(v){return String(v==null?'':v).replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]})}
function attr(v){return esc(v)}
function profileByID(id){for(var i=0;i<state.profiles.length;i++){if(state.profiles[i].id===id)return state.profiles[i]}return null}
function fmtTime(v){if(!v||String(v).indexOf('0001-01-01')===0)return '从未';var d=new Date(v);return isNaN(d.getTime())?'从未':d.toLocaleString('zh-CN',{hour12:false})}
function formatDuration(seconds){seconds=Number(seconds||0);if(!seconds)return '未启用定时轮换';if(seconds%3600===0)return (seconds/3600)+' 小时';if(seconds%60===0)return (seconds/60)+' 分钟';return seconds+' 秒'}
function routeName(t){return {exact:'单文件',regex:'正则',type:'类型',global:'全局',inherit:'继承全局',none:'未分配'}[t]||t||'未分配'}
function setBusy(button,busy,label){if(!button)return;if(busy){button.dataset.oldHtml=button.innerHTML;button.disabled=true;button.innerHTML='<span class="spinner"></span>'+(label||'处理中')}else{button.disabled=false;if(button.dataset.oldHtml)button.innerHTML=button.dataset.oldHtml}}
function progressStart(){state.pending++;var p=el('progress');p.classList.remove('done');p.classList.add('show');p.style.width=Math.min(88,18+state.pending*16)+'%'}
function progressEnd(){state.pending=Math.max(0,state.pending-1);if(state.pending===0){var p=el('progress');p.classList.add('done');setTimeout(function(){p.classList.remove('show','done');p.style.width='0'},280)}}
function toast(title,message,type){var item=document.createElement('div');item.className='toast'+(type==='error'?' error':'');item.innerHTML='<strong>'+esc(title)+'</strong><p>'+esc(message||'')+'</p>';el('toastArea').appendChild(item);setTimeout(function(){item.remove()},4200)}
function activity(message,type,data){state.activity.unshift({time:new Date(),message:message,type:type||'info',data:data});if(state.activity.length>100)state.activity.length=100;renderActivity()}
function modal(id,show){el(id).classList.toggle('show',show)}
function showConnect(force){el('managementKey').value=key();modal('connectModal',true);if(force)el('managementKey').focus()}
function updateConnection(){el('connectionDot').classList.toggle('connected',state.connected);el('connectionState').textContent=state.connected?'已连接':'未连接';el('connectionDetail').textContent=state.connected?(state.status?'插件 '+state.status.version:'管理接口可用'):'需要管理密钥'}
async function api(path,method,body){
  if(!key())throw new Error('请先输入 CLIProxyAPI 管理密钥');
  progressStart();
  try{
    var headers={'Authorization':'Bearer '+key()};if(body!==undefined)headers['Content-Type']='application/json';
    var response=await fetch('/v0/management/warp-egress'+path,{method:method||'GET',headers:headers,body:body===undefined?undefined:JSON.stringify(body)});
    var text=await response.text();var data;try{data=JSON.parse(text)}catch(e){data={error:text||('HTTP '+response.status)}}
    if(response.status===401||response.status===403){state.connected=false;resetKeyCache();updateConnection();showConnect(true)}
    if(!response.ok)throw new Error(data.error||('HTTP '+response.status));
    state.connected=true;updateConnection();return data;
  }finally{progressEnd()}
}
async function connect(){
  var btn=el('connectButton');var value=el('managementKey').value.trim();if(!value){toast('缺少密钥','请输入管理密钥','error');return}
  localStorage.setItem('warp-egress-key',value);resetKeyCache();setBusy(btn,true,'连接中');
  try{await refreshAll();modal('connectModal',false);toast('连接成功','插件数据已加载');activity('连接 CLIProxyAPI 管理接口成功','success')}
  catch(e){state.connected=false;updateConnection();toast('连接失败',e.message,'error');activity('连接失败：'+e.message,'error')}
  finally{setBusy(btn,false)}
}
function disconnect(){localStorage.removeItem('warp-egress-key');resetKeyCache();state.connected=false;state.status=null;updateConnection();modal('connectModal',true);toast('密钥已清除','已断开连接')}
async function refreshAll(){
  if(!key()){showConnect(true);return}
  try{
    var result=await Promise.all([api('/status'),api('/rules'),api('/auto'),api('/auth-files')]);
    state.status=result[0];state.profiles=state.status.profiles||[];state.rules=normalizeRules(result[1]);state.savedRules=clone(state.rules);state.auto=result[2]||state.auto;state.files=(result[3]&&result[3].files)||[];
    state.connected=true;renderAll();activity('刷新插件状态完成','success');
  }catch(e){state.connected=false;renderError(e.message);updateConnection();throw e}
}
function normalizeRules(r){r=r||{};return {global_profile_id:r.global_profile_id||'',type_rules:Array.isArray(r.type_rules)?r.type_rules:[],regex_rules:Array.isArray(r.regex_rules)?r.regex_rules:[],exact_rules:r.exact_rules||{}}}
function renderAll(){renderError('');updateConnection();renderBanner();renderOverview();renderProfiles();renderRouting();renderAuth();renderAutomation();renderActivity()}
function renderError(message){el('errorBanner').classList.toggle('hidden',!message);el('errorBannerText').textContent=message||''}
function renderBanner(){var s=state.status||{};el('requiredProxy').textContent=s.required_host_proxy_url||s.global_relay_url||'-';el('configBanner').classList.toggle('hidden',!s.required_host_proxy_url)}
function healthBadge(profile){if(!profile)return '<span class="badge outline">未选择</span>';return profile.healthy?'<span class="badge blue">健康</span>':'<span class="badge red">异常</span>'}
function renderOverview(){
  var s=state.status||{};var current=s.global_profile||profileByID(s.global_profile_id);var healthy=state.profiles.filter(function(p){return p.healthy}).length;var duplicates=Object.keys(s.duplicate_exit_ips||{}).length;
  el('metricGlobal').textContent=current?(current.exit_ip||current.name):'未选择';el('metricGlobalMeta').textContent=current?(current.name+' · '+(current.colo||'未知节点')):'请选择一个全局配置';el('globalHealthBadge').outerHTML=healthBadge(current).replace('<span','<span id="globalHealthBadge"');
  el('metricRelay').textContent=s.global_relay_url||'-';el('metricRelayMeta').textContent=s.global_relay_running?'中继正在监听':'中继尚未启动';
  el('metricHealthy').textContent=healthy+' / '+state.profiles.length;el('metricHealthyMeta').textContent=state.profiles.length?('最近检查：'+latestCheck()):'尚未创建配置';
  el('metricDuplicates').textContent=duplicates;el('metricDuplicatesMeta').textContent=duplicates?('存在 '+duplicates+' 组重复公网 IP'):'没有检测到重复 IP';
  el('topologyRelay').textContent=s.global_relay_url||'-';el('topologyProfile').textContent=current?current.name:'未选择配置';el('topologyIP').textContent=current?(current.exit_ip||'等待检测'):'-';el('topologyColo').textContent=current?((current.colo||'未知节点')+' · '+(current.latency_ms||0)+' ms'):'等待检测';
  var preview=state.profiles.slice().sort(function(a,b){return Number(b.healthy)-Number(a.healthy)}).slice(0,3);el('overviewProfiles').innerHTML=preview.length?preview.map(profileCard).join(''):'<div class="empty" style="grid-column:1/-1">尚无 WARP 配置，先创建一个出口。</div>';
  var a=state.auto||{};var last=a.last_switch_at?fmtTime(a.last_switch_at):'从未切换';el('overviewAuto').innerHTML='<div class="priority-list"><div class="priority-item"><div class="priority-number">A</div><div><strong>'+(a.enabled?'定时轮换已启用':'定时轮换未启用')+'</strong><span>'+esc(formatDuration(a.rotate_interval_seconds))+'</span></div></div><div class="priority-item"><div class="priority-number">F</div><div><strong>'+(a.failover_enabled?'异常故障转移已启用':'异常故障转移已关闭')+'</strong><span>'+(a.require_different_ip?'要求不同公网 IP':'允许相同公网 IP')+'</span></div></div><div class="priority-item"><div class="priority-number">L</div><div><strong>最近切换</strong><span>'+esc(last+(a.last_reason?' · '+a.last_reason:''))+'</span></div></div></div>';
}
function latestCheck(){var latest=0;state.profiles.forEach(function(p){var n=new Date(p.last_checked||0).getTime();if(n>latest)latest=n});return latest?new Date(latest).toLocaleString('zh-CN',{hour12:false}):'从未'}
function profileCard(p){
  var current=state.status&&p.id===state.status.global_profile_id;var status=p.healthy?'<span class="badge blue">健康</span>':'<span class="badge red">异常</span>';var mode=p.mode==='managed'?'托管 WARP':'外部 SOCKS5';
  var actions='';if(!current)actions+='<button class="btn soft small" data-action="switch-global" data-id="'+attr(p.id)+'">设为全局</button>';actions+='<button class="btn secondary small" data-action="profile-check" data-id="'+attr(p.id)+'">检测</button>';
  if(p.mode==='managed'){actions+='<button class="btn secondary small" data-action="profile-'+(p.running?'stop':'start')+'" data-id="'+attr(p.id)+'">'+(p.running?'停止':'启动')+'</button><button class="btn secondary small" data-action="profile-recreate" data-id="'+attr(p.id)+'">重新注册</button>'}
  actions+='<button class="btn danger small" data-action="profile-delete" data-id="'+attr(p.id)+'">删除</button>';
  return '<article class="profile-card '+(current?'current':'')+'"><div class="profile-top"><div class="profile-name"><strong>'+esc(p.name)+'</strong><small>'+esc(mode)+(current?' · 当前全局':'')+'</small></div>'+status+'</div><div class="profile-body"><div class="profile-ip mono">'+esc(p.exit_ip||'等待检测')+'</div><div class="profile-meta"><div class="meta-box"><span>Cloudflare 节点</span><b>'+esc(p.colo||'-')+'</b></div><div class="meta-box"><span>延迟</span><b>'+(p.latency_ms?p.latency_ms+' ms':'-')+'</b></div><div class="meta-box"><span>本地代理</span><b class="mono">'+esc(p.proxy_url||'-')+'</b></div><div class="meta-box"><span>最后检查</span><b>'+esc(fmtTime(p.last_checked))+'</b></div></div>'+(p.last_error?'<div class="profile-error">'+esc(p.last_error)+'</div>':'')+'</div><div class="profile-actions">'+actions+'</div></article>';
}
function renderProfiles(){
  var q=(el('profileSearch')?el('profileSearch').value:'').toLowerCase();var filter=el('profileFilter')?el('profileFilter').value:'all';var list=state.profiles.filter(function(p){var hay=[p.name,p.exit_ip,p.colo,p.proxy_url,p.mode].join(' ').toLowerCase();if(q&&hay.indexOf(q)<0)return false;if(filter==='healthy'&&!p.healthy)return false;if(filter==='unhealthy'&&p.healthy)return false;if(filter==='managed'&&p.mode!=='managed')return false;if(filter==='external'&&p.mode!=='external')return false;return true});
  el('profilesGrid').innerHTML=list.length?list.map(profileCard).join(''):'<div class="empty" style="grid-column:1/-1">没有符合筛选条件的配置</div>';
}
function profileOptions(selected,inherit,healthyOnly){var out=inherit?'<option value="">继承规则</option>':'';state.profiles.forEach(function(p){if(healthyOnly&&!p.healthy)return;out+='<option value="'+attr(p.id)+'" '+(p.id===selected?'selected':'')+'>'+esc(p.name)+' · '+esc(p.exit_ip||p.proxy_url||'未检测')+'</option>'});return out}
function renderRouting(){
  el('globalSelect').innerHTML='<option value="">未设置全局出口</option>'+profileOptions(state.rules.global_profile_id,false,false);renderTypeRules();renderRegexRules();updateRulesDirty();
}
function renderTypeRules(){
  var target=el('typeRules');target.innerHTML=state.rules.type_rules.map(function(r,i){return '<div class="rule-row"><label class="switch"><input type="checkbox" data-rule-kind="type" data-rule-index="'+i+'" data-field="enabled" '+(r.enabled?'checked':'')+'><span></span></label><input class="input" data-rule-kind="type" data-rule-index="'+i+'" data-field="key" value="'+attr(r.key)+'" placeholder="认证类型或服务商"><select class="select" data-rule-kind="type" data-rule-index="'+i+'" data-field="profile_id">'+profileOptions(r.profile_id,false,false)+'</select><div class="cell-sub">匹配 provider 或 type</div><button class="btn danger small" data-action="remove-type-rule" data-index="'+i+'">删除</button></div>'}).join('');el('typeRulesEmpty').classList.toggle('hidden',state.rules.type_rules.length>0)
}
function regexValid(pattern){try{new RegExp(pattern);return true}catch(e){return false}}
function renderRegexRules(){
  var targets=[['name','文件名'],['email','邮箱'],['label','标签'],['type','类型'],['provider','服务商'],['auth_index','auth_index'],['all','全部字段']];
  el('regexRules').innerHTML=state.rules.regex_rules.map(function(r,i){var targetOptions=targets.map(function(t){return '<option value="'+t[0]+'" '+(r.target===t[0]?'selected':'')+'>'+t[1]+'</option>'}).join('');return '<div class="rule-row regex '+(regexValid(r.pattern)?'':'invalid')+'"><label class="switch"><input type="checkbox" data-rule-kind="regex" data-rule-index="'+i+'" data-field="enabled" '+(r.enabled?'checked':'')+'><span></span></label><select class="select" data-rule-kind="regex" data-rule-index="'+i+'" data-field="target">'+targetOptions+'</select><input class="input mono" data-rule-kind="regex" data-rule-index="'+i+'" data-field="pattern" value="'+attr(r.pattern)+'" placeholder="例如：@example\\.com$"><select class="select" data-rule-kind="regex" data-rule-index="'+i+'" data-field="profile_id">'+profileOptions(r.profile_id,false,false)+'</select><button class="btn danger small" data-action="remove-regex-rule" data-index="'+i+'">删除</button></div>'}).join('');el('regexRulesEmpty').classList.toggle('hidden',state.rules.regex_rules.length>0)
}
function markRulesDirty(){updateRulesDirty()}
function updateRulesDirty(){var dirty=JSON.stringify(state.rules)!==JSON.stringify(state.savedRules);el('rulesDirty').classList.toggle('show',dirty)}
async function saveRules(apply){
  var invalid=state.rules.regex_rules.some(function(r){return r.enabled&&!regexValid(r.pattern)});if(invalid){toast('规则无效','存在无法解析的正则表达式','error');switchView('routing');return false}
  try{state.rules=normalizeRules(await api('/rules/save','POST',state.rules));state.savedRules=clone(state.rules);updateRulesDirty();activity('路由规则已保存','success');if(apply){var result=await api('/rules/apply','POST',{});activity('已应用路由规则：修改 '+result.changed+'，跳过 '+result.skipped+'，失败 '+result.failed,result.failed?'error':'success',result);toast('规则已应用','修改 '+result.changed+' 个认证文件，失败 '+result.failed,result.failed?'error':undefined);await loadAuthFiles()}else{toast('保存成功','路由规则已写入插件状态')}return true}catch(e){toast('保存失败',e.message,'error');activity('保存规则失败：'+e.message,'error');return false}
}
async function loadAuthFiles(){var data=await api('/auth-files');state.files=data.files||[];renderAuth();activity('重新读取认证文件完成','success')}
function renderAuth(){
  var providers={};state.files.forEach(function(f){var p=f.provider||f.type||'未知';providers[p]=true});var filter=el('authProviderFilter');var old=filter.value;filter.innerHTML='<option value="all">全部类型</option>'+Object.keys(providers).sort().map(function(p){return '<option value="'+attr(p)+'">'+esc(p)+'</option>'}).join('');if(providers[old])filter.value=old;
  el('bulkProfile').innerHTML=profileOptions('',true,false);renderAuthRows();
}
function filteredAuth(){var q=(el('authSearch').value||'').toLowerCase();var provider=el('authProviderFilter').value;var route=el('authRouteFilter').value;return state.files.filter(function(f){var p=f.provider||f.type||'未知';var hay=[f.name,f.email,f.label,f.auth_index,p,f.proxy_url].join(' ').toLowerCase();if(q&&hay.indexOf(q)<0)return false;if(provider!=='all'&&p!==provider)return false;var rt=(f.effective&&f.effective.rule_type)||'none';if(route!=='all'&&rt!==route)return false;return true})}
function renderAuthRows(){
  var list=filteredAuth();el('authFiles').innerHTML=list.length?list.map(function(f){var selected=(state.rules.exact_rules||{})[f.auth_index]||'';var disabled=f.runtime_only?'disabled':'';var checked=state.selectedAuth[f.auth_index]?'checked':'';var effective=f.effective||{};return '<tr><td><input type="checkbox" class="auth-check" data-auth-index="'+attr(f.auth_index)+'" '+checked+' '+disabled+'></td><td><div class="cell-title">'+esc(f.name)+'</div><div class="cell-sub mono">'+esc(f.auth_index)+'</div>'+(f.email?'<div class="cell-sub">'+esc(f.email)+'</div>':'')+'</td><td><span class="badge outline">'+esc(f.provider||f.type||'-')+'</span><div class="cell-sub">'+esc(f.label||'')+'</div></td><td><div class="mono">'+esc(f.proxy_url||'(继承全局 proxy-url)')+'</div></td><td><span class="badge '+(effective.rule_type==='exact'?'blue':'outline')+'">'+esc(routeName(effective.rule_type))+'</span><div class="cell-sub">'+esc(effective.rule_key||effective.profile_id||'')+'</div></td><td><select class="select exact-select" data-auth-index="'+attr(f.auth_index)+'" '+disabled+'>'+profileOptions(selected,true,false)+'</select>'+(f.runtime_only?'<div class="cell-sub">运行时虚拟认证不可修改</div>':'')+'</td></tr>'}).join(''):'<tr><td colspan="6" class="empty">没有符合筛选条件的认证文件</td></tr>';updateAuthSelection();el('selectAllAuth').checked=false
}
function updateAuthSelection(){var count=Object.keys(state.selectedAuth).filter(function(k){return state.selectedAuth[k]}).length;el('authSelectionCount').textContent='已选 '+count}
async function assignExact(authIndex,profileId,silent){
  var result=await api('/auth-files/assign','POST',{auth_index:authIndex,profile_id:profileId,apply_now:true});if(profileId)state.rules.exact_rules[authIndex]=profileId;else delete state.rules.exact_rules[authIndex];state.savedRules=clone(state.rules);if(!silent){toast('单文件出口已更新',result.name||authIndex);activity('更新认证文件 '+(result.name||authIndex)+' 的单文件出口','success',result)}return result
}
async function bulkAssign(){
  var indexes=Object.keys(state.selectedAuth).filter(function(k){return state.selectedAuth[k]});if(!indexes.length){toast('未选择认证文件','请先勾选需要批量设置的文件','error');return}var profile=el('bulkProfile').value;var ok=await confirmBox('批量设置出口','将更新 '+indexes.length+' 个认证文件，并立即写入 proxy_url。','确认批量设置',false);if(!ok)return;
  var btn=document.querySelector('[data-action="bulk-assign"]');setBusy(btn,true,'处理中');var success=0,failed=0;for(var i=0;i<indexes.length;i++){try{await assignExact(indexes[i],profile,true);success++}catch(e){failed++;activity('批量设置 '+indexes[i]+' 失败：'+e.message,'error')}}setBusy(btn,false);state.selectedAuth={};await loadAuthFiles();toast('批量设置完成','成功 '+success+'，失败 '+failed,failed?'error':undefined);activity('批量设置认证文件完成：成功 '+success+'，失败 '+failed,failed?'error':'success')
}
function renderAutomation(){var a=state.auto||{};el('autoEnabled').checked=!!a.enabled;el('autoFailover').checked=!!a.failover_enabled;el('autoDifferent').checked=!!a.require_different_ip;el('autoInterval').value=Math.round((a.rotate_interval_seconds||0)/60);el('autoSummary').innerHTML='<div><strong>最近一次自动切换</strong><p>'+(a.last_switch_at?esc(fmtTime(a.last_switch_at)):'尚未执行')+(a.last_reason?' · '+esc(a.last_reason):'')+(a.last_profile_id?' · 配置 '+esc((profileByID(a.last_profile_id)||{}).name||a.last_profile_id):'')+'</p></div>'}
async function saveAuto(){var config={enabled:el('autoEnabled').checked,failover_enabled:el('autoFailover').checked,rotate_interval_seconds:Math.max(0,Number(el('autoInterval').value||0))*60,require_different_ip:el('autoDifferent').checked};try{state.auto=await api('/auto/save','POST',config);renderAutomation();renderOverview();toast('自动切换设置已保存',formatDuration(state.auto.rotate_interval_seconds));activity('自动切换设置已保存','success')}catch(e){toast('保存失败',e.message,'error');activity('保存自动切换设置失败：'+e.message,'error')}}
async function runAuto(){var ok=await confirmBox('立即执行自动切换','插件会按当前策略选择一个健康出口并切换全局中继。','立即切换',false);if(!ok)return;try{var result=await api('/auto/run','POST',{});state.auto=result.auto_switch||state.auto;toast('自动切换完成',result.profile?(result.profile.name+' · '+(result.profile.exit_ip||'未检测')):'未找到可切换配置');activity('立即自动切换完成','success',result);await refreshAll()}catch(e){toast('自动切换失败',e.message,'error');activity('自动切换失败：'+e.message,'error')}}
async function createProfile(){var name=el('createName').value.trim();var mode=el('createMode').value;var proxy=el('createProxy').value.trim();if(!name){toast('缺少名称','请输入配置名称','error');return}if(mode==='external'&&!proxy){toast('缺少代理地址','外部 SOCKS5 模式必须填写地址','error');return}var btn=el('createButton');setBusy(btn,true,'创建中');try{var p=await api('/profiles/create','POST',{name:name,mode:mode,proxy_url:proxy,auto_start:el('createAutoStart').checked});modal('createModal',false);toast('配置已创建',p.name);activity('创建 WARP 配置 '+p.name,'success',p);el('createName').value='';el('createProxy').value='';await refreshAll();switchView('profiles')}catch(e){toast('创建失败',e.message,'error');activity('创建配置失败：'+e.message,'error')}finally{setBusy(btn,false)}}
async function profileAction(id,action){var p=profileByID(id);if(!p)return;var confirmNeeded=action==='recreate';if(confirmNeeded){var ok=await confirmBox('重新注册 WARP 配置','将停止并重新生成 '+p.name+' 的 WARP 注册信息，出口 IP 可能变化。','重新注册',true);if(!ok)return}try{await api('/profiles/action','POST',{id:id,action:action});toast('操作完成',p.name+' · '+action);activity('配置 '+p.name+' 执行 '+action,'success');await refreshAll()}catch(e){toast('操作失败',e.message,'error');activity('配置 '+p.name+' 操作失败：'+e.message,'error')}}
async function deleteProfile(id){var p=profileByID(id);if(!p)return;var ok=await confirmBox('删除 WARP 配置','将删除 '+p.name+' 的实例数据，并移除引用它的规则。此操作不可恢复。','确认删除',true);if(!ok)return;try{await api('/profiles/delete','POST',{id:id});toast('配置已删除',p.name);activity('删除 WARP 配置 '+p.name,'success');await refreshAll()}catch(e){toast('删除失败',e.message,'error');activity('删除配置失败：'+e.message,'error')}}
async function switchGlobal(id){var p=profileByID(id);if(!p)return;try{await api('/global/switch','POST',{profile_id:id});toast('全局出口已切换',p.name+' · '+(p.exit_ip||'未检测'));activity('全局出口切换到 '+p.name,'success',p);modal('switchModal',false);await refreshAll()}catch(e){toast('切换失败',e.message,'error');activity('切换全局出口失败：'+e.message,'error')}}
function openSwitch(preselected){el('switchProfile').innerHTML=profileOptions(preselected||((state.status||{}).global_profile_id),false,true);if(!el('switchProfile').options.length)el('switchProfile').innerHTML=profileOptions(preselected||'',false,false);updateSwitchPreview();modal('switchModal',true)}
function updateSwitchPreview(){var p=profileByID(el('switchProfile').value);el('switchPreview').innerHTML=p?'<div><strong>'+esc(p.name)+'</strong><p>'+esc(p.exit_ip||'未检测')+' · '+esc(p.colo||'未知节点')+' · '+esc(p.proxy_url||'')+'</p></div>':'<div><strong>没有可用配置</strong><p>请先创建并检测 WARP 配置。</p></div>';el('switchButton').disabled=!p}
async function checkAll(button){var list=state.profiles.slice();if(!list.length){toast('没有配置','请先创建 WARP 配置','error');return}button=button||document.querySelector('[data-action="check-all"]');setBusy(button,true,'检测中');var failed=0;for(var i=0;i<list.length;i++){try{await api('/profiles/action','POST',{id:list[i].id,action:'check'})}catch(e){failed++;activity('检测 '+list[i].name+' 失败：'+e.message,'error')}}setBusy(button,false);await refreshAll();toast('检测完成','共 '+list.length+' 个配置，失败 '+failed,failed?'error':undefined)}
function switchView(name){if(!viewMeta[name])return;state.view=name;document.querySelectorAll('.view').forEach(function(v){v.classList.toggle('active',v.id==='view-'+name)});document.querySelectorAll('.nav [data-view]').forEach(function(b){b.classList.toggle('active',b.dataset.view===name)});el('pageHeading').textContent=viewMeta[name][0];el('pageDescription').textContent=viewMeta[name][1];el('sidebar').classList.remove('open');if(name==='profiles')renderProfiles();if(name==='auth')renderAuthRows()}
function confirmBox(title,text,okLabel,danger){el('confirmTitle').textContent=title;el('confirmText').textContent=text;el('confirmOK').textContent=okLabel||'确认';el('confirmOK').className='btn '+(danger?'danger':'');modal('confirmModal',true);return new Promise(function(resolve){state.confirmResolve=resolve})}
function resolveConfirm(value){modal('confirmModal',false);if(state.confirmResolve){state.confirmResolve(value);state.confirmResolve=null}}
function copyText(text,label){if(!text){toast('没有可复制内容','当前值为空','error');return}navigator.clipboard.writeText(text).then(function(){toast('已复制',label||text)}).catch(function(){var ta=document.createElement('textarea');ta.value=text;document.body.appendChild(ta);ta.select();document.execCommand('copy');ta.remove();toast('已复制',label||text)})}
function renderActivity(){var list=el('activityList');var empty=el('activityEmpty');if(!list||!empty)return;empty.classList.toggle('hidden',state.activity.length>0);list.innerHTML=state.activity.map(function(a){var detail=a.data?'<div class="cell-sub mono">'+esc(JSON.stringify(a.data))+'</div>':'';return '<div class="activity-item"><div class="activity-time">'+a.time.toLocaleTimeString('zh-CN',{hour12:false})+'</div><div class="activity-text '+(a.type||'')+'">'+esc(a.message)+detail+'</div></div>'}).join('')}
function handleAction(action,node){
  var id=node.dataset.id;switch(action){
    case 'toggle-sidebar':el('sidebar').classList.toggle('open');break;case 'refresh':refreshAll().catch(function(e){toast('刷新失败',e.message,'error')});break;case 'open-connect':showConnect(false);break;case 'close-connect':modal('connectModal',false);break;case 'connect':connect();break;case 'disconnect':disconnect();break;
    case 'copy-proxy':copyText((state.status||{}).global_relay_url,'全局代理地址');break;case 'copy-required-proxy':copyText((state.status||{}).required_host_proxy_url,'CLIProxyAPI proxy-url');break;
    case 'goto-profiles':switchView('profiles');break;case 'goto-automation':switchView('automation');break;case 'open-create':modal('createModal',true);setTimeout(function(){el('createName').focus()},50);break;case 'close-create':modal('createModal',false);break;case 'create-profile':createProfile();break;
    case 'open-switch':openSwitch();break;case 'close-switch':modal('switchModal',false);break;case 'confirm-switch':switchGlobal(el('switchProfile').value);break;case 'switch-global':openSwitch(id);break;case 'switch-selected-global':openSwitch(el('globalSelect').value);break;
    case 'profile-check':profileAction(id,'check');break;case 'profile-start':profileAction(id,'start');break;case 'profile-stop':profileAction(id,'stop');break;case 'profile-recreate':profileAction(id,'recreate');break;case 'profile-delete':deleteProfile(id);break;case 'check-all':checkAll(node);break;
    case 'add-type-rule':if(!state.profiles.length){toast('没有可用出口','请先创建 WARP 配置','error');break}state.rules.type_rules.push({key:'',profile_id:state.profiles[0]?state.profiles[0].id:'',enabled:true});renderTypeRules();markRulesDirty();break;case 'remove-type-rule':state.rules.type_rules.splice(Number(node.dataset.index),1);renderTypeRules();markRulesDirty();break;
    case 'add-regex-rule':if(!state.profiles.length){toast('没有可用出口','请先创建 WARP 配置','error');break}state.rules.regex_rules.push({id:'rule-'+Date.now(),pattern:'',target:'all',profile_id:state.profiles[0]?state.profiles[0].id:'',enabled:true});renderRegexRules();markRulesDirty();break;case 'remove-regex-rule':state.rules.regex_rules.splice(Number(node.dataset.index),1);renderRegexRules();markRulesDirty();break;
    case 'save-rules':saveRules(false);break;case 'apply-rules':saveRules(true);break;case 'discard-rules':state.rules=clone(state.savedRules||normalizeRules({}));renderRouting();toast('已放弃修改','规则已恢复到上次保存状态');break;
    case 'load-auth':loadAuthFiles().catch(function(e){toast('读取失败',e.message,'error')});break;case 'bulk-assign':bulkAssign();break;case 'save-auto':saveAuto();break;case 'run-auto':runAuto();break;
    case 'clear-activity':state.activity=[];renderActivity();break;case 'confirm-ok':resolveConfirm(true);break;case 'confirm-cancel':resolveConfirm(false);break;
  }
}
document.addEventListener('click',function(e){var view=e.target.closest('[data-view]');if(view){switchView(view.dataset.view);return}var action=e.target.closest('[data-action]');if(action)handleAction(action.dataset.action,action)});
document.addEventListener('input',function(e){
  if(e.target.id==='profileSearch')renderProfiles();if(e.target.id==='authSearch')renderAuthRows();
  var kind=e.target.dataset.ruleKind;if(kind){var index=Number(e.target.dataset.ruleIndex),field=e.target.dataset.field,arr=kind==='type'?state.rules.type_rules:state.rules.regex_rules;if(arr[index]){arr[index][field]=e.target.type==='checkbox'?e.target.checked:e.target.value;if(kind==='regex'&&field==='pattern'){var row=e.target.closest('.rule-row');if(row)row.classList.toggle('invalid',!regexValid(e.target.value))}markRulesDirty()}}
});
document.addEventListener('change',function(e){
  if(e.target.id==='profileFilter')renderProfiles();if(e.target.id==='authProviderFilter'||e.target.id==='authRouteFilter')renderAuthRows();if(e.target.id==='createMode')el('externalProxyField').classList.toggle('hidden',e.target.value!=='external');if(e.target.id==='switchProfile')updateSwitchPreview();
  if(e.target.id==='globalSelect'){state.rules.global_profile_id=e.target.value;markRulesDirty()}
  if(e.target.classList.contains('auth-check')){state.selectedAuth[e.target.dataset.authIndex]=e.target.checked;updateAuthSelection()}
  if(e.target.id==='selectAllAuth'){filteredAuth().forEach(function(f){if(!f.runtime_only)state.selectedAuth[f.auth_index]=e.target.checked});renderAuthRows();el('selectAllAuth').checked=e.target.checked}
  if(e.target.classList.contains('exact-select')){var select=e.target;select.disabled=true;assignExact(select.dataset.authIndex,select.value,false).then(function(){return loadAuthFiles()}).catch(function(err){toast('设置失败',err.message,'error');activity('设置单文件出口失败：'+err.message,'error');return loadAuthFiles()})}
  var kind=e.target.dataset.ruleKind;if(kind){var index=Number(e.target.dataset.ruleIndex),field=e.target.dataset.field,arr=kind==='type'?state.rules.type_rules:state.rules.regex_rules;if(arr[index]){arr[index][field]=e.target.type==='checkbox'?e.target.checked:e.target.value;markRulesDirty()}}
});
document.addEventListener('keydown',function(e){if(e.key==='Escape'){modal('createModal',false);modal('switchModal',false);modal('connectModal',false);if(el('confirmModal').classList.contains('show'))resolveConfirm(false)}if(e.key==='Enter'&&el('connectModal').classList.contains('show'))connect()});
window.addEventListener('beforeunload',function(e){if(JSON.stringify(state.rules)!==JSON.stringify(state.savedRules)){e.preventDefault();e.returnValue=''}});
el('managementKey').value=key();updateConnection();renderActivity();if(key()){refreshAll().catch(function(){showConnect(true)})}else{showConnect(true)}
})();
</script>
<script>window.__ONBOARDING__=/*__ONBOARDING_INJECT__*/;</script>
<script>
(function(){
  try{
    var ob=window.__ONBOARDING__;
    if(!ob||ob.all_ready)return;
    try{if(localStorage.getItem('warp-egress-skip-onboarding')==='1')return}catch(_){}
    var missing=(ob.steps||[]).filter(function(s){return !s.done}).length;
    var ov=document.createElement('div');
    ov.style.cssText='position:fixed;inset:0;z-index:9999;background:rgba(15,32,58,.62);backdrop-filter:blur(6px);display:flex;align-items:center;justify-content:center;padding:24px';
    var card=document.createElement('div');
    card.style.cssText='background:#fff;border-radius:18px;max-width:640px;width:100%;max-height:88vh;overflow:auto;box-shadow:0 24px 80px rgba(10,25,50,.45);padding:26px 28px';
    var title=document.createElement('div');
    title.style.cssText='font-size:19px;font-weight:800;color:#152238;margin-bottom:6px';
    title.textContent='初始化引导 · WARP 出口管理';
    var sub=document.createElement('div');
    sub.style.cssText='font-size:12px;color:#687890;margin-bottom:18px;line-height:1.7';
    sub.textContent='检测到还有 '+missing+' 项未完成。按顺序完成以下步骤，即可让 CPA 流量经过 WARP 出口：';
    card.appendChild(title);card.appendChild(sub);
    (ob.steps||[]).forEach(function(step,idx){
      var row=document.createElement('div');
      row.style.cssText='display:flex;gap:12px;padding:13px 14px;border:1px solid '+(step.done?'#cdeeda':'#e3ebf5')+';border-radius:12px;margin-bottom:10px;background:'+(step.done?'#f4fbf6':'#fff');
      var icon=document.createElement('div');
      icon.style.cssText='width:26px;height:26px;border-radius:50%;display:flex;align-items:center;justify-content:center;font-size:12px;font-weight:800;flex:none;background:'+(step.done?'#d9f2e3':'#fff0e6')+';color:'+(step.done?'#1d9e57':'#c76a1b')+';margin-top:1px';
      icon.textContent=step.done?'✓':String(idx+1);
      var body=document.createElement('div');
      body.style.cssText='min-width:0';
      var t=document.createElement('div');
      t.style.cssText='font-weight:700;color:#152238;font-size:13px';
      t.textContent=step.title;
      var h=document.createElement('div');
      h.style.cssText='color:#687890;font-size:12px;margin-top:4px;line-height:1.65;word-break:break-all';
      h.textContent=step.hint||'';
      body.appendChild(t);body.appendChild(h);
      row.appendChild(icon);row.appendChild(body);
      card.appendChild(row);
    });
    var actions=document.createElement('div');
    actions.style.cssText='display:flex;gap:10px;margin-top:18px';
    var recheck=document.createElement('button');
    recheck.style.cssText='flex:1;height:38px;border-radius:10px;background:#176ee8;color:#fff;font-weight:700;cursor:pointer;border:0';
    recheck.textContent='重新检测';
    recheck.onclick=function(){location.reload()};
    var skip=document.createElement('button');
    skip.style.cssText='flex:1;height:38px;border-radius:10px;background:#fff;color:#36506f;border:1px solid #c9d7e8;font-weight:700;cursor:pointer';
    skip.textContent='跳过，直接进入面板';
    skip.onclick=function(){try{localStorage.setItem('warp-egress-skip-onboarding','1')}catch(_){};ov.remove()};
    actions.appendChild(recheck);actions.appendChild(skip);
    card.appendChild(actions);
    ov.appendChild(card);
    document.body.appendChild(ov);
  }catch(e){}
})();
</script></body>
</html>`
