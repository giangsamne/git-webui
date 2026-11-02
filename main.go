package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

//go:embed static/*
var staticFS embed.FS

const (
	DefaultPort = "18090"
	Version     = "v1.0.0"
)

func getConfigPath() string {
	if p := os.Getenv("GIT_WEBUI_CONFIG"); p != "" {
		return p
	}
	if _, err := os.Stat("config.json"); err == nil {
		return "config.json"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config", "git-webui", "config.json")
}

func getPort() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return DefaultPort
}

// Structures
type Repo struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Path              string `json:"path"`
	AutoBackupEnabled bool   `json:"auto_backup_enabled"`
	Mode              string `json:"mode"` // "both", "interval", "change"
	IntervalMinutes   int    `json:"interval_minutes"`
	AutoPush          bool   `json:"auto_push"`
	LastBackup        string `json:"last_backup"`
	LastStatus        string `json:"last_status"`
	LastError         string `json:"last_error,omitempty"`
}

type CommitInfo struct {
	Hash     string `json:"hash"`
	Author   string `json:"author"`
	Date     string `json:"date,omitempty"`
	TimeAgo  string `json:"time_ago"`
	Message  string `json:"message"`
}

type RepoWithGit struct {
	Repo
	IsGit            bool        `json:"is_git"`
	Branch           string      `json:"branch"`
	RemoteURL        string      `json:"remote_url"`
	UncommittedCount int         `json:"uncommitted_count"`
	StatusSummary    string      `json:"status_summary"`
	LastCommit       *CommitInfo `json:"last_commit"`
}

type LogEntry struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	RepoName  string `json:"repo_name"`
	Action    string `json:"action"`
	Message   string `json:"message"`
	Status    string `json:"status"` // "success" or "error"
}

type ConfigData struct {
	Repos []Repo     `json:"repos"`
	Logs  []LogEntry `json:"logs"`
}

type Engine struct {
	mu             sync.RWMutex
	repos          []Repo
	logs           []LogEntry
	watcher        *fsnotify.Watcher
	pendingChanges map[string]time.Time
	lastBackupTimes map[string]time.Time
	watchedPaths   map[string]bool
}

var engine = &Engine{
	pendingChanges:  make(map[string]time.Time),
	lastBackupTimes: make(map[string]time.Time),
	watchedPaths:    make(map[string]bool),
}

// Command execution helper
func runCmd(cwd string, timeout time.Duration, name string, args ...string) (bool, string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	outStr := strings.TrimSpace(stdout.String())
	errStr := strings.TrimSpace(stderr.String())

	if err != nil {
		if errStr == "" {
			errStr = err.Error()
		}
		return false, outStr, errStr
	}
	return true, outStr, errStr
}

// Git Helpers
func isGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}

func getGitInfo(path string) RepoWithGit {
	res := RepoWithGit{}
	if !isGitRepo(path) {
		res.IsGit = false
		res.StatusSummary = "Not a git repository"
		return res
	}
	res.IsGit = true

	// Branch
	ok, branch, _ := runCmd(path, 5*time.Second, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if ok && branch != "" {
		res.Branch = branch
	} else {
		res.Branch = "main"
	}

	// Remote URL
	ok, remote, _ := runCmd(path, 5*time.Second, "git", "config", "--get", "remote.origin.url")
	if ok {
		res.RemoteURL = remote
	}

	// Uncommitted count
	ok, statusOut, _ := runCmd(path, 10*time.Second, "git", "status", "--porcelain")
	if ok && statusOut != "" {
		lines := strings.Split(statusOut, "\n")
		count := 0
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				count++
			}
		}
		res.UncommittedCount = count
		res.StatusSummary = fmt.Sprintf("%d file(s) modified/untracked", count)
	} else {
		res.UncommittedCount = 0
		res.StatusSummary = "Clean (no changes)"
	}

	// Last commit
	ok, logOut, _ := runCmd(path, 5*time.Second, "git", "log", "-1", "--format=%h|%an|%ar|%s")
	if ok && logOut != "" {
		parts := strings.SplitN(logOut, "|", 4)
		if len(parts) == 4 {
			res.LastCommit = &CommitInfo{
				Hash:    parts[0],
				Author:  parts[1],
				TimeAgo: parts[2],
				Message: parts[3],
			}
		}
	}
	return res
}

