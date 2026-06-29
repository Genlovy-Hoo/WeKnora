package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Tencent/WeKnora/internal/agent/skills"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// DefaultSkillsRoot is the default skills root directory.
// 容器内 cwd=/app，相对路径 "skills" 解析为 /app/skills。
const DefaultSkillsRoot = "skills"

// skillService implements SkillService interface
type skillService struct {
	loader      *skills.Loader
	skillsRoot  string
	skillDirs   []string // 所有 Skill 库目录（root/<library>），供 loader 与 agent 侧共用
	mu          sync.RWMutex
	initialized bool
}

// NewSkillService creates a new skill service
func NewSkillService() interfaces.SkillService {
	return &skillService{
		skillsRoot:  getSkillsRoot(),
		initialized: false,
	}
}

// getSkillsRoot returns the skills root directory.
// 优先级：WEKNORA_SKILLS_ROOT 环境变量 > 默认 "skills"（容器内即 /app/skills）。
// 根目录下每个一级子目录是一个「Skill 库(library)」，库下放 skill 子目录。
func getSkillsRoot() string {
	if r := os.Getenv("WEKNORA_SKILLS_ROOT"); strings.TrimSpace(r) != "" {
		return strings.TrimSpace(r)
	}
	return DefaultSkillsRoot
}

// getSkillDirs 返回根目录下所有「Skill 库」目录路径（每个至少含一个 skill），按名称排序。
// 供 skill service 的 loader 与 agent 侧（session_agent_qa）共用，确保两边一致。
func getSkillDirs() []string {
	return resolveLibraryDirs(getSkillsRoot())
}

// resolveLibraryDirs 枚举 root 下的所有一级子目录（每个即一个 Skill 库，含空库），按名称排序。
func resolveLibraryDirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dirs = append(dirs, filepath.Join(root, e.Name()))
	}
	sort.Strings(dirs)
	return dirs
}

// libraryNameRe 限定 Skill 库名：以字母或数字开头，仅含字母数字/下划线/点/连字符，长度 1-64。
var libraryNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// validLibraryName 校验 Skill 库名是否合法（同时排除路径穿越风险）。
func validLibraryName(name string) bool {
	name = strings.TrimSpace(name)
	if !libraryNameRe.MatchString(name) {
		return false
	}
	if strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
		return false
	}
	return true
}

// readLibraryDescription 读取库目录下的 library.json 描述字段；不存在或解析失败时返回空串。
func readLibraryDescription(libDir string) string {
	b, err := os.ReadFile(filepath.Join(libDir, skills.LibraryMetaFileName))
	if err != nil {
		return ""
	}
	var meta skills.LibraryInfo
	if err := json.Unmarshal(b, &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.Description)
}

