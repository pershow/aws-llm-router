const state = {
  adminToken: localStorage.getItem("adminToken") || "",
  config: null,
  calls: {
    page: 1,
    pageSize: 100,
    total: 0,
    totalPages: 1,
  },
};

const el = {
  loginScreen: document.getElementById("loginScreen"),
  appShell: document.getElementById("appShell"),
  loginToken: document.getElementById("loginToken"),
  loginStatus: document.getElementById("loginStatus"),
  btnLogin: document.getElementById("btnLogin"),
  btnLogout: document.getElementById("btnLogout"),
  adminTokenInput: document.getElementById("adminTokenInput"),
  adminTokenStatus: document.getElementById("adminTokenStatus"),
  btnSaveAdminToken: document.getElementById("btnSaveAdminToken"),

  statusText: document.getElementById("statusText"),
  awsStatus: document.getElementById("awsStatus"),
  modelsStatus: document.getElementById("modelsStatus"),
  pricingStatus: document.getElementById("pricingStatus"),
  billingStatus: document.getElementById("billingStatus"),

  awsForm: document.getElementById("awsForm"),
  awsRegion: document.getElementById("awsRegion"),
  awsAccessKeyId: document.getElementById("awsAccessKeyId"),
  awsSecretAccessKey: document.getElementById("awsSecretAccessKey"),
  awsSessionToken: document.getElementById("awsSessionToken"),
  awsDefaultModelId: document.getElementById("awsDefaultModelId"),

  btnRefreshModels: document.getElementById("btnRefreshModels"),
  btnSaveModels: document.getElementById("btnSaveModels"),
  btnModelsSelectAll: document.getElementById("btnModelsSelectAll"),
  btnModelsClear: document.getElementById("btnModelsClear"),
  btnModelsInvert: document.getElementById("btnModelsInvert"),
  modelsList: document.getElementById("modelsList"),

  btnSavePricing: document.getElementById("btnSavePricing"),
  pricingTableBody: document.getElementById("pricingTableBody"),
  globalCostLimit: document.getElementById("globalCostLimit"),
  currentTotalCost: document.getElementById("currentTotalCost"),
  btnSaveBilling: document.getElementById("btnSaveBilling"),

  clientForm: document.getElementById("clientForm"),
  clientId: document.getElementById("clientId"),
  clientName: document.getElementById("clientName"),
  clientKey: document.getElementById("clientKey"),
  btnGenerateApiKey: document.getElementById("btnGenerateApiKey"),
  clientRPM: document.getElementById("clientRPM"),
  clientConcurrent: document.getElementById("clientConcurrent"),
  clientModels: document.getElementById("clientModels"),
  clientDisabled: document.getElementById("clientDisabled"),
  clientTableBody: document.getElementById("clientTableBody"),

  usageFrom: document.getElementById("usageFrom"),
  usageTo: document.getElementById("usageTo"),
  usageClientId: document.getElementById("usageClientId"),
  usageClientBody: document.getElementById("usageClientBody"),
  usageModelBody: document.getElementById("usageModelBody"),

  callsLimit: document.getElementById("callsLimit"),
  callsPage: document.getElementById("callsPage"),
  callsClientId: document.getElementById("callsClientId"),
  callsPageInfo: document.getElementById("callsPageInfo"),
  callsBody: document.getElementById("callsBody"),
  btnCallsPrev: document.getElementById("btnCallsPrev"),
  btnCallsNext: document.getElementById("btnCallsNext"),

  btnReload: document.getElementById("btnReload"),
  btnUsage: document.getElementById("btnUsage"),
  btnCalls: document.getElementById("btnCalls"),
  menuList: document.getElementById("menuList"),
  contentModal: document.getElementById("contentModal"),
  contentModalTitle: document.getElementById("contentModalTitle"),
  contentModalBody: document.getElementById("contentModalBody"),
  btnCloseContentModal: document.getElementById("btnCloseContentModal"),
};

const API_BASE = "/backendSalsSavvyLLMRouter";

function apiPath(path) {
  const normalized = String(path || "");
  if (!normalized.startsWith("/")) {
    return `${API_BASE}/${normalized}`;
  }
  return `${API_BASE}${normalized}`;
}

