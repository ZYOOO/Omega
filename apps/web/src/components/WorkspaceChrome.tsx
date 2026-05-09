import type { ConnectionProvider, ProjectRecord, ProviderId, RepositoryTarget, WorkItem } from "../core";
import { LanguageToggle, useI18n, type UiLanguage } from "../i18n";

export type PrimaryNav = "Projects" | "Views" | "Issues" | "Page Pilot" | "Settings";
export type UiTheme = "light" | "dark";

type ConnectionStatusMap = Record<ProviderId, { status: string }>;
export type AgentAccessSidebarItem = {
  id: string;
  label: string;
  value: string;
  status: "ready" | "partial" | "setup";
};

type WorkspaceChromeProps = {
  activeNav: PrimaryNav;
  activeWorkItemDetail?: WorkItem;
  activeDetailRepositoryLabel: string;
  activeDetailCompleted: boolean;
  detailRunDisabled: boolean;
  detailRunLabel: string;
  detailRunVisible: boolean;
  runnerMessage: string;
  searchQuery: string;
  uiTheme: UiTheme;
  uiLanguage: UiLanguage;
  repositoryTargets: RepositoryTarget[];
  workItems: WorkItem[];
  activeRepositoryWorkspaceTargetId: string;
  workspaceSectionOpen: boolean;
  connectionsSectionOpen: boolean;
  agentAccessSectionOpen: boolean;
  agentAccessItems: AgentAccessSidebarItem[];
  visibleConnectionProviders: ConnectionProvider[];
  selectedProviderId: ProviderId;
  connections: ConnectionStatusMap;
  children: React.ReactNode;
  onBackToWorkItems: () => void;
  onHome: () => void;
  onNavigate: (nav: PrimaryNav) => void;
  onRunDetail: () => void;
  onSearchChange: (value: string) => void;
  onToggleTheme: () => void;
  onLanguageChange: (language: UiLanguage) => void;
  onToggleWorkspaceSection: (open: boolean) => void;
  onToggleConnectionsSection: (open: boolean) => void;
  onToggleAgentAccessSection: (open: boolean) => void;
  onSelectWorkspace: (target: RepositoryTarget, targetItems: WorkItem[]) => void;
  onConfigureWorkspace: (target: RepositoryTarget) => void;
  onProviderClick: (provider: ConnectionProvider) => void;
  onAgentAccessClick: (item: AgentAccessSidebarItem) => void;
  onNewRequirement: () => void;
};
const omegaLogoSrc = `${import.meta.env.BASE_URL}omega-logo.png`;

function primaryNavLabel(nav: PrimaryNav) {
  return nav === "Issues" ? "Work items" : nav;
}

function topbarSearchPlaceholder(nav: PrimaryNav) {
  if (nav === "Issues") return "Search work items...";
  if (nav === "Page Pilot") return "Search Page Pilot runs...";
  if (nav === "Settings") return "Search settings...";
  return "Search...";
}