// libraryOf 根据 skill 的 BasePath 相对 root 推导其所属 Skill 库名。
func libraryOf(root, basePath string) string {
	rel, err := filepath.Rel(root, basePath)
	if err != nil {
		return ""
	}
	parts := strings.SplitN(filepath.ToSlash(rel), "/", 2)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// ensureInitialized initializes the loader if not already done
func (s *skillService) ensureInitialized(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.initialized {
		return nil
	}

	if _, err := os.Stat(s.skillsRoot); os.IsNotExist(err) {
		logger.Warnf(ctx, "Skills root does not exist: %s", s.skillsRoot)
		if err := os.MkdirAll(s.skillsRoot, 0755); err != nil {
			logger.Warnf(ctx, "Failed to create skills root: %v", err)
		}
	}

	s.skillDirs = resolveLibraryDirs(s.skillsRoot)
	s.loader = skills.NewLoader(s.skillDirs)
	s.initialized = true

	logger.Infof(ctx, "Skill service initialized: root=%s libraries=%d dirs=%v", s.skillsRoot, len(s.skillDirs), s.skillDirs)

	return nil
}

// ListPreloadedSkills returns metadata for all skills across all libraries.
// 注意：跨库重名的 skill 在各库列表中独立保留（管理页按库展示，不应互相影响）。
// 跨库重名的运行时去重由 agent 侧的 skills.Manager/Loader 负责，不在此处处理。
func (s *skillService) ListPreloadedSkills(ctx context.Context) ([]*skills.SkillMetadata, error) {
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize skill service: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	metadata, err := s.loader.DiscoverSkills()
	if err != nil {
		logger.Errorf(ctx, "Failed to discover skills: %v", err)
		return nil, fmt.Errorf("failed to discover skills: %w", err)
	}

	// 按各 skill 所在库打标，保留全部（含跨库重名），由前端按库分组展示
	for _, m := range metadata {
		m.Library = libraryOf(s.skillsRoot, m.BasePath)
	}

	logger.Infof(ctx, "Discovered %d skills across %d libraries", len(metadata), len(s.skillDirs))

	return metadata, nil
}

// GetSkillByName retrieves a skill by its name
// resolveLibraryDir 校验库名并返回其在 skillsRoot 下的绝对路径，防止路径穿越。
func (s *skillService) resolveLibraryDir(library string) (string, error) {
	if !validLibraryName(library) {
		return "", fmt.Errorf("invalid library name: %s", library)
	}
	dir := filepath.Join(s.skillsRoot, library)
	absRoot, err := filepath.Abs(s.skillsRoot)
	if err != nil {
		return "", err
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absDir, absRoot+string(filepath.Separator)) && absDir != absRoot {
		return "", fmt.Errorf("library path outside skills root: %s", library)
	}
	return dir, nil
}

// GetSkillByName 按「库 + 名字」精确解析 skill，避免跨库重名歧义。
func (s *skillService) GetSkillByName(ctx context.Context, library string, name string) (*skills.Skill, error) {
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize skill service: %w", err)
	}

	libDir, err := s.resolveLibraryDir(library)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	skill, err := s.loader.LoadSkillFromDir(libDir, name)
	if err != nil {
		logger.Errorf(ctx, "Failed to load skill %s in library %s: %v", name, library, err)
		return nil, fmt.Errorf("failed to load skill: %w", err)
	}

	return skill, nil
}

// ListSkillFiles 返回指定库下指定 skill 目录内所有文件的相对路径。
func (s *skillService) ListSkillFiles(ctx context.Context, library string, name string) ([]string, error) {
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize skill service: %w", err)
	}

	libDir, err := s.resolveLibraryDir(library)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	files, err := s.loader.ListSkillFilesFromDir(libDir, name)
	if err != nil {
		logger.Errorf(ctx, "Failed to list files for skill %s in library %s: %v", name, library, err)
		return nil, fmt.Errorf("failed to list skill files: %w", err)
	}
	return files, nil
}

// GetSkillFile 返回指定库下指定 skill 内某个相对路径文件的内容。
func (s *skillService) GetSkillFile(ctx context.Context, library string, name string, relPath string) (*skills.SkillFile, error) {
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize skill service: %w", err)
	}

	libDir, err := s.resolveLibraryDir(library)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	file, err := s.loader.LoadSkillFileFromDir(libDir, name, relPath)
	if err != nil {
		logger.Errorf(ctx, "Failed to load file %s for skill %s in library %s: %v", relPath, name, library, err)
		return nil, fmt.Errorf("failed to load skill file: %w", err)
	}
	return file, nil
}

// ListSkillLibraries 返回所有 Skill 库的元信息（名称 + 描述），包含空库，按名称排序。
func (s *skillService) ListSkillLibraries(ctx context.Context) ([]*skills.LibraryInfo, error) {
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize skill service: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*skills.LibraryInfo, 0, len(s.skillDirs))
	for _, d := range s.skillDirs {
		out = append(out, &skills.LibraryInfo{
			Name:        filepath.Base(d),
			Description: readLibraryDescription(d),
		})
	}
	return out, nil
}