function setStatus(message, isError = false) {
  el.statusText.textContent = message;
  el.statusText.style.color = isError ? "#b91c1c" : "#526277";
}

function setLoginStatus(message, isError = false) {
  el.loginStatus.textContent = message;
  el.loginStatus.style.color = isError ? "#b91c1c" : "#526277";
}

function setAWSStatus(message, isError = false) {
  el.awsStatus.textContent = message;
  el.awsStatus.style.color = isError ? "#b91c1c" : "#526277";
}

function setModelsStatus(message, isError = false) {
  el.modelsStatus.textContent = message;
  el.modelsStatus.style.color = isError ? "#b91c1c" : "#526277";
}

function setPricingStatus(message, isError = false) {
  el.pricingStatus.textContent = message;
  el.pricingStatus.style.color = isError ? "#b91c1c" : "#526277";
}

function setBillingStatus(message, isError = false) {
  el.billingStatus.textContent = message;
  el.billingStatus.style.color = isError ? "#b91c1c" : "#526277";
}

function setAdminTokenStatus(message, isError = false) {
  el.adminTokenStatus.textContent = message;
  el.adminTokenStatus.style.color = isError ? "#b91c1c" : "#526277";
}

function showLogin() {
  el.loginScreen.classList.remove("hidden");
  el.appShell.classList.add("hidden");
  el.loginToken.focus();
}

function showApp() {
  el.loginScreen.classList.add("hidden");
  el.appShell.classList.remove("hidden");
  window.requestAnimationFrame(() => {
    syncMenuByViewport();
  });
}

function getMenuItems() {
  if (!el.menuList) {
    return [];
  }
  return Array.from(el.menuList.querySelectorAll(".menu-item[data-target]"));
}

function setActiveMenu(targetID) {
  for (const item of getMenuItems()) {
    item.classList.toggle("active", item.getAttribute("data-target") === targetID);
  }
}

function navigateToSection(targetID, smooth = true) {
  const section = document.getElementById(String(targetID || "").trim());
  if (!section) {
    return;
  }
  setActiveMenu(section.id);
  section.scrollIntoView({
    behavior: smooth ? "smooth" : "auto",
    block: "start",
  });
}

function syncMenuByViewport() {
  const items = getMenuItems();
  if (items.length === 0) {
    return;
  }

  const offset = 150;
  let activeID = items[0].getAttribute("data-target");

  for (const item of items) {
    const targetID = item.getAttribute("data-target");
    const section = document.getElementById(String(targetID || "").trim());
    if (!section) {
      continue;
    }
    const top = section.getBoundingClientRect().top;
    if (top - offset <= 0) {
      activeID = targetID;
    } else {
      break;
    }
  }

  setActiveMenu(activeID);
}

let menuNavigationBound = false;
function bindMenuNavigation() {
  if (menuNavigationBound || !el.menuList) {
    return;
  }
  menuNavigationBound = true;

  el.menuList.addEventListener("click", (event) => {
    const target = event.target instanceof Element ? event.target : null;
    if (!target) {
      return;
    }
    const button = target.closest(".menu-item[data-target]");
    if (!button) {
      return;
    }
    navigateToSection(button.getAttribute("data-target"), true);
  });

  let ticking = false;
  const sync = () => {
    if (ticking) {
      return;
    }
    ticking = true;
    window.requestAnimationFrame(() => {
      syncMenuByViewport();
      ticking = false;
    });
  };

  window.addEventListener("scroll", sync, { passive: true });
  window.addEventListener("resize", sync);
  sync();
}

function authHeaders() {
  const token = String(state.adminToken || "").trim();
  return {
    "Content-Type": "application/json",
    ...(token
      ? {
          Authorization: `Bearer ${token}`,
          "x-admin-token": token,
        }
      : {}),
  };
}

