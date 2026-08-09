// AIProxy Wails GUI 前端逻辑
// 通过 window.go.wailsapp.App.* 调用 Go 绑定方法

/* ---------- 工具 ---------- */

// 调用 Go 绑定方法并处理 (value, error) 返回
async function callGo(fn, ...args) {
  const res = await fn(...args);
  // Wails 对返回 (value, error) 的方法打包为 {value, error}
  if (res && typeof res === "object" && "error" in res && "value" in res) {
    if (res.error) throw new Error(res.error);
    return res.value;
  }
  return res;
}

// 千分位格式化
function formatInt(n) {
  if (n === null || n === undefined) return "--";
  const neg = n < 0;
  let s = String(Math.abs(n));
  let out = "";
  for (let i = 0; i < s.length; i++) {
    if (i > 0 && (s.length - i) % 3 === 0) out += ",";
    out += s[i];
  }
  return (neg ? "-" : "") + out;
}

// Token 显示模式（设置页可切换）：auto（≥100 万显示为 M）| raw（原始千分位）
let tokenDisplayMode = "auto";

// Token 显示：auto 模式下 ≥100 万显示为 M（保留 2 位小数并去尾零），如 1250000 → "1.25M"、2000000 → "2M"；
// raw 模式一律千分位显示原始数值
function formatTokens(n) {
  if (n === null || n === undefined) return "--";
  if (tokenDisplayMode !== "raw" && Math.abs(n) >= 1000000) {
    return (n / 1000000).toFixed(2).replace(/\.?0+$/, "") + "M";
  }
  return formatInt(n);
}

// 转义 HTML（用字符码构造实体，避免编辑器 HTML 实体转义干扰）
function esc(s) {
  if (s === null || s === undefined) return "";
  var a = String.fromCharCode(38); // &
  var s2 = String.fromCharCode(59); // ;
  return String(s)
    .replace(/&/g, a + "amp" + s2)
    .replace(/</g, a + "lt" + s2)
    .replace(/>/g, a + "gt" + s2)
    .replace(/"/g, a + "quot" + s2);
}

// 字符串截断（按字符）
function truncate(s, max) {
  if (!s || s.length <= max) return s || "";
  return s.slice(0, max) + "...";
}

const byId = (id) => document.getElementById(id);

// 通用自动刷新管理器：根据下拉框选择设置定时刷新，并显示倒计时
function createAutoRefresh(selectId, refreshFn, countdownId) {
  const sel = byId(selectId);
  const countdown = countdownId ? byId(countdownId) : null;
  let tickTimer = null;
  let remaining = 0;

  function render() {
    if (countdown) {
      countdown.textContent =
        remaining > 0 ? remaining + " 秒后刷新" : "正在刷新...";
    }
  }

  function clearTimers() {
    clearInterval(tickTimer);
    tickTimer = null;
  }

  function apply() {
    clearTimers();
    const v = parseInt(sel.value, 10);
    if (v <= 0) {
      remaining = 0;
      if (countdown) countdown.textContent = "已关闭";
      return;
    }
    remaining = v;
    render();
    tickTimer = setInterval(() => {
      remaining--;
      if (remaining <= 0) {
        clearTimers();
        render(); // 显示"正在刷新..."
        Promise.resolve(refreshFn()).finally(apply);
      } else {
        render();
      }
    }, 1000);
  }

  // 手动触发刷新时重置倒计时
  function reset() {
    apply();
  }

  sel.addEventListener("change", () => {
    apply();
    toast(
      parseInt(sel.value, 10) === 0
        ? "已关闭自动刷新"
        : "自动刷新间隔已设为 " + sel.value + " 秒"
    );
  });

  apply();
  return { reset };
}

/* ---------- 导航 ---------- */

function showPage(name) {
  document.querySelectorAll(".page").forEach((p) => (p.hidden = true));
  byId("page-" + name).hidden = false;
  document.querySelectorAll(".nav-item").forEach((b) => {
    b.classList.toggle("active", b.dataset.page === name);
  });
  // 进入页面时刷新对应数据
  const refreshers = {
    dashboard: refreshDashboard,
    channels: refreshChannels,
    aliases: refreshAliases,
    models: refreshModels,
    stats: refreshStats,
    logs: refreshLogs,
    settings: loadSettings,
  };
  if (refreshers[name]) refreshers[name]();
}

document.querySelectorAll(".nav-item").forEach((b) => {
  b.addEventListener("click", () => showPage(b.dataset.page));
});

/* ---------- Toast 提示 ---------- */

let toastTimer = null;
function toast(msg, isError) {
  const el = byId("toast");
  el.textContent = msg;
  el.hidden = false;
  el.style.background = isError ? "rgba(229,57,53,0.92)" : "rgba(33,33,33,0.9)";
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (el.hidden = true), 2500);
}

/* ---------- 通用对话框 ---------- */

function showConfirm(title, message, onOk, danger) {
  const overlay = byId("modal-overlay");
  byId("modal-title").textContent = title;
  const body = byId("modal-body");
  body.textContent = message;
  body.className = "modal-body";
  overlay.hidden = false;
  const okBtn = byId("modal-ok");
  okBtn.textContent = "确定";
  okBtn.className = danger ? "btn btn-danger" : "btn btn-primary";
  const closeModal = () => {
    overlay.hidden = true;
    okBtn.onclick = null;
    byId("modal-cancel").onclick = null;
    byId("modal-close").onclick = null;
  };
  okBtn.onclick = () => {
    closeModal();
    if (onOk) onOk();
  };
  byId("modal-cancel").onclick = closeModal;
  byId("modal-close").onclick = closeModal;
}

function showAlert(title, message, isError) {
  const overlay = byId("modal-overlay");
  byId("modal-title").textContent = title;
  const body = byId("modal-body");
  body.textContent = message || "";
  body.className = "modal-body " + (isError ? "error" : "success");
  overlay.hidden = false;
  const okBtn = byId("modal-ok");
  okBtn.textContent = "关闭";
  okBtn.className = "btn btn-primary";
  const closeModal = () => {
    overlay.hidden = true;
    okBtn.onclick = null;
    byId("modal-cancel").onclick = null;
    byId("modal-close").onclick = null;
  };
  okBtn.onclick = closeModal;
  byId("modal-cancel").onclick = closeModal;
  byId("modal-close").onclick = closeModal;
}

// 通用对话框关闭按钮
document.querySelectorAll("[data-close]").forEach((btn) => {
  btn.addEventListener("click", () => {
    byId(btn.dataset.close).hidden = true;
  });
});

/* ---------- 仪表盘 ---------- */

let dashboardRefreshing = false;
async function refreshDashboard() {
  if (dashboardRefreshing) return;
  dashboardRefreshing = true;
  try {
    const d = await callGo(window.go.wailsapp.App.GetDashboard);
    // 服务状态
    const status = byId("dash-status");
    if (d.running) {
      status.textContent = "● 服务运行中";
      status.className = "status-dot running";
    } else {
      status.textContent = "● 服务已停止";
      status.className = "status-dot stopped";
    }
    byId("dash-addr").textContent = d.base_url;
    byId("dash-listen-port").textContent = "监听端口 " + d.listen_port;
    // 今日统计
    byId("dash-count").textContent = formatInt(d.today_count);
    byId("dash-prompt").textContent = formatTokens(d.today_prompt);
    byId("dash-completion").textContent = formatTokens(d.today_completion);
    byId("dash-total").textContent = formatTokens(d.today_total);
    // 渠道状态
    byId("dash-enabled").textContent = d.enabled_channels;
    byId("dash-online").textContent = d.online_channels;
    byId("dash-cooling").textContent = d.cooling_channels;
    byId("dash-offline").textContent = d.offline_channels;
    byId("dash-total-ch").textContent = d.total_channels;
    byId("dash-total-models").textContent = d.total_models;
  } catch (e) {
    toast("加载仪表盘失败: " + e.message, true);
  } finally {
    dashboardRefreshing = false;
  }
}

