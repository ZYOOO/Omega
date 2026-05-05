import type { GitHubRepositoryInfo, PipelineRecordInfo } from "../omegaControlApiClient";
import type { ProjectRecord, RepositoryTarget, WorkItem } from "../core";
import { useI18n } from "../i18n";

type ProjectSurfaceProps = {
  primaryProject?: ProjectRecord;
  repositoryTargets: RepositoryTarget[];
  repositoryTargetCount: number;
  workItems: WorkItem[];
  pipelines: PipelineRecordInfo[];
  activeRepositoryWorkspace?: RepositoryTarget;
  activeRepositoryWorkspaceLabel: string;
  activeRepositoryWorkspaceKey: string;
  activeRepositoryWorkspaceItems: WorkItem[];
  activeRepositoryWorkspacePipelines: PipelineRecordInfo[];
  repositorySyncMessage: string;
  syncingRepositoryKey: string;
  newProjectName: string;
  newProjectDescription: string;
  githubRepositoriesLoading: boolean;
  githubRepositoryQuery: string;
  githubRepoOwner: string;
  githubRepoName: string;
  selectedRepositoryBound: boolean;
  localRepositoryPath: string;
  localRepositoryMessage: string;
  selectedLocalRepositoryBound: boolean;
  filteredGitHubRepositories: GitHubRepositoryInfo[];
  githubRepoInfo: GitHubRepositoryInfo | null;
  onOpenProjectConfig: () => void;
  onSyncActiveRepository: () => void;
  onOpenWorkItems: () => void;
  onNewProjectNameChange: (value: string) => void;
  onNewProjectDescriptionChange: (value: string) => void;
  onCreateProject: () => void;
  onRefreshRepositories: () => void;
  onRepositoryQueryChange: (value: string) => void;
  onCreateOrOpenWorkspace: () => void;
  onSelectGitHubRepository: (repository: GitHubRepositoryInfo) => void;
  onLocalRepositoryPathChange: (value: string) => void;
  onChooseLocalRepositoryFolder: () => void;
  onCreateOrOpenLocalWorkspace: () => void;
};

function visibleExternalReference(value?: string): string {
  const raw = value?.trim() ?? "";
  if (!raw) return "";
  if (/^(?:page-pilot:)?item_(?:manual|page_pilot)_/i.test(raw)) return "";
  if (/^req_item_manual_/i.test(raw)) return "";
  if (/^pipeline_item_(?:manual|page_pilot)_/i.test(raw)) return "";
  return raw;
}