func commitAndPush(path string, message string, autoPush bool) (bool, string) {
	if !isGitRepo(path) {
		return false, "Not a git repository"
	}

	ok, statusOut, _ := runCmd(path, 10*time.Second, "git", "status", "--porcelain")
	hasChanges := ok && strings.TrimSpace(statusOut) != ""

	if !hasChanges && !autoPush {
		return true, "No changes to commit"
	}

	if hasChanges {
		ok, _, err := runCmd(path, 30*time.Second, "git", "add", "-A")
		if !ok {
			return false, "git add failed: " + err
		}

		if message == "" {
			message = fmt.Sprintf("Auto-backup: %s", time.Now().Format("2006-01-02 15:04:05"))
		}

		ok, out, err := runCmd(path, 30*time.Second, "git", "commit", "-m", message)
		if !ok && !strings.Contains(err+out, "nothing to commit") {
			return false, "git commit failed: " + err
		}
	}

	if autoPush {
		ok, remote, _ := runCmd(path, 5*time.Second, "git", "config", "--get", "remote.origin.url")
		if ok && remote != "" {
			_, branch, _ := runCmd(path, 5*time.Second, "git", "rev-parse", "--abbrev-ref", "HEAD")
			if branch == "" {
				branch = "main"
			}
			ok, out, err := runCmd(path, 60*time.Second, "git", "push", "-u", "origin", branch)
			if !ok {
				return false, "git push failed: " + err + " " + out
			}
			return true, fmt.Sprintf("Committed & pushed to GitHub (%s)", branch)
		}
		return true, "Committed locally (no GitHub remote configured)"
	}

	return true, "Committed changes locally"
}

// Safe 1-Click Rollback
func rollbackCommit(path string, commitHash string) (bool, string) {
	if !isGitRepo(path) {
		return false, "Not a git repository"
	}

	// 1. Checkout files from specified commit into working tree
	ok, _, err := runCmd(path, 30*time.Second, "git", "checkout", commitHash, "--", ".")
	if !ok {
		return false, "git checkout failed: " + err
	}

	// 2. Clean untracked files
	runCmd(path, 20*time.Second, "git", "clean", "-fd")

	// 3. Stage everything
	runCmd(path, 20*time.Second, "git", "add", "-A")

	// 4. Create a clean rollback commit
	msg := fmt.Sprintf("Khôi phục về bản commit [%s] lúc %s", commitHash, time.Now().Format("2006-01-02 15:04:05"))
	ok, out, err := runCmd(path, 30*time.Second, "git", "commit", "-m", msg)
	if !ok && !strings.Contains(err+out, "nothing to commit") {
		return false, "git commit rollback failed: " + err
	}

	// 5. Push to GitHub if remote exists
	ok, remote, _ := runCmd(path, 5*time.Second, "git", "config", "--get", "remote.origin.url")
	if ok && remote != "" {
		_, branch, _ := runCmd(path, 5*time.Second, "git", "rev-parse", "--abbrev-ref", "HEAD")
		if branch == "" {
			branch = "main"
		}
		runCmd(path, 60*time.Second, "git", "push", "origin", branch)
	}

	return true, fmt.Sprintf("Đã khôi phục thành công về bản commit [%s]", commitHash)
}

// 1-Click Discard All Changes
func discardChanges(path string) (bool, string) {
	if !isGitRepo(path) {
		return false, "Not a git repository"
	}
	ok, _, err := runCmd(path, 20*time.Second, "git", "restore", ".")
	if !ok {
		return false, "git restore failed: " + err
	}
	runCmd(path, 20*time.Second, "git", "clean", "-fd")
	return true, "Đã hoàn tác toàn bộ thay đổi chưa lưu"
}

// Engine Implementation
func (e *Engine) loadConfig() {
	e.mu.Lock()
	defer e.mu.Unlock()

	cfgPath := getConfigPath()
	data, err := os.ReadFile(cfgPath)
	if err == nil {
		var cfg ConfigData
		if json.Unmarshal(data, &cfg) == nil {
			e.repos = cfg.Repos
			e.logs = cfg.Logs
		}
	}
}