byId("dashboard-refresh").addEventListener("click", () => {
  dashAutoRefresh.reset();
  refreshDashboard();
});

byId("dash-copy-addr").addEventListener("click", () => {
  window.go.wailsapp.App.CopyText(byId("dash-addr").textContent);
  toast("已复制地址");
});

/* ---------- 渠道管理 ---------- */

let channelsCache = [];
let selectedChannelId = null;

const channelTypeNames = {
  "openai-compatible": "OpenAI 兼容",
  anthropic: "Anthropic",
  gemini: "Gemini",
  custom: "自定义",
};

// 渠道状态中文名与（图标 + 颜色）
function channelStatusInfo(ch) {
  if (!ch.enabled) {
    return { text: "已停用", cls: "disabled" };
  }
  switch (ch.status) {
    case "online": return { text: "在线", cls: "online" };
    case "cooling": return { text: "冷却", cls: "cooling" };
    case "offline": return { text: "离线", cls: "offline" };
    default: return { text: "未知", cls: "offline" };
  }
}

// 判断渠道是否符合当前筛选条件（状态筛选 + 关键词搜索）
function channelMatchesFilter(ch) {
  const statusFilter = byId("ch-status-filter").value;
  if (statusFilter === "disabled") {
    if (ch.enabled) return false;
  } else if (statusFilter && ch.status !== statusFilter) {
    return false;
  }
  const kw = byId("ch-search").value.trim().toLowerCase();
  if (!kw) return true;
  return (
    String(ch.name || "").toLowerCase().indexOf(kw) !== -1 ||
    String(ch.base_url || "").toLowerCase().indexOf(kw) !== -1
  );
}

// 格式化最近成功时间（复用日志时间格式逻辑）
function formatLastSuccess(ch) {
  if (!ch.last_success_at) return "--";
  const d = new Date(ch.last_success_at);
  if (isNaN(d.getTime())) return "--";
  const pad = (n) => String(n).padStart(2, "0");
  return (
    d.getFullYear() + "-" + pad(d.getMonth() + 1) + "-" + pad(d.getDate()) +
    " " + pad(d.getHours()) + ":" + pad(d.getMinutes()) + ":" + pad(d.getSeconds())
  );
}

function renderChannelTable() {
  const tbody = document.querySelector("#channels-table tbody");
  tbody.innerHTML = "";

  const filtered = channelsCache.filter(channelMatchesFilter);
  byId("ch-count").textContent =
    filtered.length === channelsCache.length
      ? "共 " + channelsCache.length + " 个渠道"
      : "筛选出 " + filtered.length + " / " + channelsCache.length + " 个渠道";

  if (filtered.length === 0) {
    const tr = document.createElement("tr");
    tr.className = "empty-row";
    const td = document.createElement("td");
    td.colSpan = 7;
    td.textContent =
      channelsCache.length === 0
        ? "暂无渠道，点击「＋ 新增渠道」开始配置"
        : "没有符合筛选条件的渠道";
    tr.appendChild(td);
    tbody.appendChild(tr);
    return;
  }

  filtered.forEach((ch) => {
    const tr = document.createElement("tr");
    tr.dataset.id = ch.id;
    if (ch.id === selectedChannelId) tr.classList.add("selected");

    const st = channelStatusInfo(ch);
    const lastSuccess = formatLastSuccess(ch);
    const errFull = ch.last_error || "";
    const errTxt = truncate(errFull, 30);

    tr.innerHTML =
      '<td><div class="ch-name-cell"><span class="ch-name">' + esc(ch.name) + "</span>" +
      '<span class="ch-id">ID ' + ch.id + "</span></div></td>" +
      "<td>" + (channelTypeNames[ch.type] || esc(ch.type)) + "</td>" +
      "<td>" + ch.priority + "</td>" +
      '<td><span class="ch-status-badge ch-badge-' + st.cls + '">' + st.text + "</span></td>" +
      '<td class="' + (ch.enabled ? "" : "muted") + '">' + ch.model_count + "</td>" +
      "<td>" + (lastSuccess !== "--" ? '<span class="muted">' + lastSuccess + "</span>" : "--") + "</td>" +
      '<td title="' + esc(errFull) + '">' + (errTxt ? esc(errTxt) : '<span class="muted">—</span>') + "</td>";

    tr.addEventListener("click", () => {
      selectedChannelId = ch.id;
      document.querySelectorAll("#channels-table tbody tr").forEach((r) =>
        r.classList.toggle("selected", r.dataset.id === String(ch.id))
      );
      updateChannelActions();
    });
    tr.addEventListener("dblclick", () => {
      openChannelModal(ch);
    });
    tbody.appendChild(tr);
  });
}

async function refreshChannels() {
  try {
    channelsCache = await callGo(window.go.wailsapp.App.ListChannels);
    renderChannelTable();
    updateChannelActions();
  } catch (e) {
    const tbody = document.querySelector("#channels-table tbody");
    tbody.innerHTML = "";
    const tr = document.createElement("tr");
    tr.className = "empty-row";
    const td = document.createElement("td");
    td.colSpan = 7;
    td.textContent = "加载渠道失败: " + e.message;
    tr.appendChild(td);
    tbody.appendChild(tr);
    toast("加载渠道失败: " + e.message, true);
  }
}

// 根据选中渠道刷新操作按钮文案
function updateChannelActions() {
  const ch = getSelectedChannel();
  const toggleBtn = byId("ch-toggle");
  if (ch) {
    toggleBtn.textContent = ch.enabled ? "⏻ 停用" : "⏻ 启用";
    toggleBtn.classList.toggle("btn-danger-outline", ch.enabled);
  } else {
    toggleBtn.textContent = "⏻ 启用/停用";
    toggleBtn.classList.remove("btn-danger-outline");
  }
}

// 筛选/搜索变化时重新渲染表格（保留选中态）
byId("ch-status-filter").addEventListener("change", renderChannelTable);
byId("ch-search").addEventListener("input", renderChannelTable);

function getSelectedChannel() {
  return channelsCache.find((c) => c.id === selectedChannelId) || channelsCache[0] || null;
}

byId("ch-refresh").addEventListener("click", refreshChannels);

byId("ch-add").addEventListener("click", () => openChannelModal(null));

byId("ch-edit").addEventListener("click", () => {
  const ch = getSelectedChannel();
  if (!ch) {
    showAlert("提示", "请先选择渠道");
    return;
  }
  openChannelModal(ch);
});

byId("ch-delete").addEventListener("click", () => {
  const ch = getSelectedChannel();
  if (!ch) {
    showAlert("提示", "请先选择渠道");
    return;
  }
  showConfirm("确认删除", "确定删除渠道「" + ch.name + "」？该操作不可恢复。", async () => {
    try {
      await callGo(window.go.wailsapp.App.DeleteChannel, ch.id);
      selectedChannelId = null;
      toast("渠道已删除");
      refreshChannels();
    } catch (e) {
      toast("删除失败: " + e.message, true);
    }
  }, true);
});

byId("ch-toggle").addEventListener("click", async () => {
  const ch = getSelectedChannel();
  if (!ch) {
    showAlert("提示", "请先选择渠道");
    return;
  }
  try {
    await callGo(window.go.wailsapp.App.ToggleChannel, ch.id, !ch.enabled);
    toast(ch.enabled ? "渠道已停用" : "渠道已启用");
    refreshChannels();
  } catch (e) {
    toast("切换状态失败: " + e.message, true);
  }
});

