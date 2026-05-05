import { useEffect, useState } from "react";
import type { AgentRunnerPreflightResult, LocalCapabilityInfo, RunnerCredentialInfo, RunnerModelDiscoveryResult } from "../omegaControlApiClient";
import { useI18n } from "../i18n";

type AgentRunnerId = "codex" | "claude-code" | "opencode" | "trae-agent";
type ModelConfigurableRunnerId = "codex" | "opencode" | "trae-agent";
type AccountKeyRunnerId = "opencode" | "trae-agent";

type RunnerCredentialDraft = {
  provider: string;
  label: string;
  model: string;
  baseUrl: string;
  secret: string;
};

const runnerLabels: Record<AgentRunnerId, string> = {
  codex: "Codex",
  "claude-code": "Claude Code",
  opencode: "opencode",
  "trae-agent": "Trae Agent"
};

const providerOptionsByRunner: Record<"opencode" | "trae-agent", string[]> = {
  opencode: ["openai", "openrouter", "deepseek", "qwen", "kimi"],
  "trae-agent": ["doubao", "openai", "anthropic", "google", "kimi"]
};

const providerLabels: Record<string, string> = {
  anthropic: "Anthropic",
  doubao: "Doubao",
  deepseek: "DeepSeek",
  google: "Google",
  kimi: "Kimi (Moonshot)",
  openai: "OpenAI",
  openrouter: "OpenRouter",
  qwen: "Qwen"
};

const defaultBaseUrlByProvider: Record<string, string> = {
  anthropic: "https://api.anthropic.com/v1",
  doubao: "https://ark.cn-beijing.volces.com/api/v3",
  deepseek: "https://api.deepseek.com",
  google: "https://generativelanguage.googleapis.com/v1beta",
  kimi: "https://api.moonshot.ai/v1",
  openai: "https://api.openai.com/v1",
  openrouter: "https://openrouter.ai/api/v1",
  qwen: "https://dashscope.aliyuncs.com/compatible-mode/v1"
};

const defaultDrafts: Record<ModelConfigurableRunnerId, RunnerCredentialDraft> = {
  codex: {
    provider: "openai",
    label: "Codex local model",
    model: "",
    baseUrl: "",
    secret: ""
  },
  opencode: {
    provider: "openai",
    label: "opencode OpenAI",
    model: "",
    baseUrl: "",
    secret: ""
  },
  "trae-agent": {
    provider: "doubao",
    label: "Trae Doubao",
    model: "",
    baseUrl: "",
    secret: ""
  }
};

function canConfigureModel(runner: string): runner is ModelConfigurableRunnerId {
  return runner === "codex" || runner === "opencode" || runner === "trae-agent";
}

function canUseAccountKey(runner: string): runner is AccountKeyRunnerId {
  return runner === "opencode" || runner === "trae-agent";
}

function capabilityForRunner(capabilities: LocalCapabilityInfo[], runner: string) {
  return capabilities.find((capability) => capability.id === runner);
}

function modelPlaceholder(runner: ModelConfigurableRunnerId, provider: string) {
  if (runner === "codex") return "gpt-5.5";
  if (runner === "trae-agent") return `${provider}:model`;
  return `${provider}/model`;
}

function modelValueForRunner(runner: ModelConfigurableRunnerId, provider: string, model: string) {
  const value = model.trim();
  if (!value) return "";
  if (runner === "codex") return value;
  if (runner === "trae-agent") {
    return value.includes(":") ? value : `${provider}:${value}`;
  }
  return value.includes("/") ? value : `${provider}/${value}`;
}

function EyeIcon({ open }: { open: boolean }) {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path d="M2.5 12s3.5-6 9.5-6 9.5 6 9.5 6-3.5 6-9.5 6-9.5-6-9.5-6Z" />
      {open ? <circle cx="12" cy="12" r="3" /> : <path d="M4 4l16 16" />}
    </svg>
  );
}

