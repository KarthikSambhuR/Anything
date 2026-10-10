const searchInput = document.getElementById('search-input');
const resultsList = document.getElementById('results-list');
const appContainer = document.querySelector('.app-container');
const launcherView = document.getElementById('launcher-view');
const settingsView = document.getElementById('settings-view');

let debounceTimer;
let selectedIndex = 0;
let currentResults = [];
let isResetting = false; // NEW: Prevent event conflicts

// --- 1. WINDOW EVENT LISTENERS (CRITICAL) ---

// REMOVE or COMMENT OUT the wails:window:show handler - it conflicts with window:reset
// window.runtime.EventsOn("wails:window:show", () => {
//     console.log("❌ wails:window:show fired (DISABLED)");
// });

window.runtime.EventsOn("window:reset", () => {
    if (isResetting) return;
    isResetting = true;
    console.log("🔄 window:reset event received");

    // 1. Force View to Launcher
    settingsView.classList.add('hidden');
    launcherView.classList.remove('hidden');
    appContainer.classList.remove('expanded');

    // 2. Clear input and results fresh
    searchInput.value = '';
    renderResults([]);

    // 3. Focus input on next frame
    requestAnimationFrame(() => searchInput.focus());

    setTimeout(() => { isResetting = false; }, 200);
});

// Settings Open Event
window.runtime.EventsOn("settings:open", async () => {
    console.log("⚙️ Opening settings");
    const settings = await window.go.main.App.GetSettings();
    document.getElementById('set-hotkey').value = settings.hotkey || "Alt+Space";
    
    document.getElementById('set-ignored-paths').value = (settings.ignored_paths || []).join(", ");
    document.getElementById('set-allowed-extensions').value = (settings.allowed_extensions || []).join(", ");
    document.getElementById('set-indexed-directories').value = (settings.indexed_directories || []).join(", ");
    document.getElementById('set-priority-directories').value = (settings.priority_directories || []).join(", ");

    await refreshModelList();

    launcherView.classList.add('hidden');
    settingsView.classList.remove('hidden');
});

// Model management
let availableModels = [];

async function refreshModelList() {
    try {
        const settings = await window.go.main.App.GetSettings();
        const selectedModel = settings.embedding_model || "all-minilm-l6-v2";
        
        availableModels = await window.go.main.App.GetAvailableModels();
        const modelSelect = document.getElementById('set-ai-model');
        if (modelSelect) {
            modelSelect.value = selectedModel;
        }
        updateModelInfoUI(selectedModel);
    } catch (e) {
        console.error("Failed to load models:", e);
    }
}

function updateModelInfoUI(modelId) {
    const model = availableModels.find(m => m.id === modelId) || {
        name: modelId,
        description: "",
        downloaded: false
    };

    const nameEl = document.getElementById('model-info-name');
    const descEl = document.getElementById('model-info-desc');
    const badgeEl = document.getElementById('model-info-badge');
    const btnDl = document.getElementById('btn-download-model');

    if (nameEl) nameEl.innerText = model.name || modelId;
    if (descEl) descEl.innerText = model.description || "";
    
    if (badgeEl) {
        if (model.downloaded) {
            badgeEl.className = "model-info-status badge-ready";
            badgeEl.innerHTML = '<i class="fa-solid fa-circle-check"></i> Downloaded & Ready';
            if (btnDl) btnDl.innerHTML = '<i class="fa-solid fa-arrows-rotate"></i> Re-verify / Re-download';
        } else {
            badgeEl.className = "model-info-status badge-missing";
            badgeEl.innerHTML = '<i class="fa-solid fa-circle-exclamation"></i> Not Downloaded';
            if (btnDl) btnDl.innerHTML = '<i class="fa-solid fa-cloud-arrow-down"></i> Auto-Download Selected Model';
        }
    }
}

window.onModelSelectChanged = async (modelId) => {
    updateModelInfoUI(modelId);
    const settings = await window.go.main.App.GetSettings();
    settings.embedding_model = modelId;
    await window.go.main.App.SaveSettings(settings);
};

window.triggerDownloadModel = () => {
    const select = document.getElementById('set-ai-model');
    const modelId = select ? select.value : "all-minilm-l6-v2";
    window.go.main.App.DownloadModel(modelId);
};