byId("ch-test").addEventListener("click", async () => {
  const ch = getSelectedChannel();
  if (!ch) {
    showAlert("提示", "请先选择渠道");
    return;
  }
  try {
    const n = await callGo(window.go.wailsapp.App.TestChannel, ch.id);
    showAlert("测试成功", "渠道可用，共 " + n + " 个模型");
  } catch (e) {
    showAlert("测试失败", "测试失败: " + e.message, true);
  }
});

byId("ch-refresh-models").addEventListener("click", async () => {
  const ch = getSelectedChannel();
  if (!ch) {
    showAlert("提示", "请先选择渠道");
    return;
  }
  try {
    await callGo(window.go.wailsapp.App.RefreshChannelModels, ch.id);
    toast("渠道 " + ch.name + " 模型刷新结束");
    refreshChannels();
  } catch (e) {
    showAlert("刷新失败", "渠道 " + ch.name + "，" + e.message, true);
  }
});

// 渠道编辑对话框
async function openChannelModal(ch) {
  if (document.querySelectorAll("#ch-type option").length === 0) {
    const types = await callGo(window.go.wailsapp.App.ChannelTypes);
    const sel = byId("ch-type");
    types.forEach((t) => {
      const opt = document.createElement("option");
      opt.value = t;
      opt.textContent = channelTypeNames[t] || t;
      sel.appendChild(opt);
    });
  }
  const infoBox = byId("ch-readonly-info");
  if (ch) {
    byId("ch-name").value = ch.name || "";
    byId("ch-type").value = ch.type;
    byId("ch-baseurl").value = ch.base_url || "";
    byId("ch-keys").value = (ch.api_keys || []).join("\n");
    byId("ch-priority").value = ch.priority;

    // 只读信息区（仅编辑模式显示）
    const st = channelStatusInfo(ch);
    byId("ch-info-status").innerHTML =
      '<span class="ch-status-badge ch-badge-' + st.cls + '">' + st.text + "</span>" +
      '<span class="muted">（模型 ' + ch.model_count + ' 个）</span>';
    byId("ch-info-success").textContent = formatLastSuccess(ch);
    const errText = ch.last_error || "（无）";
    byId("ch-info-error").textContent = errText;
    byId("ch-info-error").title = errText;
    infoBox.hidden = false;
  } else {
    byId("ch-name").value = "";
    byId("ch-type").value = "openai-compatible";
    byId("ch-baseurl").value = "";
    byId("ch-keys").value = "";
    byId("ch-priority").value = "0";
    infoBox.hidden = true;
  }
  byId("channel-modal").hidden = false;
  // 存储当前编辑的渠道（用于保存时区分新增/编辑）
  byId("channel-modal").dataset.editingId = ch ? ch.id : "";
}

byId("ch-save").addEventListener("click", async () => {
  const modal = byId("channel-modal");
  const ch = {
    name: byId("ch-name").value.trim(),
    type: byId("ch-type").value,
    base_url: byId("ch-baseurl").value.trim(),
    api_keys: byId("ch-keys").value
      .split("\n")
      .map((s) => s.trim())
      .filter(Boolean),
    priority: parseInt(byId("ch-priority").value, 10) || 0,
  };
  if (!ch.name) {
    toast("名称不能为空", true);
    return;
  }
  try {
    const editId = modal.dataset.editingId;
    if (editId) {
      ch.id = parseInt(editId, 10);
      await callGo(window.go.wailsapp.App.UpdateChannel, ch);
      toast("渠道已更新");
    } else {
      ch.enabled = true;
      ch.status = "offline";
      await callGo(window.go.wailsapp.App.CreateChannel, ch);
      toast("渠道已创建");
    }
    selectedChannelId = ch.id;
    modal.hidden = true;
    refreshChannels();
  } catch (e) {
    showAlert("保存失败", "保存渠道失败: " + e.message, true);
  }
});

/* ---------- 模型管理 ---------- */

let modelsCache = [];

async function refreshModels() {
  try {
    const rows = await callGo(window.go.wailsapp.App.ListModels);
    modelsCache = rows || [];
    const tbody = document.querySelector("#models-table tbody");
    tbody.innerHTML = "";
    if (modelsCache.length === 0) {
      const tr = document.createElement("tr");
      tr.className = "empty-row";
      const td = document.createElement("td");
      td.colSpan = 6;
      td.textContent = "暂无模型，请添加渠道并同步或点击「＋ 添加自定义模型」";
      tr.appendChild(td);
      tbody.appendChild(tr);
      return;
    }
    modelsCache.forEach((r) => {
      const tr = document.createElement("tr");
      tr.dataset.model = r.model;
      const sourceBadge = r.is_custom
        ? '<span class="ch-status-badge ch-badge-online">自定义</span>'
        : '<span class="ch-status-badge ch-badge-disabled">同步</span>';
      tr.innerHTML =
        '<td><div class="ch-name-cell"><span class="ch-name">' + esc(r.model) + "</span>" +
        (r.description ? '<span class="ch-id">' + esc(r.description) + "</span>" : "") +
        "</div></td>" +
        "<td>" + sourceBadge + "</td>" +
        "<td>" + r.channel_count + "</td>" +
        "<td>" + formatTokens(r.prompt_tokens) + "</td>" +
        "<td>" + formatTokens(r.completion_tokens) + "</td>" +
        "<td>" + formatTokens(r.total_tokens) + "</td>";
      // 右键复制模型 ID
      tr.querySelector("td:first-child").addEventListener("contextmenu", (e) => {
        e.preventDefault();
        window.go.wailsapp.App.CopyText(r.model);
        toast("已复制模型 ID: " + r.model);
      });
      // 双击编辑模型
      tr.addEventListener("dblclick", () => {
        openModelModal(r);
      });
      tbody.appendChild(tr);
    });
  } catch (e) {
    toast("加载模型失败: " + e.message, true);
  }
}

byId("models-refresh").addEventListener("click", async () => {
  try {
    await callGo(window.go.wailsapp.App.SyncAllModels);
    toast("模型刷新已开始");
    // 稍等后刷新列表
    setTimeout(refreshModels, 3000);
  } catch (e) {
    toast("刷新失败: " + e.message, true);
  }
});

byId("models-add").addEventListener("click", () => openModelModal(null));

// 模型编辑对话框
async function openModelModal(row) {
  const modal = byId("model-modal");
  const channelsList = byId("md-channels-list");
  channelsList.innerHTML = "";

  // 加载所有渠道列表
  let channels = [];
  try {
    channels = await callGo(window.go.wailsapp.App.ListChannels);
  } catch (e) {
    toast("加载渠道列表失败: " + e.message, true);
    return;
  }

  if (row) {
    // 编辑模式
    byId("model-modal-title").textContent = "编辑模型";
    byId("md-name").value = row.model;
    byId("md-name").readOnly = true;
    byId("md-desc").value = row.description || "";
    byId("md-desc-row").hidden = !row.is_custom;

    // 加载当前绑定
    let bindings = [];
    try {
      bindings = await callGo(window.go.wailsapp.App.GetModelBindings, row.model);
    } catch (e) {
      // 忽略错误，按无绑定处理
    }
    const bindingMap = {};
    (bindings || []).forEach((b) => {
      bindingMap[b.channel_id] = b;
    });

    // 渲染渠道 checkbox 列表
    channels.forEach((ch) => {
      const binding = bindingMap[ch.id];
      const isExcluded = binding && binding.source === "excluded";
      const isChecked = binding && binding.source !== "excluded";
      const label = document.createElement("label");
      label.className = "md-channel-item";
      const cb = document.createElement("input");
      cb.type = "checkbox";
      cb.value = ch.id;
      cb.checked = isChecked;
      label.appendChild(cb);
      const nameSpan = document.createElement("span");
      nameSpan.textContent = ch.name || "渠道#" + ch.id;
      label.appendChild(nameSpan);
      if (isExcluded) {
        const tag = document.createElement("span");
        tag.className = "md-excluded-tag";
        tag.textContent = "已排除";
        label.appendChild(tag);
      }
      if (!ch.enabled) {
        const tag = document.createElement("span");
        tag.className = "md-disabled-tag";
        tag.textContent = "已停用";
        label.appendChild(tag);
      }
      channelsList.appendChild(label);
    });
    modal.dataset.editingModel = row.model;
    modal.dataset.isCustom = row.is_custom ? "1" : "";
  } else {
    // 新增模式
    byId("model-modal-title").textContent = "添加自定义模型";
    byId("md-name").value = "";
    byId("md-name").readOnly = false;
    byId("md-desc").value = "";
    byId("md-desc-row").hidden = false;
    channels.forEach((ch) => {
      const label = document.createElement("label");
      label.className = "md-channel-item";
      const cb = document.createElement("input");
      cb.type = "checkbox";
      cb.value = ch.id;
      cb.checked = false;
      label.appendChild(cb);
      const nameSpan = document.createElement("span");
      nameSpan.textContent = ch.name || "渠道#" + ch.id;
      label.appendChild(nameSpan);
      if (!ch.enabled) {
        const tag = document.createElement("span");
        tag.className = "md-disabled-tag";
        tag.textContent = "已停用";
        label.appendChild(tag);
      }
      channelsList.appendChild(label);
    });
    modal.dataset.editingModel = "";
    modal.dataset.isCustom = "";
  }
  modal.hidden = false;
}

