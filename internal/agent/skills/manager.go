package skills

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Tencent/WeKnora/internal/sandbox"
)

// Manager manages skills lifecycle including discovery, loading, and script execution
// It coordinates between the Loader (filesystem operations) and Sandbox (script execution)
type Manager struct {
	loader     *Loader
	sandboxMgr sandbox.Manager

	// Configuration
	skillDirs     []string
	allowedSkills []string // Empty means all skills are allowed；元素可为「库名/skill名」或旧格式裸名
	enabled       bool

	// 运行时消歧映射
	libToDir   map[string]string // 库名 -> 库目录路径
	bareToLib  map[string]string // skill 裸名 -> 库名（用于按库精确解析）
	allowedBare map[string]bool   // selected 模式下允许的裸名集合（all 模式为空，表示全部允许）

	// Cache
	metadataCache []*SkillMetadata
	mu            sync.RWMutex
}

// ManagerConfig holds configuration for the skill manager
type ManagerConfig struct {
	SkillDirs     []string // Directories to search for skills
	AllowedSkills []string // Skill names whitelist (empty = allow all)；元素可为「库名/skill名」或裸名
	Enabled       bool     // Whether skills are enabled
}

// NewManager creates a new skill manager with the given configuration
func NewManager(config *ManagerConfig, sandboxMgr sandbox.Manager) *Manager {
	if config == nil {
		config = &ManagerConfig{
			Enabled: false,
		}
	}

	m := &Manager{
		loader:        NewLoader(config.SkillDirs),
		sandboxMgr:    sandboxMgr,
		skillDirs:     config.SkillDirs,
		allowedSkills: config.AllowedSkills,
		enabled:       config.Enabled,
		libToDir:      make(map[string]string),
		bareToLib:     make(map[string]string),
		allowedBare:   make(map[string]bool),
	}
	// 库名取库目录的 base 名（skillsRoot 下的一级子目录名）
	for _, d := range config.SkillDirs {
		m.libToDir[filepath.Base(d)] = d
	}
	return m
}

// IsEnabled returns whether skills are enabled
func (m *Manager) IsEnabled() bool {
	return m.enabled
}

// Initialize discovers all skills and caches their metadata
// This should be called at startup
func (m *Manager) Initialize(ctx context.Context) error {
	if !m.enabled {
		return nil
	}

	metadata, err := m.loader.DiscoverSkills()
	if err != nil {
		return fmt.Errorf("failed to discover skills: %w", err)
	}

	// Filter by allowed skills if specified
	if len(m.allowedSkills) > 0 {
		metadata = m.filterAllowedSkills(metadata)
	}

	m.mu.Lock()
	m.metadataCache = metadata
	m.buildMappingsLocked()
	m.mu.Unlock()

	return nil
}

// buildMappingsLocked 在持有写锁时构建运行时消歧映射：
//   - allowedBare：selected 模式下允许的裸名集合（all 模式为空）
//   - bareToLib：裸名 -> 库名，用于按库精确解析（从已过滤的 metadataCache 构建）
// 必须在 metadataCache 更新后调用。
func (m *Manager) buildMappingsLocked() {
	m.allowedBare = make(map[string]bool)
	m.bareToLib = make(map[string]string)
	for _, a := range m.allowedSkills {
		bare := a
		if idx := strings.Index(a, "/"); idx >= 0 {
			bare = a[idx+1:]
		}
		m.allowedBare[bare] = true
	}
	for _, meta := range m.metadataCache {
		m.bareToLib[meta.Name] = meta.Library
	}
}

// libDirFor 按裸名查其所属库目录路径。返回 ok=false 时表示无库映射，调用方应回退到按裸名解析。
func (m *Manager) libDirFor(bareName string) (string, bool) {
	lib, ok := m.bareToLib[bareName]
	if !ok || lib == "" {
		return "", false
	}
	dir, ok := m.libToDir[lib]
	if !ok {
		return "", false
	}
	return dir, true
}

// filterAllowedSkills filters metadata to only include allowed skills.
// allowedSkills 元素可为「库名/skill名」或旧格式裸名；裸名匹配任一库的同名 skill（向后兼容）。
func (m *Manager) filterAllowedSkills(metadata []*SkillMetadata) []*SkillMetadata {
	if len(m.allowedSkills) == 0 {
		return metadata
	}

	allowedKeys := make(map[string]bool)  // "library/name"
	bareAllowed := make(map[string]bool)  // 旧格式裸名
	for _, a := range m.allowedSkills {
		if strings.Contains(a, "/") {
			allowedKeys[a] = true
		} else {
			bareAllowed[a] = true
		}
	}

	var filtered []*SkillMetadata
	for _, meta := range metadata {
		if allowedKeys[meta.Library+"/"+meta.Name] || bareAllowed[meta.Name] {
			filtered = append(filtered, meta)
		}
	}
	return filtered
}

