const portalApps = [
  ["Req", "需求问答"],
  ["Plan", "方案设计"],
  ["Code", "编码执行"],
  ["Test", "测试生成"],
  ["Rev", "代码评审"],
  ["Gate", "人工审核"],
  ["PR", "GitHub PR"],
  ["Proof", "Proof Center"],
  ["Flow", "工作流模板"],
  ["Agent", "Agent Registry"],
  ["Ops", "运行观测"],
  ["Set", "本地设置"]
];

const templateCards = [
  "新功能交付流程",
  "Bug 修复闭环",
  "前端页面迭代",
  "代码评审与返工",
  "GitHub Issue 自动处理",
  "人工审核发布"
];

type PortalHomeProps = {
  onOpenWorkboard: () => void;
  onOpenPagePilot: () => void;
  onToggleTheme: () => void;
  uiTheme: "light" | "dark";
};

export function PortalHome({ onOpenWorkboard, onOpenPagePilot, onToggleTheme, uiTheme }: PortalHomeProps) {
  return (
    <main className={`site-shell portal-shell theme-${uiTheme}`}>
      <header className="portal-topbar">
        <button type="button" className="portal-brand" onClick={onOpenWorkboard} aria-label="Open Omega Workboard">
          <img src="/omega-logo.png" alt="Omega AI DevFlow Engine" />
        </button>
        <nav className="portal-nav" aria-label="Omega portal navigation">
          <a href="#templates">案例与方案</a>
          <a href="#apps">产品功能</a>
          <a href="#agent">Omega AI</a>
          <a href="#support">合作与支持</a>
          <a href="#pricing">定价</a>
        </nav>
        <div className="portal-top-actions">
          <span className="portal-user">张涌</span>
          <button type="button" className="theme-toggle" onClick={onToggleTheme} aria-label={`Switch to ${uiTheme === "light" ? "night" : "day"} mode`}>
            <span aria-hidden="true">{uiTheme === "light" ? "☾" : "☼"}</span>
            {uiTheme === "light" ? "Night" : "Day"}
          </button>
          <button type="button" className="portal-outline" onClick={onOpenWorkboard}>
            联系团队
          </button>
          <button type="button" className="portal-primary" onClick={onOpenWorkboard} data-omega-source="apps/web/src/components/PortalHome.tsx:primary-workboard-button">
            进入 Workboard
          </button>
        </div>
      </header>

      <div className="portal-body">
        <aside className="portal-sidebar" aria-label="Omega home navigation">
          {["我的首页", "需求", "Pipeline", "Agent Runs", "GitHub", "Proof", "Workflow", "设置"].map((item, index) => (
            <button
              key={item}
              type="button"
              className={index === 0 ? "active" : ""}
              onClick={index === 0 ? undefined : onOpenWorkboard}
            >
              <span>{item.slice(0, 1)}</span>
              {item}
            </button>
          ))}
        </aside>

        <section className="portal-main">
          <section className="portal-overview" id="apps">
            <article className="portal-card portal-welcome">
              <img className="portal-hero-logo" src="/omega-logo.png" alt="Omega AI DevFlow Engine" />
              <span className="portal-avatar">张涌</span>
              <h1 data-omega-source="apps/web/src/components/PortalHome.tsx:headline">张涌，欢迎回到 Omega</h1>
              <p data-omega-source="apps/web/src/components/PortalHome.tsx:welcome-copy">把需求、Agent 编排、GitHub PR 和人工审核放进同一个可追踪工作台。</p>
              <div className="portal-button-row">
                <button type="button" className="portal-outline" onClick={onOpenWorkboard} data-omega-source="apps/web/src/components/PortalHome.tsx:open-workboard-button">
                  打开 Workboard
                </button>
                <button type="button" className="portal-primary" onClick={onOpenPagePilot} data-omega-source="apps/web/src/components/PortalHome.tsx:open-page-pilot-button">
                  打开 Page Pilot
                </button>
              </div>
            </article>

            <article className="portal-card portal-apps">
              {portalApps.map(([token, label]) => (
                <button key={label} type="button" onClick={onOpenWorkboard}>
                  <span>{token}</span>
                  {label}
                </button>
              ))}
            </article>
          </section>

          <section className="portal-card portal-templates" id="templates">
            <div className="portal-section-heading">
              <h2>最新模板推荐</h2>
              <button type="button" onClick={onOpenWorkboard}>
                模板中心
              </button>
            </div>
            <div className="portal-template-grid">
              {templateCards.map((title, index) => (
                <button key={title} type="button" className="portal-template-card" onClick={onOpenWorkboard}>
                  <span className="template-mock" aria-hidden="true">
                    <i />
                    <i />
                    <i />
                    <b />
                  </span>
                  <strong data-omega-source={`apps/web/src/components/PortalHome.tsx:template-card-${index}`}>{title}</strong>
                  <small>{index % 2 === 0 ? "需求到 PR 的完整链路" : "评审、返工、人工确认可复用"}</small>
                </button>
              ))}
            </div>
          </section>
        </section>

        <aside className="portal-right" aria-label="Omega highlights">
          <article className="portal-card portal-spotlight">
            <span>Omega</span>
            <h2>AI DevFlow 工作台</h2>
            <p>Pipeline 是骨架，Agent 是执行者，人类负责关键决策。</p>
            <button type="button" onClick={onOpenWorkboard}>
              开始演示
            </button>
          </article>
          <article className="portal-card portal-news">
            <span>赛题能力</span>
            <h3>功能一 v0Beta 已接入本地运行时</h3>
            <p>支持 Requirement、Agent trace、Human gate、GitHub PR 与 proof 记录。</p>
          </article>
          <article className="portal-card portal-course">
            <h3>下一阶段</h3>
            <div className="portal-mini-list">
              <span>页面圈选</span>
              <span>热更新预览</span>
              <span>MR 摘要</span>
            </div>
            <button type="button" className="portal-card-action" onClick={onOpenPagePilot}>
              启动 Page Pilot
            </button>
          </article>
        </aside>
      </div>
    </main>
  );
}
