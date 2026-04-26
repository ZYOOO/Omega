# Omega Persistence Schema

Omega currently uses a table-shaped workspace database as the durable Product Layer storage model. In the primary local flow it is persisted through the local Omega server into SQLite, with browser-local storage retained only as a fallback when the local server is not configured.

The table model is intentionally normalized so it can move to SQLite/Postgres without changing product concepts.

## Local Development Storage

Primary local storage:

```text
.omega/omega.db
```

Fallback browser storage key:

```text
omega.workspace.<runId>.v1
```

## Tables

### projects

Stores the product or engineering goal that owns work. A Project is not a repository; it can bind one or more repository targets.

| Column | Type | Notes |
| --- | --- | --- |
| id | text | Primary key |
| name | text | Project display name |
| description | text | Project description |
| team | text | Owning team |
| status | text | Active, Paused, Completed |
| labels | text[] | Project labels |
| repositoryTargets | json | GitHub/local repository targets bound to this project |
| defaultRepositoryTargetId | text? | Default target for new work items |
| createdAt | timestamp | ISO string |
| updatedAt | timestamp | ISO string |

### requirements

Stores user needs and imported external sources. A Requirement owns one or more executable Items; GitHub issues and future Feishu messages are represented here as sources, not as the internal execution unit.

| Column | Type | Notes |
| --- | --- | --- |
| id | text | Primary key |
| projectId | text | FK to projects.id |
| repositoryTargetId | text? | Repository target selected for this requirement, if any |
| source | text | manual, github_issue, feishu_message, api, ai_generated |
| sourceExternalRef | text? | External pointer such as `owner/repo#123` |
| title | text | Requirement title |
| rawText | text | Original user-provided text |
| structuredJson | json | Structured requirement artifact |
| acceptanceCriteriaJson | json | User-visible acceptance criteria |
| risksJson | json | Known risks / open questions |
| status | text | draft, structured, approved, converted, archived |
| createdAt | timestamp | ISO string |
| updatedAt | timestamp | ISO string |

### workItems

Stores Omega internal execution items. An Item is the smallest unit that can start a Pipeline run.

| Column | Type | Notes |
| --- | --- | --- |
| id | text | Primary key |
| requirementId | text? | FK to requirements.id |
| projectId | text | FK to projects.id |
| key | text | Display key, e.g. OMG-1 |
| title | text | Work item title |
| description | text | Item detail, usually copied or refined from the owning Requirement |
| status | text | Ready, In Review, Backlog, Done, Blocked |
| priority | text | No priority, Low, Medium, High, Urgent |
| assignee | text | Agent or human assignee |
| labels | text[] | Work item labels |
| team | text | Owning team |
| stageId | text | Pipeline stage id |
| target | text | Due target |
| source | text | manual, github_issue, feishu_message, ai_generated |
| sourceExternalRef | text? | External pointer such as `owner/repo#123` |
| repositoryTargetId | text? | Repository target this item should execute against |
| branchName | text? | Working branch when execution starts |
| acceptanceCriteria | text[] | User-visible verification criteria |
| parentItemId | text? | Optional parent item |
| blockedByItemIds | text[] | Item dependency ids |
| createdAt | timestamp | ISO string |
| updatedAt | timestamp | ISO string |

### missionControlStates

Stores the current reducer snapshot for each mission run.

| Column | Type | Notes |
| --- | --- | --- |
| runId | text | Primary key |
| projectId | text | FK to projects.id |
| workItems | json | Current Workboard projection |
| events | json | Current event list |
| syncIntents | json | Current sync plan |
| updatedAt | timestamp | ISO string |

### missionEvents

Append-only log of Mission Control events.

| Column | Type | Notes |
| --- | --- | --- |
| id | text | Primary key |
| runId | text | FK to missionControlStates.runId |
| sequence | integer | Event order |
| event | json | Typed MissionEvent payload |

### syncIntents

Stores connector actions derived from mission events.

| Column | Type | Notes |
| --- | --- | --- |
| id | text | Primary key |
| runId | text | FK to missionControlStates.runId |
| sequence | integer | Intent order |
| intent | json | Typed SyncIntent payload |

### connections

Stores provider connection state and granted permissions. OAuth tokens should continue to use `TokenStore`; this table stores product-visible connection metadata only.

| Column | Type | Notes |
| --- | --- | --- |
| providerId | text | Primary key |
| status | text | not-connected, connected, needs-config |
| grantedPermissions | text[] | Permission ids |
| connectedAs | text? | Account label |
| updatedAt | timestamp | ISO string |

### uiPreferences

Stores durable user workspace preferences. Draft text and search query are intentionally not persisted.

| Column | Type | Notes |
| --- | --- | --- |
| id | text | Primary key, `default` |
| activeNav | text | Projects, Views, Work items. Stored legacy value: `Issues` |
| selectedProviderId | text | Current provider |
| selectedWorkItemId | text | Current selected work item |
| inspectorOpen | boolean | Right panel open state |
| activeInspectorPanel | text | properties, provider |
| runnerPreset | text | local-proof, demo-code, codex |
| statusFilter | text | All or WorkItemStatus |
| assigneeFilter | text | All or assignee |
| sortDirection | text | asc, desc |
| collapsedGroups | text[] | Collapsed work item statuses |