byId("md-save").addEventListener("click", async () => {
  const modal = byId("model-modal");
  const name = byId("md-name").value.trim();
  const desc = byId("md-desc").value.trim();
  const editingModel = modal.dataset.editingModel;
  const isCustom = modal.dataset.isCustom === "1";

  if (!name) {
    toast("模型名称不能为空", true);
    return;
  }

  // 收集选中的渠道 ID
  const checkedChannels = [];
  modal.querySelectorAll("#md-channels-list input[type=checkbox]:checked").forEach((cb) => {
    checkedChannels.push(parseInt(cb.value, 10));
  });

  try {
    if (editingModel) {
      // 编辑模式
      if (isCustom) {
        // 自定义模型：先更新描述，再更新绑定
        await callGo(window.go.wailsapp.App.UpdateCustomModel, name, desc);
      }
      await callGo(window.go.wailsapp.App.SetModelBindings, name, checkedChannels);
      toast("模型配置已保存");
    } else {
      // 新增自定义模型
      await callGo(window.go.wailsapp.App.CreateCustomModel, name, desc, checkedChannels);
      toast("自定义模型已创建");
    }
    modal.hidden = true;
    availableModelsCache = []; // 清空缓存
    refreshModels();
  } catch (e) {
    showAlert("保存失败", "保存模型失败: " + e.message, true);
  }
});

/* ---------- 模型别名 ---------- */

let aliasesCache = [];
let selectedAliasId = null;

// 格式化 Unix 秒时间戳为 "YYYY-MM-DD HH:mm:ss"
function formatUnixTime(ts) {
  if (!ts) return "--";
  const d = new Date(ts * 1000);
  if (isNaN(d.getTime())) return "--";
  const pad = (n) => String(n).padStart(2, "0");
  return (
    d.getFullYear() + "-" + pad(d.getMonth() + 1) + "-" + pad(d.getDate()) +
    " " + pad(d.getHours()) + ":" + pad(d.getMinutes()) + ":" + pad(d.getSeconds())
  );
}

// 可用模型列表缓存（打开别名模态框时从后端拉取，供目标模型搜索下拉使用）
let availableModelsCache = [];

// 加载可用模型列表到缓存（已缓存则跳过）
async function ensureAvailableModels() {
  if (availableModelsCache.length > 0) return;
  try {
    const rows = await callGo(window.go.wailsapp.App.ListModels);
    availableModelsCache = rows.map((r) => r.model).sort();
  } catch (e) {
    availableModelsCache = [];
  }
}

// 为目标模型列表添加一行（model/weight/timeout 为初始值）
// 使用 CSS Grid 四列布局：模型名 | 权重 | 超时 | 操作，列头由 HTML 模板提供
function addAliasTargetRow(model, weight, timeout) {
  const list = byId("al-targets-list");
  const row = document.createElement("div");
  row.className = "al-target-row";

  // 模型名 combobox（输入框 + 下拉建议）—— 第一列
  const cbx = document.createElement("div");
  cbx.className = "al-model-combobox";
  const input = document.createElement("input");
  input.type = "text";
  input.className = "al-model-input";
  input.placeholder = "搜索或输入模型名";
  input.value = model || "";
  const sugg = document.createElement("div");
  sugg.className = "al-suggestions";
  sugg.hidden = true;
  cbx.appendChild(input);
  cbx.appendChild(sugg);

  // 权重 —— 第二列（列头已标注"权重"）
  const wInput = document.createElement("input");
  wInput.type = "number";
  wInput.className = "al-weight-input";
  wInput.min = "1";
  wInput.value = weight || 1;
  wInput.title = "权重（≥1，按权重加权轮询）";

  // 超时 —— 第三列（列头已标注"超时(s)"）
  const tInput = document.createElement("input");
  tInput.type = "number";
  tInput.className = "al-timeout-input";
  tInput.min = "0";
  tInput.value = timeout || 0;
  tInput.title = "超时秒数（0 表示沿用全局超时）";

  // 删除按钮 —— 第四列
  const delBtn = document.createElement("button");
  delBtn.type = "button";
  delBtn.className = "btn al-del-target-btn";
  delBtn.textContent = "✕";
  delBtn.title = "删除此行";
  delBtn.addEventListener("click", () => {
    row.remove();
  });

  row.appendChild(cbx);
  row.appendChild(wInput);
  row.appendChild(tInput);
  row.appendChild(delBtn);
  list.appendChild(row);

  // 绑定搜索下拉逻辑
  bindCombobox(input, sugg);
}