// Progress Events (Indexing & Download)
window.runtime.EventsOn("progress:update", (data) => {
    let bar, txt, det;

    if (data.type === 'indexing') {
        bar = document.getElementById('idx-bar');
        txt = document.getElementById('idx-percent');
        det = document.getElementById('idx-details');

        // Multi-stage tracker
        const stageScan = document.getElementById('stage-scan');
        const stageContent = document.getElementById('stage-content');
        const stageVectors = document.getElementById('stage-vectors');

        const scanSub = document.getElementById('stage-scan-sub');
        const contentSub = document.getElementById('stage-content-sub');
        const vectorsSub = document.getElementById('stage-vectors-sub');

        const msg = data.message || "";

        if (msg.includes("Scanning") || msg.includes("QuickScan")) {
            if (stageScan) stageScan.className = "stage-pill active";
            if (scanSub) scanSub.innerText = msg;
        } else if (msg.includes("Extracting") || msg.includes("DeepScan") || msg.includes("Reading")) {
            if (stageScan) stageScan.className = "stage-pill completed";
            if (scanSub) scanSub.innerText = "Discovery Completed";

            if (stageContent) stageContent.className = "stage-pill active";
            if (contentSub) contentSub.innerText = msg;
        } else if (msg.includes("Embedding") || msg.includes("Vector")) {
            if (stageScan) stageScan.className = "stage-pill completed";
            if (stageContent) stageContent.className = "stage-pill completed";
            if (contentSub) contentSub.innerText = "Extraction Completed";

            if (stageVectors) stageVectors.className = "stage-pill active";
            if (vectorsSub) vectorsSub.innerText = msg;
        } else if (data.percent === 100 || msg.includes("Complete")) {
            if (stageScan) stageScan.className = "stage-pill completed";
            if (stageContent) stageContent.className = "stage-pill completed";
            if (stageVectors) stageVectors.className = "stage-pill completed";
            if (vectorsSub) vectorsSub.innerText = "All Vectors Synchronized";
        }

    } else if (data.type === 'download') {
        bar = document.getElementById('dl-bar');
        txt = document.getElementById('dl-percent');
        det = document.getElementById('dl-details');

        // If completed, refresh model status badges
        if (data.percent === 100) {
            refreshModelList();
        }
    }

    if (bar && txt) {
        if (data.percent >= 0) {
            bar.style.width = data.percent + "%";
            txt.innerText = data.percent + "%";
        } else {
            bar.style.width = "100%";
            bar.style.opacity = "0.5";
        }
        if (det) det.innerText = data.message;
    }
});

// --- 2. SETTINGS LOGIC ---

window.closeSettings = async () => {
    const isSettings = await window.go.main.App.IsSettingsWindow();
    if (isSettings) {
        window.go.main.App.CloseSettings();
    } else {
        settingsView.classList.add('hidden');
        launcherView.classList.remove('hidden');
        forceFocus();
    }
};

window.switchTab = (tabName) => {
    document.querySelectorAll('.tab-content').forEach(el => el.classList.remove('active'));
    document.querySelectorAll('.nav-item').forEach(el => el.classList.remove('active'));

    document.getElementById(`tab-${tabName}`).classList.add('active');

    const navs = document.querySelectorAll('.nav-item');
    navs.forEach(n => {
        if (n.getAttribute('onclick').includes(tabName)) n.classList.add('active');
    });

    if (tabName === 'ai') {
        refreshModelList();
    } else if (tabName === 'apps') {
        loadApps();
    }
};

