---
id: devflow-fast-rework-test
name: DevFlow Fast Rework Test
description: 用于测试 review 失败后快速回到 rework 的工作流。
---

workflow: devflow-fast-rework-test
stages:
  - requirement: requirement
  - implementation: coding + testing
  - review: review
  - rework: coding + testing, then review
  - human_review: human gate
  - merging: delivery
  - done: delivery

states:
  - id: requirement
    title: Requirement
    agentId: requirement
    agents: [requirement]
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
    agents: [coding, testing]
    actions:
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
      passed: review
      failed: rework

  - id: review
    title: Review
    agentId: review
    agents: [review]
    actions:
      - id: review_diff
        type: run_review
        agent: review
        prompt: review
        verdicts:
          approved: human_review
          changes_requested: rework
          needs_human_info: human_review

  - id: rework
    title: Rework
    agentId: coding
    agents: [coding, testing]
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
    transitions:
      passed: review
      failed: human_review

  - id: human_review
    title: Human Review
    agentId: delivery
    agents: [human, delivery]
    humanGate: true
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
    actions:
      - id: collect_delivery_proof
        type: collect_proof
        agent: delivery