// 为输入框绑定模糊搜索下拉逻辑（输入过滤 + 键盘导航 + 点击选择）
// 下拉使用 position: fixed，通过 getBoundingClientRect 动态定位，
// 确保不受父容器 overflow 裁剪，可突破弹窗边界显示。
function bindCombobox(input, sugg) {
  let highlightIdx = -1;
  let currentItems = [];

  // 根据输入框位置动态设置下拉列表的 fixed 坐标
  function positionSuggestions() {
    var rect = input.getBoundingClientRect();
    var suggMaxH = 200; // 与 CSS max-height 一致
    var margin = 2;
    var spaceBelow = window.innerHeight - rect.bottom - margin;
    var spaceAbove = rect.top - margin;

    var top;
    if (spaceBelow >= suggMaxH || spaceBelow >= spaceAbove) {
      // 下方空间足够或比上方大：向下展开
      top = rect.bottom + margin;
    } else {
      // 上方空间更大：向上展开
      top = Math.max(margin, rect.top - suggMaxH - margin);
    }
    sugg.style.top = top + "px";
    sugg.style.left = rect.left + "px";
    sugg.style.width = rect.width + "px";
  }

  // 滚动/resize 时重新定位下拉
  function onScrollOrResize() {
    if (sugg.hidden) return;
    positionSuggestions();
  }

  // 清理事件监听器（关闭下拉时调用）
  function detachListeners() {
    window.removeEventListener("scroll", onScrollOrResize, true);
    window.removeEventListener("resize", onScrollOrResize);
  }

  // 挂载事件监听器（每次展开时调用）
  function attachListeners() {
    // scroll 使用 capture 模式，确保捕获所有父容器的滚动事件
    window.addEventListener("scroll", onScrollOrResize, true);
    window.addEventListener("resize", onScrollOrResize);
  }

  function renderItems(items) {
    currentItems = items;
    sugg.innerHTML = "";
    if (items.length === 0) {
      sugg.hidden = true;
      detachListeners();
      return;
    }
    items.forEach((m, i) => {
      var item = document.createElement("div");
      item.className = "al-suggestion-item";
      item.textContent = m;
      item.addEventListener("mousedown", (e) => {
        e.preventDefault(); // 防止 input blur 先于 click 触发
        selectItem(m);
      });
      sugg.appendChild(item);
    });
    highlightIdx = -1;
    sugg.hidden = false;
    positionSuggestions();
    attachListeners();
  }

  function selectItem(m) {
    input.value = m;
    sugg.hidden = true;
    detachListeners();
    input.focus();
  }

  function hideSuggestions() {
    sugg.hidden = true;
    detachListeners();
  }

  function updateHighlight() {
    const els = sugg.querySelectorAll(".al-suggestion-item");
    els.forEach((el, i) => {
      el.classList.toggle("active", i === highlightIdx);
    });
    if (highlightIdx >= 0 && highlightIdx < els.length) {
      els[highlightIdx].scrollIntoView({ block: "nearest" });
    }
  }

  input.addEventListener("input", () => {
    const kw = input.value.trim().toLowerCase();
    if (!kw) {
      hideSuggestions();
      return;
    }
    const matched = availableModelsCache
      .filter((m) => m.toLowerCase().indexOf(kw) !== -1)
      .slice(0, 20);
    renderItems(matched);
  });

  input.addEventListener("focus", () => {
    const kw = input.value.trim().toLowerCase();
    if (kw) {
      const matched = availableModelsCache
        .filter((m) => m.toLowerCase().indexOf(kw) !== -1)
        .slice(0, 20);
      renderItems(matched);
    } else {
      // 空输入时展示前 20 个模型供浏览选择
      renderItems(availableModelsCache.slice(0, 20));
    }
  });

  input.addEventListener("blur", () => {
    // 延迟关闭，允许 mousedown 选中先执行
    setTimeout(() => { hideSuggestions(); }, 150);
  });

  input.addEventListener("keydown", (e) => {
    if (sugg.hidden && (e.key === "ArrowDown" || e.key === "ArrowUp")) {
      // 下拉关闭时按方向键重新打开
      const kw = input.value.trim().toLowerCase();
      const matched = kw
        ? availableModelsCache.filter((m) => m.toLowerCase().indexOf(kw) !== -1).slice(0, 20)
        : availableModelsCache.slice(0, 20);
      renderItems(matched);
      if (matched.length > 0) e.preventDefault();
      return;
    }
    if (e.key === "ArrowDown") {
      e.preventDefault();
      highlightIdx = Math.min(highlightIdx + 1, currentItems.length - 1);
      updateHighlight();
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      highlightIdx = Math.max(highlightIdx - 1, 0);
      updateHighlight();
    } else if (e.key === "Enter") {
      if (!sugg.hidden && highlightIdx >= 0 && highlightIdx < currentItems.length) {
        e.preventDefault();
        selectItem(currentItems[highlightIdx]);
      }
    } else if (e.key === "Escape") {
      hideSuggestions();
    }
  });
}

// 从目标模型行列表收集 targets JSON
function collectAliasTargetsJSON() {
  const rows = byId("al-targets-list").querySelectorAll(".al-target-row");
  const targets = [];
  for (const row of rows) {
    const model = row.querySelector(".al-model-input").value.trim();
    if (!model) continue;
    const target = {
      model: model,
      weight: parseInt(row.querySelector(".al-weight-input").value, 10) || 1,
      timeout_seconds: parseInt(row.querySelector(".al-timeout-input").value, 10) || 0,
    };
    targets.push(target);
  }
  if (targets.length === 0) return "";
  return JSON.stringify(targets);
}

// 渲染别名表格
function renderAliasTable() {
  const tbody = document.querySelector("#aliases-table tbody");
  tbody.innerHTML = "";

  if (aliasesCache.length === 0) {
    const tr = document.createElement("tr");
    tr.className = "empty-row";
    const td = document.createElement("td");
    td.colSpan = 4;
    td.textContent = "暂无别名，点击「＋ 新增别名」开始配置";
    tr.appendChild(td);
    tbody.appendChild(tr);
    return;
  }

  aliasesCache.forEach((a) => {
    const tr = document.createElement("tr");
    tr.dataset.id = a.id;
    if (a.id === selectedAliasId) tr.classList.add("selected");

    let targetsText = a.targets || "[]";
    try {
      const list = JSON.parse(targetsText);
      targetsText = list.map((t) => {
        let s = t.model || "";
        if (t.weight && t.weight > 1) s += "(" + t.weight + ")";
        return s;
      }).join(", ");
    } catch (e) {
      // 保留原始 JSON
    }

    const statusBadge = a.enabled
      ? '<span class="ch-status-badge ch-badge-online">启用</span>'
      : '<span class="ch-status-badge ch-badge-disabled">停用</span>';

    tr.innerHTML =
      '<td><div class="ch-name-cell"><span class="ch-name">' + esc(a.name) + "</span>" +
      '<span class="ch-id">ID ' + a.id + "</span></div></td>" +
      "<td>" + esc(targetsText) + "</td>" +
      "<td>" + statusBadge + "</td>" +
      '<td><span class="muted">' + formatUnixTime(a.updated_at) + "</span></td>";

    tr.addEventListener("click", () => {
      selectedAliasId = a.id;
      document.querySelectorAll("#aliases-table tbody tr").forEach((r) =>
        r.classList.toggle("selected", r.dataset.id === String(a.id))
      );
      updateAliasActions();
    });
    tr.addEventListener("dblclick", () => {
      openAliasModal(a);
    });
    tbody.appendChild(tr);
  });
}

async function refreshAliases() {
  try {
    aliasesCache = await callGo(window.go.wailsapp.App.ListAliases);
    renderAliasTable();
    updateAliasActions();
  } catch (e) {
    const tbody = document.querySelector("#aliases-table tbody");
    tbody.innerHTML = "";
    const tr = document.createElement("tr");
    tr.className = "empty-row";
    const td = document.createElement("td");
    td.colSpan = 4;
    td.textContent = "加载别名失败: " + e.message;
    tr.appendChild(td);
    tbody.appendChild(tr);
    toast("加载别名失败: " + e.message, true);
  }
}

function getSelectedAlias() {
  return aliasesCache.find((a) => a.id === selectedAliasId) || aliasesCache[0] || null;
}

function updateAliasActions() {
  const a = getSelectedAlias();
  const toggleBtn = byId("al-toggle");
  if (a) {
    toggleBtn.textContent = a.enabled ? "⏻ 停用" : "⏻ 启用";
    toggleBtn.classList.toggle("btn-danger-outline", a.enabled);
  } else {
    toggleBtn.textContent = "⏻ 启用/停用";
    toggleBtn.classList.remove("btn-danger-outline");
  }
}

byId("aliases-refresh").addEventListener("click", refreshAliases);

byId("al-add").addEventListener("click", () => openAliasModal(null));

byId("al-edit").addEventListener("click", () => {
  const a = getSelectedAlias();
  if (!a) {
    showAlert("提示", "请先选择别名");
    return;
  }
  openAliasModal(a);
});

byId("al-delete").addEventListener("click", () => {
  const a = getSelectedAlias();
  if (!a) {
    showAlert("提示", "请先选择别名");
    return;
  }
  showConfirm("确认删除", "确定删除别名「" + a.name + "」？该操作不可恢复。", async () => {
    try {
      await callGo(window.go.wailsapp.App.DeleteAlias, a.id);
      selectedAliasId = null;
      toast("别名已删除");
      refreshAliases();
    } catch (e) {
      toast("删除失败: " + e.message, true);
    }
  }, true);
});