// GetAllMetadata returns metadata for all discovered skills
// This is used for system prompt injection (Level 1)
func (m *Manager) GetAllMetadata() []*SkillMetadata {
	if !m.enabled {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make([]*SkillMetadata, len(m.metadataCache))
	copy(result, m.metadataCache)
	return result
}

// LoadSkill loads the full instructions of a skill (Level 2)
func (m *Manager) LoadSkill(ctx context.Context, skillName string) (*Skill, error) {
	if !m.enabled {
		return nil, fmt.Errorf("skills are not enabled")
	}

	// Check if skill is allowed
	if !m.isSkillAllowed(skillName) {
		return nil, fmt.Errorf("skill not allowed: %s", skillName)
	}

	m.mu.RLock()
	dir, ok := m.libDirFor(skillName)
	m.mu.RUnlock()
	if ok {
		return m.loader.LoadSkillFromDir(dir, skillName)
	}
	return m.loader.LoadSkillInstructions(skillName)
}

// isSkillAllowed checks if a skill is in the allowed list (by bare name)
func (m *Manager) isSkillAllowed(skillName string) bool {
	if len(m.allowedSkills) == 0 {
		return true
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.allowedBare[skillName]
}

// ReadSkillFile reads an additional file from a skill directory (Level 3)
func (m *Manager) ReadSkillFile(ctx context.Context, skillName, filePath string) (string, error) {
	if !m.enabled {
		return "", fmt.Errorf("skills are not enabled")
	}

	if !m.isSkillAllowed(skillName) {
		return "", fmt.Errorf("skill not allowed: %s", skillName)
	}

	m.mu.RLock()
	dir, ok := m.libDirFor(skillName)
	m.mu.RUnlock()
	var file *SkillFile
	var err error
	if ok {
		file, err = m.loader.LoadSkillFileFromDir(dir, skillName, filePath)
	} else {
		file, err = m.loader.LoadSkillFile(skillName, filePath)
	}
	if err != nil {
		return "", err
	}

	return file.Content, nil
}

// ListSkillFiles lists all files in a skill directory
func (m *Manager) ListSkillFiles(ctx context.Context, skillName string) ([]string, error) {
	if !m.enabled {
		return nil, fmt.Errorf("skills are not enabled")
	}

	if !m.isSkillAllowed(skillName) {
		return nil, fmt.Errorf("skill not allowed: %s", skillName)
	}

	m.mu.RLock()
	dir, ok := m.libDirFor(skillName)
	m.mu.RUnlock()
	if ok {
		return m.loader.ListSkillFilesFromDir(dir, skillName)
	}
	return m.loader.ListSkillFiles(skillName)
}

// ExecuteScript executes a script from a skill in the sandbox
func (m *Manager) ExecuteScript(ctx context.Context, skillName, scriptPath string, args []string, stdin string) (*sandbox.ExecuteResult, error) {
	if !m.enabled {
		return nil, fmt.Errorf("skills are not enabled")
	}

	if !m.isSkillAllowed(skillName) {
		return nil, fmt.Errorf("skill not allowed: %s", skillName)
	}

	// Verify sandbox manager is available
	if m.sandboxMgr == nil {
		return nil, fmt.Errorf("sandbox is not configured")
	}

	// 按库精确解析 skill 基础路径与脚本文件
	m.mu.RLock()
	dir, ok := m.libDirFor(skillName)
	m.mu.RUnlock()

	var basePath string
	var file *SkillFile
	if ok {
		skill, err := m.loader.LoadSkillFromDir(dir, skillName)
		if err != nil {
			return nil, err
		}
		basePath = skill.BasePath
		file, err = m.loader.LoadSkillFileFromDir(dir, skillName, scriptPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load script: %w", err)
		}
	} else {
		var err error
		basePath, err = m.loader.GetSkillBasePath(skillName)
		if err != nil {
			return nil, err
		}
		file, err = m.loader.LoadSkillFile(skillName, scriptPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load script: %w", err)
		}
	}

	if !file.IsScript {
		return nil, fmt.Errorf("file is not an executable script: %s", scriptPath)
	}

	// Prepare execution config
	config := &sandbox.ExecuteConfig{
		Script:  file.Path,
		Args:    args,
		WorkDir: basePath,
		Stdin:   stdin,
	}

	// Execute in sandbox
	return m.sandboxMgr.Execute(ctx, config)
}

// GetSkillInfo returns detailed information about a skill
func (m *Manager) GetSkillInfo(ctx context.Context, skillName string) (*SkillInfo, error) {
	if !m.enabled {
		return nil, fmt.Errorf("skills are not enabled")
	}

	if !m.isSkillAllowed(skillName) {
		return nil, fmt.Errorf("skill not allowed: %s", skillName)
	}

	m.mu.RLock()
	dir, ok := m.libDirFor(skillName)
	m.mu.RUnlock()

	var skill *Skill
	var files []string
	if ok {
		var err error
		skill, err = m.loader.LoadSkillFromDir(dir, skillName)
		if err != nil {
			return nil, err
		}
		files, err = m.loader.ListSkillFilesFromDir(dir, skillName)
		if err != nil {
			files = []string{} // Non-fatal error
		}
	} else {
		var err error
		skill, err = m.loader.LoadSkillInstructions(skillName)
		if err != nil {
			return nil, err
		}
		files, err = m.loader.ListSkillFiles(skillName)
		if err != nil {
			files = []string{} // Non-fatal error
		}
	}

	return &SkillInfo{
		Name:         skill.Name,
		Description:  skill.Description,
		BasePath:     skill.BasePath,
		Instructions: skill.Instructions,
		Files:        files,
	}, nil
}

// SkillInfo provides detailed information about a skill
type SkillInfo struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	BasePath     string   `json:"base_path"`
	Instructions string   `json:"instructions"`
	Files        []string `json:"files"`
}

// Reload refreshes the skill cache by rediscovering all skills
func (m *Manager) Reload(ctx context.Context) error {
	if !m.enabled {
		return nil
	}

	metadata, err := m.loader.Reload()
	if err != nil {
		return err
	}

	if len(m.allowedSkills) > 0 {
		metadata = m.filterAllowedSkills(metadata)
	}

	m.mu.Lock()
	m.metadataCache = metadata
	m.buildMappingsLocked()
	m.mu.Unlock()

	return nil
}

// Cleanup releases resources
func (m *Manager) Cleanup(ctx context.Context) error {
	if m.sandboxMgr != nil {
		return m.sandboxMgr.Cleanup(ctx)
	}
	return nil
}
