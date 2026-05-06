import { LanguageToggle, useI18n, type UiLanguage } from "../i18n";

const featureOneItems = [
  ["Requirement", "需求进入 Work Item"],
  ["Plan", "Plan / TODO 明确化"],
  ["Agent", "Agent trace 与阶段统计"],
  ["Review", "Code Review / Rework"],
  ["Gate", "Human Review / Feishu"],
  ["Proof", "PR、checks、proof 记录"]
];

const featureTwoItems = [
  ["Select", "圈选真实页面元素"],
  ["Locate", "定位源码和 Repository Workspace"],
  ["Apply", "提交修改并刷新预览"],
  ["Review", "Confirm / Discard 审核"],
  ["Deliver", "物化 Work Item、PR 和 proof"],
  ["Trace", "保留 Page Pilot session 证据"]
];

const storyCards = [
  "Requirement → Work Item → Pipeline",
  "Agent 执行 → CI → Review",
  "Human Gate → Merge → Proof"
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
  const featureOne = isZh
    ? featureOneItems
    : [
        ["Requirement", "Requirement becomes a Work Item"],
        ["Plan", "Plan / TODO is explicit"],
        ["Agent", "Agent trace and stage metrics"],
        ["Review", "Code Review / Rework"],
        ["Gate", "Human Review / Feishu"],
        ["Proof", "PR, checks, and proof records"]
      ];
  const featureTwo = isZh
    ? featureTwoItems
    : [
        ["Select", "Select real page elements"],
        ["Locate", "Map DOM to source and workspace"],
        ["Apply", "Apply changes and refresh preview"],
        ["Review", "Confirm / Discard review"],
        ["Deliver", "Materialize Work Item, PR, and proof"],
        ["Trace", "Keep Page Pilot session evidence"]
      ];
  const story = isZh ? storyCards : ["Requirement → Work Item → Pipeline", "Agent run → CI → Review", "Human Gate → Merge → Proof"];
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
          <LanguageToggle language={uiLanguage} onLanguageChange={onLanguageChange} />
          <button type="button" className="theme-toggle" onClick={onToggleTheme} aria-label={t(uiTheme === "light" ? "Switch to night mode" : "Switch to day mode")}>
            <span aria-hidden="true">{uiTheme === "light" ? "☾" : "☼"}</span>
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
        <section className="portal-main">
          <section className="portal-entry" id="apps">
            <div className="portal-entry-heading">
              <span>{isZh ? "比赛演示入口" : "Competition demo entry"}</span>
              <h1 data-omega-source="apps/web/src/components/PortalHome.tsx:headline">{isZh ? "Omega AI DevFlow" : "Omega AI DevFlow"}</h1>
              <p data-omega-source="apps/web/src/components/PortalHome.tsx:welcome-copy">
                {isZh
                  ? "围绕 AI 原生研发流程，把需求拆解、Agent 执行、页面圈选、PR 审核和交付证据放进一条可追踪链路。"
                  : "A traceable AI-native delivery flow for requirements, Agent execution, page selection, PR review, and proof."}
              </p>
            </div>
            <div className="portal-entry-grid">
              <button type="button" className="portal-entry-card primary-entry" onClick={onOpenWorkboard} data-omega-source="apps/web/src/components/PortalHome.tsx:open-workboard-button">
                <span className="portal-entry-kicker">{isZh ? "功能一" : "Feature 1"}</span>
                <strong>{isZh ? "DevFlow 工作台" : "DevFlow Workboard"}</strong>
                <small>{isZh ? "从 Requirement 启动完整交付闭环，持续查看 Plan、Agent、CI、Review、Human Gate 和 proof。" : "Start a full delivery loop from a requirement, with Plan, Agents, CI, Review, Human Gate, and proof in one place."}</small>
                <span className="portal-entry-flow" aria-hidden="true">
                  <i>Req</i>
                  <i>Plan</i>
                  <i>PR</i>
                  <i>Proof</i>
                </span>
                <span className="portal-entry-action">{isZh ? "进入 Workboard" : "Open Workboard"}</span>
              </button>
              <button type="button" className="portal-entry-card secondary-entry" onClick={onOpenPagePilot} data-omega-source="apps/web/src/components/PortalHome.tsx:open-page-pilot-button">
                <span className="portal-entry-kicker">{isZh ? "功能二" : "Feature 2"}</span>
                <strong>{isZh ? "Page Pilot" : "Page Pilot"}</strong>
                <small>{isZh ? "圈选真实 DOM，提交页面修改，Confirm / Discard 后进入 Work Item 和 PR 证据链。" : "Select real DOM, apply page edits, then confirm or discard into the Work Item and PR evidence chain."}</small>
                <span className="portal-entry-flow" aria-hidden="true">
                  <i>Select</i>
                  <i>Apply</i>
                  <i>Review</i>
                  <i>Trace</i>
                </span>
                <span className="portal-entry-action">{isZh ? "打开 Page Pilot" : "Open Page Pilot"}</span>
              </button>
            </div>
          </section>

          <section className="portal-intro" id="templates">
            <div className="portal-section-heading">
              <span>{isZh ? "为什么是 Omega" : "Why Omega"}</span>
              <h2>{isZh ? "不是生成一段代码，而是交付一条可信流程" : "Not just code generation, a trusted delivery flow"}</h2>
              <button type="button" onClick={onOpenWorkboard}>
                {isZh ? "查看工作台" : "View workbench"}
              </button>
            </div>
            <div className="portal-story-grid">
              {story.map((title, index) => (
                <article key={title} className="portal-story-card">
                  <span>{`0${index + 1}`}</span>
                  <strong data-omega-source={`apps/web/src/components/PortalHome.tsx:story-card-${index}`}>{title}</strong>
                  <small>
                    {isZh
                      ? ["明确仓库和任务边界，避免误写项目本身。", "记录每个 Agent 的职责、耗时、输出和验证。", "Human Review 后保留可复核的交付证据。"][index]
                      : ["Lock repository and task boundaries before execution.", "Record each Agent role, duration, output, and validation.", "Keep reviewable delivery evidence after Human Review."][index]}
                  </small>
                </article>
              ))}
            </div>
          </section>
        </section>

        <aside className="portal-right" aria-label={t("Omega highlights")}>
          <article className="portal-card portal-feature">
            <span>{isZh ? "功能一" : "Feature 1"}</span>
            <h3>{isZh ? "Local-first DevFlow 闭环" : "Local-first DevFlow loop"}</h3>
            <div className="portal-capability-list">
              {featureOne.map(([token, label]) => (
                <span key={token}>
                  <b>{token}</b>
                  {label}
                </span>
              ))}
            </div>
          </article>
          <article className="portal-card portal-feature">
            <span>{isZh ? "功能二" : "Feature 2"}</span>
            <h3>{isZh ? "Page Pilot 页面改造链路" : "Page Pilot page iteration loop"}</h3>
            <div className="portal-capability-list">
              {featureTwo.map(([token, label]) => (
                <span key={token}>
                  <b>{token}</b>
                  {label}
                </span>
              ))}
            </div>
          </article>
        </aside>
      </div>
    </main>
  );
}
