import type { GitHubRepositoryInfo, PipelineRecordInfo } from "../omegaControlApiClient";
import type { ProjectRecord, RepositoryTarget, WorkItem } from "../core";

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
};

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
  onSelectGitHubRepository
}: ProjectSurfaceProps) {
  return (
    <section className="project-surface">
      <div className="overview-panel project-hero-panel">
        <div className="project-hero-copy">
          <span className="section-label">Project</span>
          <h2>{primaryProject?.name ?? "Omega Project"}</h2>
          <p>
            {primaryProject?.description ||
              "A delivery space that groups requirements, repository workspaces, agent pipelines, human review, and delivery proof."}
          </p>
          {repositoryTargets.length > 0 ? (
            <div className="target-chip-list" aria-label="Project repository targets">
              {repositoryTargets.map((target) => (
                <span key={target.id}>{target.kind === "github" ? `${target.owner}/${target.repo}` : target.path}</span>
              ))}
            </div>
          ) : null}
          <button type="button" className="project-config-link" onClick={onOpenProjectConfig}>
            Project config
          </button>
        </div>
        <div className="project-stat-grid" aria-label="Project delivery summary">
          <span>
            <small>Work items</small>
            <strong>{workItems.length}</strong>
          </span>
          <span>
            <small>Repository workspaces</small>
            <strong>{repositoryTargetCount}</strong>
          </span>
          <span>
            <small>Pipeline runs</small>
            <strong>{pipelines.length}</strong>
          </span>
        </div>
        <div className="project-flow-strip" aria-label="Project delivery flow">
          <span>Requirements</span>
          <span>Repository Workspace</span>
          <span>Agent Pipeline</span>
          <span>Human Review</span>
          <span>Delivery</span>
        </div>
        <div className="project-create-form" aria-label="Create project">
          <label>
            <span>Project name</span>
            <input
              value={newProjectName}
              onChange={(event) => onNewProjectNameChange(event.currentTarget.value)}
              placeholder="New delivery project"
            />
          </label>
          <label>
            <span>Description</span>
            <input
              value={newProjectDescription}
              onChange={(event) => onNewProjectDescriptionChange(event.currentTarget.value)}
              placeholder="Optional project context"
            />
          </label>
          <button type="button" className="primary-action" onClick={onCreateProject} disabled={!newProjectName.trim()}>
            Create project
          </button>
        </div>
      </div>

      {activeRepositoryWorkspace ? (
        <div className="overview-panel repository-workspace-panel repository-detail-panel">
          <div className="control-card-header">
            <div>
              <span className="section-label">Repository workspace</span>
              <h2>{activeRepositoryWorkspaceLabel}</h2>
            </div>
            <div className="repository-actions">
              <button
                className="primary-action"
                disabled={syncingRepositoryKey === activeRepositoryWorkspaceKey}
                onClick={onSyncActiveRepository}
              >
                {syncingRepositoryKey === activeRepositoryWorkspaceKey ? "Syncing..." : "Sync GitHub issues"}
              </button>
              <button onClick={onOpenWorkItems} disabled={activeRepositoryWorkspaceItems.length === 0}>
                View work items
              </button>
            </div>
          </div>
          {repositorySyncMessage ? (
            <p className="sync-feedback" role="status">
              {repositorySyncMessage}
            </p>
          ) : null}
          <div className="workspace-metrics">
            <span>{activeRepositoryWorkspaceItems.length} work items</span>
            <span>{activeRepositoryWorkspacePipelines.length} pipelines</span>
            <span>0 pull requests</span>
          </div>
          <div className="repository-workspace-grid">
            <section>
              <span className="section-label">Work items</span>
              {activeRepositoryWorkspaceItems.length > 0 ? (
                <div className="imported-issue-list">
                  {activeRepositoryWorkspaceItems.slice(0, 8).map((item) => (
                    <div key={item.id}>
                      <span>{item.title}</span>
                      <small>{item.sourceExternalRef ?? item.key}</small>
                    </div>
                  ))}
                </div>
              ) : (
                <p>No work items synced yet.</p>
              )}
            </section>
            <section>
              <span className="section-label">Pull requests</span>
              <p>No pull requests linked yet.</p>
            </section>
          </div>
        </div>
      ) : null}

      <div className="overview-panel repository-panel">
        <div className="control-card-header">
          <div>
            <span className="section-label">Repository workspace</span>
            <h2>Attach GitHub repositories</h2>
            <p>Choose the repository targets this Project is allowed to run agents inside.</p>
          </div>
          <button onClick={onRefreshRepositories} disabled={githubRepositoriesLoading}>
            {githubRepositoriesLoading ? "Loading..." : "Refresh repositories"}
          </button>
        </div>
        <div className="repository-picker">
          <label>
            <span>Search repositories</span>
            <input
              value={githubRepositoryQuery}
              onChange={(event) => onRepositoryQueryChange(event.currentTarget.value)}
              placeholder="Search by repo name or description"
            />
          </label>
          <div className="repository-actions">
            <button disabled={!githubRepoOwner || !githubRepoName} className="primary-action" onClick={onCreateOrOpenWorkspace}>
              {selectedRepositoryBound ? "Open workspace" : "Create workspace"}
            </button>
          </div>
        </div>
        <div className="repository-list" aria-label="GitHub repositories">
          {filteredGitHubRepositories.length === 0 ? (
            <p>{githubRepositoriesLoading ? "Loading repositories..." : "No repositories loaded. Refresh repositories after connecting GitHub."}</p>
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
                    <small>{repository.description || "No description"}</small>
                  </span>
                  <small>{repository.isPrivate ? "private" : "public"} · {repository.defaultBranchRef?.name || "branch unknown"}</small>
                </button>
              );
            })
          )}
        </div>
        {githubRepoInfo ? (
          <div className="repository-summary">
            <strong>{githubRepoInfo.nameWithOwner ?? `${githubRepoInfo.owner?.login ?? githubRepoOwner}/${githubRepoInfo.name}`}</strong>
            <span>{githubRepoInfo.description || "No repository description"}</span>
            <small>
              {selectedRepositoryBound ? "Attached to project" : "Not attached yet"} · {githubRepoInfo.defaultBranchRef?.name ?? "default branch unknown"}
            </small>
          </div>
        ) : null}
      </div>
    </section>
  );
}
