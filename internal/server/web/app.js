const apiKeyStorage = "gtcp_api_key";
const refreshButton = document.getElementById("refresh");
const modal = document.getElementById("auth-modal");
const apiKeyInput = document.getElementById("api-key-input");
const saveKeyButton = document.getElementById("save-key");
const connectionDot = document.getElementById("connection-dot");
const connectionText = document.getElementById("connection-text");

let apiKey = localStorage.getItem(apiKeyStorage) || "";

function showModal() {
  modal.classList.remove("hidden");
}

function hideModal() {
  modal.classList.add("hidden");
}

function setConnection(status, message) {
  connectionDot.style.background = status === "ok" ? "#34d399" : "#f97316";
  connectionText.textContent = message;
}

function setCounts({ rigs, agents, convoys, hooks, alerts }) {
  document.getElementById("rig-count").textContent = rigs.length;
  document.getElementById("agent-count").textContent = agents.length;
  document.getElementById("convoy-count").textContent = convoys.length;
  document.getElementById("hook-count").textContent = hooks.length;
  document.getElementById("alert-count").textContent = alerts.length;
}

function renderTable(targetId, rows, renderRow) {
  const body = document.getElementById(targetId);
  body.innerHTML = rows.map(renderRow).join("");
}

function renderActivity(events) {
  const container = document.getElementById("activity");
  container.innerHTML = events
    .map((event) => {
      const time = event.occurred_at ? new Date(event.occurred_at).toLocaleTimeString() : "";
      const parts = [event.type, event.rig, event.agent, event.status].filter(Boolean).join(" · ");
      return `<div class="activity-item"><strong>${time}</strong> ${parts} ${event.message || ""}</div>`;
    })
    .join("");
}

async function fetchJSON(path) {
  const response = await fetch(path, {
    headers: {
      Authorization: `Bearer ${apiKey}`,
    },
  });
  if (!response.ok) {
    throw new Error(`Request failed: ${response.status}`);
  }
  return response.json();
}

async function refresh() {
  if (!apiKey) {
    showModal();
    return;
  }
  try {
    const [rigs, agents, convoys, hooks, events, alerts] = await Promise.all([
      fetchJSON("/v1/rigs"),
      fetchJSON("/v1/agents"),
      fetchJSON("/v1/convoys"),
      fetchJSON("/v1/hooks"),
      fetchJSON("/v1/activity?limit=50"),
      fetchJSON("/v1/alerts"),
    ]);

    setCounts({ rigs, agents, convoys, hooks, alerts });
    renderTable("rigs-table", rigs, (rig) => `<tr><td>${rig.name}</td><td>${rig.path || "-"}</td></tr>`);
    renderTable(
      "agents-table",
      agents,
      (agent) =>
        `<tr><td>${agent.name}</td><td>${agent.rig || "-"}</td><td>${agent.status || "-"}</td><td>${formatTime(agent.last_seen_at)}</td></tr>`
    );
    renderTable(
      "convoys-table",
      convoys,
      (convoy) => `<tr><td>${convoy.name}</td><td>${convoy.rig || "-"}</td><td>${convoy.status || "-"}</td></tr>`
    );
    renderTable(
      "hooks-table",
      hooks,
      (hook) => `<tr><td>${hook.name}</td><td>${hook.rig || "-"}</td><td>${hook.status || "-"}</td></tr>`
    );
    renderActivity(events);
    setConnection("ok", "Connected");
    hideModal();
  } catch (err) {
    setConnection("warn", "Auth failed or server offline");
    showModal();
  }
}

function formatTime(value) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "-";
  }
  return date.toLocaleTimeString();
}

saveKeyButton.addEventListener("click", () => {
  const value = apiKeyInput.value.trim();
  if (!value) {
    return;
  }
  apiKey = value;
  localStorage.setItem(apiKeyStorage, apiKey);
  refresh();
});

refreshButton.addEventListener("click", () => {
  refresh();
});

if (!apiKey) {
  showModal();
} else {
  hideModal();
}

refresh();
setInterval(refresh, 15000);
