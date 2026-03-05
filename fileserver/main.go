package main

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed templates/*
var templateFS embed.FS

type FileInfo struct {
	Name    string
	Size    int64
	ModTime time.Time
	IsDir   bool
	Path    string
	Ext     string
}

type DirData struct {
	Path  string
	Parts []PathPart
	Files []FileInfo
}

type PathPart struct {
	Name string
	Path string
}

// 优先读取 DATA_DIR 环境变量，容器内由 entrypoint.sh 设置为 /data/files
var dataRoot = func() string {
	if v := os.Getenv("DATA_DIR"); v != "" {
		return v
	}
	return "data/files"
}()

func main() {
	port := "8080"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}
	if err := os.MkdirAll(dataRoot, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot create %s: %v\n", dataRoot, err)
	}
	http.HandleFunc("/__last_update__", lastUpdateHandler)
	http.HandleFunc("/__syncing__", syncingHandler)
	http.HandleFunc("/", handler)
	fmt.Printf("🚀 FileServer running at http://localhost:%s\n", port)
	fmt.Printf("📁 Serving: %s\n", dataRoot)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// lastUpdateFile 与 dataRoot 同级的上一层目录下的 .last_update 文件
var lastUpdateFile = func() string {
	if v := os.Getenv("DATA_DIR"); v != "" {
		// DATA_DIR=/data/files  →  /data/.last_update
		return filepath.Join(filepath.Dir(v), ".last_update")
	}
	return filepath.Join(filepath.Dir("data/files"), ".last_update")
}()

// syncingFile：同步进行中时由 entrypoint.sh 创建，完成后删除
var syncingFile = func() string {
	if v := os.Getenv("DATA_DIR"); v != "" {
		return filepath.Join(filepath.Dir(v), ".syncing")
	}
	return filepath.Join(filepath.Dir("data/files"), ".syncing")
}()

func isSyncing() bool {
	_, err := os.Stat(syncingFile)
	return err == nil
}

func syncingHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if isSyncing() {
		w.Write([]byte("1"))
	} else {
		w.Write([]byte("0"))
	}
}

func lastUpdateHandler(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(lastUpdateFile)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		w.Write([]byte(""))
		return
	}
	w.Write([]byte(strings.TrimSpace(string(data))))
}

var funcMap = template.FuncMap{
	"formatSize": formatSize,
	"formatTime": func(t time.Time) string { return t.Format("2006-01-02 15:04") },
	"fileIcon":   fileIcon,
	"langClass":  langClass,
	"add":        func(a, b int) int { return a + b },
}

func handler(w http.ResponseWriter, r *http.Request) {
	// 同步进行中时，返回同步中提示页（通过 Accept 头判断浏览器）
	if isSyncing() && strings.Contains(r.Header.Get("Accept"), "text/html") {
		serveSyncing(w, r)
		return
	}
	urlPath := strings.TrimPrefix(r.URL.Path, "/")
	fsPath := filepath.Join(dataRoot, filepath.FromSlash(urlPath))
	absData, _ := filepath.Abs(dataRoot)
	absPath, err := filepath.Abs(fsPath)
	if err != nil || !strings.HasPrefix(absPath, absData) {
		http.Error(w, "Forbidden", 403)
		return
	}
	info, err := os.Stat(fsPath)
	if err != nil {
		http.Error(w, "Not Found: "+urlPath, 404)
		return
	}
	if info.IsDir() {
		// 目录：浏览器显示列表页，wget/curl 返回 404
		if strings.Contains(r.Header.Get("Accept"), "text/html") {
			serveDir(w, r, fsPath, urlPath)
		} else {
			http.Error(w, "Not a file: "+urlPath, 404)
		}
		return
	}
	// 文件：统一返回原始内容，浏览器触发下载或直接渲染
	serveRaw(w, r, fsPath)
}

func serveSyncing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	const syncingHTML = `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>正在同步 — FileServer</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    background: #f6f8fa;
    color: #1f2328;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 20px;
  }
  .card {
    background: #fff;
    border: 1px solid #d0d7de;
    border-radius: 8px;
    padding: 40px 48px;
    text-align: center;
    max-width: 400px;
    width: 90%;
  }
  .spinner {
    width: 36px;
    height: 36px;
    border: 3px solid #d0d7de;
    border-top-color: #0969da;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
    margin: 0 auto 20px;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
  h2 { font-size: 16px; font-weight: 600; margin-bottom: 8px; color: #1f2328; }
  p  { font-size: 13px; color: #57606a; line-height: 1.6; }
</style>
</head>
<body>
  <div class="card">
    <div class="spinner"></div>
    <h2>文件正在同步中</h2>
    <p>请稍候，同步完成后页面将自动刷新。</p>
  </div>
  <script>
    // 每 2 秒轮询一次，同步完成后刷新页面
    function check() {
      fetch('/__syncing__')
        .then(r => r.text())
        .then(t => { if (t.trim() === '0') location.reload(); else setTimeout(check, 2000); })
        .catch(() => setTimeout(check, 2000));
    }
    setTimeout(check, 2000);
  </script>
</body>
</html>`
	fmt.Fprint(w, syncingHTML)
}

func serveDir(w http.ResponseWriter, r *http.Request, fsPath, urlPath string) {
	entries, err := os.ReadDir(fsPath)
	if err != nil {
		http.Error(w, "Error reading directory", 500)
		return
	}
	var files []FileInfo
	for _, e := range entries {
		info, _ := e.Info()
		filePath := urlPath
		if filePath != "" {
			filePath += "/"
		}
		filePath += e.Name()
		ext := ""
		if !e.IsDir() {
			ext = strings.ToLower(filepath.Ext(e.Name()))
		}
		files = append(files, FileInfo{
			Name:    e.Name(),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			Path:    "/" + filePath,
			Ext:     ext,
		})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})
	data := DirData{
		Path:  urlPath,
		Parts: buildParts(urlPath),
		Files: files,
	}
	tmpl := template.Must(template.New("dir.html").Funcs(funcMap).ParseFS(templateFS, "templates/dir.html"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, data)
}

func serveRaw(w http.ResponseWriter, r *http.Request, fsPath string) {
	ext := strings.ToLower(filepath.Ext(fsPath))
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		ct = "application/octet-stream"
	}
	f, err := os.Open(fsPath)
	if err != nil {
		http.Error(w, "Not Found", 404)
		return
	}
	defer f.Close()
	info, _ := f.Stat()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	if !isTextFile(ext) && !isImageFile(ext) {
		w.Header().Set("Content-Disposition",
			fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(fsPath)))
	}
	io.Copy(w, f)
}

func buildParts(urlPath string) []PathPart {
	parts := []PathPart{{Name: "root", Path: "/"}}
	if urlPath == "" {
		return parts
	}
	for _, seg := range strings.Split(urlPath, "/") {
		if seg == "" {
			continue
		}
		prev := parts[len(parts)-1].Path
		if prev == "/" {
			prev = ""
		}
		parts = append(parts, PathPart{Name: seg, Path: prev + "/" + seg})
	}
	return parts
}

func formatSize(size int64) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%d B", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	case size < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(size)/(1024*1024*1024))
	}
}

func isTextFile(ext string) bool {
	switch ext {
	case ".txt", ".md", ".go", ".py", ".js", ".ts", ".jsx", ".tsx",
		".html", ".htm", ".css", ".scss", ".json", ".xml", ".yaml", ".yml",
		".toml", ".sh", ".bash", ".zsh", ".c", ".cpp", ".h", ".java",
		".rs", ".rb", ".php", ".swift", ".kt", ".scala", ".r", ".sql",
		".graphql", ".proto", ".conf", ".ini", ".env", ".log", ".csv",
		".vue", ".svelte", ".tf", ".gitignore", ".gitattributes", ".dockerfile":
		return true
	}
	return false
}

func isImageFile(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".bmp", ".ico":
		return true
	}
	return false
}

func langClass(ext string) string {
	m := map[string]string{
		".go": "go", ".py": "python", ".js": "javascript", ".ts": "typescript",
		".jsx": "javascript", ".tsx": "typescript", ".html": "html", ".htm": "html",
		".css": "css", ".scss": "scss", ".json": "json", ".xml": "xml",
		".yaml": "yaml", ".yml": "yaml", ".toml": "toml", ".sh": "bash",
		".bash": "bash", ".c": "c", ".cpp": "cpp", ".h": "c", ".java": "java",
		".rs": "rust", ".rb": "ruby", ".php": "php", ".swift": "swift",
		".kt": "kotlin", ".sql": "sql", ".md": "markdown", ".r": "r",
		".proto": "protobuf", ".graphql": "graphql", ".tf": "hcl",
	}
	if lang, ok := m[ext]; ok {
		return lang
	}
	return "plaintext"
}

func fileIcon(f FileInfo) string {
	if f.IsDir {
		return "📁"
	}
	icons := map[string]string{
		".go": "🐹", ".py": "🐍", ".js": "🟨", ".ts": "🔷", ".jsx": "⚛️", ".tsx": "⚛️",
		".html": "🌐", ".css": "🎨", ".json": "📋", ".md": "📝", ".txt": "📄",
		".sh": "⚙️", ".bash": "⚙️", ".pdf": "📕", ".zip": "📦", ".tar": "📦",
		".gz": "📦", ".png": "🖼️", ".jpg": "🖼️", ".jpeg": "🖼️", ".gif": "🖼️",
		".svg": "🖼️", ".mp4": "🎬", ".mp3": "🎵", ".wav": "🎵", ".yml": "⚙️",
		".yaml": "⚙️", ".toml": "⚙️", ".sql": "🗄️", ".rs": "🦀", ".java": "☕",
		".rb": "💎", ".php": "🐘", ".swift": "🍎", ".kt": "🎯", ".csv": "📊",
		".log": "📜", ".env": "🔐", ".conf": "⚙️", ".xml": "📋",
	}
	if icon, ok := icons[f.Ext]; ok {
		return icon
	}
	return "📄"
}