async function request(url, options = {}) {
  const response = await fetch(url, {
    ...options,
    headers: {
      ...(options.headers || {}),
      ...authHeaders(),
    },
  });

  const contentType = response.headers.get("content-type") || "";
  const body = contentType.includes("application/json") ? await response.json() : {};
  if (!response.ok) {
    const errorMessage = body.error || body?.error?.message || `HTTP ${response.status}`;
    const error = new Error(errorMessage);
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

function parsePositiveInt(value, fallback) {
  const parsed = Number.parseInt(String(value ?? ""), 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return fallback;
  }
  return parsed;
}

function setDefaultDates() {
  const today = new Date();
  const from = new Date(today);
  from.setDate(from.getDate() - 7);
  el.usageTo.value = today.toISOString().slice(0, 10);
  el.usageFrom.value = from.toISOString().slice(0, 10);
}

function escapeHTML(value) {
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}

function escapeAttr(value) {
  return escapeHTML(value).replace(/"/g, "&quot;");
}

function normalizeStrings(items) {
  return Array.from(new Set((items || []).map((x) => String(x || "").trim()).filter((x) => x.length > 0))).sort();
}

function getModelCheckboxes() {
  return Array.from(el.modelsList.querySelectorAll("input[data-model-id]"));
}

function selectedEnabledModelIDs() {
  return normalizeStrings(
    getModelCheckboxes()
      .filter((node) => node.checked)
      .map((node) => node.getAttribute("data-model-id"))
  );
}

function applyModelSelection(mode) {
  const inputs = getModelCheckboxes();
  if (inputs.length === 0) {
    updateModelSelectionStatus();
    return;
  }

  if (mode === "all") {
    inputs.forEach((input) => {
      input.checked = true;
    });
  } else if (mode === "none") {
    inputs.forEach((input) => {
      input.checked = false;
    });
  } else if (mode === "invert") {
    inputs.forEach((input) => {
      input.checked = !input.checked;
    });
  }

  updateModelSelectionStatus();
}

function updateModelSelectionStatus() {
  const total = getModelCheckboxes().length;
  const selected = getModelCheckboxes().filter((input) => input.checked).length;
  if (total === 0) {
    setModelsStatus("No models available. Check AWS config and click Refresh.", true);
    return;
  }
  setModelsStatus(`${selected}/${total} models enabled.`);
}

function renderModels(availableModelIDs, enabledModelIDs) {
  const available = normalizeStrings(availableModelIDs);
  const explicitEnabled = normalizeStrings(enabledModelIDs);
  const enabledSet = new Set((explicitEnabled.length > 0 ? explicitEnabled : available).map((x) => x.trim()));

  el.modelsList.innerHTML = "";
  if (available.length === 0) {
    updateModelSelectionStatus();
    return;
  }

  for (const modelID of available) {
    const label = document.createElement("label");
    label.className = "model-item";
    label.innerHTML = `
      <input type="checkbox" data-model-id="${escapeAttr(modelID)}" ${enabledSet.has(modelID) ? "checked" : ""} />
      <code>${escapeHTML(modelID)}</code>
    `;
    el.modelsList.appendChild(label);
  }

  updateModelSelectionStatus();
}

function renderPricing(enabledModelIDs, pricingItems) {
  const pricingByModel = new Map();
  for (const item of pricingItems || []) {
    const modelID = String(item.model_id || "").trim();
    if (!modelID) continue;
    pricingByModel.set(modelID, {
      input: Number(item.input_price_per_1k || 0),
      output: Number(item.output_price_per_1k || 0),
    });
  }

  const modelIDs = normalizeStrings(enabledModelIDs || []);

  el.pricingTableBody.innerHTML = "";
  if (modelIDs.length === 0) {
    setPricingStatus("No models available for pricing.", true);
    return;
  }

  for (const modelID of modelIDs) {
    const pricing = pricingByModel.get(modelID) || { input: 0, output: 0 };
    const tr = document.createElement("tr");
    tr.setAttribute("data-model-id", modelID);
    tr.innerHTML = `
      <td><code>${escapeHTML(modelID)}</code></td>
      <td>
        <input class="price-input" type="number" min="0" step="0.000001" data-field="input" value="${escapeAttr(
          String(pricing.input)
        )}" />
      </td>
      <td>
        <input class="price-input" type="number" min="0" step="0.000001" data-field="output" value="${escapeAttr(
          String(pricing.output)
        )}" />
      </td>
    `;
    el.pricingTableBody.appendChild(tr);
  }

  setPricingStatus(`Pricing rows: ${modelIDs.length}`);
}

function renderBilling(billingConfig, currentTotalCost) {
  const limitValue = Number(billingConfig?.global_cost_limit_usd || 0);
  const safeLimitValue = Number.isFinite(limitValue) && limitValue >= 0 ? limitValue : 0;
  const totalValue = Number.isFinite(Number(currentTotalCost)) ? Number(currentTotalCost) : 0;

  el.globalCostLimit.value = String(safeLimitValue);
  el.currentTotalCost.textContent = `$${formatUSD(totalValue)}`;

  if (safeLimitValue <= 0) {
    setBillingStatus("Global cost limit: unlimited.");
    return;
  }

  if (totalValue >= safeLimitValue) {
    setBillingStatus(
      `Global cost limit reached: $${formatUSD(totalValue)} / $${formatUSD(safeLimitValue)}.`,
      true
    );
    return;
  }

  const remaining = Math.max(0, safeLimitValue - totalValue);
  setBillingStatus(`Remaining budget: $${formatUSD(remaining)} (limit $${formatUSD(safeLimitValue)}).`);
}

function collectPricingItems() {
  const rows = Array.from(el.pricingTableBody.querySelectorAll("tr[data-model-id]"));
  const items = [];

  for (const row of rows) {
    const modelID = String(row.getAttribute("data-model-id") || "").trim();
    if (!modelID) continue;

    const inputRaw = row.querySelector("input[data-field='input']")?.value ?? "0";
    const outputRaw = row.querySelector("input[data-field='output']")?.value ?? "0";

    const input = Number(inputRaw === "" ? 0 : inputRaw);
    const output = Number(outputRaw === "" ? 0 : outputRaw);

    if (!Number.isFinite(input) || !Number.isFinite(output) || input < 0 || output < 0) {
      throw new Error(`Invalid pricing for model: ${modelID}`);
    }

    items.push({
      model_id: modelID,
      input_price_per_1k: input,
      output_price_per_1k: output,
    });
  }

  return items;
}

function renderClients(clients) {
  el.clientTableBody.innerHTML = "";
  for (const client of clients || []) {
    const allowedModels = (client.allowed_models || []).join(", ");
    const isDisabled = Boolean(client.disabled);
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><code>${escapeHTML(client.id)}</code></td>
      <td>${escapeHTML(client.name)}</td>
      <td><code>${escapeHTML(client.api_key)}</code></td>
      <td>rpm=${formatNumber(client.max_requests_per_minute)} / conc=${formatNumber(client.max_concurrent)}</td>
      <td>${escapeHTML(allowedModels || "*")}</td>
      <td><span class="client-status ${isDisabled ? "disabled" : ""}">${isDisabled ? "Disabled" : "Enabled"}</span></td>
      <td>
        <div class="client-actions">
          <button class="ghost client-action-btn" type="button" data-action="toggle" data-id="${escapeAttr(client.id)}">${
            isDisabled ? "Enable" : "Disable"
          }</button>
          <button class="danger client-action-btn" type="button" data-action="delete" data-id="${escapeAttr(client.id)}">Delete</button>
        </div>
      </td>
    `;
    tr.querySelector("button[data-action='toggle']").addEventListener("click", async () => {
      const nextDisabled = !isDisabled;
      try {
        await request(apiPath("/config/clients"), {
          method: "POST",
          body: JSON.stringify({
            id: client.id,
            name: client.name,
            api_key: client.api_key,
            max_requests_per_minute: Number(client.max_requests_per_minute || 0),
            max_concurrent: Number(client.max_concurrent || 0),
            allowed_models: Array.isArray(client.allowed_models) ? client.allowed_models : [],
            disabled: nextDisabled,
          }),
        });
        await loadConfig();
        setStatus(`Client ${client.id} ${nextDisabled ? "disabled" : "enabled"}.`);
      } catch (error) {
        setStatus(error.message, true);
      }
    });
    tr.querySelector("button[data-action='delete']").addEventListener("click", async () => {
      if (!confirm(`Delete client ${client.id}?`)) return;
      try {
        await request(apiPath(`/config/clients?id=${encodeURIComponent(client.id)}`), { method: "DELETE" });
        await loadConfig();
        setStatus("Client deleted.");
      } catch (error) {
        setStatus(error.message, true);
      }
    });
    el.clientTableBody.appendChild(tr);
  }
}

function renderUsage(byClient, byClientModel, totalCost) {
  el.usageClientBody.innerHTML = "";
  for (const row of byClient || []) {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><code>${escapeHTML(row.client_id)}</code></td>
      <td>${formatNumber(row.input_tokens)}</td>
      <td>${formatNumber(row.output_tokens)}</td>
      <td>${formatNumber(row.total_tokens)}</td>
      <td>${formatNumber(row.request_count)}</td>
      <td>${formatUSD(row.cost_amount)}</td>
    `;
    el.usageClientBody.appendChild(tr);
  }

  el.usageModelBody.innerHTML = "";
  for (const row of byClientModel || []) {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><code>${escapeHTML(row.client_id)}</code></td>
      <td><code>${escapeHTML(row.model)}</code></td>
      <td>${formatNumber(row.input_tokens)}</td>
      <td>${formatNumber(row.output_tokens)}</td>
      <td>${formatNumber(row.total_tokens)}</td>
      <td>${formatNumber(row.request_count)}</td>
      <td>${formatUSD(row.cost_amount)}</td>
    `;
    el.usageModelBody.appendChild(tr);
  }

  setStatus(`Usage loaded. Total cost: $${formatUSD(totalCost)}`);
}

function closeContentModal() {
  if (!el.contentModal) {
    return;
  }
  el.contentModal.classList.add("hidden");
  if (el.contentModalTitle) {
    el.contentModalTitle.textContent = "Content";
  }
  if (el.contentModalBody) {
    el.contentModalBody.innerHTML = "";
  }
}

function openContentModal(title, content) {
  if (!el.contentModal || !el.contentModalTitle || !el.contentModalBody) {
    return;
  }
  el.contentModalTitle.textContent = String(title || "Content");
  el.contentModalBody.innerHTML = formatRichText(content);
  el.contentModal.classList.remove("hidden");
}

function createContentCell(title, content) {
  const td = document.createElement("td");
  const text = String(content || "");
  if (!text) {
    td.innerHTML = "<span class=\"muted\">-</span>";
    return td;
  }

  const container = document.createElement("div");
  container.className = "content-cell";

  const summary = document.createElement("span");
  summary.className = "content-summary";
  summary.textContent = text.length > 80 ? `${text.slice(0, 80)}...` : text;

  const button = document.createElement("button");
  button.type = "button";
  button.className = "ghost content-view-btn";
  button.textContent = "查看";
  button.addEventListener("click", () => {
    openContentModal(title, text);
  });

  container.appendChild(summary);
  container.appendChild(button);
  td.appendChild(container);
  return td;
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
  let html = "";
  let match;
  while ((match = fencePattern.exec(text)) !== null) {
    const [fullMatch, lang = "", code = ""] = match;
    const index = match.index;
    if (index > last) {
      html += formatRichInline(text.slice(last, index));
    }
    const langLabel = lang ? `<span class="code-lang">${escapeHTML(lang)}</span>` : "";
    html += `<pre class="log-pre">${langLabel}<code>${escapeHTML(code)}</code></pre>`;
    last = index + fullMatch.length;
  }
  if (last < text.length) {
    html += formatRichInline(text.slice(last));
  }
  return html;
}

function formatRichInline(segment) {
  let html = escapeHTML(segment);
  html = html.replace(/`([^`\n]+)`/g, "<code>$1</code>");
  html = html.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
  html = html.replace(/(^|[\s(])(https?:\/\/[^\s<]+)/g, '$1<a href="$2" target="_blank" rel="noreferrer noopener">$2</a>');
  html = html.replace(/\n/g, "<br>");
  return html;
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

function renderCallsPagination(meta) {
  const page = parsePositiveInt(meta?.page, 1);
  const pageSize = parsePositiveInt(meta?.page_size, parsePositiveInt(el.callsLimit.value, 100));
  const total = Math.max(0, Number(meta?.total || 0));
  const totalPages = Math.max(1, parsePositiveInt(meta?.total_pages, 1));

  state.calls.page = page;
  state.calls.pageSize = pageSize;
  state.calls.total = total;
  state.calls.totalPages = totalPages;

  el.callsPage.value = String(page);
  el.callsLimit.value = String(pageSize);
  el.btnCallsPrev.disabled = !(Boolean(meta?.has_prev) && page > 1);
  el.btnCallsNext.disabled = !(Boolean(meta?.has_next) && page < totalPages);

  if (total <= 0) {
    el.callsPageInfo.textContent = "No call records.";
    return;
  }

  el.callsPageInfo.textContent = `Page ${page}/${totalPages} · ${formatNumber(total)} records`;
}

function renderCalls(items, totalCost, pagination) {
  el.callsBody.innerHTML = "";
  for (const item of items || []) {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td>${escapeHTML(item.created_at)}</td>
      <td><code>${escapeHTML(item.client_id)}</code></td>
      <td><code>${escapeHTML(item.model)}</code></td>
      <td>${formatNumber(item.input_tokens)}</td>
      <td>${formatNumber(item.output_tokens)}</td>
      <td>${formatNumber(item.total_tokens)}</td>
      <td>${formatUSD(item.cost_amount)}</td>
      <td>${escapeHTML(String(item.status_code))}</td>
      <td>${escapeHTML(item.error_message || "")}</td>
    `;
    tr.appendChild(createContentCell("Prompt", item.request_content));
    tr.appendChild(createContentCell("Response", item.response_content));
    el.callsBody.appendChild(tr);
  }

  renderCallsPagination(pagination || {});
  setStatus(
    `Calls loaded. Page ${state.calls.page}/${state.calls.totalPages}. Total ${formatNumber(state.calls.total)} records. Cost: $${formatUSD(totalCost)}`
  );
}

function renderAWSConfig(awsConfig, bedrockClientReady) {
  const aws = awsConfig || {};
  el.awsRegion.value = aws.region || "";
  el.awsAccessKeyId.value = aws.access_key_id || "";
  el.awsSecretAccessKey.value = aws.secret_access_key || "";
  el.awsSessionToken.value = aws.session_token || "";
  el.awsDefaultModelId.value = aws.default_model_id || "";
  setAWSStatus(
    bedrockClientReady
      ? "Bedrock client is ready."
      : "Bedrock client is not ready. Save valid AWS config.",
    !bedrockClientReady
  );
}

function generateRandomAPIKey() {
  const prefix = "sk-router-";
  const size = 24;

  if (window.crypto && typeof window.crypto.getRandomValues === "function") {
    const bytes = new Uint8Array(size);
    window.crypto.getRandomValues(bytes);
    const payload = Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
    return `${prefix}${payload}`;
  }

  let payload = "";
  for (let i = 0; i < size; i += 1) {
    payload += Math.floor(Math.random() * 256)
      .toString(16)
      .padStart(2, "0");
  }
  return `${prefix}${payload}`;
}

async function loadConfig() {
  const data = await request(apiPath("/config"));
  state.config = data;
  renderAWSConfig(data.aws || {}, Boolean(data.bedrock_client_ready));
  renderModels(data.available_models || [], data.enabled_model_ids || []);
  renderPricing(data.enabled_model_ids || [], data.model_pricing || []);
  renderBilling(data.billing || {}, Number(data.current_total_cost || 0));
  renderClients(data.clients || []);
  setPricingStatus(`Pricing unit: USD / ${Number(data.pricing_unit_tokens || 1000)} tokens`);
}

async function loadUsage() {
  const params = new URLSearchParams();
  if (el.usageFrom.value) params.set("from", el.usageFrom.value);
  if (el.usageTo.value) params.set("to", el.usageTo.value);
  if (el.usageClientId.value.trim()) params.set("client_id", el.usageClientId.value.trim());

  const data = await request(apiPath(`/usage?${params.toString()}`));
  renderUsage(data.by_client || [], data.by_client_model || [], Number(data.total_cost || 0));
}

async function loadCalls(pageOverride) {
  const pageSize = parsePositiveInt(el.callsLimit.value, state.calls.pageSize || 100);
  const page = parsePositiveInt(pageOverride ?? el.callsPage.value, state.calls.page || 1);

  const params = new URLSearchParams();
  params.set("limit", String(pageSize));
  params.set("page", String(page));
  if (el.callsClientId.value.trim()) params.set("client_id", el.callsClientId.value.trim());
  const data = await request(apiPath(`/calls?${params.toString()}`));
  renderCalls(data.items || [], Number(data.total_cost || 0), data);
}

function parseAllowedModels(value) {
  return normalizeStrings(
    value
      .split(",")
      .map((x) => x.trim())
      .filter((x) => x.length > 0)
  );
}

async function connect() {
  const token = el.loginToken.value.trim();
  if (!token) {
    setLoginStatus("Please input admin token.", true);
    return;
  }

  state.adminToken = token;
  localStorage.setItem("adminToken", token);

  try {
    await loadConfig();
    await loadUsage();
    await loadCalls(1);
    setLoginStatus("", false);
    setAdminTokenStatus("");
    showApp();
    setStatus("Connected.");
  } catch (error) {
    setLoginStatus(error.message, true);
    showLogin();
  }
}

function logout() {
  closeContentModal();
  state.adminToken = "";
  localStorage.removeItem("adminToken");
  el.loginToken.value = "";
  setStatus("Logged out.");
  showLogin();
}

async function init() {
  setDefaultDates();
  bindMenuNavigation();

  el.callsLimit.value = String(state.calls.pageSize);
  el.callsPage.value = String(state.calls.page);
  renderCallsPagination({
    page: state.calls.page,
    page_size: state.calls.pageSize,
    total: state.calls.total,
    total_pages: state.calls.totalPages,
    has_prev: false,
    has_next: false,
  });

  el.loginToken.value = state.adminToken;

  el.btnLogin.addEventListener("click", async () => {
    await connect();
  });

  el.loginToken.addEventListener("keydown", async (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      await connect();
    }
  });

  el.btnLogout.addEventListener("click", () => {
    logout();
  });

  el.btnReload.addEventListener("click", async () => {
    try {
      await loadConfig();
      await loadUsage();
      await loadCalls();
      setStatus("Reloaded.");
    } catch (error) {
      setStatus(error.message, true);
    }
  });

  el.btnSaveAdminToken.addEventListener("click", async () => {
    try {
      const newToken = el.adminTokenInput.value.trim();
      if (!newToken) {
        setAdminTokenStatus("New admin token is required.", true);
        return;
      }
      await request(apiPath("/config/admin-token"), {
        method: "POST",
        body: JSON.stringify({
          admin_token: newToken,
        }),
      });
      state.adminToken = newToken;
      localStorage.setItem("adminToken", newToken);
      el.loginToken.value = newToken;
      el.adminTokenInput.value = "";
      setAdminTokenStatus("Admin token updated.");
      setStatus("Admin token updated.");
    } catch (error) {
      if (error?.status === 401 || error?.status === 403) {
        setAdminTokenStatus("当前登录令牌无效或已过期，请重新登录后再修改。", true);
        return;
      }
      setAdminTokenStatus(error.message, true);
    }
  });

  el.awsForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    try {
      await request(apiPath("/config/aws"), {
        method: "POST",
        body: JSON.stringify({
          region: el.awsRegion.value.trim(),
          access_key_id: el.awsAccessKeyId.value.trim(),
          secret_access_key: el.awsSecretAccessKey.value.trim(),
          session_token: el.awsSessionToken.value.trim(),
          default_model_id: el.awsDefaultModelId.value.trim(),
        }),
      });
      await loadConfig();
      setStatus("AWS config saved.");
    } catch (error) {
      setAWSStatus(error.message, true);
    }
  });

  el.btnRefreshModels.addEventListener("click", async () => {
    try {
      await request(apiPath("/config/models/refresh"), { method: "POST", body: "{}" });
      await loadConfig();
      setStatus("Model list refreshed from AWS.");
    } catch (error) {
      setModelsStatus(error.message, true);
    }
  });

  el.btnSaveModels.addEventListener("click", async () => {
    try {
      const enabledModelIDs = selectedEnabledModelIDs();
      if (enabledModelIDs.length === 0) {
        setModelsStatus("Select at least one model.", true);
        return;
      }
      await request(apiPath("/config/models"), {
        method: "POST",
        body: JSON.stringify({ enabled_model_ids: enabledModelIDs }),
      });
      await loadConfig();
      setStatus("Enabled models saved.");
    } catch (error) {
      setModelsStatus(error.message, true);
    }
  });

  el.btnModelsSelectAll.addEventListener("click", () => {
    applyModelSelection("all");
  });

  el.btnModelsClear.addEventListener("click", () => {
    applyModelSelection("none");
  });

  el.btnModelsInvert.addEventListener("click", () => {
    applyModelSelection("invert");
  });

  el.btnSavePricing.addEventListener("click", async () => {
    try {
      const items = collectPricingItems();
      await request(apiPath("/config/model-pricing"), {
        method: "POST",
        body: JSON.stringify({ items }),
      });
      await loadConfig();
      setPricingStatus("Model pricing saved.");
    } catch (error) {
      setPricingStatus(error.message, true);
    }
  });

  el.btnSaveBilling.addEventListener("click", async () => {
    try {
      const raw = el.globalCostLimit.value.trim();
      const limit = Number(raw === "" ? 0 : raw);
      if (!Number.isFinite(limit) || limit < 0) {
        setBillingStatus("Invalid global cost limit.", true);
        return;
      }
      await request(apiPath("/config/billing"), {
        method: "POST",
        body: JSON.stringify({
          global_cost_limit_usd: limit,
        }),
      });
      await loadConfig();
      setStatus("Global cost limit saved.");
    } catch (error) {
      setBillingStatus(error.message, true);
    }
  });

  el.modelsList.addEventListener("change", () => {
    updateModelSelectionStatus();
  });

  el.btnGenerateApiKey.addEventListener("click", () => {
    el.clientKey.value = generateRandomAPIKey();
    el.clientKey.focus();
    setStatus("Generated API key.");
  });

  el.clientForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    try {
      await request(apiPath("/config/clients"), {
        method: "POST",
        body: JSON.stringify({
          id: el.clientId.value.trim(),
          name: el.clientName.value.trim(),
          api_key: el.clientKey.value.trim(),
          max_requests_per_minute: Number(el.clientRPM.value || 0),
          max_concurrent: Number(el.clientConcurrent.value || 0),
          allowed_models: parseAllowedModels(el.clientModels.value),
          disabled: Boolean(el.clientDisabled?.checked),
        }),
      });
      el.clientForm.reset();
      await loadConfig();
      setStatus("Client saved.");
    } catch (error) {
      setStatus(error.message, true);
    }
  });

  el.btnUsage.addEventListener("click", async () => {
    try {
      await loadUsage();
    } catch (error) {
      setStatus(error.message, true);
    }
  });

  el.btnCalls.addEventListener("click", async () => {
    try {
      await loadCalls();
    } catch (error) {
      setStatus(error.message, true);
    }
  });

  el.btnCallsPrev.addEventListener("click", async () => {
    try {
      await loadCalls(state.calls.page - 1);
    } catch (error) {
      setStatus(error.message, true);
    }
  });

  el.btnCallsNext.addEventListener("click", async () => {
    try {
      await loadCalls(state.calls.page + 1);
    } catch (error) {
      setStatus(error.message, true);
    }
  });

  el.callsPage.addEventListener("keydown", async (event) => {
    if (event.key !== "Enter") {
      return;
    }
    event.preventDefault();
    try {
      await loadCalls();
    } catch (error) {
      setStatus(error.message, true);
    }
  });

  if (el.btnCloseContentModal) {
    el.btnCloseContentModal.addEventListener("click", () => {
      closeContentModal();
    });
  }

  if (el.contentModal) {
    el.contentModal.addEventListener("click", (event) => {
      if (event.target === el.contentModal) {
        closeContentModal();
      }
    });
  }

  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && el.contentModal && !el.contentModal.classList.contains("hidden")) {
      closeContentModal();
    }
  });

  if (state.adminToken) {
    await connect();
    return;
  }

  showLogin();
  setLoginStatus("Use admin token to login. Default is admin123.");
}

init();