// CreateSkillLibrary 在 skills root 下创建一个新的 Skill 库目录并写入 library.json。
// 库名必须合法且不与已有库重名。
func (s *skillService) CreateSkillLibrary(ctx context.Context, name string, description string) error {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if !validLibraryName(name) {
		return fmt.Errorf("invalid library name: %q (允许字母数字开头，仅含字母数字/下划线/点/连字符，长度 1-64)", name)
	}
	if len(description) > 500 {
		return fmt.Errorf("description too long (max 500 chars)")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.initialized {
		if _, err := os.Stat(s.skillsRoot); os.IsNotExist(err) {
			if err := os.MkdirAll(s.skillsRoot, 0755); err != nil {
				return fmt.Errorf("failed to create skills root: %w", err)
			}
		}
		s.skillDirs = resolveLibraryDirs(s.skillsRoot)
		s.loader = skills.NewLoader(s.skillDirs)
		s.initialized = true
	}

	target := filepath.Join(s.skillsRoot, name)
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("skill library %q already exists", name)
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		return fmt.Errorf("failed to create library directory: %w", err)
	}

	meta := skills.LibraryInfo{Name: name, Description: description}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode library metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(target, skills.LibraryMetaFileName), b, 0644); err != nil {
		return fmt.Errorf("failed to write library metadata: %w", err)
	}

	// 更新内存状态：纳入新库并重建 loader
	s.skillDirs = append(s.skillDirs, target)
	sort.Strings(s.skillDirs)
	s.loader = skills.NewLoader(s.skillDirs)

	logger.Infof(ctx, "Created skill library %q at %s", name, target)
	return nil
}

// UpdateSkillLibrary 更新一个已存在的 Skill 库：可改文件夹名（若 newName 与 oldName 不同）并写入新的 library.json。
func (s *skillService) UpdateSkillLibrary(ctx context.Context, oldName string, newName string, description string) error {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	description = strings.TrimSpace(description)
	if !validLibraryName(oldName) {
		return fmt.Errorf("invalid current library name: %q", oldName)
	}
	if !validLibraryName(newName) {
		return fmt.Errorf("invalid new library name: %q (允许字母数字开头，仅含字母数字/下划线/点/连字符，长度 1-64)", newName)
	}
	if len(description) > 500 {
		return fmt.Errorf("description too long (max 500 chars)")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.initialized {
		if _, err := os.Stat(s.skillsRoot); os.IsNotExist(err) {
			if err := os.MkdirAll(s.skillsRoot, 0755); err != nil {
				return fmt.Errorf("failed to create skills root: %w", err)
			}
		}
		s.skillDirs = resolveLibraryDirs(s.skillsRoot)
		s.loader = skills.NewLoader(s.skillDirs)
		s.initialized = true
	}

	oldDir := filepath.Join(s.skillsRoot, oldName)
	if _, err := os.Stat(oldDir); os.IsNotExist(err) {
		return fmt.Errorf("skill library %q does not exist", oldName)
	}

	targetDir := oldDir
	if newName != oldName {
		newDir := filepath.Join(s.skillsRoot, newName)
		if _, err := os.Stat(newDir); err == nil {
			return fmt.Errorf("skill library %q already exists", newName)
		}
		if err := os.Rename(oldDir, newDir); err != nil {
			return fmt.Errorf("failed to rename library directory: %w", err)
		}
		targetDir = newDir
	}

	// 写入 library.json（新的 name + description）
	meta := skills.LibraryInfo{Name: newName, Description: description}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode library metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, skills.LibraryMetaFileName), b, 0644); err != nil {
		return fmt.Errorf("failed to write library metadata: %w", err)
	}

	// 重建内存状态与 loader（旧 BasePath 缓存失效）
	s.skillDirs = resolveLibraryDirs(s.skillsRoot)
	s.loader = skills.NewLoader(s.skillDirs)

	logger.Infof(ctx, "Updated skill library %q -> %q", oldName, newName)
	return nil
}

// DeleteSkillLibrary 删除一个已存在的 Skill 库目录（连同其下所有 skills），并重建 loader。
func (s *skillService) DeleteSkillLibrary(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if !validLibraryName(name) {
		return fmt.Errorf("invalid library name: %q", name)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.initialized {
		if _, err := os.Stat(s.skillsRoot); os.IsNotExist(err) {
			if err := os.MkdirAll(s.skillsRoot, 0755); err != nil {
				return fmt.Errorf("failed to create skills root: %w", err)
			}
		}
		s.skillDirs = resolveLibraryDirs(s.skillsRoot)
		s.loader = skills.NewLoader(s.skillDirs)
		s.initialized = true
	}

	target := filepath.Join(s.skillsRoot, name)
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return fmt.Errorf("skill library %q does not exist", name)
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("failed to delete library directory: %w", err)
	}

	s.skillDirs = resolveLibraryDirs(s.skillsRoot)
	s.loader = skills.NewLoader(s.skillDirs)

	logger.Infof(ctx, "Deleted skill library %q", name)
	return nil
}