type GlobalAgentAccessPanelProps = {
  selectedRunnerId: AgentRunnerId;
  localCapabilities: LocalCapabilityInfo[];
  runnerCredentials: RunnerCredentialInfo[];
  runnerModelDiscoveryResults: Record<string, RunnerModelDiscoveryResult>;
  runnerPreflightResults: Record<string, AgentRunnerPreflightResult>;
  discoveringRunnerModelsKey: string;
  testingRunnerId: string;
  onSelectRunner: (runner: AgentRunnerId) => void;
  onSaveRunnerCredential: (input: {
    id?: string;
    runner: string;
    provider: string;
    label?: string;
    model?: string;
    baseUrl?: string;
    secret?: string;
  }) => void;
  onDiscoverRunnerModels: (input: {
    runner: string;
    provider: string;
    model?: string;
    baseUrl?: string;
    secret?: string;
  }) => void;
  onTestRunner: (input: {
    runner: AgentRunnerId;
    provider?: string;
    model?: string;
    baseUrl?: string;
    secret?: string;
  }) => void;
};

export function GlobalAgentAccessPanel({
  selectedRunnerId,
  localCapabilities,
  runnerCredentials,
  runnerModelDiscoveryResults,
  runnerPreflightResults,
  discoveringRunnerModelsKey,
  testingRunnerId,
  onSelectRunner,
  onSaveRunnerCredential,
  onDiscoverRunnerModels,
  onTestRunner
}: GlobalAgentAccessPanelProps) {
  const { t } = useI18n();
  const [draft, setDraft] = useState<RunnerCredentialDraft>(defaultDrafts["opencode"]);
  const [secretVisible, setSecretVisible] = useState(false);
  const editableRunner = canConfigureModel(selectedRunnerId) ? selectedRunnerId : "opencode";
  const accountRunner = canUseAccountKey(selectedRunnerId);
  const selectedCapability = capabilityForRunner(localCapabilities, selectedRunnerId);
  const selectedCredential = canConfigureModel(selectedRunnerId)
    ? runnerCredentials.find((credential) => credential.runner === selectedRunnerId && credential.provider === draft.provider)
    : undefined;
  const discoveryKey = `${editableRunner}:${draft.provider}`;
  const discovery = runnerModelDiscoveryResults[discoveryKey];
  const preflight = runnerPreflightResults[selectedRunnerId];
  const discoveredModelOptions = discovery?.status === "ready" ? discovery.models ?? [] : [];

  useEffect(() => {
    if (!canConfigureModel(selectedRunnerId)) return;
    const stored = runnerCredentials.find((credential) => credential.runner === selectedRunnerId);
    setDraft({
      ...defaultDrafts[selectedRunnerId],
      ...(stored
        ? {
            provider: stored.provider,
            label: stored.label,
            model: stored.model,
            baseUrl: stored.baseUrl || defaultDrafts[selectedRunnerId].baseUrl,
            secret: ""
          }
        : {})
    });
    setSecretVisible(false);
  }, [selectedRunnerId, runnerCredentials]);

  function updateDraft(patch: Partial<RunnerCredentialDraft>) {
    setDraft((current) => ({ ...current, ...patch }));
  }

  return (
    <div className="global-agent-panel">
      <div className="agent-runner-picker" role="tablist" aria-label="Global agent runners">
        {(Object.keys(runnerLabels) as AgentRunnerId[]).map((runner) => {
          const capability = capabilityForRunner(localCapabilities, runner);
          return (
            <button
              key={runner}
              type="button"
              className={selectedRunnerId === runner ? "active" : ""}
              onClick={() => onSelectRunner(runner)}
              role="tab"
              aria-selected={selectedRunnerId === runner}
            >
              <span>{runnerLabels[runner]}</span>
              <small>{capability?.available ? t("ready") : t("missing")}</small>
            </button>
          );
        })}
      </div>

      <div className={selectedCapability?.available ? "provider-tool-status ready runner-tool-status" : "provider-tool-status missing runner-tool-status"}>
        <div>
          <strong>{selectedCapability?.available ? `${runnerLabels[selectedRunnerId]} ${t("ready")}` : `${runnerLabels[selectedRunnerId]} ${t("missing")}`}</strong>
          <span>{selectedCapability?.version || selectedCapability?.description || selectedCapability?.command || t("Runner command is not visible in PATH.")}</span>
        </div>
        <button
          type="button"
          className="secondary-action compact-action"
          disabled={testingRunnerId === selectedRunnerId}
          onClick={() =>
            onTestRunner({
              runner: selectedRunnerId,
              provider: canConfigureModel(selectedRunnerId) ? draft.provider : "",
              model: canConfigureModel(selectedRunnerId) ? draft.model : "",
              baseUrl: accountRunner ? draft.baseUrl : "",
              secret: accountRunner ? draft.secret : ""
            })
          }
        >
          {testingRunnerId === selectedRunnerId ? t("Testing...") : t("Test connection")}
        </button>
      </div>
      {preflight ? (
        <div className={`agent-preflight-result ${preflight.status === "ready" ? "ready" : ""}`} role="status">
          <strong>{preflight.status === "ready" ? t("Ready") : t("Needs attention")}</strong>
          <span>{preflight.message ?? (preflight.status === "ready" ? t("Runner preflight passed.") : t("Runner preflight failed."))}</span>
          {preflight.effectiveModel ? <small>{t("model:")} {preflight.effectiveModel}</small> : null}
          {preflight.path ? <small>{preflight.path}</small> : null}
        </div>
      ) : null}

      {canConfigureModel(selectedRunnerId) ? (
        <div className="runner-account-panel global-runner-account">
          <div className="runner-account-heading">
            <div>
              <span className="section-label">{accountRunner ? t("Runner account") : t("Runner model")}</span>
              <strong>{runnerLabels[selectedRunnerId]}</strong>
            </div>
            <span
              className={
                accountRunner
                  ? selectedCredential?.secretConfigured
                    ? "runner-availability ready"
                    : "runner-availability missing"
                  : selectedCredential?.model
                    ? "runner-availability ready"
                    : "runner-availability neutral"
              }
            >
              {accountRunner
                ? selectedCredential?.secretConfigured
                  ? t("Key saved")
                  : t("Key not configured")
                : selectedCredential?.model
                  ? t("Model saved")
                  : t("Local CLI")}
            </span>
          </div>
          <div className="runner-account-grid">
            {accountRunner ? (
              <label>
                <span>{t("Provider")}</span>
                <select
                  value={draft.provider}
                  onChange={(event) => {
                    const provider = event.currentTarget.value;
                    const stored = runnerCredentials.find((credential) => credential.runner === selectedRunnerId && credential.provider === provider);
                    setDraft({
                      provider,
                      label: stored?.label ?? `${runnerLabels[selectedRunnerId]} ${providerLabels[provider] ?? provider}`,
                      model: stored?.model ?? "",
                      baseUrl: stored?.baseUrl ?? defaultBaseUrlByProvider[provider] ?? "",
                      secret: ""
                    });
                  }}
                >
                  {providerOptionsByRunner[selectedRunnerId].map((provider) => (
                    <option key={provider} value={provider}>
                      {providerLabels[provider] ?? provider}
                    </option>
                  ))}
                </select>
              </label>
            ) : null}
            <label>
              <span>{selectedRunnerId === "trae-agent" ? "EP ID / model" : t("Model")}</span>
              <input
                value={draft.model}
                placeholder={modelPlaceholder(selectedRunnerId, draft.provider)}
                onChange={(event) => updateDraft({ model: event.currentTarget.value })}
              />
              {discoveredModelOptions.length ? (
                <div className="runner-model-suggestions" aria-label={t("Discovered models")}>
                  {discoveredModelOptions.slice(0, 8).map((model) => {
                    const value = modelValueForRunner(selectedRunnerId, draft.provider, model);
                    return (
                      <button key={value} type="button" onClick={() => updateDraft({ model: value })}>
                        {value}
                      </button>
                    );
                  })}
                </div>
              ) : null}
            </label>
            {accountRunner ? (
              <>
                <label>
                  <span>Base URL</span>
                  <input
                    value={draft.baseUrl}
                    placeholder={t("Optional provider base URL")}
                    onChange={(event) => updateDraft({ baseUrl: event.currentTarget.value })}
                  />
                </label>
                <label className="secret-input-field">
                  <span>API key</span>
                  <span className="secret-input-shell">
                    <input
                      type={secretVisible ? "text" : "password"}
                      value={draft.secret}
                      placeholder={selectedCredential?.secretConfigured ? selectedCredential.secretMasked ?? "********" : t("Paste API key")}
                      onChange={(event) => updateDraft({ secret: event.currentTarget.value })}
                      autoComplete="off"
                    />
                    <button
                      type="button"
                      className="secret-toggle-button"
                      aria-label={secretVisible ? t("Hide API key") : t("Show API key")}
                      onClick={() => setSecretVisible((open) => !open)}
                    >
                      <EyeIcon open={secretVisible} />
                    </button>
                  </span>
                </label>
              </>
            ) : (
              <p className="runner-local-model-note">
                {t("Codex uses the local Codex CLI sign-in. Omega stores only the default model and passes it to codex --model when this runner starts.")}
              </p>
            )}
          </div>
          <div className="runner-account-actions">
            <button
              type="button"
              className="primary-action"
              onClick={() => {
                onSaveRunnerCredential({
                  id: selectedCredential?.id,
                  runner: selectedRunnerId,
                  provider: draft.provider,
                  label: draft.label,
                  model: draft.model,
                  baseUrl: accountRunner ? draft.baseUrl : "",
                  secret: accountRunner ? draft.secret : ""
                });
                updateDraft({ secret: "" });
                setSecretVisible(false);
              }}
            >
              {accountRunner ? t("Save account") : t("Save model")}
            </button>
            <button
              type="button"
              className="secondary-action"
              disabled={discoveringRunnerModelsKey === discoveryKey}
              onClick={() => {
                onDiscoverRunnerModels({
                  runner: selectedRunnerId,
                  provider: draft.provider,
                  model: draft.model,
                  baseUrl: accountRunner ? draft.baseUrl : "",
                  secret: accountRunner ? draft.secret : ""
                });
              }}
            >
              {discoveringRunnerModelsKey === discoveryKey ? t("Discovering...") : t("Discover models")}
            </button>
            <small>
              {accountRunner
                ? t("Stored locally as encrypted ciphertext. The key is decrypted only when this runner starts or model discovery runs.")
                : t("Model discovery reads the local Codex model cache when available. No API key is stored for Codex.")}
            </small>
          </div>
          {discovery ? (
            <div className={`agent-preflight-result ${discovery.status === "ready" ? "ready" : ""}`}>
              <strong>{discovery.status === "ready" ? t("Models ready") : t("Model discovery failed")}</strong>
              <span>
                {discovery.status === "ready"
                  ? `${discovery.models.length} ${t("model(s) from")} ${discovery.source ?? discovery.provider}.`
                  : discovery.message ?? t("No model list returned.")}
              </span>
            </div>
          ) : null}
        </div>
      ) : (
        <div className="provider-panel global-agent-local-auth">
          <h2>{runnerLabels[selectedRunnerId]}</h2>
          <p>
            {selectedRunnerId === "codex"
              ? t("Codex uses the local Codex CLI account and model configuration. Workspace Agent Studio only chooses where this runner is used.")
              : t("Claude Code uses the local Claude CLI account. Workspace Agent Studio only chooses where this runner is used.")}
          </p>
        </div>
      )}
    </div>
  );
}