async function loadApps() {
    const container = document.getElementById('apps-list-container');
    if (!container) return;
    
    container.innerHTML = `
        <div style="padding: 20px; text-align: center; color: var(--text-secondary); grid-column: 1 / -1;">
            <i class="fa-solid fa-circle-notch fa-spin" style="margin-right: 8px;"></i> Loading applications...
        </div>`;
        
    try {
        const apps = await window.go.main.App.GetIndexedApplications();
        if (!apps || apps.length === 0) {
            container.innerHTML = `
                <div style="padding: 20px; text-align: center; color: var(--text-secondary); grid-column: 1 / -1;">
                    <i class="fa-solid fa-ghost" style="margin-right: 8px;"></i> No applications found in index yet.
                </div>`;
            return;
        }
        
        container.innerHTML = '';
        apps.forEach(app => {
            const card = document.createElement('div');
            card.className = 'app-card';
            
            const separator = app.Path.includes('\\') ? '\\' : '/';
            const parts = app.Path.split(separator);
            const name = parts.pop() || app.Path;
            const dir = parts.join(separator);
            
            let iconHtml = '';
            if (app.IconData && app.IconData.startsWith("data:")) {
                iconHtml = `<img src="${app.IconData}" class="app-card-icon" />`;
            } else {
                iconHtml = `<div class="app-card-icon-placeholder"><i class="fa-brands fa-windows"></i></div>`;
            }
            
            card.innerHTML = `
                ${iconHtml}
                <div class="app-card-name" title="${name}">${escapeHtml(name)}</div>
                <div class="app-card-path" title="${dir}">${escapeHtml(dir)}</div>
            `;
            
            card.onclick = () => window.go.main.App.OpenFile(app.Path);
            card.style.cursor = 'pointer';
            
            container.appendChild(card);
        });
    } catch (e) {
        console.error("Failed to load apps:", e);
        container.innerHTML = `
            <div style="padding: 20px; text-align: center; color: #f38ba8; grid-column: 1 / -1;">
                <i class="fa-solid fa-triangle-exclamation" style="margin-right: 8px;"></i> Error loading applications.
            </div>`;
    }
}

window.triggerRebuild = () => {
    window.go.main.App.RebuildIndex();
};

window.triggerDownload = () => {
    window.go.main.App.DownloadModels();
};

document.getElementById('set-hotkey').addEventListener('change', async (e) => {
    const settings = await window.go.main.App.GetSettings();
    settings.hotkey = e.target.value;
    await window.go.main.App.SaveSettings(settings);
});

function setupSettingsArrayListener(id, key) {
    const el = document.getElementById(id);
    if (!el) return;
    el.addEventListener('change', async (e) => {
        const settings = await window.go.main.App.GetSettings();
        const val = e.target.value;
        settings[key] = val.split(',').map(s => s.trim()).filter(s => s !== "");
        await window.go.main.App.SaveSettings(settings);
    });
}

setupSettingsArrayListener('set-ignored-paths', 'ignored_paths');
setupSettingsArrayListener('set-allowed-extensions', 'allowed_extensions');
setupSettingsArrayListener('set-indexed-directories', 'indexed_directories');
setupSettingsArrayListener('set-priority-directories', 'priority_directories');

// --- 3. SEARCH LOGIC (Standard) ---

searchInput.addEventListener('input', (e) => {
    const query = e.target.value;
    clearTimeout(debounceTimer);
    if (query.trim() === "") {
        renderResults([]);
        resizeWindow(false);
        return;
    }
    debounceTimer = setTimeout(() => performSearch(query), 100);
});

document.addEventListener('keydown', (e) => {
    // If settings open, ESC closes settings
    if (!settingsView.classList.contains('hidden')) {
        if (e.key === 'Escape') window.closeSettings();
        return;
    }

    // Normal Launcher Keys
    if (e.key === 'Escape') {
        console.log("🔒 ESC pressed - hiding window");
        window.go.main.App.OnHide();
        window.runtime.WindowHide();
    }
    else if (e.key === 'ArrowDown') {
        e.preventDefault();
        if (currentResults.length > 0) {
            selectedIndex = (selectedIndex + 1) % currentResults.length;
            updateSelection();
        }
    }
    else if (e.key === 'ArrowUp') {
        e.preventDefault();
        if (currentResults.length > 0) {
            selectedIndex = (selectedIndex - 1 + currentResults.length) % currentResults.length;
            updateSelection();
        }
    }
    else if (e.key === 'Enter') {
        if (currentResults.length > 0) {
            window.go.main.App.OpenFile(currentResults[selectedIndex].Path);
        }
    }
});

// --- FOCUS HELPER ---
function forceFocus() {
    searchInput.focus();
}

// When OS gives focus to the window, redirect into input
window.addEventListener('focus', () => {
    if (settingsView.classList.contains('hidden')) {
        searchInput.focus();
    }
});

