import { LanguageToggle, useI18n, type UiLanguage } from "../i18n";

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
  onLanguageChange: (language: UiLanguage) => void;
  uiTheme: "light" | "dark";
  uiLanguage: UiLanguage;
};

export function PortalHome({ onOpenWorkboard, onOpenPagePilot, onToggleTheme, onLanguageChange, uiTheme, uiLanguage }: PortalHomeProps) {
  const { language, t } = useI18n();
  const isZh = language === "zh-CN";
  const navItems = isZh ? ["我的首页", "需求", "Pipeline", "Agent 运行", "GitHub", "Proof", "Workflow", "设置"] : ["Home", "Requirements", "Pipeline", "Agent Runs", "GitHub", "Proof", "Workflow", "Settings"];
  return (
    <main className={`site-shell portal-shell theme-${uiTheme}`}>
      <header className="portal-topbar">
        <button type="button" className="portal-brand" onClick={onOpenWorkboard} aria-label={t("Open Omega Workboard")}>
          <img src="/omega-logo.png" alt="Omega AI DevFlow Engine" />
        </button>
        <nav className="portal-nav" aria-label={t("Omega portal navigation")}>
          <a href="#templates">{isZh ? "案例与方案" : "Templates"}</a>
          <a href="#apps">{isZh ? "产品功能" : "Product"}</a>
          <a href="#agent">Omega AI</a>
          <a href="#support">{isZh ? "合作与支持" : "Support"}</a>
          <a href="#pricing">{isZh ? "定价" : "Pricing"}</a>
        </nav>
        <div className="portal-top-actions">
          <span className="portal-user">张涌</span>
          <LanguageToggle language={uiLanguage} onLanguageChange={onLanguageChange} />
          <button type="button" className="theme-toggle" onClick={onToggleTheme} aria-label={t(uiTheme === "light" ? "Switch to night mode" : "Switch to day mode")}>
            <span aria-hidden="true">{uiTheme === "light" ? "☾" : "☼"}</span>
            {t(uiTheme === "light" ? "Night" : "Day")}
          </button>
          <button type="button" className="portal-outline" onClick={onOpenWorkboard}>
            {isZh ? "联系团队" : "Contact team"}
          </button>
          <button type="button" className="portal-primary" onClick={onOpenWorkboard} data-omega-source="apps/web/src/components/PortalHome.tsx:primary-workboard-button">
            {isZh ? "进入 Workboard" : "Open Workboard"}
          </button>
        </div>
      </header>

      <div className="portal-body">
        <aside className="portal-sidebar" aria-label={t("Omega home navigation")}>
          {navItems.map((item, index) => (
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
              <h1 data-omega-source="apps/web/src/components/PortalHome.tsx:headline">{isZh ? "张涌，欢迎回到 Omega" : "Welcome back to Omega"}</h1>
              <p data-omega-source="apps/web/src/components/PortalHome.tsx:welcome-copy">{isZh ? "把需求、Agent 编排、GitHub PR 和人工审核放进同一个可追踪工作台。" : "Bring requirements, Agent orchestration, GitHub PRs, and human review into one traceable workbench."}</p>
              <div className="portal-button-row">
                <button type="button" className="portal-outline" onClick={onOpenWorkboard} data-omega-source="apps/web/src/components/PortalHome.tsx:open-workboard-button">
                  {isZh ? "打开 Workboard" : "Open Workboard"}
                </button>
                <button type="button" className="portal-primary" onClick={onOpenPagePilot} data-omega-source="apps/web/src/components/PortalHome.tsx:open-page-pilot-button">
                  {isZh ? "打开 Page Pilot" : "Open Page Pilot"}
                </button>
              </div>
            </article>

            <article className="portal-card portal-apps">
              {portalApps.map(([token, label], index) => (
                <button key={label} type="button" onClick={onOpenWorkboard}>
                  <span>{token}</span>
                  {isZh ? label : ["Requirement Q&A", "Planning", "Coding", "Test generation", "Code review", "Human gate", "GitHub PR", "Proof Center", "Workflow templates", "Agent Registry", "Operations", "Local settings"][index]}
                </button>
              ))}
            </article>
          </section>

          <section className="portal-card portal-templates" id="templates">
            <div className="portal-section-heading">
              <h2>{isZh ? "最新模板推荐" : "Recommended templates"}</h2>
              <button type="button" onClick={onOpenWorkboard}>
                {isZh ? "模板中心" : "Template center"}
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
                  <strong data-omega-source={`apps/web/src/components/PortalHome.tsx:template-card-${index}`}>{isZh ? title : ["Feature delivery flow", "Bug fix loop", "Frontend iteration", "Code review and rework", "GitHub Issue automation", "Human review release"][index]}</strong>
                  <small>{isZh ? (index % 2 === 0 ? "需求到 PR 的完整链路" : "评审、返工、人工确认可复用") : (index % 2 === 0 ? "Requirement-to-PR delivery path" : "Reusable review, rework, and human gate")}</small>
                </button>
              ))}
            </div>
          </section>
        </section>

        <aside className="portal-right" aria-label={t("Omega highlights")}>
          <article className="portal-card portal-spotlight">
            <span>Omega</span>
            <h2>{isZh ? "AI DevFlow 工作台" : "AI DevFlow Workbench"}</h2>
            <p>{isZh ? "Pipeline 是骨架，Agent 是执行者，人类负责关键决策。" : "Pipeline provides the structure, Agents execute the work, and humans own key decisions."}</p>
            <button type="button" onClick={onOpenWorkboard}>
              {isZh ? "开始演示" : "Start demo"}
            </button>
          </article>
          <article className="portal-card portal-news">
            <span>{isZh ? "赛题能力" : "Capability"}</span>
            <h3>{isZh ? "功能一 v0Beta 已接入本地运行时" : "DevFlow v0Beta is connected to the local runtime"}</h3>
            <p>{isZh ? "支持 Requirement、Agent trace、Human gate、GitHub PR 与 proof 记录。" : "Supports Requirement, Agent trace, Human gate, GitHub PR, and proof records."}</p>
          </article>
          <article className="portal-card portal-course">
            <h3>{isZh ? "下一阶段" : "Next stage"}</h3>
            <div className="portal-mini-list">
              <span>{isZh ? "页面圈选" : "Element selection"}</span>
              <span>{isZh ? "热更新预览" : "Live preview"}</span>
              <span>{isZh ? "MR 摘要" : "PR summary"}</span>
            </div>
            <button type="button" className="portal-card-action" onClick={onOpenPagePilot}>
              {isZh ? "启动 Page Pilot" : "Start Page Pilot"}
            </button>
          </article>
        </aside>
      </div>
    </main>
  );
}