func (e *Engine) saveConfig() {
	e.mu.Lock()
	defer e.mu.Unlock()

	cfg := ConfigData{
		Repos: e.repos,
		Logs:  e.logs,
	}
	if len(cfg.Logs) > 100 {
		cfg.Logs = cfg.Logs[:100]
	}

	cfgPath := getConfigPath()
	_ = os.MkdirAll(filepath.Dir(cfgPath), 0755)
	bytes, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(cfgPath, bytes, 0644)
}

func (e *Engine) addLog(repoName, action, message, status string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	entry := LogEntry{
		ID:        fmt.Sprintf("%x", time.Now().UnixNano())[:8],
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		RepoName:  repoName,
		Action:    action,
		Message:   message,
		Status:    status,
	}
	e.logs = append([]LogEntry{entry}, e.logs...)
	if len(e.logs) > 200 {
		e.logs = e.logs[:200]
	}
}

func (e *Engine) updateWatcher() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.watcher != nil {
		_ = e.watcher.Close()
	}
	var err error
	e.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Watcher init error: %v", err)
		return
	}
	e.watchedPaths = make(map[string]bool)

	for _, r := range e.repos {
		if !r.AutoBackupEnabled || (r.Mode != "change" && r.Mode != "both") {
			continue
		}
		// Walk and add directories
		_ = filepath.Walk(r.Path, func(path string, info os.FileInfo, err error) error {
			if err != nil || !info.IsDir() {
				return nil
			}
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "__pycache__" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if !e.watchedPaths[path] {
				_ = e.watcher.Add(path)
				e.watchedPaths[path] = true
			}
			return nil
		})
	}
}

func (e *Engine) start() {
	e.loadConfig()
	e.updateWatcher()

	// Inotify event loop
	go func() {
		for {
			if e.watcher == nil {
				time.Sleep(2 * time.Second)
				continue
			}
			select {
			case event, ok := <-e.watcher.Events:
				if !ok {
					time.Sleep(time.Second)
					continue
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) {
					p := event.Name
					if strings.Contains(p, "/.git/") || strings.Contains(p, "\\.git\\") || strings.HasSuffix(p, ".swp") || strings.HasSuffix(p, "~") {
						continue
					}
					// Find which repo this belongs to
					e.mu.Lock()
					for _, r := range e.repos {
						if strings.HasPrefix(p, r.Path) {
							e.pendingChanges[r.ID] = time.Now()
							break
						}
					}
					e.mu.Unlock()
				}
			case err, ok := <-e.watcher.Errors:
				if ok && err != nil {
					log.Printf("Watcher error: %v", err)
				}
			}
		}
	}()

	// Scheduled & Debounce Loop
	go func() {
		for {
			time.Sleep(5 * time.Second)
			e.mu.RLock()
			reposCopy := make([]Repo, len(e.repos))
			copy(reposCopy, e.repos)
			e.mu.RUnlock()

			now := time.Now()
			for _, r := range reposCopy {
				if !r.AutoBackupEnabled {
					continue
				}

				shouldBackup := false
				triggerType := "auto"

				// 1. Check debounce for file change
				if r.Mode == "change" || r.Mode == "both" {
					e.mu.Lock()
					if lastEdit, ok := e.pendingChanges[r.ID]; ok {
						if now.Sub(lastEdit) >= 15*time.Second {
							delete(e.pendingChanges, r.ID)
							shouldBackup = true
							triggerType = "file-change"
						}
					}
					e.mu.Unlock()
				}

				// 2. Check interval
				if !shouldBackup && (r.Mode == "interval" || r.Mode == "both") {
					e.mu.Lock()
					lastBackup := e.lastBackupTimes[r.ID]
					interval := time.Duration(r.IntervalMinutes) * time.Minute
					if interval < time.Minute {
						interval = time.Minute
					}
					if now.Sub(lastBackup) >= interval {
						shouldBackup = true
						triggerType = "schedule"
					}
					e.mu.Unlock()
				}

				if shouldBackup {
					go func(repo Repo, trig string) {
						success, msg := commitAndPush(repo.Path, "", repo.AutoPush)
						status := "success"
						if !success {
							status = "error"
						}

						e.mu.Lock()
						for i := range e.repos {
							if e.repos[i].ID == repo.ID {
								e.repos[i].LastBackup = time.Now().Format("2006-01-02 15:04:05")
								if success {
									e.repos[i].LastStatus = "Success"
									e.repos[i].LastError = ""
								} else {
									e.repos[i].LastStatus = "Error"
									e.repos[i].LastError = msg
								}
								break
							}
						}
						e.lastBackupTimes[repo.ID] = time.Now()
						e.mu.Unlock()

						if msg != "No changes to commit" {
							e.addLog(repo.Name, trig, msg, status)
							e.saveConfig()
						}
					}(r, triggerType)
				}
			}
		}
	}()
}

