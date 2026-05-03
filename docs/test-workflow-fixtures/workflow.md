---
id: devflow-pr-test
name: DevFlow PR Test
description: 用于本地测试的默认交付工作流。
---

workflow: devflow-pr-test
stages:
  - requirement: requirement
  - implementation: architect + coding + testing
  - code_review: review
  - rework: coding + testing, then code_review
  - human_review: human gate
  - merging: delivery
  - done: delivery

states:
  - id: requirement
    title: Requirement
    agentId: requirement
    agents: [requirement]
    outputArtifacts: [structured-requirement, acceptance-criteria, dispatch-plan]
    actions:
      - id: capture_requirement
        type: write_requirement_artifact
        agent: requirement
        prompt: requirement
    transitions:
      passed: implementation

  - id: implementation
    title: Implementation
    agentId: coding
    agents: [architect, coding, testing]
    inputArtifacts: [structured-requirement, acceptance-criteria, repository-target]
    outputArtifacts: [technical-plan, code-diff, changed-files, test-report, pull-request]
    actions:
      - id: architecture_handoff
        type: run_agent
        agent: architect
        prompt: architect
      - id: implement_change
        type: run_agent
        agent: coding
        prompt: coding
        requiresDiff: true
      - id: validate_repository
        type: run_validation
        agent: testing
        prompt: testing
      - id: publish_pull_request
        type: ensure_pr
        agent: delivery
    transitions:
      passed: code_review
      failed: rework

  - id: code_review
    title: Code Review
    agentId: review
    agents: [review]
    inputArtifacts: [code-diff, changed-files, test-report]
    outputArtifacts: [review-report, blocking-risks, merge-recommendation]
    actions:
      - id: review_diff
        type: run_review
        agent: review
        prompt: review
        diffSource: local_diff
        verdicts:
          approved: human_review
          changes_requested: rework
          needs_human_info: human_review

  - id: rework
    title: Rework
    agentId: coding
    agents: [coding, testing]
    inputArtifacts: [review-report, human-decision, check-log, code-diff]
    outputArtifacts: [rework-checklist, code-diff, changed-files, test-report]
    actions:
      - id: build_rework_checklist
        type: build_rework_checklist
        agent: master
      - id: apply_rework
        type: run_agent
        agent: coding
        prompt: rework
        requiresDiff: true
      - id: validate_rework
        type: run_validation
        agent: testing
        prompt: testing
      - id: update_pull_request
        type: ensure_pr
        agent: delivery
    transitions:
      passed: code_review
      failed: human_review

  - id: human_review
    title: Human Review
    agentId: delivery
    agents: [human, review, delivery]
    humanGate: true
    inputArtifacts: [pull-request, review-report, test-report, run-report]
    outputArtifacts: [human-decision, review-notes]
    actions:
      - id: wait_human_decision
        type: human_gate
        agent: human
    transitions:
      approved: merging
      changes_requested: rework

  - id: merging
    title: Merging
    agentId: delivery
    agents: [delivery]
    inputArtifacts: [human-decision, pull-request]
    outputArtifacts: [merge-report, proof-records]
    actions:
      - id: merge_pull_request
        type: merge_pr
        agent: delivery
    transitions:
      passed: done
      failed: human_review

  - id: done
    title: Done
    agentId: delivery
    agents: [delivery]
    outputArtifacts: [handoff-bundle, proof-records]
    actions:
      - id: collect_delivery_proof
        type: collect_proof
        agent: delivery