export function ProjectSurface({
  primaryProject,
  repositoryTargets,
  repositoryTargetCount,
  workItems,
  pipelines,
  activeRepositoryWorkspace,
  activeRepositoryWorkspaceLabel,
  activeRepositoryWorkspaceKey,
  activeRepositoryWorkspaceItems,
  activeRepositoryWorkspacePipelines,
  repositorySyncMessage,
  syncingRepositoryKey,
  newProjectName,
  newProjectDescription,
  githubRepositoriesLoading,
  githubRepositoryQuery,
  githubRepoOwner,
  githubRepoName,
  selectedRepositoryBound,
  localRepositoryPath,
  localRepositoryMessage,
  selectedLocalRepositoryBound,
  filteredGitHubRepositories,
  githubRepoInfo,
  onOpenProjectConfig,
  onSyncActiveRepository,
  onOpenWorkItems,
  onNewProjectNameChange,
  onNewProjectDescriptionChange,
  onCreateProject,
  onRefreshRepositories,
  onRepositoryQueryChange,
  onCreateOrOpenWorkspace,
  onSelectGitHubRepository,
  onLocalRepositoryPathChange,
  onChooseLocalRepositoryFolder,
  onCreateOrOpenLocalWorkspace
}: ProjectSurfaceProps) {
  const { t } = useI18n();
  return (
    <section className="project-surface">
      <div className="overview-panel project-hero-panel">
        <div className="project-hero-copy">
          <span className="section-label">{t("Project")}</span>
          <h2>{primaryProject?.name ?? "Omega Project"}</h2>
          <p>
            {primaryProject?.description ||
              t("A delivery space that groups requirements, repository workspaces, agent pipelines, human review, and delivery proof.")}
          </p>
          {repositoryTargets.length > 0 ? (
            <div className="target-chip-list" aria-label="Project repository targets">
              {repositoryTargets.map((target) => (
                <span key={target.id}>{target.kind === "github" ? `${target.owner}/${target.repo}` : target.path}</span>
              ))}
            </div>
          ) : null}
          <button type="button" className="project-config-link" onClick={onOpenProjectConfig}>
            {t("Project config")}
          </button>
        </div>
        <div className="project-stat-grid" aria-label="Project delivery summary">
          <span>
            <small>{t("Work items")}</small>
            <strong>{workItems.length}</strong>
          </span>
          <span>
            <small>{t("Repository workspaces")}</small>
            <strong>{repositoryTargetCount}</strong>
          </span>
          <span>
            <small>{t("Pipeline runs")}</small>
            <strong>{pipelines.length}</strong>
          </span>
        </div>
        <div className="project-create-form" aria-label="Create project">
          <label>
          <span>{t("Project name")}</span>
            <input
              value={newProjectName}
              onChange={(event) => onNewProjectNameChange(event.currentTarget.value)}
              placeholder={t("New delivery project")}
            />
          </label>
          <label>
          <span>{t("Description")}</span>
            <input
              value={newProjectDescription}
              onChange={(event) => onNewProjectDescriptionChange(event.currentTarget.value)}
              placeholder={t("Optional project context")}
            />
          </label>
          <button type="button" className="primary-action" onClick={onCreateProject} disabled={!newProjectName.trim()}>
            {t("Create project")}
          </button>
        </div>
      </div>

      {activeRepositoryWorkspace ? (
        <div className="overview-panel repository-workspace-panel repository-detail-panel">
          <div className="control-card-header">
            <div>
              <span className="section-label">{t("Repository workspace")}</span>
              <h2>{activeRepositoryWorkspaceLabel}</h2>
            </div>
            <div className="repository-actions">
              <button
                className="primary-action"
                disabled={activeRepositoryWorkspace.kind !== "github" || syncingRepositoryKey === activeRepositoryWorkspaceKey}
                onClick={onSyncActiveRepository}
              >
                {activeRepositoryWorkspace.kind !== "github"
                  ? t("Local workspace")
                  : syncingRepositoryKey === activeRepositoryWorkspaceKey
                    ? t("Syncing...")
                    : t("Sync GitHub issues")}
              </button>
              <button onClick={onOpenWorkItems} disabled={activeRepositoryWorkspaceItems.length === 0}>
                {t("View work items")}
              </button>
            </div>
          </div>
          {repositorySyncMessage ? (
            <p className="sync-feedback" role="status">
              {repositorySyncMessage}
            </p>
          ) : null}
          <div className="workspace-metrics">
            <span>{t("{count} work items", { count: activeRepositoryWorkspaceItems.length })}</span>
            <span>{activeRepositoryWorkspacePipelines.length} pipelines</span>
            <span>0 pull requests</span>
          </div>
          <div className="repository-workspace-grid">
            <section>
              <span className="section-label">{t("Work items")}</span>
              {activeRepositoryWorkspaceItems.length > 0 ? (
                <div className="imported-issue-list">
                  {activeRepositoryWorkspaceItems.slice(0, 8).map((item) => (
                    <div key={item.id}>
                      <span>{item.title}</span>
                      <small>{visibleExternalReference(item.sourceExternalRef) || item.key}</small>
                    </div>
                  ))}
                </div>
              ) : (
                <p>{t("No work items synced yet.")}</p>
              )}
            </section>
            <section>
              <span className="section-label">Pull requests</span>
              <p>{t("No pull requests linked yet.")}</p>
            </section>
          </div>
        </div>
      ) : null}

      <div className="overview-panel repository-panel">
        <div className="control-card-header">
          <div>
            <span className="section-label">{t("Repository workspace")}</span>
            <h2>{t("Attach GitHub repositories")}</h2>
            <p>{t("Choose the repository targets this Project is allowed to run agents inside.")}</p>
          </div>
          <button onClick={onRefreshRepositories} disabled={githubRepositoriesLoading}>
            {githubRepositoriesLoading ? t("Loading...") : t("Refresh repositories")}
          </button>
        </div>
        <div className="repository-picker">
          <label>
            <span>{t("Search repositories")}</span>
            <input
              value={githubRepositoryQuery}
              onChange={(event) => onRepositoryQueryChange(event.currentTarget.value)}
              placeholder={t("Search by repo name or description")}
            />
          </label>
          <div className="repository-actions">
            <button disabled={!githubRepoOwner || !githubRepoName} className="primary-action" onClick={onCreateOrOpenWorkspace}>
              {selectedRepositoryBound ? t("Open workspace") : t("Create workspace")}
            </button>
          </div>
        </div>
        <div className="repository-list" aria-label="GitHub repositories">
          {filteredGitHubRepositories.length === 0 ? (
            <p>{githubRepositoriesLoading ? t("Loading repositories...") : t("No repositories loaded. Refresh repositories after connecting GitHub.")}</p>
          ) : (
            filteredGitHubRepositories.slice(0, 20).map((repository) => {
              const nameWithOwner = repository.nameWithOwner ?? `${repository.owner?.login ?? ""}/${repository.name}`;
              const selected = nameWithOwner === `${githubRepoOwner}/${githubRepoName}`;
              return (
                <button
                  key={nameWithOwner}
                  className={selected ? "repository-option selected" : "repository-option"}
                  onClick={() => onSelectGitHubRepository(repository)}
                >
                  <span>
                    <strong>{nameWithOwner}</strong>
                    <small>{repository.description || t("No description")}</small>
                  </span>
                  <small>{repository.isPrivate ? t("private") : t("public")} · {repository.defaultBranchRef?.name || t("branch unknown")}</small>
                </button>
              );
            })
          )}
        </div>
        {githubRepoInfo ? (
          <div className="repository-summary">
            <strong>{githubRepoInfo.nameWithOwner ?? `${githubRepoInfo.owner?.login ?? githubRepoOwner}/${githubRepoInfo.name}`}</strong>
            <span>{githubRepoInfo.description || t("No repository description")}</span>
            <small>
              {selectedRepositoryBound ? t("Attached to project") : t("Not attached yet")} · {githubRepoInfo.defaultBranchRef?.name ?? t("default branch unknown")}
            </small>
          </div>
        ) : null}
      </div>

      <div className="overview-panel repository-panel local-repository-panel">
        <div className="control-card-header">
          <div>
            <span className="section-label">{t("Local repository")}</span>
            <h2>{t("Attach project directory")}</h2>
            <p>{t("Choose an existing local git worktree that Omega can use as a repository workspace.")}</p>
          </div>
        </div>
        <div className="repository-picker local-repository-picker">
          <label>
            <span>{t("Project directory")}</span>
            <input
              value={localRepositoryPath}
              onChange={(event) => onLocalRepositoryPathChange(event.currentTarget.value)}
              placeholder="/Users/you/Projects/app"
            />
          </label>
          <div className="repository-actions">
            <button type="button" onClick={onChooseLocalRepositoryFolder}>
              {t("Choose folder")}
            </button>
            <button
              type="button"
              disabled={!localRepositoryPath.trim()}
              className="primary-action"
              onClick={onCreateOrOpenLocalWorkspace}
            >
              {selectedLocalRepositoryBound ? t("Open workspace") : t("Create workspace")}
            </button>
          </div>
        </div>
        {localRepositoryMessage ? (
          <p className="sync-feedback" role="status">
            {localRepositoryMessage}
          </p>
        ) : null}
      </div>
    </section>
  );
}
