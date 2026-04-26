---
id: devflow-pr
name: DevFlow PR cycle
description: Local-first Omega workflow: intake, implementation, review, human gate, delivery.
stages:
  - id: todo
    title: Todo intake
    agentId: requirement
    agents: [requirement]
    humanGate: false
    outputArtifacts: [structured-requirement, acceptance-criteria, dispatch-plan]
  - id: in_progress
    title: Implementation and PR
    agentId: coding
    agents: [architect, coding, testing]
    humanGate: false
    outputArtifacts: [code-diff, changed-files, implementation-notes, test-report]
  - id: code_review_round_1
    title: Code Review Round 1
    agentId: review
    agents: [review]
    humanGate: false
    outputArtifacts: [review-report, blocking-risks, merge-recommendation]
  - id: code_review_round_2
    title: Code Review Round 2
    agentId: review
    agents: [review]
    humanGate: false
    outputArtifacts: [review-report, blocking-risks, merge-recommendation]
  - id: rework
    title: Rework
    agentId: coding
    agents: [coding, testing]
    humanGate: false
    inputArtifacts: [review-report, blocking-risks, code-diff, changed-files]
    outputArtifacts: [code-diff, changed-files, implementation-notes, test-report]
  - id: human_review
    title: Human Review
    agentId: delivery
    agents: [human, review, delivery]
    humanGate: true
    outputArtifacts: [human-decision, review-notes]
  - id: merging
    title: Merging
    agentId: delivery
    agents: [delivery]
    humanGate: false
    outputArtifacts: [pull-request, delivery-summary, rollback-plan]
  - id: done
    title: Done
    agentId: delivery
    agents: [delivery]
    humanGate: false
    outputArtifacts: [handoff-bundle, proof-records]
reviewRounds:
  - stageId: code_review_round_1
    artifact: code-review-round-1.md
    focus: correctness, regressions, and acceptance criteria
    diffSource: local_diff
    changesRequestedTo: rework
    needsHumanInfoTo: human_review
  - stageId: code_review_round_2
    artifact: code-review-round-2.md
    focus: maintainability, tests, edge cases, and delivery readiness
    diffSource: pr_diff
    changesRequestedTo: rework
    needsHumanInfoTo: human_review
runtime:
  maxReviewCycles: 3
transitions:
  - from: todo
    on: passed
    to: in_progress
  - from: in_progress
    on: passed
    to: code_review_round_1
  - from: code_review_round_1
    on: approved
    to: code_review_round_2
  - from: code_review_round_1
    on: changes_requested
    to: rework
  - from: code_review_round_2
    on: approved
    to: human_review
  - from: code_review_round_2
    on: changes_requested
    to: rework
  - from: rework
    on: passed
    to: code_review_round_1
  - from: human_review
    on: approved
    to: merging
  - from: merging
    on: passed
    to: done
---

# Omega DevFlow PR Workflow

This is the default workflow contract for a repository-scoped Omega item.

Runtime policy:

1. Work only inside the isolated repository workspace.
2. The item must be bound to a repository target before any runner starts.
3. Implementation must produce a real git diff in the target repository.
4. Validation must run before review.
5. Review agents are read-only and must emit one explicit verdict line.
6. `CHANGES_REQUESTED` is a normal workflow transition into Rework, not an execution failure.
7. Rework runs in the same repository workspace, same branch, and same pull request, with the review report as input.
8. Human Review is a blocking gate. Delivery and merge must wait for an explicit approval.
9. Reject sends the work back for rework with the human reason preserved.

Review verdict contract:

```text
Verdict: APPROVED
```

or

```text
Verdict: CHANGES_REQUESTED
```

or

```text
Verdict: NEEDS_HUMAN_INFO
```
