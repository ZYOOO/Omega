import type { ConnectionProvider, ProjectRecord, ProviderId, RepositoryTarget, WorkItem } from "../core";

export type PrimaryNav = "Projects" | "Views" | "Issues" | "Page Pilot" | "Settings";
export type UiTheme = "light" | "dark";

type ConnectionStatusMap = Record<ProviderId, { status: string }>;

type WorkspaceChromeProps = {
  activeNav: PrimaryNav;
  activeWorkItemDetail?: WorkItem;
  activeDetailRepositoryLabel: string;
  activeDetailCompleted: boolean;
  detailRunDisabled: boolean;
  detailRunLabel: string;
  runnerMessage: string;
  searchQuery: string;
  uiTheme: UiTheme;
  repositoryTargets: RepositoryTarget[];
  workItems: WorkItem[];
  activeRepositoryWorkspaceTargetId: string;
  workspaceSectionOpen: boolean;
  connectionsSectionOpen: boolean;
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
  onToggleWorkspaceSection: (open: boolean) => void;
  onToggleConnectionsSection: (open: boolean) => void;
  onSelectWorkspace: (target: RepositoryTarget, targetItems: WorkItem[]) => void;
  onConfigureWorkspace: (target: RepositoryTarget) => void;
  onProviderClick: (provider: ConnectionProvider) => void;
  onNewRequirement: () => void;
};

export function primaryNavLabel(nav: PrimaryNav) {
  return nav === "Issues" ? "Work items" : nav;
}

export function topbarSearchPlaceholder(nav: PrimaryNav) {
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
  runnerMessage,
  searchQuery,
  uiTheme,
  repositoryTargets,
  workItems,
  activeRepositoryWorkspaceTargetId,
  workspaceSectionOpen,
  connectionsSectionOpen,
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
  onToggleWorkspaceSection,
  onToggleConnectionsSection,
  onSelectWorkspace,
  onConfigureWorkspace,
  onProviderClick,
  onNewRequirement
}: WorkspaceChromeProps) {
  return (
    <>
      <aside className="sidebar" aria-label="Workspace navigation">
        <div className="brand-lockup">
          <img className="brand-logo" src="/omega-logo.png" alt="Omega AI DevFlow Engine" />
          <button type="button" className="sidebar-home-button" onClick={onHome}>
            Home
          </button>
        </div>

        <nav className="nav-stack">
          {(["Projects", "Views", "Issues", "Page Pilot"] as const).map((item) => (
            <button key={item} className={item === activeNav ? "nav-item active" : "nav-item"} onClick={() => onNavigate(item)}>
              <span>{primaryNavLabel(item)}</span>
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
              <span className="section-label">Workspaces</span>
            </summary>
            <nav className="workspace-stack" aria-label="Project workspaces">
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
                        <small>{targetItems.length} items</small>
                      </span>
                    </button>
                    <button
                      type="button"
                      className="workspace-config-button"
                      aria-label={`Configure ${label}`}
                      title="Workspace config"
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
            <span className="section-label">Connections</span>
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
                <small>{connections[provider.id].status === "connected" ? "on" : "off"}</small>
              </button>
            ))}
          </div>
        </details>
      </aside>

      <section className="workbench">
        <header className={activeWorkItemDetail ? "topbar detail-mode" : "topbar"}>
          {activeWorkItemDetail ? (
            <>
              <nav className="detail-breadcrumb" aria-label="Issue detail breadcrumb">
                <button type="button" onClick={onBackToWorkItems}>
                  Work items
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
                <ThemeToggle uiTheme={uiTheme} onToggleTheme={onToggleTheme} />
                <button type="button" onClick={() => navigator.clipboard?.writeText(activeWorkItemDetail.target)}>
                  Copy target
                </button>
                <button type="button" className="primary-action" disabled={detailRunDisabled} onClick={onRunDetail}>
                  {detailRunLabel}
                </button>
              </div>
            </>
          ) : (
            <>
              <div>
                <p className="section-label">Omega</p>
                <h1>{primaryNavLabel(activeNav)}</h1>
              </div>
              <div className="search-control">
                <input
                  className="command-input"
                  value={searchQuery}
                  onChange={(event) => onSearchChange(event.currentTarget.value)}
                  placeholder={topbarSearchPlaceholder(activeNav)}
                />
                <button type="button">Search</button>
              </div>
              <div className="topbar-actions">
                {activeNav === "Issues" ? (
                  <button type="button" className="topbar-create" onClick={onNewRequirement}>
                    <span className="topbar-create-label">New requirement</span>
                  </button>
                ) : null}
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
  return (
    <button type="button" className="theme-toggle" onClick={onToggleTheme} aria-label={`Switch to ${uiTheme === "light" ? "night" : "day"} mode`}>
      <span aria-hidden="true">{uiTheme === "light" ? "☾" : "☼"}</span>
      {uiTheme === "light" ? "Night" : "Day"}
    </button>
  );
}

function runnerMessageSummary(message: string): string {
  if (message.length <= 72) return message;
  return `${message.slice(0, 69)}...`;
}