// HTTP API Handlers
func jsonResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	ok, ghUserOut, _ := runCmd("", 5*time.Second, "gh", "api", "user", "--jq", "{login: .login, name: .name, email: .email}")
	var ghUser map[string]interface{}
	if ok && ghUserOut != "" {
		_ = json.Unmarshal([]byte(ghUserOut), &ghUser)
	}

	engine.mu.RLock()
	totalRepos := len(engine.repos)
	watchers := len(engine.watchedPaths)
	engine.mu.RUnlock()

	jsonResponse(w, map[string]interface{}{
		"status":           "online",
		"version":          Version,
		"engine":           fmt.Sprintf("Go Native (%s/%s)", runtime.GOOS, runtime.GOARCH),
		"github_connected": ghUser != nil,
		"github_user":      ghUser,
		"active_watchers":  watchers,
		"total_repos":      totalRepos,
	}, http.StatusOK)
}

func handleRepos(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		engine.mu.RLock()
		reposCopy := make([]Repo, len(engine.repos))
		copy(reposCopy, engine.repos)
		engine.mu.RUnlock()

		result := make([]RepoWithGit, 0, len(reposCopy))
		for _, rep := range reposCopy {
			gw := getGitInfo(rep.Path)
			gw.Repo = rep
			result = append(result, gw)
		}
		jsonResponse(w, result, http.StatusOK)
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			Path            string `json:"path"`
			Name            string `json:"name"`
			InitGit         bool   `json:"init_git"`
			CreateGithub    bool   `json:"create_github"`
			IsPrivate       bool   `json:"is_private"`
			IntervalMinutes int    `json:"interval_minutes"`
			Mode            string `json:"mode"`
			AutoBackup      bool   `json:"auto_backup"`
			AutoPush        bool   `json:"auto_push"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResponse(w, map[string]string{"error": "Invalid request body"}, http.StatusBadRequest)
			return
		}

		path := filepath.Clean(os.ExpandEnv(req.Path))
		if path == "" {
			jsonResponse(w, map[string]string{"error": "Path cannot be empty"}, http.StatusBadRequest)
			return
		}

		engine.mu.RLock()
		for _, ex := range engine.repos {
			if ex.Path == path {
				engine.mu.RUnlock()
				jsonResponse(w, map[string]string{"error": "Repository already exists"}, http.StatusBadRequest)
				return
			}
		}
		engine.mu.RUnlock()

		name := req.Name
		if name == "" {
			name = filepath.Base(path)
		}

		_ = os.MkdirAll(path, 0755)

		if req.InitGit && !isGitRepo(path) {
			runCmd(path, 10*time.Second, "git", "init")
			runCmd(path, 5*time.Second, "git", "branch", "-M", "main")
			gitignorePath := filepath.Join(path, ".gitignore")
			if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
				_ = os.WriteFile(gitignorePath, []byte("# GitWebUI\n__pycache__/\n*.swp\n.DS_Store\nnode_modules/\n"), 0644)
				runCmd(path, 5*time.Second, "git", "add", ".gitignore")
				runCmd(path, 10*time.Second, "git", "commit", "-m", "Initial commit with .gitignore")
			}
		}

		if req.CreateGithub {
			vis := "--private"
			if !req.IsPrivate {
				vis = "--public"
			}
			runCmd(path, 60*time.Second, "gh", "repo", "create", name, vis, "--source=.", "--remote=origin", "--push")
		}

		newRepo := Repo{
			ID:                fmt.Sprintf("%x", time.Now().UnixNano())[:8],
			Name:              name,
			Path:              path,
			AutoBackupEnabled: req.AutoBackup,
			Mode:              req.Mode,
			IntervalMinutes:   req.IntervalMinutes,
			AutoPush:          req.AutoPush,
			LastBackup:        "Never",
			LastStatus:        "Idle",
		}

		engine.mu.Lock()
		engine.repos = append(engine.repos, newRepo)
		engine.mu.Unlock()

		engine.saveConfig()
		engine.updateWatcher()
		engine.addLog(name, "project-added", "Project added: "+path, "success")

		jsonResponse(w, map[string]interface{}{"success": true, "repo": newRepo}, http.StatusOK)
		return
	}
}

func handleRepoOps(w http.ResponseWriter, r *http.Request) {
	// Pattern: /api/repos/{id}/{action}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	repoID := parts[2]
	action := ""
	if len(parts) >= 4 {
		action = parts[3]
	}

	engine.mu.RLock()
	var currentRepo *Repo
	for _, rp := range engine.repos {
		if rp.ID == repoID {
			repCopy := rp
			currentRepo = &repCopy
			break
		}
	}
	engine.mu.RUnlock()

	if currentRepo == nil {
		jsonResponse(w, map[string]string{"error": "Repo not found"}, http.StatusNotFound)
		return
	}

	// 1. DELETE /api/repos/{id}
	if r.Method == http.MethodDelete && action == "" {
		engine.mu.Lock()
		newRepos := make([]Repo, 0, len(engine.repos))
		for _, rp := range engine.repos {
			if rp.ID != repoID {
				newRepos = append(newRepos, rp)
			}
		}
		engine.repos = newRepos
		delete(engine.pendingChanges, repoID)
		delete(engine.lastBackupTimes, repoID)
		engine.mu.Unlock()

		engine.saveConfig()
		engine.updateWatcher()
		engine.addLog(currentRepo.Name, "project-removed", "Removed from GitWebUI", "success")
		jsonResponse(w, map[string]bool{"success": true}, http.StatusOK)
		return
	}

	// 2. PUT /api/repos/{id}/settings
	if r.Method == http.MethodPut && action == "settings" {
		var req struct {
			AutoBackupEnabled bool   `json:"auto_backup_enabled"`
			Mode              string `json:"mode"`
			IntervalMinutes   int    `json:"interval_minutes"`
			AutoPush          bool   `json:"auto_push"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResponse(w, map[string]string{"error": "Invalid body"}, http.StatusBadRequest)
			return
		}
		engine.mu.Lock()
		for i := range engine.repos {
			if engine.repos[i].ID == repoID {
				engine.repos[i].AutoBackupEnabled = req.AutoBackupEnabled
				engine.repos[i].Mode = req.Mode
				engine.repos[i].IntervalMinutes = req.IntervalMinutes
				engine.repos[i].AutoPush = req.AutoPush
				break
			}
		}
		engine.mu.Unlock()
		engine.saveConfig()
		engine.updateWatcher()
		jsonResponse(w, map[string]bool{"success": true}, http.StatusOK)
		return
	}

	// 3. POST /api/repos/{id}/backup
	if r.Method == http.MethodPost && action == "backup" {
		var req struct {
			Message      string `json:"message"`
			PushToRemote bool   `json:"push_to_remote"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		success, msg := commitAndPush(currentRepo.Path, req.Message, true)
		status := "success"
		if !success {
			status = "error"
		}
		engine.addLog(currentRepo.Name, "manual-backup", msg, status)
		jsonResponse(w, map[string]interface{}{"success": success, "message": msg}, http.StatusOK)
		return
	}

	// 4. POST /api/repos/{id}/pull
	if r.Method == http.MethodPost && action == "pull" {
		ok, out, err := runCmd(currentRepo.Path, 45*time.Second, "git", "pull")
		msg := out
		if !ok {
			msg = err
		}
		status := "success"
		if !ok {
			status = "error"
		}
		engine.addLog(currentRepo.Name, "pull", msg, status)
		jsonResponse(w, map[string]interface{}{"success": ok, "message": msg}, http.StatusOK)
		return
	}

	// 5. POST /api/repos/{id}/rollback (1-CLICK ROLLBACK)
	if r.Method == http.MethodPost && action == "rollback" {
		var req struct {
			CommitHash string `json:"commit_hash"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CommitHash == "" {
			jsonResponse(w, map[string]string{"error": "Invalid commit_hash"}, http.StatusBadRequest)
			return
		}
		success, msg := rollbackCommit(currentRepo.Path, req.CommitHash)
		status := "success"
		if !success {
			status = "error"
		}
		engine.addLog(currentRepo.Name, "rollback", msg, status)
		engine.saveConfig()
		jsonResponse(w, map[string]interface{}{"success": success, "message": msg}, http.StatusOK)
		return
	}

	// 6. POST /api/repos/{id}/discard (1-CLICK DISCARD)
	if r.Method == http.MethodPost && action == "discard" {
		success, msg := discardChanges(currentRepo.Path)
		status := "success"
		if !success {
			status = "error"
		}
		engine.addLog(currentRepo.Name, "discard", msg, status)
		jsonResponse(w, map[string]interface{}{"success": success, "message": msg}, http.StatusOK)
		return
	}

	// 7. GET /api/repos/{id}/status_details
	if r.Method == http.MethodGet && action == "status_details" {
		ok, statusOut, _ := runCmd(currentRepo.Path, 10*time.Second, "git", "status", "--porcelain")
		type FileItem struct {
			Status string `json:"status"`
			File   string `json:"file"`
		}
		files := []FileItem{}
		if ok && statusOut != "" {
			for _, l := range strings.Split(statusOut, "\n") {
				if len(l) >= 3 {
					files = append(files, FileItem{
						Status: strings.TrimSpace(l[:2]),
						File:   strings.TrimSpace(l[3:]),
					})
				}
			}
		}
		_, diffOut, _ := runCmd(currentRepo.Path, 10*time.Second, "git", "diff", "HEAD")
		if diffOut == "" {
			_, diffOut, _ = runCmd(currentRepo.Path, 10*time.Second, "git", "diff")
		}
		if len(diffOut) > 15000 {
			diffOut = diffOut[:15000] + "\n...[truncated]"
		}
		jsonResponse(w, map[string]interface{}{"files": files, "diff": diffOut}, http.StatusOK)
		return
	}

	// 8. GET /api/repos/{id}/commits
	if r.Method == http.MethodGet && action == "commits" {
		ok, logOut, _ := runCmd(currentRepo.Path, 10*time.Second, "git", "log", "-25", "--format=%h|%an|%ad|%ar|%s", "--date=iso")
		commits := []CommitInfo{}
		if ok && logOut != "" {
			for _, l := range strings.Split(logOut, "\n") {
				p := strings.Split(l, "|")
				if len(p) >= 5 {
					commits = append(commits, CommitInfo{
						Hash:    p[0],
						Author:  p[1],
						Date:    p[2],
						TimeAgo: p[3],
						Message: strings.Join(p[4:], "|"),
					})
				}
			}
		}
		jsonResponse(w, commits, http.StatusOK)
		return
	}
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	jsonResponse(w, engine.logs, http.StatusOK)
}

