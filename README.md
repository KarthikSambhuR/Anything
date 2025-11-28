# Anything Search

**Anything** is a local, offline, semantic search engine and launcher for your desktop. It combines the speed of filename search with the intelligence of vector embeddings, allowing you to find files based on what they contain, not just what they are named.

## ✨ Features

### Intelligent Search
* **Local Semantic Search:** Powered by **ONNX Runtime** and the `all-MiniLM-L6-v2` model. Search for "invoice" and find `budget.pdf` even if the word "invoice" isn't in the file.
* **Hybrid Ranking:** Uses **Reciprocal Rank Fusion** to combine exact keyword matches (SQLite FTS5) with semantic vector matches (Cosine Similarity).
* **Smart Chunking:** Splits large documents (PDFs, DOCX) into analyzed segments, allowing you to find specific paragraphs on page 50 of a report.

### ⚡ Performance & Privacy
* **100% Offline:** No cloud APIs. No data leaves your machine.
* **Blazingly Fast:** Vector embeddings are cached in RAM for sub-millisecond search speeds.
* **Multi-Phase Indexing:**
    1.  **Quick Scan:** Indexes filenames instantly.
    2.  **Deep Scan:** Extracts text from `.txt`, `.md`, `.pdf`, `.docx` in the background.
    3.  **Embedding Scan:** Converts text to vectors using local AI.

### Natural Language Dates
Filter your files using human-readable phrases:
* *"Report from last month"*
* *"Notes from yesterday"*
* *"Budget from January"*

---

## 🛠️ Installation

### Prerequisites
* **Go 1.22+** installed.
* **GCC Compiler**.

### Build from Source
Since this project uses SQLite FTS5 (Fast Text Search), you **must** include the build tag.

```bash
git clone [https://github.com/yourusername/anything.git](https://github.com/KarthikSambhuR/Anything.git)
cd Anything
go build -tags fts5 -o Anything.exe main.go
```