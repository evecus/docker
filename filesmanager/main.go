package main

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed templates/*
var templateFS embed.FS

var dataRoot = func() string {
	if v := os.Getenv("DATA_DIR"); v != "" {
		return v
	}
	return "data/files"
}()

var (
	authUser = os.Getenv("U")
	authPass = os.Getenv("P")
)

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

type FileData struct {
	Name      string
	Size      int64
	Ext       string
	Parts     []PathPart
	RawURL    string
	WgetCmd   string
	IsText    bool
	IsImage   bool
	IsTooBig  bool
	Content   template.HTML
	ImageMime string
	ImageB64  string
}

func main() {
	port := "8080"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}
	if err := os.MkdirAll(dataRoot, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot create %s: %v\n", dataRoot, err)
	}

	http.HandleFunc("/api/", apiHandler)
	http.HandleFunc("/__update_action__", updateActionHandler)
	http.HandleFunc("/update", updateHandler)
	http.HandleFunc("/update/", updateHandler)
	http.HandleFunc("/__raw__/", rawHandler)
	http.HandleFunc("/", handler)

	fmt.Printf("🚀 FilesManager running at http://localhost:%s\n", port)
	fmt.Printf("📁 Serving: %s\n", dataRoot)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// ── Auth ──────────────────────────────────────────────────────────────────────

func checkBasicAuth(r *http.Request) bool {
	if authUser == "" && authPass == "" {
		return true
	}
	u, p, ok := r.BasicAuth()
	return ok && u == authUser && p == authPass
}

func checkSessionAuth(r *http.Request) bool {
	if authUser == "" && authPass == "" {
		return true
	}
	cookie, err := r.Cookie("fs_auth")
	if err != nil {
		return false
	}
	expected := base64.StdEncoding.EncodeToString([]byte(authUser + ":" + authPass))
	return cookie.Value == expected
}

func setSessionCookie(w http.ResponseWriter, user, pass string) {
	val := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	http.SetCookie(w, &http.Cookie{
		Name: "fs_auth", Value: val, Path: "/",
		HttpOnly: true, MaxAge: 86400 * 7,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "fs_auth", Value: "", Path: "/", MaxAge: -1})
}

// ── Path safety ───────────────────────────────────────────────────────────────

func safePath(urlPath string) (string, error) {
	fsPath := filepath.Join(dataRoot, filepath.FromSlash(urlPath))
	absData, _ := filepath.Abs(dataRoot)
	absPath, err := filepath.Abs(fsPath)
	if err != nil || !strings.HasPrefix(absPath, absData) {
		return "", fmt.Errorf("forbidden")
	}
	return fsPath, nil
}

// ── Templates / funcMap ───────────────────────────────────────────────────────

var funcMap = template.FuncMap{
	"formatSize": formatSize,
	"formatTime": func(t time.Time) string { return t.Format("2006-01-02 15:04") },
	"fileIcon":   fileIcon,
	"langClass":  langClass,
	"add":        func(a, b int) int { return a + b },
	"not":        func(b bool) bool { return !b },
}

// ── Public handler ────────────────────────────────────────────────────────────

func rawHandler(w http.ResponseWriter, r *http.Request) {
	urlPath := strings.TrimPrefix(r.URL.Path, "/__raw__/")
	fsPath, err := safePath(urlPath)
	if err != nil {
		http.Error(w, "Forbidden", 403)
		return
	}
	serveRaw(w, r, fsPath)
}

func handler(w http.ResponseWriter, r *http.Request) {
	urlPath := strings.TrimPrefix(r.URL.Path, "/")
	fsPath, err := safePath(urlPath)
	if err != nil {
		http.Error(w, "Forbidden", 403)
		return
	}
	info, err := os.Stat(fsPath)
	if err != nil {
		http.Error(w, "Not Found: "+urlPath, 404)
		return
	}
	if info.IsDir() {
		if strings.Contains(r.Header.Get("Accept"), "text/html") {
			serveDir(w, r, fsPath, urlPath)
		} else {
			http.Error(w, "Not a file: "+urlPath, 404)
		}
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		serveFilePage(w, r, fsPath, urlPath)
	} else {
		serveRaw(w, r, fsPath)
	}
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
		fp := urlPath
		if fp != "" {
			fp += "/"
		}
		fp += e.Name()
		ext := ""
		if !e.IsDir() {
			ext = strings.ToLower(filepath.Ext(e.Name()))
		}
		files = append(files, FileInfo{
			Name: e.Name(), IsDir: e.IsDir(), Size: info.Size(),
			ModTime: info.ModTime(), Path: "/" + fp, Ext: ext,
		})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})
	data := DirData{Path: urlPath, Parts: buildParts(urlPath), Files: files}
	tmpl := template.Must(template.New("dir.html").Funcs(funcMap).ParseFS(templateFS, "templates/dir.html"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, data)
}

func serveFilePage(w http.ResponseWriter, r *http.Request, fsPath, urlPath string) {
	info, err := os.Stat(fsPath)
	if err != nil {
		http.Error(w, "Not Found", 404)
		return
	}
	ext := strings.ToLower(filepath.Ext(fsPath))
	name := filepath.Base(fsPath)
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	rawURL := fmt.Sprintf("%s://%s/__raw__/%s", scheme, r.Host, urlPath)
	fd := FileData{
		Name: name, Size: info.Size(), Ext: ext,
		Parts: buildParts(urlPath), RawURL: rawURL,
		WgetCmd: "wget " + rawURL,
	}
	const maxPreview = 512 * 1024
	if isImageFile(ext) {
		data, err := os.ReadFile(fsPath)
		if err == nil {
			fd.IsImage = true
			fd.ImageMime = mime.TypeByExtension(ext)
			fd.ImageB64 = base64.StdEncoding.EncodeToString(data)
		}
	} else if isTextFile(ext) {
		if info.Size() > maxPreview {
			fd.IsTooBig = true
		} else {
			data, err := os.ReadFile(fsPath)
			if err == nil {
				fd.IsText = true
				fd.Content = template.HTML(template.HTMLEscapeString(string(data)))
			}
		}
	}
	tmpl := template.Must(template.New("file.html").Funcs(funcMap).ParseFS(templateFS, "templates/file.html"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, fd)
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

// ── /update  Admin Web UI ─────────────────────────────────────────────────────

func updateHandler(w http.ResponseWriter, r *http.Request) {
	// Logout
	if r.URL.Query().Get("logout") == "1" {
		clearSessionCookie(w)
		http.Redirect(w, r, "/update", http.StatusSeeOther)
		return
	}
	// Login POST
	if r.Method == http.MethodPost && (r.URL.Path == "/update" || r.URL.Path == "/update/") {
		r.ParseForm()
		u := r.FormValue("username")
		p := r.FormValue("password")
		if u == authUser && p == authPass {
			setSessionCookie(w, u, p)
			http.Redirect(w, r, "/update/", http.StatusSeeOther)
		} else {
			renderLogin(w, "用户名或密码错误")
		}
		return
	}
	if !checkSessionAuth(r) {
		renderLogin(w, "")
		return
	}
	// Browse
	urlPath := strings.TrimPrefix(r.URL.Path, "/update/")
	urlPath = strings.TrimPrefix(urlPath, "/update")
	fsPath, err := safePath(urlPath)
	if err != nil {
		http.Error(w, "Forbidden", 403)
		return
	}
	info, _ := os.Stat(fsPath)
	if info != nil && !info.IsDir() {
		serveRaw(w, r, fsPath)
		return
	}
	renderAdminDir(w, fsPath, urlPath)
}

func renderLogin(w http.ResponseWriter, errMsg string) {
	tmpl := template.Must(template.New("update_login.html").Funcs(funcMap).ParseFS(templateFS, "templates/update_login.html"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, map[string]string{"Error": errMsg})
}

func buildUpdateParts(urlPath string) []PathPart {
	parts := []PathPart{{Name: "root", Path: "/update/"}}
	if urlPath == "" {
		return parts
	}
	for _, seg := range strings.Split(urlPath, "/") {
		if seg == "" {
			continue
		}
		prev := parts[len(parts)-1].Path
		parts = append(parts, PathPart{Name: seg, Path: prev + seg + "/"})
	}
	return parts
}

func renderAdminDir(w http.ResponseWriter, fsPath, urlPath string) {
	entries, _ := os.ReadDir(fsPath)
	var files []FileInfo
	for _, e := range entries {
		info, _ := e.Info()
		fp := urlPath
		if fp != "" {
			fp += "/"
		}
		fp += e.Name()
		ext := ""
		if !e.IsDir() {
			ext = strings.ToLower(filepath.Ext(e.Name()))
		}
		files = append(files, FileInfo{
			Name: e.Name(), IsDir: e.IsDir(), Size: info.Size(),
			ModTime: info.ModTime(), Path: fp, Ext: ext,
		})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})
	data := map[string]interface{}{
		"Path":  urlPath,
		"Parts": buildUpdateParts(urlPath),
		"Files": files,
	}
	tmpl := template.Must(template.New("update.html").Funcs(funcMap).ParseFS(templateFS, "templates/update.html"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, data)
}

// ── /update_action ────────────────────────────────────────────────────────────

func updateActionHandler(w http.ResponseWriter, r *http.Request) {
	if !checkSessionAuth(r) {
		http.Error(w, "Unauthorized", 401)
		return
	}
	action := r.URL.Query().Get("action")
	dirParam := r.URL.Query().Get("dir")
	redirectTo := "/update/"
	if dirParam != "" {
		redirectTo = "/update/" + strings.Trim(dirParam, "/") + "/"
	}

	switch action {
	case "upload":
		r.ParseMultipartForm(512 << 20)
		dir := r.FormValue("dir")
		destDir, err := safePath(dir)
		if err != nil {
			http.Error(w, "Forbidden", 403)
			return
		}
		os.MkdirAll(destDir, 0755)
		for _, fhs := range r.MultipartForm.File {
			for _, fh := range fhs {
				if err := saveUploadedFile(fh, destDir); err != nil {
					http.Error(w, "Upload error: "+err.Error(), 500)
					return
				}
			}
		}
		http.Redirect(w, r, redirectTo, http.StatusSeeOther)

	case "delete":
		r.ParseForm()
		target := r.FormValue("path")
		fsPath, err := safePath(target)
		if err != nil {
			http.Error(w, "Forbidden", 403)
			return
		}
		os.RemoveAll(fsPath)
		http.Redirect(w, r, redirectTo, http.StatusSeeOther)

	case "mkdir":
		r.ParseForm()
		dir := r.FormValue("dir")
		name := r.FormValue("name")
		if name == "" || strings.ContainsAny(name, "/\\..") {
			http.Error(w, "Invalid name", 400)
			return
		}
		fsPath, err := safePath(filepath.Join(dir, name))
		if err != nil {
			http.Error(w, "Forbidden", 403)
			return
		}
		os.MkdirAll(fsPath, 0755)
		http.Redirect(w, r, redirectTo, http.StatusSeeOther)

	default:
		http.Error(w, "Unknown action", 400)
	}
}

func saveUploadedFile(fh *multipart.FileHeader, destDir string) error {
	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(filepath.Join(destDir, filepath.Base(fh.Filename)))
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}

// ── /api  REST API (Basic Auth) ───────────────────────────────────────────────

func apiHandler(w http.ResponseWriter, r *http.Request) {
	if !checkBasicAuth(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="FilesManager"`)
		http.Error(w, "Unauthorized", 401)
		return
	}
	sub := strings.TrimPrefix(r.URL.Path, "/api/")
	sub = strings.TrimSuffix(sub, "/")
	switch sub {
	case "list":
		apiList(w, r)
	case "download":
		apiDownload(w, r)
	case "upload":
		apiUpload(w, r)
	case "delete":
		apiDelete(w, r)
	case "mkdir":
		apiMkdir(w, r)
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, `FilesManager API  (Basic Auth: -u user:pass)

  GET  /api/list?path=<dir>        List directory (JSON)
  GET  /api/download?path=<file>   Download file
  POST /api/upload?path=<dir>      Upload file(s), field: file
  POST /api/delete?path=<path>     Delete file or directory
  POST /api/mkdir?path=<dir>       Create directory

Examples:
  curl -u user:pass https://host/api/list
  curl -u user:pass https://host/api/list?path=subdir
  curl -u user:pass https://host/api/download?path=foo/bar.txt -o bar.txt
  curl -u user:pass -F "file=@local.txt" https://host/api/upload?path=subdir
  curl -u user:pass -X POST https://host/api/delete?path=foo/bar.txt
  curl -u user:pass -X POST https://host/api/mkdir?path=newfolder
`)
	}
}

type APIFileInfo struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
	Path    string `json:"path"`
}

func apiList(w http.ResponseWriter, r *http.Request) {
	urlPath := r.URL.Query().Get("path")
	fsPath, err := safePath(urlPath)
	if err != nil {
		jsonError(w, "Forbidden", 403)
		return
	}
	entries, err := os.ReadDir(fsPath)
	if err != nil {
		jsonError(w, "Cannot read directory: "+err.Error(), 500)
		return
	}
	var list []APIFileInfo
	for _, e := range entries {
		info, _ := e.Info()
		p := urlPath
		if p != "" {
			p += "/"
		}
		p += e.Name()
		list = append(list, APIFileInfo{
			Name: e.Name(), IsDir: e.IsDir(), Size: info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"), Path: p,
		})
	}
	if list == nil {
		list = []APIFileInfo{}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].IsDir != list[j].IsDir {
			return list[i].IsDir
		}
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"path": urlPath, "files": list})
}

func apiDownload(w http.ResponseWriter, r *http.Request) {
	urlPath := r.URL.Query().Get("path")
	if urlPath == "" {
		jsonError(w, "path required", 400)
		return
	}
	fsPath, err := safePath(urlPath)
	if err != nil {
		jsonError(w, "Forbidden", 403)
		return
	}
	info, err := os.Stat(fsPath)
	if err != nil {
		jsonError(w, "Not found", 404)
		return
	}
	if info.IsDir() {
		jsonError(w, "Path is a directory", 400)
		return
	}
	serveRaw(w, r, fsPath)
}

func apiUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "POST required", 405)
		return
	}
	urlPath := r.URL.Query().Get("path")
	destDir, err := safePath(urlPath)
	if err != nil {
		jsonError(w, "Forbidden", 403)
		return
	}
	os.MkdirAll(destDir, 0755)
	r.ParseMultipartForm(512 << 20)
	var uploaded []string
	for _, fhs := range r.MultipartForm.File {
		for _, fh := range fhs {
			if err := saveUploadedFile(fh, destDir); err != nil {
				jsonError(w, "Upload error: "+err.Error(), 500)
				return
			}
			uploaded = append(uploaded, fh.Filename)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "uploaded": uploaded})
}

func apiDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		jsonError(w, "POST or DELETE required", 405)
		return
	}
	urlPath := r.URL.Query().Get("path")
	if urlPath == "" {
		jsonError(w, "path required", 400)
		return
	}
	fsPath, err := safePath(urlPath)
	if err != nil {
		jsonError(w, "Forbidden", 403)
		return
	}
	if _, err := os.Stat(fsPath); err != nil {
		jsonError(w, "Not found", 404)
		return
	}
	if err := os.RemoveAll(fsPath); err != nil {
		jsonError(w, "Delete error: "+err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "deleted": urlPath})
}

func apiMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "POST required", 405)
		return
	}
	urlPath := r.URL.Query().Get("path")
	if urlPath == "" {
		jsonError(w, "path required", 400)
		return
	}
	fsPath, err := safePath(urlPath)
	if err != nil {
		jsonError(w, "Forbidden", 403)
		return
	}
	if err := os.MkdirAll(fsPath, 0755); err != nil {
		jsonError(w, "Mkdir error: "+err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "path": urlPath})
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ── Misc helpers ──────────────────────────────────────────────────────────────

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