byId("al-toggle").addEventListener("click", async () => {
  const a = getSelectedAlias();
  if (!a) {
    showAlert("提示", "请先选择别名");
    return;
  }
  try {
    await callGo(window.go.wailsapp.App.ToggleAlias, a.id, !a.enabled);
    toast(a.enabled ? "别名已停用" : "别名已启用");
    refreshAliases();
  } catch (e) {
    toast("切换状态失败: " + e.message, true);
  }
});

// 别名编辑对话框
async function openAliasModal(a) {
  const modal = byId("alias-modal");
  // 确保可用模型列表已加载（供搜索下拉使用）
  await ensureAvailableModels();
  // 清空目标模型行列表
  byId("al-targets-list").innerHTML = "";
  if (a) {
    byId("al-name").value = a.name || "";
    // 解析已有 targets，为每个目标添加一行
    let targets = [];
    try {
      targets = JSON.parse(a.targets || "[]");
    } catch (e) {
      targets = [];
    }
    if (targets.length === 0) {
      addAliasTargetRow("", 1, 0);
    } else {
      targets.forEach((t) => {
        addAliasTargetRow(t.model || "", t.weight || 1, t.timeout_seconds || 0);
      });
    }
    byId("al-enabled").checked = a.enabled;
    modal.dataset.editingId = a.id;
  } else {
    byId("al-name").value = "";
    addAliasTargetRow("", 1, 0);
    byId("al-enabled").checked = true;
    modal.dataset.editingId = "";
  }
  modal.hidden = false;
}

// 添加目标模型按钮
byId("al-add-target").addEventListener("click", () => {
  addAliasTargetRow("", 1, 0);
});

byId("al-save").addEventListener("click", async () => {
  const modal = byId("alias-modal");
  const name = byId("al-name").value.trim();
  if (!name) {
    toast("别名不能为空", true);
    return;
  }
  const targetsJSON = collectAliasTargetsJSON();
  if (!targetsJSON) {
    toast("目标模型不能为空", true);
    return;
  }
  const alias = {
    name: name,
    targets: targetsJSON,
    enabled: byId("al-enabled").checked,
  };
  try {
    const editId = modal.dataset.editingId;
    if (editId) {
      alias.id = parseInt(editId, 10);
      await callGo(window.go.wailsapp.App.UpdateAlias, alias);
      toast("别名已更新");
    } else {
      const id = await callGo(window.go.wailsapp.App.CreateAlias, alias);
      alias.id = id;
      toast("别名已创建");
    }
    selectedAliasId = alias.id;
    modal.hidden = true;
    refreshAliases();
  } catch (e) {
    showAlert("保存失败", "保存别名失败: " + e.message, true);
  }
});

/* ---------- 用量统计 ---------- */

let statsTab = "daily";
let statsRefreshing = false;

async function refreshStats() {
  if (statsRefreshing) return;
  statsRefreshing = true;
  try {
    const range = byId("stats-range").value;
    const p = {
      range: range,
      channel_id: parseInt(byId("stats-channel").value, 10) || 0,
      model: byId("stats-model").value,
    };
    if (range === "custom") {
      const startDate = byId("stats-start-date").value;
      const endDate = byId("stats-end-date").value;
      if (!startDate || !endDate) {
        toast("请选择自定义时间范围的开始和结束日期", true);
        return;
      }
      if (endDate < startDate) {
        toast("结束日期不能早于开始日期", true);
        return;
      }
      p.start_date = startDate;
      p.end_date = endDate;
    }
    const data = await callGo(window.go.wailsapp.App.GetStats, p);
    // 汇总卡片（当前所选时间范围）
    if (data.today) {
      byId("stats-today-count").textContent = formatInt(data.today.count);
      byId("stats-today-prompt").textContent = formatTokens(data.today.prompt_tokens);
      byId("stats-today-completion").textContent = formatTokens(data.today.completion_tokens);
      byId("stats-today-total").textContent = formatTokens(data.today.total_tokens);
    }
    byId("stats-range-label").textContent = data.range.start + " ~ " + data.range.end;

    const table = byId("stats-table");
    const thead = table.querySelector("thead");
    const tbody = table.querySelector("tbody");
    tbody.innerHTML = "";

    if (statsTab === "daily") {
      thead.innerHTML =
        "<tr><th>日期</th><th>调用次数</th><th>输入 Token</th><th>输出 Token</th><th>总 Token</th><th>成功/失败</th></tr>";
      (data.daily || []).forEach((r) => {
        const tr = document.createElement("tr");
        tr.innerHTML =
          "<td>" + esc(r.date) + "</td>" +
          "<td>" + formatInt(r.count) + "</td>" +
          "<td>" + formatTokens(r.prompt_tokens) + "</td>" +
          "<td>" + formatTokens(r.completion_tokens) + "</td>" +
          "<td>" + formatTokens(r.total_tokens) + "</td>" +
          "<td>" + formatInt(r.success_count) + " / " + formatInt(r.fail_count) + "</td>";
        tbody.appendChild(tr);
      });
    } else if (statsTab === "model") {
      thead.innerHTML =
        "<tr><th>模型</th><th>调用次数</th><th>输入 Token</th><th>输出 Token</th><th>总 Token</th><th>成功/失败</th></tr>";
      (data.by_model || []).forEach((r) => {
        const tr = document.createElement("tr");
        tr.innerHTML =
          "<td>" + esc(r.model) + "</td>" +
          "<td>" + formatInt(r.count) + "</td>" +
          "<td>" + formatTokens(r.prompt_tokens) + "</td>" +
          "<td>" + formatTokens(r.completion_tokens) + "</td>" +
          "<td>" + formatTokens(r.total_tokens) + "</td>" +
          "<td>" + formatInt(r.success_count) + " / " + formatInt(r.fail_count) + "</td>";
        tbody.appendChild(tr);
      });
    } else {
      thead.innerHTML =
        "<tr><th>渠道</th><th>调用次数</th><th>输入 Token</th><th>输出 Token</th><th>总 Token</th><th>成功/失败</th></tr>";
      (data.by_channel || []).forEach((r) => {
        const name = r.channel_name || "渠道#" + r.channel_id;
        const tr = document.createElement("tr");
        tr.innerHTML =
          "<td>" + esc(name) + "</td>" +
          "<td>" + formatInt(r.count) + "</td>" +
          "<td>" + formatTokens(r.prompt_tokens) + "</td>" +
          "<td>" + formatTokens(r.completion_tokens) + "</td>" +
          "<td>" + formatTokens(r.total_tokens) + "</td>" +
          "<td>" + formatInt(r.success_count) + " / " + formatInt(r.fail_count) + "</td>";
        tbody.appendChild(tr);
      });
    }
  } catch (e) {
    toast("加载统计失败: " + e.message, true);
  } finally {
    statsRefreshing = false;
  }
}

async function loadStatsOptions() {
  try {
    const opts = await callGo(window.go.wailsapp.App.GetStatsOptions);
    const chSel = byId("stats-channel");
    chSel.innerHTML = '<option value="0">全部渠道</option>';
    (opts.channels || []).forEach((c) => {
      const opt = document.createElement("option");
      opt.value = c.id;
      opt.textContent = c.name || "渠道#" + c.id;
      chSel.appendChild(opt);
    });
    const mSel = byId("stats-model");
    mSel.innerHTML = '<option value="">全部模型</option>';
    (opts.models || []).forEach((m) => {
      const opt = document.createElement("option");
      opt.value = m;
      opt.textContent = m;
      mSel.appendChild(opt);
    });
  } catch (e) {
    // 忽略下拉加载失败
  }
}

