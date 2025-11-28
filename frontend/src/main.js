const searchInput = document.getElementById('search-input');
const resultsList = document.getElementById('results-list');
const appContainer = document.querySelector('.app-container');

let debounceTimer;
let selectedIndex = 0;
let currentResults = [];

searchInput.focus();

searchInput.addEventListener('input', (e) => {
    const query = e.target.value;
    clearTimeout(debounceTimer);

    if (query.trim() === "") {
        renderResults([]);
        resizeWindow(false);
        return;
    }

    debounceTimer = setTimeout(() => {
        performSearch(query);
    }, 100);
});

document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
        if (searchInput.value !== "") {
            searchInput.value = "";
            renderResults([]);
            resizeWindow(false);
        } else {
            window.runtime.Quit();
        }
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
            const selectedFile = currentResults[selectedIndex];
            window.go.main.App.OpenFile(selectedFile.Path);
        }
    }
});

async function performSearch(query) {
    try {
        currentResults = await window.go.main.App.Search(query);
        selectedIndex = 0;
        renderResults(currentResults);
        resizeWindow(currentResults.length > 0);
    } catch (err) {
        console.error(err);
    }
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

        // --- FIX: Use IconData, not IconPath ---
        let iconHtml = "";

        // Check if backend sent a Base64 string
        if (res.IconData && res.IconData.startsWith("data:")) {
            iconHtml = `<img src="${res.IconData}" style="width:24px; height:24px; object-fit:contain;" />`;
        } else {
            // Fallback to SVG mapper
            iconHtml = getIconForPath(res.Path);
        }
        // ----------------------------------------

        let displaySnippet = "";
        if (res.Snippet) {
            displaySnippet = res.Snippet
                .replace(/\[/g, '<span class="highlight">')
                .replace(/\]/g, '</span>');
        }

        item.innerHTML = `
            <div class="icon">${iconHtml}</div>
            <div class="content">
                <div class="filename">${filename}</div>
                <div class="path">${dir}</div>
                ${displaySnippet ? `<div class="snippet">${displaySnippet}</div>` : ''}
            </div>
            ${res.Score ? `<div class="score">${res.Score.toFixed(1)}</div>` : ''}
        `;

        resultsList.appendChild(item);
    });
}

function updateSelection() {
    const items = document.querySelectorAll('.result-item');
    items.forEach((item, index) => {
        if (index === selectedIndex) {
            item.classList.add('selected');
            item.scrollIntoView({ block: 'nearest' });
        } else {
            item.classList.remove('selected');
        }
    });
}

function resizeWindow(hasResults) {
    if (hasResults) {
        window.runtime.WindowSetSize(700, 600);
        appContainer.classList.add('expanded');
    } else {
        window.runtime.WindowSetSize(700, 60);
        appContainer.classList.remove('expanded');
    }
    setTimeout(() => { window.runtime.WindowCenter(); }, 10);
}

function getIconForPath(path) {
    const lower = path.toLowerCase();
    if (lower.endsWith(".pdf")) {
        return `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#f38ba8" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><line x1="16" y1="13" x2="8" y2="13"></line><line x1="16" y1="17" x2="8" y2="17"></line><polyline points="10 9 9 9 8 9"></polyline></svg>`;
    }
    if (lower.endsWith(".docx") || lower.endsWith(".doc") || lower.endsWith(".rtf")) {
        return `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#89b4fa" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><line x1="16" y1="13" x2="8" y2="13"></line><line x1="16" y1="17" x2="8" y2="17"></line><polyline points="10 9 9 9 8 9"></polyline></svg>`;
    }
    if (lower.endsWith(".jpg") || lower.endsWith(".png") || lower.endsWith(".jpeg") || lower.endsWith(".svg")) {
        return `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#a6e3a1" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect><circle cx="8.5" cy="8.5" r="1.5"></circle><polyline points="21 15 16 10 5 21"></polyline></svg>`;
    }
    if (lower.endsWith(".go") || lower.endsWith(".js") || lower.endsWith(".py") || lower.endsWith(".html") || lower.endsWith(".css") || lower.endsWith(".json")) {
        return `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#f9e2af" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"></polyline><polyline points="8 6 2 12 8 18"></polyline></svg>`;
    }
    return `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#a6adc8" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"></path><polyline points="13 2 13 9 20 9"></polyline></svg>`;
}