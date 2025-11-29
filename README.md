# Anything Search

**A local, offline, semantic search engine and launcher for your desktop.**

**Anything** combines the speed of filename search with the intelligence of vector embeddings. It allows you to find files based on what they *contain*, not just what they are named, and serves as a complete Spotlight-style launcher for Windows.

## ✨ Features

### 🧠 Intelligent & Semantic Search
* **Local Semantic Search:** Powered by **ONNX Runtime** and the `all-MiniLM-L6-v2` model. Search for "invoice" and find `budget.pdf` even if the word "invoice" never appears in the file.
* **Hybrid Ranking:** Uses **Reciprocal Rank Fusion** to combine exact keyword matches (SQLite FTS5) with semantic vector matches (Cosine Similarity) for the best of both worlds.
* **Smart Chunking:** Splits large documents (PDFs, DOCX) into analyzed segments, allowing you to locate specific paragraphs deep within a report.
* **Natural Language Dates:** Filter files using human phrases like *"Report from last month"*, *"Notes from yesterday"*, or *"Budget from January"*.

### 👁️ AI Vision System (New in v0.5)
* **Auto-Tagging:** Integrated **MobileNet V2** automatically tags images based on content (e.g., "receipt", "cat", "screenshot").
* **Native OCR:** Uses Native Windows OCR (0MB dependency) to read and index text inside images.
* **Visual Search:** Images are now searchable by both their visual content tags and the text written inside them.

### 🚀 Launcher & Productivity
* **Global Hotkey:** Press `Alt + Space` to toggle the launcher instantly.
* **Smart Learning:** A dedicated `usage_stats` DB learns from your behavior. Apps and files you open frequently automatically jump to the top of search results.
* **Focus Management:** Uses `AttachThreadInput` to ensure the window correctly steals focus when summoned, so you can start typing immediately.
* **App Scanning:** Native app scanning with a 10x ranking boost for `.exe` and `.lnk` files.

### 🛡️ Privacy & Performance
* **100% Offline:** No cloud APIs. No data leaves your machine.
* **Blazingly Fast:** Vector embeddings are cached in RAM for sub-millisecond search speeds.
* **Lazy Loading:** Thumbnails load asynchronously so typing is never laggy.

---

## 🛠️ Under the Hood

**Anything** is built for stability and speed using **Wails** (Go backend + Vanilla JS frontend).

### Indexing Pipeline
1.  **Quick Scan:** Indexes filenames instantly.
2.  **Deep Scan:** Extracts text from `.txt`, `.md`, `.pdf`, `.docx` in the background.
3.  **Embedding Scan:** Converts text to vectors using local AI.

### System Integration
* **Storage:** Database and AI models are stored in `%APPDATA%` for stability.
* **Icons:** Native icons are extracted using raw `Shell32` API calls and stored via Base64 to prevent file watcher crash loops.

---

## 🆕 Release Notes (v0.5)

* **UI Overhaul:** Switched to a frameless Spotlight-style UI with the Poppins font, larger icons, and dynamic window resizing based on result count.
* **Smart Ranking:** Implemented frequency-based result boosting; frequently used apps bubble to the top.
* **Vision & OCR:** Added MobileNet V2 and Windows Native OCR for image search.
* **Fixes:** Fixed ONNX DLL version mismatch and SQL NULL scan errors.

---

## 📥 Installation

### Prerequisites
* **Go 1.22+**
* **GCC Compiler** (Required for SQLite CGO).

### Build from Source
Because this project utilizes SQLite FTS5 (Fast Text Search), you **must** include the specific build tag during compilation.

```bash
# 1. Clone the repository
git clone [https://github.com/KarthikSambhuR/Anything.git](https://github.com/KarthikSambhuR/Anything.git)
cd Anything

# 2. Build with FTS5 tags
wails build -tags fts5
```
### Running Anything
Once the build is complete, you can run the application using the `Anything.exe`. The app will run in the background.
Press `Alt + Space` to open the launcher.