function updateCustomRangeVisibility() {
  const show = byId("stats-range").value === "custom";
  byId("stats-custom-range").hidden = !show;
  if (show) {
    // 首次打开自定义时，默认填充最近 7 天
    if (!byId("stats-start-date").value && !byId("stats-end-date").value) {
      const end = new Date();
      const start = new Date();
      start.setDate(start.getDate() - 6);
      const fmt = (d) =>
        d.getFullYear() + "-" +
        String(d.getMonth() + 1).padStart(2, "0") + "-" +
        String(d.getDate()).padStart(2, "0");
      byId("stats-start-date").value = fmt(start);
      byId("stats-end-date").value = fmt(end);
    }
  }
}

byId("stats-refresh").addEventListener("click", () => {
  statsAutoRefresh.reset();
  loadStatsOptions();
  refreshStats();
});
byId("stats-range").addEventListener("change", () => {
  updateCustomRangeVisibility();
  refreshStats();
});
byId("stats-channel").addEventListener("change", refreshStats);
byId("stats-model").addEventListener("change", refreshStats);
byId("stats-start-date").addEventListener("change", refreshStats);
byId("stats-end-date").addEventListener("change", refreshStats);

document.querySelectorAll(".tab").forEach((t) => {
  t.addEventListener("click", () => {
    document.querySelectorAll(".tab").forEach((x) => x.classList.remove("active"));
    t.classList.add("active");
    statsTab = t.dataset.tab;
    refreshStats();
  });
});

/* ---------- 请求日志 ---------- */

// 格式化日志时间：优先取数据库原始时间字符串，截取到秒级 "YYYY-MM-DD HH:mm:ss"
function formatLogTime(r) {
  if (r.created_at_raw) {
    const s = String(r.created_at_raw);
    // 兼容带 "T" 分隔符的 ISO 格式（如 "2026-08-05T14:19:39..."）
    return s.replace("T", " ").slice(0, 19);
  }
  if (r.created_at) {
    const d = new Date(r.created_at);
    if (!isNaN(d.getTime())) {
      const pad = (n) => String(n).padStart(2, "0");
      return (
        d.getFullYear() + "-" + pad(d.getMonth() + 1) + "-" + pad(d.getDate()) +
        " " + pad(d.getHours()) + ":" + pad(d.getMinutes()) + ":" + pad(d.getSeconds())
      );
    }
  }
  return "";
}

let logsRefreshing = false;
async function refreshLogs() {
  if (logsRefreshing) return;
  logsRefreshing = true;
  try {
    const channelId = parseInt(byId("logs-channel").value, 10) || 0;
    const model = byId("logs-model").value;
    const logs = await callGo(window.go.wailsapp.App.GetLogs, 200, channelId, model);
    const tbody = document.querySelector("#logs-table tbody");
    tbody.innerHTML = "";
    logs.forEach((r) => {
      const tr = document.createElement("tr");
      const timeStr = formatLogTime(r);
      tr.innerHTML =
        "<td>" + esc(timeStr) + "</td>" +
        "<td>" + esc(r.channel_name) + "</td>" +
        "<td>" + esc(r.model) + "</td>" +
        "<td>" + r.status_code + "</td>" +
        "<td>" + r.duration_ms + "</td>" +
        "<td>" + r.prompt_tokens + " / " + r.completion_tokens + "</td>" +
        "<td>" + esc(truncate(r.error || "", 15)) + "</td>";
      tr.style.cursor = "pointer";
      tr.addEventListener("dblclick", () => showLogDetail(r));
      tbody.appendChild(tr);
    });
  } catch (e) {
    toast("加载日志失败: " + e.message, true);
  } finally {
    logsRefreshing = false;
  }
}

function showLogDetail(r) {
  const body = byId("logdetail-body");
  const errText = r.error || "（无）";
  body.textContent =
    "时间：" + formatLogTime(r) + "\n" +
    "渠道：" + (r.channel_name || "") + "（ID:" + r.channel_id + "）\n" +
    "模型：" + (r.model || "") + "\n" +
    "状态码：" + r.status_code + "\n" +
    "耗时：" + r.duration_ms + " ms\n" +
    "输入/输出 Token：" + r.prompt_tokens + " / " + r.completion_tokens + "\n" +
    "是否成功：" + (r.is_success ? "是" : "否") + "\n\n" +
    "错误信息：\n" + errText;
  byId("logdetail-modal").hidden = false;
}

async function loadLogOptions() {
  try {
    const opts = await callGo(window.go.wailsapp.App.GetLogOptions);
    const chSel = byId("logs-channel");
    const currentCh = chSel.value;
    chSel.innerHTML = '<option value="0">全部渠道</option>';
    (opts.channels || []).forEach((c) => {
      const opt = document.createElement("option");
      opt.value = c.id;
      opt.textContent = c.name || "渠道#" + c.id;
      chSel.appendChild(opt);
    });
    chSel.value = currentCh;

    const mSel = byId("logs-model");
    const currentM = mSel.value;
    mSel.innerHTML = '<option value="">全部模型</option>';
    (opts.models || []).forEach((m) => {
      const opt = document.createElement("option");
      opt.value = m;
      opt.textContent = m;
      mSel.appendChild(opt);
    });
    mSel.value = currentM;
  } catch (e) {
    // 忽略
  }
}

byId("logs-refresh").addEventListener("click", () => {
  logsAutoRefresh.reset();
  loadLogOptions();
  refreshLogs();
});
byId("logs-channel").addEventListener("change", refreshLogs);
byId("logs-model").addEventListener("change", refreshLogs);

/* ---------- 设置 ---------- */

// 根据代理服务运行状态更新设置页"服务配置"卡片的锁定态与服务状态行
function updateServiceConfigLock(running) {
  const locked = !!running;
  ["set-listen-addr", "set-listen-port", "set-token", "set-token-gen", "set-auth"].forEach((id) => {
    byId(id).disabled = locked;
  });

  const status = byId("set-service-status");
  const toggleBtn = byId("set-toggle-service");
  if (running) {
    status.textContent = "● 服务运行中";
    status.className = "status-dot running";
    toggleBtn.textContent = "停止服务";
    toggleBtn.className = "btn";
  } else {
    status.textContent = "● 服务已停止";
    status.className = "status-dot stopped";
    toggleBtn.textContent = "启动服务";
    toggleBtn.className = "btn btn-primary";
  }
}

async function loadSettings() {
  try {
    const s = await callGo(window.go.wailsapp.App.GetSettings);
    byId("set-listen-addr").value = s.listen_addr;
    byId("set-listen-port").value = s.listen_port;
    byId("set-token").value = s.access_token;
    byId("set-auth").checked = s.auth_enabled;
    byId("set-sync-interval").value = s.model_sync_interval_minutes;
    byId("set-timeout").value = s.proxy_timeout_seconds;
    byId("set-breaker-threshold").value = s.breaker_threshold;
    byId("set-breaker-cooldown").value = s.breaker_cooldown_seconds;
    byId("set-log-retention").value = s.log_retention_days;
    byId("set-debug").checked = s.debug;
    const autostart = byId("set-autostart");
    autostart.checked = s.autostart;
    autostart.disabled = !s.autostart_supported;
    byId("set-start-minimized").checked = s.start_minimized;
    byId("set-token-display").value = s.token_display || "auto";
    tokenDisplayMode = s.token_display || "auto";
    updateServiceConfigLock(s.proxy_running);
    resetAllDirty();
  } catch (e) {
    toast("加载设置失败: " + e.message, true);
  }
}

// 设置页服务配置卡片：启动/停止服务（同一按钮）。
// 仅刷新状态行与锁定态，不整体重载表单，避免覆盖其他卡片正在编辑的内容。
async function toggleService() {
  try {
    await callGo(window.go.wailsapp.App.ToggleProxy);
    refreshSettingsState();
  } catch (e) {
    toast("操作失败: " + e.message, true);
  }
}
byId("set-toggle-service").addEventListener("click", toggleService);