export function WorkspaceChrome({
  activeNav,
  activeWorkItemDetail,
  activeDetailRepositoryLabel,
  detailRunDisabled,
  detailRunLabel,
  detailRunVisible,
  runnerMessage,
  searchQuery,
  uiTheme,
  uiLanguage,
  repositoryTargets,
  workItems,
  activeRepositoryWorkspaceTargetId,
  workspaceSectionOpen,
  connectionsSectionOpen,
  agentAccessSectionOpen,
  agentAccessItems,
  visibleConnectionProviders,
  selectedProviderId,
  connections,
  children,
  onBackToWorkItems,
  onHome,
  onNavigate,
  onRunDetail,
  onSearchChange,
  onToggleTheme,
  onLanguageChange,
  onToggleWorkspaceSection,
  onToggleConnectionsSection,
  onToggleAgentAccessSection,
  onSelectWorkspace,
  onConfigureWorkspace,
  onProviderClick,
  onAgentAccessClick,
  onNewRequirement
}: WorkspaceChromeProps) {
  const { t } = useI18n();
  return (
    <>
      <aside className="sidebar" aria-label={t("Workspace navigation")}>
        <div className="brand-lockup">
          <img className="brand-logo" src={omegaLogoSrc} alt="Omega AI DevFlow Engine" />
          <button type="button" className="sidebar-home-button" onClick={onHome}>
            {t("Home")}
          </button>
        </div>

        <nav className="nav-stack">
          {(["Projects", "Views", "Issues", "Page Pilot"] as const).map((item) => (
            <button key={item} className={item === activeNav ? "nav-item active" : "nav-item"} onClick={() => onNavigate(item)}>
              <span>{t(primaryNavLabel(item))}</span>
            </button>
          ))}
        </nav>

        {repositoryTargets.length > 0 ? (
          <details
            className="sidebar-section workspace-section"
            open={workspaceSectionOpen}
            onToggle={(event) => onToggleWorkspaceSection(event.currentTarget.open)}
          >
            <summary>
              <span className="section-label">{t("Workspaces")}</span>
            </summary>
            <nav className="workspace-stack" aria-label={t("Project workspaces")}>
              {repositoryTargets.map((target) => {
                const label = target.kind === "github" ? `${target.owner}/${target.repo}` : target.path;
                const targetItems = workItems.filter((item) => item.repositoryTargetId === target.id);
                const selected = target.id === activeRepositoryWorkspaceTargetId;
                return (
                  <div key={target.id} className={selected ? "workspace-entry selected" : "workspace-entry"}>
                    <button
                      className="workspace-row"
                      aria-label={`${label} ${targetItems.length}`}
                      onClick={() => onSelectWorkspace(target, targetItems)}
                    >
                      <span className="dot online" aria-hidden="true" />
                      <span>
                        <strong>{label}</strong>
                        <small>{t(targetItems.length === 1 ? "{count} item" : "{count} items", { count: targetItems.length })}</small>
                      </span>
                    </button>
                    <button
                      type="button"
                      className="workspace-config-button"
                      aria-label={t("Configure {label}", { label })}
                      title={t("Workspace config")}
                      onClick={() => onConfigureWorkspace(target)}
                    >
                      <span aria-hidden="true">⚙</span>
                    </button>
                  </div>
                );
              })}
            </nav>
          </details>
        ) : null}

        <details
          className="sidebar-section"
          open={connectionsSectionOpen}
          onToggle={(event) => onToggleConnectionsSection(event.currentTarget.open)}
        >
            <summary>
            <span className="section-label">{t("Connections")}</span>
          </summary>
          <div className="connection-stack">
            {visibleConnectionProviders.map((provider) => (
              <button
                key={provider.id}
                className={`connection-row ${selectedProviderId === provider.id ? "selected" : ""}`}
                onClick={() => onProviderClick(provider)}
              >
                <span className={connections[provider.id].status === "connected" ? "dot online" : "dot"} />
                <span>{provider.name}</span>
                <small>{t(connections[provider.id].status === "connected" ? "on" : "off")}</small>
              </button>
            ))}
          </div>
        </details>

        <details
          className="sidebar-section agent-access-section"
          open={agentAccessSectionOpen}
          onToggle={(event) => onToggleAgentAccessSection(event.currentTarget.open)}
        >
            <summary>
            <span className="section-label">{t("Agents")}</span>
          </summary>
          <div className="connection-stack agent-access-stack">
            {agentAccessItems.map((item) => (
              <button
                key={item.id}
                type="button"
                className="connection-row agent-access-row"
                aria-label={`${item.label}: ${t(item.value)}`}
                onClick={() => onAgentAccessClick(item)}
                title={`${item.label}: ${t(item.value)}`}
              >
                <span className={item.status === "ready" ? "dot online" : item.status === "partial" ? "dot warning" : "dot"} />
                <span>{item.label}</span>
                <small>{t(item.value)}</small>
              </button>
            ))}
          </div>
        </details>
      </aside>

      <section className="workbench">
        <header className={activeWorkItemDetail ? "topbar detail-mode" : "topbar"}>
          {activeWorkItemDetail ? (
            <>
              <nav className="detail-breadcrumb" aria-label={t("Work item")}>
                <button type="button" onClick={onBackToWorkItems}>
                  {t("Work items")}
                </button>
                <span>›</span>
                {activeDetailRepositoryLabel ? <span>{activeDetailRepositoryLabel}</span> : null}
                {activeDetailRepositoryLabel ? <span>›</span> : null}
                <strong>{activeWorkItemDetail.key}</strong>
                <span>{activeWorkItemDetail.title}</span>
              </nav>
              <div className="detail-toolbar">
                {runnerMessage ? (
                  <span className="detail-runner-chip" role="status" title={runnerMessage}>
                    {runnerMessageSummary(runnerMessage)}
                  </span>
                ) : null}
                <LanguageToggle language={uiLanguage} onLanguageChange={onLanguageChange} />
                <ThemeToggle uiTheme={uiTheme} onToggleTheme={onToggleTheme} />
                <button type="button" onClick={() => navigator.clipboard?.writeText(activeWorkItemDetail.target)}>
                  {t("Copy target")}
                </button>
                {detailRunVisible ? (
                  <button type="button" className="primary-action" disabled={detailRunDisabled} onClick={onRunDetail}>
                    {detailRunLabel}
                  </button>
                ) : null}
              </div>
            </>
          ) : (
            <>
              <div>
                <p className="section-label">Omega</p>
                <h1>{t(primaryNavLabel(activeNav))}</h1>
              </div>
              <div className="search-control">
                <input
                  className="command-input"
                  value={searchQuery}
                  onChange={(event) => onSearchChange(event.currentTarget.value)}
                  placeholder={t(topbarSearchPlaceholder(activeNav))}
                />
                <button type="button">{t("Search")}</button>
              </div>
              <div className="topbar-actions">
                {activeNav === "Issues" ? (
                  <button type="button" className="topbar-create" onClick={onNewRequirement}>
                    <span className="topbar-create-label">{t("New requirement")}</span>
                  </button>
                ) : null}
                <LanguageToggle language={uiLanguage} onLanguageChange={onLanguageChange} />
                <ThemeToggle uiTheme={uiTheme} onToggleTheme={onToggleTheme} />
              </div>
            </>
          )}
        </header>
        {children}
      </section>
    </>
  );
}

function ThemeToggle({ uiTheme, onToggleTheme }: { uiTheme: UiTheme; onToggleTheme: () => void }) {
  const { t } = useI18n();
  return (
    <button type="button" className="theme-toggle" onClick={onToggleTheme} aria-label={t(uiTheme === "light" ? "Switch to night mode" : "Switch to day mode")}>
      <span aria-hidden="true">{uiTheme === "light" ? "☾" : "☼"}</span>
    </button>
  );
}

function runnerMessageSummary(message: string): string {
  if (message.length <= 72) return message;
  return `${message.slice(0, 69)}...`;
}
