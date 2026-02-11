import React, { useEffect, useMemo, useRef, useState } from "https://esm.sh/react@18.2.0";
import { createRoot } from "https://esm.sh/react-dom@18.2.0/client";
import htm from "https://esm.sh/htm@3.1.1";

const html = htm.bind(React.createElement);

const API_BASE = "/backendSalsSavvyLLMRouter";
const STORAGE_TOKEN_KEY = "salessavvyToken";

const MENU_GROUPS = [
  {
    title: "Core",
    items: [
      { id: "section-overview", label: "Overview" },
      { id: "section-security", label: "Security" },
      { id: "section-aws", label: "AWS Config" },
      { id: "section-models", label: "Models" },
      { id: "section-pricing", label: "Pricing" },
      { id: "section-billing", label: "Billing" },
      { id: "section-clients", label: "API Keys" },
    ],
  },
  {
    title: "Observability",
    items: [
      { id: "section-usage", label: "Usage" },
      { id: "section-calls", label: "Recent Calls" },
      { id: "section-logs", label: "Debug Logs" },
    ],
  },
];

const MENU_ORDER = MENU_GROUPS.flatMap((group) => group.items.map((item) => item.id));

function apiPath(path) {
  const normalized = String(path || "").trim();
  if (!normalized.startsWith("/")) {
    return `${API_BASE}/${normalized}`;
  }
  return `${API_BASE}${normalized}`;
}

function buildAuthHeaders(token) {
  const safeToken = String(token || "").trim();
  return {
    "Content-Type": "application/json",
    ...(safeToken
      ? {
          Authorization: `Bearer ${safeToken}`,
          "x-salessavvy-token": safeToken,
        }
      : {}),
  };
}

async function requestJSON(path, token, options = {}) {
  const response = await fetch(apiPath(path), {
    ...options,
    headers: {
      ...(options.headers || {}),
      ...buildAuthHeaders(token),
    },
  });

  const contentType = response.headers.get("content-type") || "";
  let body = {};
  if (contentType.includes("application/json")) {
    body = await response.json();
  } else if (!response.ok) {
    body = { error: await response.text() };
  }

  if (!response.ok) {
    const message = body?.error || body?.error?.message || `HTTP ${response.status}`;
    const error = new Error(message);
    error.status = response.status;
    throw error;
  }

  return body;
}

function formatNumber(value) {
  return Number(value || 0).toLocaleString();
}

function formatUSD(value) {
  return Number(value || 0).toLocaleString(undefined, {
    minimumFractionDigits: 6,
    maximumFractionDigits: 9,
  });
}