byId("set-token-copy").addEventListener("click", () => {
  const v = byId("set-token").value;
  if (!v) {
    showAlert("提示", "访问令牌为空，暂无内容可复制");
    return;
  }
  window.go.wailsapp.App.CopyText(v);
  toast("已复制");
});

byId("set-token-gen").addEventListener("click", async () => {
  try {
    byId("set-token").value = await callGo(window.go.wailsapp.App.GenerateAccessToken);
    setGroupDirty("service", true);
  } catch (e) {
    toast("生成失败: " + e.message, true);
  }
});

byId("set-autostart").addEventListener("change", async (e) => {
  try {
    await callGo(window.go.wailsapp.App.SetAutoStart, e.target.checked);
  } catch (err) {
    toast("设置失败: " + err.message, true);
    loadSettings();
  }
});

byId("set-start-minimized").addEventListener("change", async (e) => {
  try {
    await callGo(window.go.wailsapp.App.SetStartMinimized, e.target.checked);
    toast(e.target.checked ? "已开启启动时最小化（下次启动生效）" : "已关闭启动时最小化");
  } catch (err) {
    toast("设置失败: " + err.message, true);
    loadSettings();
  }
});

byId("set-clear-logs").addEventListener("click", () => {
  showConfirm("确认清除", "确定清除全部请求日志？该操作不可恢复。", async () => {
    try {
      const n = await callGo(window.go.wailsapp.App.ClearLogs);
      toast("已清除 " + n + " 条请求日志");
    } catch (e) {
      toast("清除日志失败: " + e.message, true);
    }
  }, true);
});

// 设置页分组保存：各组独立提交、独立生效，并跟踪未保存的修改
const settingGroups = {
  service: {
    dirtyEl: "set-dirty-service",
    inputs: ["set-listen-addr", "set-listen-port", "set-token", "set-auth"],
  },
  sync: {
    dirtyEl: "set-dirty-sync",
    inputs: ["set-sync-interval"],
  },
  breaker: {
    dirtyEl: "set-dirty-breaker",
    inputs: ["set-timeout", "set-breaker-threshold", "set-breaker-cooldown"],
  },
  log: {
    dirtyEl: "set-dirty-log",
    inputs: ["set-log-retention", "set-debug"],
  },
  display: {
    dirtyEl: "set-dirty-display",
    inputs: ["set-token-display"],
  },
};

// 设置组未保存标记（false/undefined 隐藏，true 显示）
function setGroupDirty(group, dirty) {
  const cfg = settingGroups[group];
  if (cfg) byId(cfg.dirtyEl).hidden = !dirty;
}

// 重置所有组的未保存标记（进入设置页/加载配置后调用）
function resetAllDirty() {
  Object.keys(settingGroups).forEach((g) => setGroupDirty(g, false));
}

// 表单改动即标记对应组为"未保存"
Object.entries(settingGroups).forEach(([group, cfg]) => {
  cfg.inputs.forEach((id) => {
    const el = byId(id);
    el.addEventListener("input", () => setGroupDirty(group, true));
    el.addEventListener("change", () => setGroupDirty(group, true));
  });
});

// 保存服务配置（仅停止态可修改；保存后启动服务时生效）
byId("set-save-service").addEventListener("click", async () => {
  try {
    await callGo(
      window.go.wailsapp.App.SaveServiceConfig,
      byId("set-listen-addr").value.trim(),
      parseInt(byId("set-listen-port").value, 10),
      byId("set-token").value,
      byId("set-auth").checked
    );
    toast("服务配置已保存，启动服务后生效");
    setGroupDirty("service", false);
  } catch (e) {
    showAlert("保存失败", e.message, true);
  }
});

// 保存模型同步配置（立即生效）
byId("set-save-sync").addEventListener("click", async () => {
  try {
    await callGo(
      window.go.wailsapp.App.SaveModelSyncConfig,
      parseInt(byId("set-sync-interval").value, 10)
    );
    toast("模型同步设置已保存并立即生效");
    setGroupDirty("sync", false);
  } catch (e) {
    showAlert("保存失败", e.message, true);
  }
});

// 保存代理容错配置（立即热更新）
byId("set-save-breaker").addEventListener("click", async () => {
  try {
    await callGo(
      window.go.wailsapp.App.SaveBreakerConfig,
      parseInt(byId("set-timeout").value, 10),
      parseInt(byId("set-breaker-threshold").value, 10),
      parseInt(byId("set-breaker-cooldown").value, 10)
    );
    toast("代理容错设置已保存并立即生效");
    setGroupDirty("breaker", false);
  } catch (e) {
    showAlert("保存失败", e.message, true);
  }
});

// 保存日志与调试配置（立即热更新）
byId("set-save-log").addEventListener("click", async () => {
  try {
    await callGo(
      window.go.wailsapp.App.SaveLogConfig,
      parseInt(byId("set-log-retention").value, 10),
      byId("set-debug").checked
    );
    toast("日志与调试设置已保存并立即生效");
    setGroupDirty("log", false);
  } catch (e) {
    showAlert("保存失败", e.message, true);
  }
});

// 保存显示设置（Token 显示方式，立即生效）
byId("set-save-display").addEventListener("click", async () => {
  try {
    const mode = byId("set-token-display").value;
    await callGo(window.go.wailsapp.App.SaveTokenDisplay, mode);
    tokenDisplayMode = mode;
    toast("显示设置已保存并立即生效");
    setGroupDirty("display", false);
  } catch (e) {
    showAlert("保存失败", e.message, true);
  }
});

/* ---------- 事件订阅 ---------- */

// 后端状态变化时刷新当前页面
window.runtime.EventsOn("state:changed", () => {
  // 模型同步等状态变化后清空可用模型缓存，使下次打开别名模态框时重新拉取
  availableModelsCache = [];
  const activePage = document.querySelector(".nav-item.active").dataset.page;
  if (activePage === "settings") {
    // 设置页仅刷新服务运行状态与锁定态，避免覆盖用户正在编辑的表单
    refreshSettingsState();
    return;
  }
  const refreshers = {
    dashboard: refreshDashboard,
    channels: refreshChannels,
    aliases: refreshAliases,
    models: refreshModels,
    stats: refreshStats,
    logs: refreshLogs,
  };
  if (refreshers[activePage]) {
    setTimeout(refreshers[activePage], 200);
  }
});

// 轻量获取服务运行状态并刷新设置页锁定态
async function refreshSettingsState() {
  try {
    const s = await callGo(window.go.wailsapp.App.GetSettings);
    updateServiceConfigLock(s.proxy_running);
  } catch (e) {
    // 忽略刷新失败
  }
}

// 单实例激活：恢复显示（wails 端已处理 WindowShow，前端无需额外操作）
window.runtime.EventsOn("app:activate", () => {
  // 可选：恢复到上次页面刷新
});

/* ---------- 初始化 ---------- */

// 页面自动刷新（仪表盘、渠道管理、用量统计、请求日志），间隔由页头下拉框设置，并显示倒计时
const dashAutoRefresh = createAutoRefresh(
  "dash-refresh-interval",
  refreshDashboard,
  "dash-refresh-countdown"
);
const channelsAutoRefresh = createAutoRefresh(
  "ch-refresh-interval",
  refreshChannels,
  "ch-refresh-countdown"
);
const statsAutoRefresh = createAutoRefresh(
  "stats-refresh-interval",
  refreshStats,
  "stats-refresh-countdown"
);
const logsAutoRefresh = createAutoRefresh(
  "logs-refresh-interval",
  refreshLogs,
  "logs-refresh-countdown"
);

// 初始加载
updateCustomRangeVisibility();
showPage("dashboard");
loadStatsOptions();
loadLogOptions();
