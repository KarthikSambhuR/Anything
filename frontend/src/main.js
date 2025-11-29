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

    // THE FIX: Remove 'expanded' class immediately
    appContainer.classList.remove('expanded');

    // 2. Clear Search Data
    if (searchInput.value.trim() !== "") {
        // If there is text, re-run search to populate results and expand window
        performSearch(searchInput.value);
    } else {
        // If empty, ensure UI is clean and collapsed
        renderResults([]);
        appContainer.classList.remove('expanded');
    }

    // Note: We don't need to call WindowSetSize here anymore 
    // because the backend hideWindow already did it!

    // 3. Force Focus (Aggressively)
    forceFocus();

    // Reset lock after animation
    setTimeout(() => { isResetting = false; }, 200);
});

// Settings Open Event
window.runtime.EventsOn("settings:open", async () => {
    console.log("⚙️ Opening settings");
    const settings = await window.go.main.App.GetSettings();
    document.getElementById('set-hotkey').value = settings.hotkey || "Alt+Space";

    launcherView.classList.add('hidden');
    settingsView.classList.remove('hidden');
});

// Progress Events (Indexing & Download)
window.runtime.EventsOn("progress:update", (data) => {
    let bar, txt, det;

    if (data.type === 'indexing') {
        bar = document.getElementById('idx-bar');
        txt = document.getElementById('idx-percent');
        det = document.getElementById('idx-details');
    } else if (data.type === 'download') {
        bar = document.getElementById('dl-bar');
        txt = document.getElementById('dl-percent');
        det = document.getElementById('dl-details');
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

window.closeSettings = () => {
    settingsView.classList.add('hidden');
    launcherView.classList.remove('hidden');
    window.go.main.App.CloseSettings();

    forceFocus(); // <--- ADD THIS LINE
};

window.switchTab = (tabName) => {
    document.querySelectorAll('.tab-content').forEach(el => el.classList.remove('active'));
    document.querySelectorAll('.nav-item').forEach(el => el.classList.remove('active'));

    document.getElementById(`tab-${tabName}`).classList.add('active');

    const navs = document.querySelectorAll('.nav-item');
    navs.forEach(n => {
        if (n.getAttribute('onclick').includes(tabName)) n.classList.add('active');
    });
};

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
    searchInput.select();
    // Frame-perfect focus attempt
    requestAnimationFrame(() => searchInput.focus());
}

// CRITICAL: When OS gives focus to the window, force it into the input
window.addEventListener('focus', () => {
    // Only if we are in Launcher mode
    if (settingsView.classList.contains('hidden')) {
        forceFocus();
    }
});

async function performSearch(query) {
    try {
        currentResults = await window.go.main.App.Search(query);
        selectedIndex = 0;
        renderResults(currentResults);
        resizeWindow(currentResults.length > 0);
    } catch (err) { console.error(err); }
}

function renderResults(results) {
    resultsList.innerHTML = '';
    if (!results || results.length === 0) {
        currentResults = [];
        return;
    }

    results.forEach((res, index) => {
        const item = document.createElement('div');
        item.className = 'result-item';
        if (index === 0) item.classList.add('selected');

        item.onclick = () => window.go.main.App.OpenFile(res.Path);
        item.onmouseenter = () => { selectedIndex = index; updateSelection(); };

        const separator = res.Path.includes('\\') ? '\\' : '/';
        const parts = res.Path.split(separator);
        const filename = parts.pop();
        const dir = parts.join(separator);

        let iconHtml = "";
        if (res.Path === "anything://settings") {
            iconHtml = `<div class="icon-wrapper" style="background: rgba(122, 162, 247, 0.2);"><i class="fa-solid fa-gear" style="font-size: 20px; color: #7aa2f7;"></i></div>`;
        } else if (res.IconData && res.IconData.startsWith("data:")) {
            iconHtml = `<div class="icon-wrapper"><img src="${res.IconData}" /></div>`;
        } else {
            iconHtml = `<div class="icon-wrapper">${getIconForPath(res.Path)}</div>`;
        }

        item.innerHTML = `
            ${iconHtml}
            <div class="content">
                <div class="filename">${filename}</div>
                <div class="path">${dir}</div>
            </div>
            ${res.Score ? `<div class="score">${res.Score.toFixed(1)}</div>` : ''}
        `;
        resultsList.appendChild(item);
    });
}

function resizeWindow(hasResults) {
    const inputHeight = 60;
    const itemHeight = 50;
    const padding = 14;
    const maxItemsVisible = 5;

    if (hasResults) {
        const count = Math.min(currentResults.length, maxItemsVisible);
        const newHeight = inputHeight + (count * itemHeight) + padding;
        window.runtime.WindowSetSize(700, newHeight);
        appContainer.classList.add('expanded');
    } else {
        window.runtime.WindowSetSize(700, inputHeight);
        appContainer.classList.remove('expanded');
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