function formatBytes(value) {
  const size = Number(value || 0);
  if (!Number.isFinite(size) || size <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let current = size;
  let idx = 0;
  while (current >= 1024 && idx < units.length - 1) {
    current /= 1024;
    idx += 1;
  }
  return `${current.toFixed(idx === 0 ? 0 : 2)} ${units[idx]}`;
}

function parsePositiveInt(value, fallback) {
  const parsed = Number.parseInt(String(value ?? ""), 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return fallback;
  }
  return parsed;
}

function normalizeStrings(items) {
  return Array.from(
    new Set(
      (items || [])
        .map((value) => String(value || "").trim())
        .filter((value) => value.length > 0)
    )
  ).sort();
}

function parseAllowedModels(raw) {
  return normalizeStrings(
    String(raw || "")
      .split(",")
      .map((item) => item.trim())
      .filter((item) => item.length > 0)
  );
}

function generateRandomAPIKey() {
  const prefix = "sk-router-";
  const size = 24;

  if (window.crypto && typeof window.crypto.getRandomValues === "function") {
    const bytes = new Uint8Array(size);
    window.crypto.getRandomValues(bytes);
    const payload = Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
    return `${prefix}${payload}`;
  }

  let payload = "";
  for (let index = 0; index < size; index += 1) {
    payload += Math.floor(Math.random() * 256)
      .toString(16)
      .padStart(2, "0");
  }
  return `${prefix}${payload}`;
}

function buildDefaultUsageFilters() {
  const today = new Date();
  const from = new Date(today);
  from.setDate(from.getDate() - 7);
  return {
    from: from.toISOString().slice(0, 10),
    to: today.toISOString().slice(0, 10),
    clientId: "",
  };
}

function escapeHTML(value) {
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}

function isLikelyJSON(value) {
  try {
    JSON.parse(value);
    return true;
  } catch {
    return false;
  }
}

function formatJSON(value) {
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch {
    return value;
  }
}

function formatRichInline(segment) {
  let rich = escapeHTML(segment);
  rich = rich.replace(/`([^`\n]+)`/g, "<code>$1</code>");
  rich = rich.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
  rich = rich.replace(/(^|[\s(])(https?:\/\/[^\s<]+)/g, '$1<a href="$2" target="_blank" rel="noreferrer noopener">$2</a>');
  rich = rich.replace(/\n/g, "<br>");
  return rich;
}

function formatRichText(rawText) {
  const text = String(rawText || "");
  if (!text) return "";

  const trimmed = text.trim();
  if ((trimmed.startsWith("{") || trimmed.startsWith("[")) && isLikelyJSON(trimmed)) {
    return `<pre class="log-pre"><code>${escapeHTML(formatJSON(trimmed))}</code></pre>`;
  }

  const fencePattern = /```([a-zA-Z0-9_-]+)?\n?([\s\S]*?)```/g;
  let last = 0;
  let result = "";
  let match;

  while ((match = fencePattern.exec(text)) !== null) {
    const [fullMatch, lang = "", code = ""] = match;
    const index = match.index;
    if (index > last) {
      result += formatRichInline(text.slice(last, index));
    }
    const langLabel = lang ? `<span class="code-lang">${escapeHTML(lang)}</span>` : "";
    result += `<pre class="log-pre">${langLabel}<code>${escapeHTML(code)}</code></pre>`;
    last = index + fullMatch.length;
  }

  if (last < text.length) {
    result += formatRichInline(text.slice(last));
  }

  return result;
}

function buildPricingRows(enabledModelIDs, pricingItems) {
  const pricingByModel = new Map();
  for (const item of pricingItems || []) {
    const modelID = String(item?.model_id || "").trim();
    if (!modelID) continue;
    pricingByModel.set(modelID, {
      input: Number(item?.input_price_per_1k || 0),
      output: Number(item?.output_price_per_1k || 0),
    });
  }

  return normalizeStrings(enabledModelIDs).map((modelID) => {
    const pricing = pricingByModel.get(modelID) || { input: 0, output: 0 };
    return {
      modelID,
      input: String(pricing.input),
      output: String(pricing.output),
    };
  });
}

function statusColor(isError) {
  return isError ? "#b91c1c" : "hsl(var(--muted-foreground))";
}

function StatusLine({ id, status }) {
  if (!status?.message) {
    return html`<p id=${id} className="muted"></p>`;
  }
  return html`<p id=${id} className="muted" style=${{ color: statusColor(status.error) }}>${status.message}</p>`;
}
function App() {
  const savedToken = window.localStorage.getItem(STORAGE_TOKEN_KEY) || "";

  const [adminToken, setAdminToken] = useState(savedToken);
  const [loginToken, setLoginToken] = useState(savedToken);
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isBusy, setIsBusy] = useState(false);

  const [status, setStatus] = useState({
    overview: { message: "", error: false },
    login: {
      message: savedToken ? "Trying saved salessavvy token..." : "Use salessavvy token to login. Default is admin123.",
      error: false,
    },
    token: { message: "", error: false },
    aws: { message: "", error: false },
    models: { message: "", error: false },
    pricing: { message: "", error: false },
    billing: { message: "", error: false },
    logs: { message: "", error: false },
  });

  const [awsForm, setAwsForm] = useState({
    region: "",
    accessKeyId: "",
    secretAccessKey: "",
    sessionToken: "",
    defaultModelId: "",
  });

  const [securityTokenInput, setSecurityTokenInput] = useState("");
  const [availableModels, setAvailableModels] = useState([]);
  const [enabledModelIDs, setEnabledModelIDs] = useState([]);
  const [selectedModels, setSelectedModels] = useState([]);
  const [pricingRows, setPricingRows] = useState([]);
  const [pricingUnitTokens, setPricingUnitTokens] = useState(1000);
  const [globalCostLimitInput, setGlobalCostLimitInput] = useState("0");
  const [currentTotalCost, setCurrentTotalCost] = useState(0);
  const [bedrockReady, setBedrockReady] = useState(false);

  const [clients, setClients] = useState([]);
  const [clientForm, setClientForm] = useState({
    id: "",
    name: "",
    apiKey: "",
    rpm: "",
    concurrent: "",
    models: "",
    disabled: false,
  });

  const [usageFilters, setUsageFilters] = useState(buildDefaultUsageFilters());
  const [usageData, setUsageData] = useState({
    byClient: [],
    byClientModel: [],
    totalCost: 0,
  });

  const [callsFilters, setCallsFilters] = useState({
    limit: "100",
    page: "1",
    clientId: "",
  });
  const [callsData, setCallsData] = useState({
    items: [],
    totalCost: 0,
    page: 1,
    pageSize: 100,
    total: 0,
    totalPages: 1,
    hasPrev: false,
    hasNext: false,
  });

  const [logsLimit, setLogsLimit] = useState("100");
  const [logsData, setLogsData] = useState({
    items: [],
    enabled: false,
    logDir: "./debug_logs",
  });

  const [activeSection, setActiveSection] = useState(MENU_ORDER[0]);
  const [contentModal, setContentModal] = useState({
    open: false,
    title: "Content",
    content: "",
  });

  const sectionRefs = useRef(new Map());
  const autoConnectDone = useRef(false);

  const setSectionStatus = (key, message, error = false) => {
    setStatus((previous) => ({
      ...previous,
      [key]: {
        message: String(message || ""),
        error: Boolean(error),
      },
    }));
  };

  const setBillingStatusFromValues = (limit, total) => {
    if (limit <= 0) {
      setSectionStatus("billing", "Global cost limit: unlimited.");
      return;
    }

    if (total >= limit) {
      setSectionStatus(
        "billing",
        `Global cost limit reached: $${formatUSD(total)} / $${formatUSD(limit)}.`,
        true
      );
      return;
    }

    const remaining = Math.max(0, limit - total);
    setSectionStatus(
      "billing",
      `Remaining budget: $${formatUSD(remaining)} (limit $${formatUSD(limit)}).`
    );
  };

  const registerSectionRef = (sectionID) => (node) => {
    if (node) {
      sectionRefs.current.set(sectionID, node);
    } else {
      sectionRefs.current.delete(sectionID);
    }
  };

  const hydrateConfig = (data) => {
    const aws = data?.aws || {};
    setAwsForm({
      region: aws.region || "",
      accessKeyId: aws.access_key_id || "",
      secretAccessKey: aws.secret_access_key || "",
      sessionToken: aws.session_token || "",
      defaultModelId: aws.default_model_id || "",
    });

    const ready = Boolean(data?.bedrock_client_ready);
    setBedrockReady(ready);
    setSectionStatus(
      "aws",
      ready ? "Bedrock client is ready." : "Bedrock client is not ready. Save valid AWS config.",
      !ready
    );

    const available = normalizeStrings(data?.available_models || []);
    const explicitEnabled = normalizeStrings(data?.enabled_model_ids || []);
    const preselected = explicitEnabled.length > 0 ? explicitEnabled : available;

    setAvailableModels(available);
    setEnabledModelIDs(explicitEnabled);
    setSelectedModels(preselected.filter((modelID) => available.includes(modelID)));

    const rows = buildPricingRows(explicitEnabled, data?.model_pricing || []);
    setPricingRows(rows);

    const unit = Number(data?.pricing_unit_tokens || 1000);
    setPricingUnitTokens(unit);
    if (rows.length > 0) {
      setSectionStatus("pricing", `Pricing unit: USD / ${unit} tokens | rows: ${rows.length}`);
    } else {
      setSectionStatus("pricing", "No models available for pricing.", true);
    }

    const limitValue = Number(data?.billing?.global_cost_limit_usd || 0);
    const safeLimit = Number.isFinite(limitValue) && limitValue >= 0 ? limitValue : 0;
    const safeTotal = Number.isFinite(Number(data?.current_total_cost)) ? Number(data?.current_total_cost) : 0;

    setGlobalCostLimitInput(String(safeLimit));
    setCurrentTotalCost(safeTotal);
    setBillingStatusFromValues(safeLimit, safeTotal);

    setClients(Array.isArray(data?.clients) ? data.clients : []);
  };

  const loadConfig = async (token) => {
    const payload = await requestJSON("/config", token);
    hydrateConfig(payload);
    return payload;
  };

  const loadUsage = async (token) => {
    const params = new URLSearchParams();
    if (usageFilters.from) params.set("from", usageFilters.from);
    if (usageFilters.to) params.set("to", usageFilters.to);
    if (String(usageFilters.clientId || "").trim()) {
      params.set("client_id", String(usageFilters.clientId || "").trim());
    }

    const payload = await requestJSON(`/usage?${params.toString()}`, token);
    setUsageData({
      byClient: payload?.by_client || [],
      byClientModel: payload?.by_client_model || [],
      totalCost: Number(payload?.total_cost || 0),
    });
    setSectionStatus("overview", `Usage loaded. Total cost: $${formatUSD(payload?.total_cost || 0)}`);
    return payload;
  };

  const loadCalls = async (token, pageOverride) => {
    const pageSize = parsePositiveInt(callsFilters.limit, callsData.pageSize || 100);
    const targetPage = parsePositiveInt(pageOverride ?? callsFilters.page, callsData.page || 1);

    const params = new URLSearchParams();
    params.set("limit", String(pageSize));
    params.set("page", String(targetPage));
    if (String(callsFilters.clientId || "").trim()) {
      params.set("client_id", String(callsFilters.clientId || "").trim());
    }

    const payload = await requestJSON(`/calls?${params.toString()}`, token);

    const page = parsePositiveInt(payload?.page, 1);
    const pageSizeFromPayload = parsePositiveInt(payload?.page_size, pageSize);
    const total = Math.max(0, Number(payload?.total || 0));
    const totalPages = Math.max(1, parsePositiveInt(payload?.total_pages, 1));

    setCallsData({
      items: payload?.items || [],
      totalCost: Number(payload?.total_cost || 0),
      page,
      pageSize: pageSizeFromPayload,
      total,
      totalPages,
      hasPrev: Boolean(payload?.has_prev) && page > 1,
      hasNext: Boolean(payload?.has_next) && page < totalPages,
    });

    setCallsFilters((previous) => ({
      ...previous,
      limit: String(pageSizeFromPayload),
      page: String(page),
    }));

    setSectionStatus(
      "overview",
      `Calls loaded. Page ${page}/${totalPages}. Total ${formatNumber(total)} records. Cost: $${formatUSD(payload?.total_cost || 0)}`
    );

    return payload;
  };

  const loadLogs = async (token) => {
    const safeLimit = parsePositiveInt(logsLimit, 100);
    setLogsLimit(String(safeLimit));

    const payload = await requestJSON(`/logs?limit=${encodeURIComponent(String(safeLimit))}`, token);
    const items = payload?.items || [];
    const enabled = Boolean(payload?.enabled);
    const dir = String(payload?.log_dir || "./debug_logs");

    setLogsData({
      items,
      enabled,
      logDir: dir,
    });

    setSectionStatus(
      "logs",
      `Debug logging is ${enabled ? "enabled" : "disabled"}. Directory: ${dir}. Files: ${formatNumber(items.length)}.`
    );

    return payload;
  };

  const connect = async (tokenCandidate) => {
    const token = String(tokenCandidate ?? loginToken).trim();
    if (!token) {
      setSectionStatus("login", "Please input salessavvy token.", true);
      return false;
    }

    setIsBusy(true);
    setAdminToken(token);
    setLoginToken(token);
    window.localStorage.setItem(STORAGE_TOKEN_KEY, token);

    try {
      await Promise.all([loadConfig(token), loadUsage(token), loadCalls(token, 1), loadLogs(token)]);
      setIsAuthenticated(true);
      setSectionStatus("login", "");
      setSectionStatus("token", "");
      setSectionStatus("overview", "Connected.");
      return true;
    } catch (error) {
      setIsAuthenticated(false);
      setSectionStatus("login", error.message || "Authentication failed.", true);
      return false;
    } finally {
      setIsBusy(false);
    }
  };
  const logout = () => {
    setContentModal({ open: false, title: "Content", content: "" });
    setIsAuthenticated(false);
    setAdminToken("");
    setLoginToken("");
    setSecurityTokenInput("");
    window.localStorage.removeItem(STORAGE_TOKEN_KEY);
    setSectionStatus("overview", "Logged out.");
    setSectionStatus("login", "Use salessavvy token to login. Default is admin123.");
  };

  const reloadAll = async () => {
    if (!adminToken) return;
    setIsBusy(true);
    try {
      await Promise.all([loadConfig(adminToken), loadUsage(adminToken), loadCalls(adminToken), loadLogs(adminToken)]);
      setSectionStatus("overview", "Reloaded.");
    } catch (error) {
      setSectionStatus("overview", error.message || "Reload failed.", true);
    } finally {
      setIsBusy(false);
    }
  };

  const navigateToSection = (sectionID) => {
    const node = sectionRefs.current.get(sectionID);
    if (!node) return;
    setActiveSection(sectionID);
    node.scrollIntoView({ behavior: "smooth", block: "start" });
  };

  const applyModelSelection = (mode) => {
    if (mode === "all") {
      setSelectedModels(availableModels);
      return;
    }
    if (mode === "none") {
      setSelectedModels([]);
      return;
    }
    if (mode === "invert") {
      setSelectedModels((previous) => availableModels.filter((modelID) => !previous.includes(modelID)));
    }
  };

  const toggleModel = (modelID) => {
    setSelectedModels((previous) => {
      if (previous.includes(modelID)) {
        return previous.filter((item) => item !== modelID);
      }
      return normalizeStrings([...previous, modelID]);
    });
  };

  const updateAwsField = (field, value) => {
    setAwsForm((previous) => ({
      ...previous,
      [field]: value,
    }));
  };

  const updateClientField = (field, value) => {
    setClientForm((previous) => ({
      ...previous,
      [field]: value,
    }));
  };

  const closeModal = () => {
    setContentModal({ open: false, title: "Content", content: "" });
  };

  const openModal = (title, content) => {
    setContentModal({
      open: true,
      title: String(title || "Content"),
      content: String(content || ""),
    });
  };

  const handleSaveAdminToken = async () => {
    const newToken = securityTokenInput.trim();
    if (!newToken) {
      setSectionStatus("token", "New salessavvy token is required.", true);
      return;
    }

    try {
      await requestJSON("/config/salessavvy-token", adminToken, {
        method: "POST",
        body: JSON.stringify({ salessavvy_token: newToken }),
      });

      setAdminToken(newToken);
      setLoginToken(newToken);
      window.localStorage.setItem(STORAGE_TOKEN_KEY, newToken);
      setSecurityTokenInput("");
      setSectionStatus("token", "Salessavvy token updated successfully. Using new token for future requests.");
      setSectionStatus("overview", "Salessavvy token updated.");
    } catch (error) {
      if (error?.status === 401 || error?.status === 403) {
        setSectionStatus("token", "Authentication failed. Please logout and login again with the current token.", true);
        return;
      }
      setSectionStatus("token", error.message || "Failed to update token.", true);
    }
  };

  const handleSaveAWS = async (event) => {
    event.preventDefault();
    try {
      await requestJSON("/config/aws", adminToken, {
        method: "POST",
        body: JSON.stringify({
          region: awsForm.region.trim(),
          access_key_id: awsForm.accessKeyId.trim(),
          secret_access_key: awsForm.secretAccessKey.trim(),
          session_token: awsForm.sessionToken.trim(),
          default_model_id: awsForm.defaultModelId.trim(),
        }),
      });

      await loadConfig(adminToken);
      setSectionStatus("overview", "AWS config saved.");
    } catch (error) {
      setSectionStatus("aws", error.message || "Failed to save AWS config.", true);
    }
  };

  const handleRefreshModels = async () => {
    try {
      await requestJSON("/config/models/refresh", adminToken, {
        method: "POST",
        body: "{}",
      });
      await loadConfig(adminToken);
      setSectionStatus("overview", "Model list refreshed from AWS.");
    } catch (error) {
      setSectionStatus("models", error.message || "Failed to refresh models.", true);
    }
  };

  const handleSaveModels = async () => {
    const enabled = normalizeStrings(selectedModels);
    if (enabled.length === 0) {
      setSectionStatus("models", "Select at least one model.", true);
      return;
    }

    try {
      await requestJSON("/config/models", adminToken, {
        method: "POST",
        body: JSON.stringify({ enabled_model_ids: enabled }),
      });
      await loadConfig(adminToken);
      setSectionStatus("overview", "Enabled models saved.");
    } catch (error) {
      setSectionStatus("models", error.message || "Failed to save enabled models.", true);
    }
  };

  const handleSavePricing = async () => {
    try {
      const items = pricingRows.map((row) => {
        const input = Number(row.input === "" ? 0 : row.input);
        const output = Number(row.output === "" ? 0 : row.output);
        if (!Number.isFinite(input) || !Number.isFinite(output) || input < 0 || output < 0) {
          throw new Error(`Invalid pricing for model: ${row.modelID}`);
        }
        return {
          model_id: row.modelID,
          input_price_per_1k: input,
          output_price_per_1k: output,
        };
      });

      await requestJSON("/config/model-pricing", adminToken, {
        method: "POST",
        body: JSON.stringify({ items }),
      });
      await loadConfig(adminToken);
      setSectionStatus("pricing", "Model pricing saved.");
    } catch (error) {
      setSectionStatus("pricing", error.message || "Failed to save pricing.", true);
    }
  };

  const handleSaveBilling = async () => {
    const parsed = Number(globalCostLimitInput === "" ? 0 : globalCostLimitInput);
    if (!Number.isFinite(parsed) || parsed < 0) {
      setSectionStatus("billing", "Invalid global cost limit.", true);
      return;
    }

    try {
      await requestJSON("/config/billing", adminToken, {
        method: "POST",
        body: JSON.stringify({ global_cost_limit_usd: parsed }),
      });

      await loadConfig(adminToken);
      setSectionStatus("overview", "Global cost limit saved.");
    } catch (error) {
      setSectionStatus("billing", error.message || "Failed to save global cost limit.", true);
    }
  };

  const handleSaveClient = async (event) => {
    event.preventDefault();
    try {
      await requestJSON("/config/clients", adminToken, {
        method: "POST",
        body: JSON.stringify({
          id: clientForm.id.trim(),
          name: clientForm.name.trim(),
          api_key: clientForm.apiKey.trim(),
          max_requests_per_minute: Number(clientForm.rpm || 0),
          max_concurrent: Number(clientForm.concurrent || 0),
          allowed_models: parseAllowedModels(clientForm.models),
          disabled: Boolean(clientForm.disabled),
        }),
      });

      setClientForm({
        id: "",
        name: "",
        apiKey: "",
        rpm: "",
        concurrent: "",
        models: "",
        disabled: false,
      });

      await loadConfig(adminToken);
      setSectionStatus("overview", "Client saved.");
    } catch (error) {
      setSectionStatus("overview", error.message || "Failed to save client.", true);
    }
  };

  const handleToggleClient = async (client) => {
    try {
      await requestJSON("/config/clients", adminToken, {
        method: "POST",
        body: JSON.stringify({
          id: client.id,
          name: client.name,
          api_key: client.api_key,
          max_requests_per_minute: Number(client.max_requests_per_minute || 0),
          max_concurrent: Number(client.max_concurrent || 0),
          allowed_models: Array.isArray(client.allowed_models) ? client.allowed_models : [],
          disabled: !Boolean(client.disabled),
        }),
      });
      await loadConfig(adminToken);
      setSectionStatus("overview", `Client ${client.id} ${client.disabled ? "enabled" : "disabled"}.`);
    } catch (error) {
      setSectionStatus("overview", error.message || "Failed to update client status.", true);
    }
  };

  const handleDeleteClient = async (clientID) => {
    if (!window.confirm(`Delete client ${clientID}?`)) {
      return;
    }

    try {
      await requestJSON(`/config/clients?id=${encodeURIComponent(clientID)}`, adminToken, {
        method: "DELETE",
      });
      await loadConfig(adminToken);
      setSectionStatus("overview", "Client deleted.");
    } catch (error) {
      setSectionStatus("overview", error.message || "Failed to delete client.", true);
    }
  };

  const handleLoadUsage = async () => {
    try {
      await loadUsage(adminToken);
    } catch (error) {
      setSectionStatus("overview", error.message || "Failed to load usage.", true);
    }
  };

  const handleLoadCalls = async (pageOverride) => {
    try {
      await loadCalls(adminToken, pageOverride);
    } catch (error) {
      setSectionStatus("overview", error.message || "Failed to load calls.", true);
    }
  };

  const handleLoadLogs = async () => {
    try {
      await loadLogs(adminToken);
    } catch (error) {
      setSectionStatus("logs", error.message || "Failed to load debug logs.", true);
    }
  };

  const handleDownloadLog = async (name) => {
    try {
      const response = await fetch(apiPath(`/logs/download?name=${encodeURIComponent(name)}`), {
        method: "GET",
        headers: buildAuthHeaders(adminToken),
      });

      if (!response.ok) {
        const contentType = response.headers.get("content-type") || "";
        let message = `HTTP ${response.status}`;
        if (contentType.includes("application/json")) {
          const body = await response.json();
          message = body?.error || body?.error?.message || message;
        } else {
          message = (await response.text()) || message;
        }
        throw new Error(message);
      }

      const blob = await response.blob();
      const blobURL = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = blobURL;
      link.download = String(name || "debug-log.txt");
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(blobURL);

      setSectionStatus("overview", `Downloaded: ${name}`);
    } catch (error) {
      setSectionStatus("logs", error.message || "Failed to download debug log.", true);
    }
  };

  useEffect(() => {
    if (!isAuthenticated) return;

    const syncActive = () => {
      let next = MENU_ORDER[0];
      for (const sectionID of MENU_ORDER) {
        const node = sectionRefs.current.get(sectionID);
        if (!node) continue;
        const top = node.getBoundingClientRect().top;
        if (top - 150 <= 0) {
          next = sectionID;
        } else {
          break;
        }
      }
      setActiveSection(next);
    };

    let ticking = false;
    const onChange = () => {
      if (ticking) return;
      ticking = true;
      window.requestAnimationFrame(() => {
        syncActive();
        ticking = false;
      });
    };

    window.addEventListener("scroll", onChange, { passive: true });
    window.addEventListener("resize", onChange);
    syncActive();

    return () => {
      window.removeEventListener("scroll", onChange);
      window.removeEventListener("resize", onChange);
    };
  }, [isAuthenticated]);

  useEffect(() => {
    if (!isAuthenticated) return;
    if (availableModels.length === 0) {
      setSectionStatus("models", "No models available. Check AWS config and click Refresh.", true);
      return;
    }
    setSectionStatus("models", `${selectedModels.length}/${availableModels.length} models enabled.`);
  }, [isAuthenticated, availableModels, selectedModels]);

  useEffect(() => {
    if (!contentModal.open) return undefined;
    const onKeyDown = (event) => {
      if (event.key === "Escape") {
        closeModal();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [contentModal.open]);

  useEffect(() => {
    if (autoConnectDone.current) return;
    autoConnectDone.current = true;
    if (savedToken) {
      void connect(savedToken);
    }
  }, []);

  const callsPageInfo = useMemo(() => {
    if (callsData.total <= 0) {
      return "No call records.";
    }
    return `Page ${callsData.page}/${callsData.totalPages} of ${formatNumber(callsData.total)} records`;
  }, [callsData]);

  const selectedModelSet = useMemo(() => new Set(selectedModels), [selectedModels]);

  const renderContentCell = (title, content) => {
    const text = String(content || "");
    if (!text) {
      return html`<span className="muted">-</span>`;
    }

    const preview = text.length > 80 ? `${text.slice(0, 80)}...` : text;
    return html`
      <div className="content-cell">
        <span className="content-summary">${preview}</span>
        <button type="button" className="ghost content-view-btn" onClick=${() => openModal(title, text)}>View</button>
      </div>
    `;
  };
  if (!isAuthenticated) {
    return html`
      <div className="app-root">
        <div className="bg-orb orb-a"></div>
        <div className="bg-orb orb-b"></div>
        <div className="bg-grid"></div>

        <main id="loginScreen" className="login-shell">
          <section className="login-card">
            <p className="eyebrow">AWS CURSOR ROUTER</p>
            <h1>Admin Login</h1>
            <p className="muted">Enter your salessavvy token to log in (default: admin123).</p>
            <form
              className="login-row"
              onSubmit=${async (event) => {
                event.preventDefault();
                await connect(loginToken);
              }}
            >
              <input
                id="loginToken"
                type="password"
                placeholder="salessavvy token"
                autoComplete="current-password"
                value=${loginToken}
                onInput=${(event) => setLoginToken(event.target.value)}
              />
              <button id="btnLogin" type="submit" disabled=${isBusy}>${isBusy ? "Connecting..." : "Login"}</button>
            </form>
            <${StatusLine} id="loginStatus" status=${status.login} />
          </section>
        </main>
      </div>
    `;
  }

  return html`
    <div className="app-root">
      <div className="bg-orb orb-a"></div>
      <div className="bg-orb orb-b"></div>
      <div className="bg-grid"></div>

      <main id="appShell" className="app-shell">
        <div className="app-layout">
          <aside className="card side-menu">
            <div className="sidebar-brand">
              <div className="sidebar-brand-icon">AR</div>
              <div>
                <p className="sidebar-title">AWS Router</p>
                <p className="sidebar-subtitle">Admin Console</p>
              </div>
            </div>

            <nav id="menuList" className="menu-stack">
              ${MENU_GROUPS.map(
                (group) => html`
                  <div className="menu-group" key=${group.title}>
                    <p className="menu-group-title">${group.title}</p>
                    ${group.items.map(
                      (item) => html`
                        <button
                          type="button"
                          key=${item.id}
                          className=${`menu-item ${activeSection === item.id ? "active" : ""}`}
                          onClick=${() => navigateToSection(item.id)}
                          data-target=${item.id}
                        >
                          ${item.label}
                        </button>
                      `
                    )}
                  </div>
                `
              )}
            </nav>
          </aside>

          <div className="main-content">
            <header className="topbar card">
              <div>
                <p className="eyebrow">Dashboard</p>
                <h1>AWS Cursor Router Admin</h1>
                <p className="muted">React-based admin console for managing and monitoring the router.</p>
              </div>
              <div className="topbar-actions">
                <button id="btnReload" className="secondary" type="button" onClick=${reloadAll} disabled=${isBusy}>Reload</button>
                <button id="btnLogout" className="ghost" type="button" onClick=${logout}>Logout</button>
              </div>
            </header>

            <section id="section-overview" ref=${registerSectionRef("section-overview")} className="card section-card">
              <h2>Overview</h2>
              <p className="muted">Summary of system status and key metrics.</p>
              <${StatusLine} id="statusText" status=${status.overview} />
            </section>

            <section id="section-security" ref=${registerSectionRef("section-security")} className="card section-card">
              <h2>Salessavvy Security</h2>
              <p className="muted">The default admin token is <code>admin123</code>. Please change it after your first login.</p>
              <div className="row">
                <input
                  id="adminTokenInput"
                  type="password"
                  placeholder="new salessavvy token"
                  autoComplete="new-password"
                  value=${securityTokenInput}
                  onInput=${(event) => setSecurityTokenInput(event.target.value)}
                />
                <button id="btnSaveAdminToken" type="button" onClick=${handleSaveAdminToken}>Update Salessavvy Token</button>
              </div>
              <${StatusLine} id="adminTokenStatus" status=${status.token} />
            </section>

            <section id="section-aws" ref=${registerSectionRef("section-aws")} className="card section-card">
              <h2>AWS Bedrock Config</h2>
              <form id="awsForm" className="grid" onSubmit=${handleSaveAWS}>
                <input id="awsRegion" placeholder="region (example: us-east-1)" required value=${awsForm.region} onInput=${(event) => updateAwsField("region", event.target.value)} />
                <input id="awsAccessKeyId" placeholder="access key id (optional if IAM role)" value=${awsForm.accessKeyId} onInput=${(event) => updateAwsField("accessKeyId", event.target.value)} />
                <input id="awsSecretAccessKey" type="password" placeholder="secret access key (optional if IAM role)" value=${awsForm.secretAccessKey} onInput=${(event) => updateAwsField("secretAccessKey", event.target.value)} />
                <input id="awsSessionToken" placeholder="session token (optional)" value=${awsForm.sessionToken} onInput=${(event) => updateAwsField("sessionToken", event.target.value)} />
                <input id="awsDefaultModelId" placeholder="default model id (optional)" value=${awsForm.defaultModelId} onInput=${(event) => updateAwsField("defaultModelId", event.target.value)} />
                <button type="submit">Save AWS Config</button>
              </form>
              <${StatusLine} id="awsStatus" status=${status.aws} />
            </section>

            <section id="section-models" ref=${registerSectionRef("section-models")} className="card section-card">
              <h2>Available Models</h2>
              <p className="muted">Models discovered from AWS Bedrock. Choose which models to expose to clients.${bedrockReady ? " (Bedrock ready)" : ""}</p>
              <div className="row">
                <button id="btnRefreshModels" className="secondary" type="button" onClick=${handleRefreshModels}>Refresh From AWS</button>
                <button id="btnSaveModels" type="button" onClick=${handleSaveModels}>Save Enabled Models</button>
                <button id="btnModelsSelectAll" className="ghost" type="button" onClick=${() => applyModelSelection("all")}>Select All</button>
                <button id="btnModelsClear" className="ghost" type="button" onClick=${() => applyModelSelection("none")}>Clear</button>
                <button id="btnModelsInvert" className="ghost" type="button" onClick=${() => applyModelSelection("invert")}>Invert</button>
              </div>
              <${StatusLine} id="modelsStatus" status=${status.models} />
              <div id="modelsList" className="models-list">
                ${availableModels.map(
                  (modelID) => html`
                    <label className="model-item" key=${modelID}>
                      <input type="checkbox" checked=${selectedModelSet.has(modelID)} onChange=${() => toggleModel(modelID)} />
                      <code>${modelID}</code>
                    </label>
                  `
                )}
              </div>
            </section>

            <section id="section-pricing" ref=${registerSectionRef("section-pricing")} className="card section-card">
              <h2>Model Pricing</h2>
              <p className="muted">Configure pricing for enabled models in USD / ${pricingUnitTokens} tokens.</p>
              <div className="row">
                <button id="btnSavePricing" type="button" onClick=${handleSavePricing}>Save Pricing</button>
              </div>
              <${StatusLine} id="pricingStatus" status=${status.pricing} />
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Model</th>
                      <th>Input Price / 1000</th>
                      <th>Output Price / 1000</th>
                    </tr>
                  </thead>
                  <tbody id="pricingTableBody">
                    ${pricingRows.length === 0
                      ? html`<tr><td colSpan="3" className="muted">No enabled models.</td></tr>`
                      : pricingRows.map(
                          (row) => html`
                            <tr key=${row.modelID}>
                              <td><code>${row.modelID}</code></td>
                              <td>
                                <input
                                  className="price-input"
                                  type="number"
                                  min="0"
                                  step="0.000001"
                                  value=${row.input}
                                  onInput=${(event) => {
                                    const value = event.target.value;
                                    setPricingRows((previous) =>
                                      previous.map((item) => (item.modelID === row.modelID ? { ...item, input: value } : item))
                                    );
                                  }}
                                />
                              </td>
                              <td>
                                <input
                                  className="price-input"
                                  type="number"
                                  min="0"
                                  step="0.000001"
                                  value=${row.output}
                                  onInput=${(event) => {
                                    const value = event.target.value;
                                    setPricingRows((previous) =>
                                      previous.map((item) => (item.modelID === row.modelID ? { ...item, output: value } : item))
                                    );
                                  }}
                                />
                              </td>
                            </tr>
                          `
                        )}
                  </tbody>
                </table>
              </div>
            </section>

            <section id="section-billing" ref=${registerSectionRef("section-billing")} className="card section-card">
              <h2>Global Cost Guard</h2>
              <p className="muted">Set a global cost limit. A value of <code>0</code> means no limit. When exceeded, all client requests will be rejected.</p>
              <div className="row">
                <input id="globalCostLimit" type="number" min="0" step="0.000001" placeholder="global cost limit (USD)" value=${globalCostLimitInput} onInput=${(event) => setGlobalCostLimitInput(event.target.value)} />
                <button id="btnSaveBilling" type="button" onClick=${handleSaveBilling}>Save Global Limit</button>
              </div>
              <p className="muted">Current total cost: <strong id="currentTotalCost">$${formatUSD(currentTotalCost)}</strong></p>
              <${StatusLine} id="billingStatus" status=${status.billing} />
            </section>
            <section id="section-clients" ref=${registerSectionRef("section-clients")} className="card section-card">
              <h2>Client API Keys</h2>
              <form id="clientForm" className="grid" onSubmit=${handleSaveClient}>
                <input id="clientId" placeholder="id (example: team-a)" required value=${clientForm.id} onInput=${(event) => updateClientField("id", event.target.value)} />
                <input id="clientName" placeholder="name" required value=${clientForm.name} onInput=${(event) => updateClientField("name", event.target.value)} />
                <div className="field-with-action">
                  <input id="clientKey" placeholder="api_key" required value=${clientForm.apiKey} onInput=${(event) => updateClientField("apiKey", event.target.value)} />
                  <button id="btnGenerateApiKey" className="secondary" type="button" onClick=${() => {
                    updateClientField("apiKey", generateRandomAPIKey());
                    setSectionStatus("overview", "Generated API key.");
                  }}>Generate</button>
                </div>
                <input id="clientRPM" type="number" min="1" placeholder="max rpm (1200)" value=${clientForm.rpm} onInput=${(event) => updateClientField("rpm", event.target.value)} />
                <input id="clientConcurrent" type="number" min="1" placeholder="max concurrent (64)" value=${clientForm.concurrent} onInput=${(event) => updateClientField("concurrent", event.target.value)} />
                <input id="clientModels" placeholder="allowed model IDs, comma separated" value=${clientForm.models} onInput=${(event) => updateClientField("models", event.target.value)} />
                <label className="checkbox-row">
                  <input id="clientDisabled" type="checkbox" checked=${clientForm.disabled} onChange=${(event) => updateClientField("disabled", Boolean(event.target.checked))} />
                  Disable this API key
                </label>
                <button type="submit">Save Client</button>
              </form>

              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>ID</th>
                      <th>Name</th>
                      <th>API Key</th>
                      <th>Limits</th>
                      <th>Allowed Models</th>
                      <th>Status</th>
                      <th>Action</th>
                    </tr>
                  </thead>
                  <tbody id="clientTableBody">
                    ${clients.length === 0
                      ? html`<tr><td colSpan="7" className="muted">No client keys yet.</td></tr>`
                      : clients.map(
                          (client) => html`
                            <tr key=${client.id}>
                              <td><code>${client.id}</code></td>
                              <td>${client.name}</td>
                              <td><code>${client.api_key}</code></td>
                              <td>rpm=${formatNumber(client.max_requests_per_minute)} / conc=${formatNumber(client.max_concurrent)}</td>
                              <td>${(client.allowed_models || []).join(", ") || "*"}</td>
                              <td><span className=${`client-status ${client.disabled ? "disabled" : ""}`}>${client.disabled ? "Disabled" : "Enabled"}</span></td>
                              <td>
                                <div className="client-actions">
                                  <button className="ghost client-action-btn" type="button" onClick=${() => handleToggleClient(client)}>
                                    ${client.disabled ? "Enable" : "Disable"}
                                  </button>
                                  <button className="danger client-action-btn" type="button" onClick=${() => handleDeleteClient(client.id)}>Delete</button>
                                </div>
                              </td>
                            </tr>
                          `
                        )}
                  </tbody>
                </table>
              </div>
            </section>

            <section id="section-usage" ref=${registerSectionRef("section-usage")} className="card section-card">
              <h2>Usage</h2>
              <div className="row">
                <input id="usageFrom" type="date" value=${usageFilters.from} onInput=${(event) => setUsageFilters((previous) => ({ ...previous, from: event.target.value }))} />
                <input id="usageTo" type="date" value=${usageFilters.to} onInput=${(event) => setUsageFilters((previous) => ({ ...previous, to: event.target.value }))} />
                <input id="usageClientId" placeholder="client id (optional)" value=${usageFilters.clientId} onInput=${(event) => setUsageFilters((previous) => ({ ...previous, clientId: event.target.value }))} />
                <button id="btnUsage" type="button" onClick=${handleLoadUsage}>Load Usage</button>
              </div>

              <h3>By Client</h3>
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Client</th>
                      <th>Input</th>
                      <th>Output</th>
                      <th>Total</th>
                      <th>Requests</th>
                      <th>Cost (USD)</th>
                    </tr>
                  </thead>
                  <tbody id="usageClientBody">
                    ${usageData.byClient.length === 0
                      ? html`<tr><td colSpan="6" className="muted">No usage data.</td></tr>`
                      : usageData.byClient.map(
                          (row, index) => html`
                            <tr key=${`${row.client_id}-${index}`}>
                              <td><code>${row.client_id}</code></td>
                              <td>${formatNumber(row.input_tokens)}</td>
                              <td>${formatNumber(row.output_tokens)}</td>
                              <td>${formatNumber(row.total_tokens)}</td>
                              <td>${formatNumber(row.request_count)}</td>
                              <td>${formatUSD(row.cost_amount)}</td>
                            </tr>
                          `
                        )}
                  </tbody>
                </table>
              </div>

              <h3>By Client + Model</h3>
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Client</th>
                      <th>Model</th>
                      <th>Input</th>
                      <th>Output</th>
                      <th>Total</th>
                      <th>Requests</th>
                      <th>Cost (USD)</th>
                    </tr>
                  </thead>
                  <tbody id="usageModelBody">
                    ${usageData.byClientModel.length === 0
                      ? html`<tr><td colSpan="7" className="muted">No usage breakdown data.</td></tr>`
                      : usageData.byClientModel.map(
                          (row, index) => html`
                            <tr key=${`${row.client_id}-${row.model}-${index}`}>
                              <td><code>${row.client_id}</code></td>
                              <td><code>${row.model}</code></td>
                              <td>${formatNumber(row.input_tokens)}</td>
                              <td>${formatNumber(row.output_tokens)}</td>
                              <td>${formatNumber(row.total_tokens)}</td>
                              <td>${formatNumber(row.request_count)}</td>
                              <td>${formatUSD(row.cost_amount)}</td>
                            </tr>
                          `
                        )}
                  </tbody>
                </table>
              </div>
            </section>

            <section id="section-calls" ref=${registerSectionRef("section-calls")} className="card section-card">
              <h2>Recent Calls</h2>
              <div className="row">
                <input id="callsLimit" type="number" min="1" max="500" placeholder="page size" value=${callsFilters.limit} onInput=${(event) => setCallsFilters((previous) => ({ ...previous, limit: event.target.value }))} />
                <input
                  id="callsPage"
                  type="number"
                  min="1"
                  placeholder="page"
                  value=${callsFilters.page}
                  onInput=${(event) => setCallsFilters((previous) => ({ ...previous, page: event.target.value }))}
                  onKeyDown=${(event) => {
                    if (event.key !== "Enter") return;
                    event.preventDefault();
                    void handleLoadCalls();
                  }}
                />
                <input id="callsClientId" placeholder="client id (optional)" value=${callsFilters.clientId} onInput=${(event) => setCallsFilters((previous) => ({ ...previous, clientId: event.target.value }))} />
                <button id="btnCallsPrev" className="ghost" type="button" onClick=${() => handleLoadCalls(callsData.page - 1)} disabled=${!callsData.hasPrev}>Prev</button>
                <button id="btnCallsNext" className="ghost" type="button" onClick=${() => handleLoadCalls(callsData.page + 1)} disabled=${!callsData.hasNext}>Next</button>
                <button id="btnCalls" type="button" onClick=${() => handleLoadCalls()}>Load Calls</button>
              </div>
              <p id="callsPageInfo" className="muted">${callsPageInfo}</p>
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Time</th>
                      <th>Client</th>
                      <th>Model</th>
                      <th>Input</th>
                      <th>Output</th>
                      <th>Tokens</th>
                      <th>Cost (USD)</th>
                      <th>Status</th>
                      <th>Error</th>
                      <th>Prompt</th>
                      <th>Response</th>
                    </tr>
                  </thead>
                  <tbody id="callsBody">
                    ${callsData.items.length === 0
                      ? html`<tr><td colSpan="11" className="muted">No call records.</td></tr>`
                      : callsData.items.map(
                          (item, index) => html`
                            <tr key=${`${item.created_at}-${item.client_id}-${index}`}>
                              <td>${item.created_at}</td>
                              <td><code>${item.client_id}</code></td>
                              <td><code>${item.model}</code></td>
                              <td>${formatNumber(item.input_tokens)}</td>
                              <td>${formatNumber(item.output_tokens)}</td>
                              <td>${formatNumber(item.total_tokens)}</td>
                              <td>${formatUSD(item.cost_amount)}</td>
                              <td>${String(item.status_code)}</td>
                              <td>${item.error_message || ""}</td>
                              <td>${renderContentCell("Prompt", item.request_content)}</td>
                              <td>${renderContentCell("Response", item.response_content)}</td>
                            </tr>
                          `
                        )}
                  </tbody>
                </table>
              </div>
            </section>

            <section id="section-logs" ref=${registerSectionRef("section-logs")} className="card section-card">
              <h2>Debug Logs</h2>
              <p className="muted">Download debug files generated by this router.</p>
              <div className="row">
                <input id="logsLimit" type="number" min="1" max="1000" placeholder="limit" value=${logsLimit} onInput=${(event) => setLogsLimit(event.target.value)} />
                <button id="btnLogs" type="button" onClick=${handleLoadLogs}>Load Logs</button>
              </div>
              <${StatusLine} id="logsStatus" status=${status.logs} />
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>File</th>
                      <th>Size</th>
                      <th>Modified (UTC)</th>
                      <th>Action</th>
                    </tr>
                  </thead>
                  <tbody id="logsBody">
                    ${logsData.items.length === 0
                      ? html`<tr><td colSpan="4" className="muted">No debug logs found.</td></tr>`
                      : logsData.items.map(
                          (item) => html`
                            <tr key=${item.name}>
                              <td><code>${item.name || ""}</code></td>
                              <td>${formatBytes(item.size_bytes)}</td>
                              <td>${item.modified_at || ""}</td>
                              <td><button className="ghost" type="button" onClick=${() => handleDownloadLog(item.name || "")}>Download</button></td>
                            </tr>
                          `
                        )}
                  </tbody>
                </table>
              </div>
            </section>
          </div>
        </div>
      </main>

      ${contentModal.open
        ? html`
            <div id="contentModal" className="modal" role="dialog" aria-modal="true" aria-labelledby="contentModalTitle" onClick=${(event) => {
              if (event.target === event.currentTarget) {
                closeModal();
              }
            }}>
              <div className="modal-card">
                <div className="modal-head">
                  <h3 id="contentModalTitle">${contentModal.title}</h3>
                  <button id="btnCloseContentModal" className="ghost" type="button" onClick=${closeModal}>Close</button>
                </div>
                <div id="contentModalBody" className="modal-body log-rich" dangerouslySetInnerHTML=${{ __html: formatRichText(contentModal.content) }}></div>
              </div>
            </div>
          `
        : null}
    </div>
  `;
}

const container = document.getElementById("root");
if (!container) {
  throw new Error("Missing #root container");
}

createRoot(container).render(html`<${App} />`);
