const state = {
    tables: [],
    incDescriptors: [],
    selectedTable: "",
    selectedInc: null,
    operation: "read-key",
    messages: [],
    lastEntriesQuery: null,
    health: null,
};

const formState = {
    "read-key": { table: "", key: "", resolve: true },
    "list-entries": { table: "", limit: "100", cursor: "" },
    "search-regex": { table: "", regex: "", limit: "100" },
    "read-inc": { table: "", key: "", mode: "first", limit: "50" },
};

function init() {
    bindEvents();
    renderOperationFields();
    updateOperationPreview();
    loadTables();
    loadIncDescriptors();
    loadHealth();
}

function bindEvents() {
    document.getElementById("operation-select")?.addEventListener("change", (event) => {
        setOperation(event.target.value);
    });

    document.getElementById("run-operation")?.addEventListener("click", (event) => {
        event.preventDefault();
        runOperation();
    });

    document.getElementById("refresh-health")?.addEventListener("click", (event) => {
        event.preventDefault();
        loadHealth();
    });

    document.getElementById("refresh-objects")?.addEventListener("click", (event) => {
        event.preventDefault();
        loadTables(true);
        loadIncDescriptors(true);
    });

    document.querySelector(".tab-bar")?.addEventListener("click", (event) => {
        const tab = event.target.closest(".tab");
        if (!tab) {
            return;
        }
        switchResultsTab(tab.dataset.tab);
    });

    document.getElementById("object-tree")?.addEventListener("click", (event) => {
        const toggle = event.target.closest(".tree-toggle");
        if (toggle) {
            const targetId = toggle.dataset.target;
            const target = document.getElementById(targetId);
            if (target) {
                const isOpen = !target.classList.contains("is-collapsed");
                target.classList.toggle("is-collapsed", isOpen);
                toggle.classList.toggle("is-open", !isOpen);
            }
            return;
        }
        const item = event.target.closest(".tree-item");
        if (item) {
            const table = item.dataset.table;
            const incIndex = item.dataset.incIndex;
            if (incIndex) {
                const descriptor = state.incDescriptors[Number(incIndex)];
                if (descriptor) {
                    selectIncDescriptor(descriptor);
                }
            } else if (table) {
                selectTable(table);
            }
        }
    });
}

async function loadTables(force = false) {
    if (!force && state.tables.length) {
        renderTablesList();
        return;
    }
    try {
        const data = await fetchJSON("/api/admin/tables");
        state.tables = data.tables || [];
        renderTablesList();
    } catch (error) {
        appendMessage(`Tables load failed: ${error.message}`, "error");
        renderTablesList(true);
    }
}

async function loadIncDescriptors(force = false) {
    if (!force && state.incDescriptors.length) {
        renderIncList();
        return;
    }
    try {
        const data = await fetchJSON("/api/admin/inc_descriptors");
        state.incDescriptors = data.descriptors || [];
        renderIncList();
    } catch (error) {
        appendMessage(`Incremental scan failed: ${error.message}`, "error");
        renderIncList(true);
    }
}

async function loadHealth() {
    try {
        const data = await fetchJSON("/api/health");
        state.health = data;
        const stamp = new Date().toLocaleTimeString();
        const total = data.api?.total_requests ?? 0;
        appendMessage(`Health OK · total requests: ${formatNumber(total)}`, "info");
        updateHealthTimestamp(stamp);
    } catch (error) {
        updateHealthTimestamp("");
        appendMessage(`Health check failed: ${error.message}`, "error");
    }
}

function updateHealthTimestamp(value) {
    const target = document.getElementById("health-updated");
    if (target) {
        target.textContent = value ? `Updated ${value}` : "";
    }
}

function renderTablesList(showError = false) {
    const list = document.getElementById("tree-tables");
    if (!list) {
        return;
    }
    list.innerHTML = "";
    if (showError) {
        list.innerHTML = `<li class="muted">Unable to load tables.</li>`;
        return;
    }
    if (!state.tables.length) {
        list.innerHTML = `<li class="muted">No tables found.</li>`;
        return;
    }
    state.tables.forEach((table) => {
        const li = document.createElement("li");
        const button = document.createElement("button");
        button.type = "button";
        button.className = "tree-item";
        button.dataset.table = table;
        button.textContent = table;
        if (state.selectedTable === table && !state.selectedInc) {
            button.classList.add("is-active");
        }
        li.appendChild(button);
        list.appendChild(li);
    });
}

