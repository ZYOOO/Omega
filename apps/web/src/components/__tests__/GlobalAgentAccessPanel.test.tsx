import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../../i18n";
import type { LocalCapabilityInfo } from "../../omegaControlApiClient";
import { GlobalAgentAccessPanel } from "../GlobalAgentAccessPanel";

const capabilities: LocalCapabilityInfo[] = [
  {
    id: "codex",
    command: "codex",
    category: "ai-runner",
    description: "Codex",
    available: true,
    required: false,
    version: "codex-cli 0.98.0"
  },
  {
    id: "opencode",
    command: "opencode",
    category: "ai-runner",
    description: "OpenCode",
    available: true,
    required: false,
    version: "1.14.31"
  }
];

describe("GlobalAgentAccessPanel", () => {
  afterEach(() => cleanup());

  it("offers Kimi as a first-class provider and fills the Moonshot base URL", () => {
    const onSaveRunnerCredential = vi.fn();
    render(
      <I18nProvider language="en">
        <GlobalAgentAccessPanel
          selectedRunnerId="opencode"
          localCapabilities={capabilities}
          runnerCredentials={[]}
          runnerModelDiscoveryResults={{}}
          runnerPreflightResults={{}}
          discoveringRunnerModelsKey=""
          testingRunnerId=""
          onSelectRunner={vi.fn()}
          onSaveRunnerCredential={onSaveRunnerCredential}
          onDiscoverRunnerModels={vi.fn()}
          onTestRunner={vi.fn()}
        />
      </I18nProvider>
    );

    fireEvent.change(screen.getByLabelText("Provider"), { target: { value: "kimi" } });

    expect(screen.getByDisplayValue("https://api.moonshot.ai/v1")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("kimi/model")).toBeInTheDocument();
    expect(screen.queryByText("openai/gpt-5.4-mini")).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Model"), { target: { value: "kimi/kimi-k2-0711-preview" } });
    fireEvent.click(screen.getByRole("button", { name: "Save account" }));

    expect(onSaveRunnerCredential).toHaveBeenCalledWith(
      expect.objectContaining({
        runner: "opencode",
        provider: "kimi",
        baseUrl: "https://api.moonshot.ai/v1",
        model: "kimi/kimi-k2-0711-preview"
      })
    );
  });

  it("shows only discovered provider models instead of static presets", () => {
    render(
      <I18nProvider language="en">
        <GlobalAgentAccessPanel
          selectedRunnerId="opencode"
          localCapabilities={capabilities}
          runnerCredentials={[]}
          runnerModelDiscoveryResults={{
            "opencode:kimi": {
              runner: "opencode",
              provider: "kimi",
              status: "ready",
              source: "https://api.moonshot.ai/v1/models",
              models: ["moonshot-v1-8k", "moonshot-v1-32k"]
            }
          }}
          runnerPreflightResults={{}}
          discoveringRunnerModelsKey=""
          testingRunnerId=""
          onSelectRunner={vi.fn()}
          onSaveRunnerCredential={vi.fn()}
          onDiscoverRunnerModels={vi.fn()}
          onTestRunner={vi.fn()}
        />
      </I18nProvider>
    );

    fireEvent.change(screen.getByLabelText("Provider"), { target: { value: "kimi" } });
    fireEvent.click(screen.getByRole("button", { name: "kimi/moonshot-v1-32k" }));

    expect(screen.getByLabelText("Model")).toHaveValue("kimi/moonshot-v1-32k");
    expect(screen.queryByText("openrouter/deepseek-r1")).not.toBeInTheDocument();
  });

  it("lets Codex save and discover a local CLI model without API key fields", () => {
    const onSaveRunnerCredential = vi.fn();
    const onDiscoverRunnerModels = vi.fn();
    render(
      <I18nProvider language="en">
        <GlobalAgentAccessPanel
          selectedRunnerId="codex"
          localCapabilities={capabilities}
          runnerCredentials={[
            {
              id: "codex-openai",
              runner: "codex",
              provider: "openai",
              label: "Codex local model",
              model: "gpt-5.5",
              baseUrl: "",
              secretConfigured: false,
              updatedAt: "2026-05-05T00:00:00Z"
            }
          ]}
          runnerModelDiscoveryResults={{
            "codex:openai": {
              runner: "codex",
              provider: "openai",
              status: "ready",
              source: "Codex local model cache",
              models: ["gpt-5.5", "gpt-5.4"]
            }
          }}
          runnerPreflightResults={{}}
          discoveringRunnerModelsKey=""
          testingRunnerId=""
          onSelectRunner={vi.fn()}
          onSaveRunnerCredential={onSaveRunnerCredential}
          onDiscoverRunnerModels={onDiscoverRunnerModels}
          onTestRunner={vi.fn()}
        />
      </I18nProvider>
    );

    expect(screen.getByText("Model saved")).toBeInTheDocument();
    expect(screen.queryByText("API key")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "gpt-5.4" }));
    fireEvent.click(screen.getByRole("button", { name: "Save model" }));
    fireEvent.click(screen.getByRole("button", { name: "Discover models" }));

    expect(onSaveRunnerCredential).toHaveBeenCalledWith(
      expect.objectContaining({
        runner: "codex",
        provider: "openai",
        model: "gpt-5.4",
        secret: ""
      })
    );
    expect(onDiscoverRunnerModels).toHaveBeenCalledWith(
      expect.objectContaining({
        runner: "codex",
        provider: "openai",
        secret: ""
      })
    );
  });

  it("runs a real preflight test for the selected account runner", () => {
    const onTestRunner = vi.fn();
    render(
      <I18nProvider language="en">
        <GlobalAgentAccessPanel
          selectedRunnerId="opencode"
          localCapabilities={capabilities}
          runnerCredentials={[]}
          runnerModelDiscoveryResults={{}}
          runnerPreflightResults={{
            opencode: {
              agentId: "global-opencode",
              runner: "opencode",
              status: "ready",
              message: "Runner command and local account preflight passed.",
              effectiveModel: "kimi/kimi-for-coding",
              path: "/usr/local/bin/opencode"
            }
          }}
          discoveringRunnerModelsKey=""
          testingRunnerId=""
          onSelectRunner={vi.fn()}
          onSaveRunnerCredential={vi.fn()}
          onDiscoverRunnerModels={vi.fn()}
          onTestRunner={onTestRunner}
        />
      </I18nProvider>
    );

    fireEvent.change(screen.getByLabelText("Provider"), { target: { value: "kimi" } });
    fireEvent.change(screen.getByLabelText("Model"), { target: { value: "kimi-for-coding" } });
    fireEvent.change(screen.getByLabelText("Base URL"), { target: { value: "https://kimi.a7m.com.cn" } });
    fireEvent.click(screen.getByRole("button", { name: "Test connection" }));

    expect(screen.getByText("Ready")).toBeInTheDocument();
    expect(screen.getByText("model: kimi/kimi-for-coding")).toBeInTheDocument();
    expect(onTestRunner).toHaveBeenCalledWith(
      expect.objectContaining({
        runner: "opencode",
        provider: "kimi",
        model: "kimi-for-coding",
        baseUrl: "https://kimi.a7m.com.cn"
      })
    );
  });
});