func handleBrowse(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("path")
	if target == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			target = home
		} else {
			target = "."
		}
	}
	target = filepath.Clean(os.ExpandEnv(target))

	type DirItem struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsGit bool   `json:"is_git"`
	}

	entries, err := os.ReadDir(target)
	items := []DirItem{}
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() || (strings.HasPrefix(e.Name(), ".") && e.Name() != ".git") {
				continue
			}
			p := filepath.Join(target, e.Name())
			items = append(items, DirItem{
				Name:  e.Name(),
				Path:  p,
				IsGit: isGitRepo(p),
			})
		}
	}

	parent := filepath.Dir(target)
	jsonResponse(w, map[string]interface{}{
		"current_path": target,
		"parent_path":  parent,
		"items":        items,
	}, http.StatusOK)
}

func main() {
	engine.start()

	mux := http.NewServeMux()

	// API Routes
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/repos", handleRepos)
	mux.HandleFunc("/api/repos/", handleRepoOps)
	mux.HandleFunc("/api/logs", handleLogs)
	mux.HandleFunc("/api/browse", handleBrowse)

	// Static UI Server (embedded)
	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("SubFS error: %v", err)
	}
	fileServer := http.FileServer(http.FS(subFS))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			data, err := subFS.Open("index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer data.Close()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.Copy(w, data)
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	port := getPort()
	addr := "0.0.0.0:" + port
	log.Printf("🚀 GitWebUI %s (Go Native) listening on http://%s", Version, addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