function renderIncList(showError = false) {
    const list = document.getElementById("tree-inc");
    if (!list) {
        return;
    }
    list.innerHTML = "";
    if (showError) {
        list.innerHTML = `<li class="muted">Unable to load descriptors.</li>`;
        return;
    }
    if (!state.incDescriptors.length) {
        list.innerHTML = `<li class="muted">No incremental descriptors.</li>`;
        return;
    }
    state.incDescriptors.forEach((descriptor, index) => {
        const li = document.createElement("li");
        const button = document.createElement("button");
        button.type = "button";
        button.className = "tree-item";
        button.dataset.incIndex = String(index);
        button.textContent = `${descriptor.table} › ${descriptor.key}`;
        if (state.selectedInc && descriptor.table === state.selectedInc.table && descriptor.key === state.selectedInc.key) {
            button.classList.add("is-active");
        }
        li.appendChild(button);
        list.appendChild(li);
    });
}

function selectTable(table) {
    state.selectedTable = table;
    state.selectedInc = null;
    Object.keys(formState).forEach((op) => {
        formState[op].table = table;
    });
    renderTablesList();
    renderIncList();
    renderOperationFields();
    updateOperationPreview();
    switchResultsTab("results");
}

function selectIncDescriptor(descriptor) {
    state.selectedTable = descriptor.table;
    state.selectedInc = descriptor;
    formState["read-inc"].table = descriptor.table;
    formState["read-inc"].key = descriptor.key;
    setOperation("read-inc");
    renderTablesList();
    renderIncList();
}

function setOperation(operation) {
    if (!formState[operation]) {
        return;
    }
    state.operation = operation;
    const select = document.getElementById("operation-select");
    if (select && select.value !== operation) {
        select.value = operation;
    }
    renderOperationFields();
    updateOperationPreview();
}

function renderOperationFields() {
    const container = document.getElementById("operation-fields");
    if (!container) {
        return;
    }
    const values = formState[state.operation];
    const selectedTable = values.table || state.selectedTable || "";

    let markup = "";
    switch (state.operation) {
        case "read-key":
            markup = `
                <label class="field">
                    <span>Table</span>
                    <input data-field="table" type="text" value="${escapeHTML(selectedTable)}" placeholder="table name">
                </label>
                <label class="field">
                    <span>Key</span>
                    <input data-field="key" type="text" value="${escapeHTML(values.key || "")}" placeholder="users.tbl:users:john">
                </label>
                <label class="field checkbox-field">
                    <span>
                        <input data-field="resolve" type="checkbox" ${values.resolve !== false ? "checked" : ""}>
                        Resolve nested data
                    </span>
                </label>
            `;
            break;
        case "list-entries":
            markup = `
                <label class="field">
                    <span>Table</span>
                    <input data-field="table" type="text" value="${escapeHTML(selectedTable)}" placeholder="table name">
                </label>
                <label class="field">
                    <span>Limit</span>
                    <input data-field="limit" type="number" min="1" max="500" value="${escapeHTML(values.limit || "100")}">
                </label>
                <label class="field">
                    <span>Start after (cursor)</span>
                    <input data-field="cursor" type="text" value="${escapeHTML(values.cursor || "")}" placeholder="optional key">
                </label>
            `;
            break;
        case "search-regex":
            markup = `
                <label class="field">
                    <span>Table</span>
                    <input data-field="table" type="text" value="${escapeHTML(selectedTable)}" placeholder="table name">
                </label>
                <label class="field">
                    <span>Regex</span>
                    <input data-field="regex" type="text" value="${escapeHTML(values.regex || "")}" placeholder="^users">
                </label>
                <label class="field">
                    <span>Limit</span>
                    <input data-field="limit" type="number" min="1" max="500" value="${escapeHTML(values.limit || "100")}">
                </label>
            `;
            break;
        case "read-inc":
            markup = `
                <label class="field">
                    <span>Table</span>
                    <input data-field="table" type="text" value="${escapeHTML(selectedTable)}" placeholder="table name">
                </label>
                <label class="field">
                    <span>Key</span>
                    <input data-field="key" type="text" value="${escapeHTML(values.key || "")}" placeholder="inc key">
                </label>
                <label class="field">
                    <span>Mode</span>
                    <select data-field="mode">
                        <option value="first" ${values.mode === "first" ? "selected" : ""}>First entries (oldest → newer)</option>
                        <option value="last" ${values.mode === "last" ? "selected" : ""}>Last entries (newest → older)</option>
                    </select>
                </label>
                <label class="field">
                    <span>Limit</span>
                    <input data-field="limit" type="number" min="1" max="500" value="${escapeHTML(values.limit || "50")}">
                </label>
            `;
            break;
        default:
            markup = "";
    }

    container.innerHTML = markup;
    container.querySelectorAll("input, select").forEach((element) => {
        element.addEventListener("input", onFieldChange);
        element.addEventListener("change", onFieldChange);
    });
}

