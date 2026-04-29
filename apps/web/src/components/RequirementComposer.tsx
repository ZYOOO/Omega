import type { ReactNode } from "react";

type RequirementDescriptionMode = "write" | "preview";

type RequirementComposerProps = {
  variant: "inline" | "empty";
  title: string;
  description: string;
  assignee: string;
  target: string;
  hasRepositoryWorkspace: boolean;
  repositoryWorkspaceLabel?: string;
  isExpanded: boolean;
  descriptionMode: RequirementDescriptionMode;
  isCreating: boolean;
  descriptionPreview: ReactNode;
  onTitleChange: (value: string) => void;
  onDescriptionChange: (value: string) => void;
  onAssigneeChange: (value: string) => void;
  onTargetChange: (value: string) => void;
  onTitleFocus: () => void;
  onDescriptionModeChange: (mode: RequirementDescriptionMode) => void;
  onCreate: () => void;
};

const requirementAgentOptions = ["requirement", "architect", "coding", "testing", "review", "delivery"];

export function RequirementComposer({
  variant,
  title,
  description,
  assignee,
  target,
  hasRepositoryWorkspace,
  repositoryWorkspaceLabel,
  isExpanded,
  descriptionMode,
  isCreating,
  descriptionPreview,
  onTitleChange,
  onDescriptionChange,
  onAssigneeChange,
  onTargetChange,
  onTitleFocus,
  onDescriptionModeChange,
  onCreate
}: RequirementComposerProps) {
  if (variant === "empty") {
    return (
      <div className="empty-create">
        <div className="empty-copy">
          <h2>Create your first work item</h2>
          <p>Start the Workboard with a concrete requirement Mission Control can turn into an operation.</p>
        </div>
        <input value={title} onChange={(event) => onTitleChange(event.currentTarget.value)} placeholder="Work item title" />
        <textarea
          value={description}
          onChange={(event) => onDescriptionChange(event.currentTarget.value)}
          placeholder="Optional description"
        />
        {hasRepositoryWorkspace ? null : (
          <input
            value={target}
            onChange={(event) => onTargetChange(event.currentTarget.value)}
            placeholder="Local repository path or GitHub repo URL"
          />
        )}
        <div className="empty-create-footer">
          <RequirementAssigneeSelect assignee={assignee} onAssigneeChange={onAssigneeChange} />
          <CreateRequirementButton isCreating={isCreating} label="Create item" onCreate={onCreate} />
        </div>
      </div>
    );
  }

  return (
    <section className="inline-create">
      <div className="inline-create-form">
        <label className="inline-title-field">
          <span>Add a title *</span>
          <input
            value={title}
            onFocus={onTitleFocus}
            onChange={(event) => onTitleChange(event.currentTarget.value)}
            placeholder="Title"
          />
        </label>
        <RequirementAssigneeSelect assignee={assignee} onAssigneeChange={onAssigneeChange} />
        <CreateRequirementButton isCreating={isCreating} label="Create" onCreate={onCreate} />
        {isExpanded ? (
          <div className="description-composer">
            <span>Add a description</span>
            <div className="composer-tabs">
              <button
                type="button"
                className={descriptionMode === "write" ? "active" : ""}
                onClick={() => onDescriptionModeChange("write")}
              >
                Write
              </button>
              <button
                type="button"
                className={descriptionMode === "preview" ? "active" : ""}
                onClick={() => onDescriptionModeChange("preview")}
              >
                Preview
              </button>
            </div>
            {descriptionMode === "write" ? (
              <textarea
                value={description}
                onChange={(event) => onDescriptionChange(event.currentTarget.value)}
                placeholder="Type your description here..."
              />
            ) : (
              <div className="description-preview" aria-label="Description preview">
                {descriptionPreview}
              </div>
            )}
          </div>
        ) : null}
      </div>
      <aside className="inline-create-note">
        <span>Requirement to item</span>
        <p>
          {hasRepositoryWorkspace && repositoryWorkspaceLabel
            ? `A requirement will be stored under ${repositoryWorkspaceLabel}, then converted into its first executable item.`
            : "No repository workspace selected."}
        </p>
      </aside>
    </section>
  );
}

function RequirementAssigneeSelect({
  assignee,
  onAssigneeChange
}: {
  assignee: string;
  onAssigneeChange: (value: string) => void;
}) {
  return (
    <select value={assignee} onChange={(event) => onAssigneeChange(event.currentTarget.value)}>
      {requirementAgentOptions.map((agent) => (
        <option key={agent}>{agent}</option>
      ))}
    </select>
  );
}

function CreateRequirementButton({
  isCreating,
  label,
  onCreate
}: {
  isCreating: boolean;
  label: string;
  onCreate: () => void;
}) {
  return (
    <button type="button" className="primary-action" onClick={onCreate} disabled={isCreating}>
      {isCreating ? "Creating..." : label}
    </button>
  );
}
