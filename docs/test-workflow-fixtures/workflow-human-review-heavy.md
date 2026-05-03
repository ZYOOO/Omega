---
id: devflow-human-review-test
name: DevFlow Human Review Test
description: 用于测试人工审核、通知、request changes 和合并前确认的工作流。
---

workflow: devflow-human-review-test
stages:
  - requirement: requirement
  - implementation: architect + coding + testing
  - review_packet: review + delivery
  - human_review: human gate
  - rework: coding + testing, then review_packet
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
    agents: [architect, coding, testing]
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
      passed: review_packet
      failed: rework

  - id: review_packet
    title: Review Packet
    agentId: review
    agents: [review, delivery]
    actions:
      - id: review_diff
        type: run_review
        agent: review
        prompt: review
        verdicts:
          approved: human_review
          changes_requested: rework
      - id: build_review_packet
        type: build_review_packet
        agent: delivery
    transitions:
      passed: human_review
      failed: rework

  - id: human_review
    title: Human Review
    agentId: delivery
    agents: [human, delivery]
    humanGate: true
    actions:
      - id: notify_human_review
        type: notify_review
        agent: delivery
      - id: wait_human_decision
        type: human_gate
        agent: human
    transitions:
      approved: merging
      changes_requested: rework

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
      - id: update_pull_request
        type: ensure_pr
        agent: delivery
    transitions:
      passed: review_packet
      failed: human_review

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