function onFieldChange(event) {
    const field = event.target.dataset.field;
    if (!field) {
        return;
    }
    const values = formState[state.operation];
    const value = event.target.type === "checkbox" ? event.target.checked : event.target.value;
    values[field] = value;
    if (field === "table" && !value) {
        values[field] = state.selectedTable || "";
    }
    updateOperationPreview();
}

function collectOperationParams() {
    const values = formState[state.operation];
    switch (state.operation) {
        case "read-key": {
            const table = (values.table || state.selectedTable || "").trim();
            const key = (values.key || "").trim();
            if (!table) {
                throw new Error("Select a table");
            }
            if (!key) {
                throw new Error("Provide a key");
            }
            return {
                table,
                key,
                resolveNested: values.resolve !== false,
            };
        }
        case "list-entries": {
            const table = (values.table || state.selectedTable || "").trim();
            if (!table) {
                throw new Error("Select a table");
            }
            const limit = clampLimit(Number(values.limit) || 100);
            const cursor = (values.cursor || "").trim();
            return { table, limit, cursor };
        }
        case "search-regex": {
            const table = (values.table || state.selectedTable || "").trim();
            if (!table) {
                throw new Error("Select a table");
            }
            const regex = (values.regex || "").trim();
            if (!regex) {
                throw new Error("Provide a regex pattern");
            }
            const limit = clampLimit(Number(values.limit) || 100);
            return { table, regex, limit };
        }
        case "read-inc": {
            const table = (values.table || state.selectedTable || "").trim();
            const key = (values.key || "").trim();
            if (!table) {
                throw new Error("Select a table");
            }
            if (!key) {
                throw new Error("Provide incremental key");
            }
            const mode = values.mode === "last" ? "last" : "first";
            const limit = clampLimit(Number(values.limit) || 50);
            return { table, key, mode, limit };
        }
        default:
            throw new Error("Unsupported operation");
    }
}

function clampLimit(limit) {
    if (limit < 1) {
        return 1;
    }
    if (limit > 500) {
        return 500;
    }
    return limit;
}

async function runOperation() {
    try {
        const params = collectOperationParams();
        setStatus("Running…", false);
        switch (state.operation) {
            case "read-key":
                await executeReadKey(params);
                break;
            case "list-entries":
                await executeEntriesQuery(params, { label: "List entries" });
                break;
            case "search-regex":
                await executeEntriesQuery({ ...params, regex: params.regex }, { label: `Regex: ${params.regex}` });
                break;
            case "read-inc":
                await executeReadIncremental(params);
                break;
            default:
                throw new Error("Operation not implemented");
        }
        setStatus("Completed", false);
    } catch (error) {
        setStatus(error.message, true);
        appendMessage(error.message, "error");
    }
}

async function executeReadKey(params) {
    const url = `/api/admin/tables/${encodeURIComponent(params.table)}/entries/${encodeURIComponent(params.key)}`;
    const detail = await fetchJSON(url);
    appendMessage(`Read key ${params.key} from ${params.table}`, "info");
    renderResults("read-key", { detail, resolveNested: params.resolveNested });
}

async function executeEntriesQuery(params, context) {
    const query = new URLSearchParams();
    query.set("limit", String(params.limit));
    if (params.cursor) {
        query.set("cursor", params.cursor);
    }
    if (params.regex) {
        query.set("regex", params.regex);
    }
    const url = `/api/admin/tables/${encodeURIComponent(params.table)}/entries?${query.toString()}`;
    const payload = await fetchJSON(url);
    const entries = payload.entries || [];
    const nextCursor = payload.next_cursor || "";
    state.lastEntriesQuery = {
        operation: state.operation,
        params: { ...params },
        nextCursor,
    };
    const summary = `${context.label || "Entries"} · ${entries.length} row(s)`;
    appendMessage(`${summary} from ${params.table}`, "info");
    renderResults("entries-list", {
        table: params.table,
        entries,
        nextCursor,
        query: { ...params },
        label: context.label || "Entries",
    });
}