async function performSearch(query) {
    try {
        console.log("🔍 [FRONTEND] Searching for:", query);
        currentResults = await window.go.main.App.Search(query);
        console.log("🔍 [FRONTEND] Got", currentResults ? currentResults.length : 0, "results:", currentResults);
        selectedIndex = 0;
        renderResults(currentResults);
        resizeWindow(currentResults && currentResults.length > 0);
    } catch (err) {
        console.error("❌ [FRONTEND] Search error:", err);
    }
}

function renderResults(results) {
    resultsList.innerHTML = '';
    if (!results || results.length === 0) {
        currentResults = [];
        return;
    }

    currentResults = results;

    results.forEach((res, index) => {
        const item = document.createElement('div');
        item.className = 'result-item';
        if (index === 0) item.classList.add('selected');

        const path = res.Path || res.path || "";
        const snippet = res.Snippet || res.snippet || "";
        const score = res.Score !== undefined ? res.Score : res.score;
        const iconData = res.IconData || res.iconData || "";

        item.onclick = () => window.go.main.App.OpenFile(path);
        item.onmouseenter = () => { selectedIndex = index; updateSelection(); };

        let iconHtml = "";
        let title = "";
        let subtitle = "";

        if (path === "anything://settings") {
            iconHtml = `<div class="icon-wrapper" style="background: rgba(122, 162, 247, 0.2);"><i class="fa-solid fa-gear" style="font-size: 20px; color: #7aa2f7;"></i></div>`;
            title = "Settings";
            subtitle = snippet || "Configure AI, Indexing, and Hotkeys";
        } else {
            const separator = path.includes('\\') ? '\\' : '/';
            const parts = path.split(separator);
            title = parts.pop() || path;
            subtitle = parts.join(separator) || path;

            if (iconData && iconData.startsWith("data:")) {
                iconHtml = `<div class="icon-wrapper"><img src="${iconData}" /></div>`;
            } else {
                iconHtml = `<div class="icon-wrapper">${getIconForPath(path)}</div>`;
            }
        }

        item.innerHTML = `
            ${iconHtml}
            <div class="content">
                <div class="filename">${escapeHtml(title)}</div>
                <div class="path">${escapeHtml(subtitle)}</div>
            </div>
            ${score ? `<div class="score">${score.toFixed(1)}</div>` : ''}
        `;
        resultsList.appendChild(item);
    });
}

function escapeHtml(text) {
    if (!text) return "";
    return text.replace(/[&<>"']/g, function(m) {
        return {
            '&': '&amp;',
            '<': '&lt;',
            '>': '&gt;',
            '"': '&quot;',
            "'": '&#039;'
        }[m];
    });
}

function resizeWindow(hasResults) {
    if (hasResults) {
        appContainer.classList.add('expanded');
        setTimeout(() => {
            let height = 60; // Search box height
            if (currentResults && currentResults.length > 0) {
                height += 16; // results-container padding (8px top + 8px bottom)
                const items = document.querySelectorAll('.result-item');
                items.forEach(item => {
                    height += item.offsetHeight + 2; // height + margin-bottom
                });
                
                if (height > 440) {
                    height = 440; // Max height (380 max-height + 60 searchbox)
                }
            }
            window.go.main.App.SetHeight(height);
            setTimeout(() => window.runtime.WindowCenter(), 10);
        }, 10);
    } else {
        appContainer.classList.remove('expanded');
        window.go.main.App.SetHeight(60);
        setTimeout(() => window.runtime.WindowCenter(), 10);
    }
}

function updateSelection() {
    const items = document.querySelectorAll('.result-item');
    items.forEach((item, index) => {
        if (index === selectedIndex) {
            item.classList.add('selected');
            item.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
        } else {
            item.classList.remove('selected');
        }
    });
}

function getIconForPath(path) {
    return `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#a6adc8" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"></path><polyline points="13 2 13 9 20 9"></polyline></svg>`;
}

document.addEventListener('DOMContentLoaded', async () => {
    // Check if this process is the dedicated settings window
    setTimeout(async () => {
        try {
            const isSettings = await window.go.main.App.IsSettingsWindow();
            if (isSettings) {
                document.querySelector('.app-container').classList.add('expanded');
                document.querySelector('.app-container').style.maxHeight = 'none';
                document.querySelector('.app-container').style.height = '100vh';
                
                // Directly trigger the event handler
                window.runtime.EventsEmit("settings:open");
            }
        } catch (e) {
            console.error(e);
        }
    }, 100);
});