async function executeReadIncremental(params) {
    const query = new URLSearchParams();
    query.set("mode", params.mode);
    query.set("limit", String(params.limit));
    const url = `/api/admin/inc_tables/${encodeURIComponent(params.table)}/${encodeURIComponent(params.key)}?${query.toString()}`;
    const payload = await fetchJSON(url);
    appendMessage(`Read incremental ${params.key} (${params.mode})`, "info");
    renderResults("inc-entries", payload);
}

function renderResults(type, payload) {
    const container = document.getElementById("results-output");
    if (!container) {
        return;
    }
    switch (type) {
        case "read-key":
            container.innerHTML = renderEntryDetail(payload.detail, payload.resolveNested);
            attachEntryDetailEvents();
            break;
        case "entries-list":
            container.innerHTML = renderEntriesTable(payload);
            attachEntriesTableEvents(payload);
            break;
        case "inc-entries":
            container.innerHTML = renderIncResults(payload);
            break;
        default:
            container.innerHTML = `<div class="muted">No results.</div>`;
    }
    switchResultsTab("results");
}

function renderEntryDetail(detail, resolveNested) {
    const meta = [
        `Table: ${escapeHTML(detail.table)}`,
        `Size: ${formatBytes(detail.size)}`,
        `Ptr: ${detail.start_ptr} → ${detail.end_ptr}`,
    ].join(" · ");

    let resolvedBlock = "";
    if (resolveNested && detail.nested && detail.nested.resolved) {
        resolvedBlock = `
            <div class="result-block">
                <h3>Resolved nested</h3>
                <pre>${escapeHTML(JSON.stringify(detail.nested.resolved, null, 2))}</pre>
            </div>
        `;
    }

    let pointerBlock = "";
    const pointers = detail.nested?.pointers || [];
    if (pointers.length) {
        const rows = pointers
            .map((ptr) => {
                const summary = ptr.error ? `Error: ${ptr.error}` : formatPointerData(ptr.data);
                const escaped = escapeHTML(summary.length > 240 ? `${summary.slice(0, 240)}…` : summary);
                return `<tr><td>${escapeHTML(ptr.path || "(root)")}</td><td>${escapeHTML(ptr.id)}</td><td>${escaped}</td></tr>`;
            })
            .join("");
        pointerBlock = `
            <div class="result-block">
                <h3>Nested pointers</h3>
                <table class="result-table">
                    <thead><tr><th>Path</th><th>ID</th><th>Preview</th></tr></thead>
                    <tbody>${rows}</tbody>
                </table>
            </div>
        `;
    }

    return `
        <div class="result-meta">${meta}</div>
        <div class="result-block">
            <h3>Raw value</h3>
            <pre>${escapeHTML(detail.data)}</pre>
        </div>
        ${resolvedBlock}
        ${pointerBlock}
    `;
}

function attachEntryDetailEvents() {
    // Placeholder for future actions (editing, etc.)
}

function renderEntriesTable(payload) {
    if (!payload.entries.length) {
        return `<div class="muted">No entries returned.</div>`;
    }
    const rows = payload.entries
        .map((entry, index) => {
            const preview = escapeHTML(entry.preview || "");
            return `
                <tr data-key="${escapeHTML(entry.key)}" data-index="${index}">
                    <td>${escapeHTML(entry.key)}</td>
                    <td>${formatBytes(entry.size)}</td>
                    <td>${entry.has_nested ? "yes" : "no"}</td>
                    <td>${preview}</td>
                </tr>
            `;
        })
        .join("");

    const nextButton = payload.nextCursor
        ? `<button id="entries-next" class="ghost">Load more</button>`
        : "";

    return `
        <div class="result-meta">${escapeHTML(payload.label)} from ${escapeHTML(payload.table)}</div>
        <table class="result-table">
            <thead>
                <tr><th>Key</th><th>Size</th><th>Nested</th><th>Preview</th></tr>
            </thead>
            <tbody>${rows}</tbody>
        </table>
        <div style="margin-top:0.75rem;display:flex;gap:0.5rem;align-items:center;">${nextButton}<span class="muted">Double-click a row to read the key.</span></div>
    `;
}

function attachEntriesTableEvents(payload) {
    const table = document.querySelector(".result-table tbody");
    if (table) {
        table.addEventListener("dblclick", (event) => {
            const row = event.target.closest("tr[data-key]");
            if (!row) {
                return;
            }
            const key = row.dataset.key;
            formState["read-key"].table = payload.table;
            formState["read-key"].key = key;
            formState["read-key"].resolve = true;
            selectTable(payload.table);
            setOperation("read-key");
            runOperation();
        });
    }
    const nextButton = document.getElementById("entries-next");
    if (nextButton) {
        nextButton.addEventListener("click", () => {
            if (!state.lastEntriesQuery || !state.lastEntriesQuery.nextCursor) {
                return;
            }
            const { operation, params, nextCursor } = state.lastEntriesQuery;
            params.cursor = nextCursor;
            setOperation(operation);
            formState[operation].table = params.table;
            if (operation === "search-regex") {
                executeEntriesQuery(params, { label: `Regex: ${params.regex}` });
            } else {
                executeEntriesQuery(params, { label: "List entries" });
            }
        });
    }
}

function renderIncResults(payload) {
    const descriptor = payload.descriptor || {};
    const entries = payload.entries || [];
    const meta = `Table: ${escapeHTML(descriptor.table || "")}` +
        ` · Key: ${escapeHTML(descriptor.key || "")}` +
        (descriptor.entry_size ? ` · Entry size: ${formatBytes(descriptor.entry_size)}` : "");

    if (!entries.length) {
        return `<div class="muted">No incremental entries decoded.</div>`;
    }

    const rows = entries
        .map((entry) => `<tr><td>${entry.id}</td><td>${escapeHTML(entry.data)}</td></tr>`)
        .join("");

    return `
        <div class="result-meta">${escapeHTML(meta)}</div>
        <table class="result-table">
            <thead><tr><th>ID</th><th>Data</th></tr></thead>
            <tbody>${rows}</tbody>
        </table>
    `;
}

function switchResultsTab(tab) {
    document.querySelectorAll(".tab").forEach((button) => {
        button.classList.toggle("is-active", button.dataset.tab === tab);
    });
    document.querySelectorAll(".tab-content").forEach((content) => {
        content.classList.toggle("is-active", content.dataset.tab === tab);
    });
}

function updateOperationPreview() {
    const preview = document.getElementById("operation-preview");
    if (!preview) {
        return;
    }
    try {
        const params = collectOperationParams();
        preview.value = `tsu.${state.operation}(${JSON.stringify(params, null, 2)})`;
    } catch (error) {
        preview.value = `// ${error.message}`;
    }
}

function setStatus(message, isError) {
    const target = document.getElementById("operation-status");
    if (!target) {
        return;
    }
    target.textContent = message;
    target.classList.toggle("error", Boolean(isError));
}

function appendMessage(text, level = "info") {
    state.messages.push({
        text,
        level,
        time: new Date(),
    });
    if (state.messages.length > 50) {
        state.messages.shift();
    }
    renderMessages();
}

function renderMessages() {
    const container = document.getElementById("messages-output");
    if (!container) {
        return;
    }
    if (!state.messages.length) {
        container.innerHTML = `<div class="muted">No messages yet.</div>`;
        return;
    }
    const items = state.messages
        .slice()
        .reverse()
        .map((message) => {
            const timestamp = message.time.toLocaleTimeString();
            return `<li class="${message.level}"><strong>${timestamp}</strong> · ${escapeHTML(message.text)}</li>`;
        })
        .join("");
    container.innerHTML = `<ul class="messages-list">${items}</ul>`;
}

async function fetchJSON(url, options) {
    const response = await fetch(url, options);
    if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `${response.status} ${response.statusText}`);
    }
    return response.json();
}

function formatBytes(bytes) {
    if (!Number.isFinite(bytes) || bytes < 0) {
        return "n/a";
    }
    const units = ["B", "KB", "MB", "GB", "TB"];
    let value = bytes;
    let unitIndex = 0;
    while (value >= 1024 && unitIndex < units.length - 1) {
        value /= 1024;
        unitIndex++;
    }
    const formatted = value >= 10 || unitIndex === 0 ? value.toFixed(0) : value.toFixed(1);
    return `${formatted} ${units[unitIndex]}`;
}

function formatNumber(value) {
    if (!Number.isFinite(value)) {
        return "0";
    }
    return value.toLocaleString();
}

function escapeHTML(value) {
    if (value === null || value === undefined) {
        return "";
    }
    return String(value)
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/"/g, "&quot;")
        .replace(/'/g, "&#039;");
}

function formatPointerData(data) {
    if (data === null) {
        return "null";
    }
    if (data === undefined) {
        return "undefined";
    }
    if (typeof data === "object") {
        try {
            return JSON.stringify(data, null, 2);
        } catch (error) {
            return String(data);
        }
    }
    return String(data);
}

init();
