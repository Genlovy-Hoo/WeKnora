package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/agent/skills"
)

// SkillService defines the interface for skill business logic
type SkillService interface {
	// ListPreloadedSkills returns metadata for all preloaded skills
	ListPreloadedSkills(ctx context.Context) ([]*skills.SkillMetadata, error)

	// GetSkillByName retrieves a skill by its name within a specific library.
	// library 用于跨库重名时精确解析到指定库的版本。
	GetSkillByName(ctx context.Context, library string, name string) (*skills.Skill, error)

	// ListSkillFiles returns the relative paths of all files in a skill directory
	// within a specific library.
	ListSkillFiles(ctx context.Context, library string, name string) ([]string, error)

	// GetSkillFile returns a single file's content from a skill directory by
	// relative path, within a specific library.
	GetSkillFile(ctx context.Context, library string, name string, relPath string) (*skills.SkillFile, error)

	// ListSkillLibraries 返回所有 Skill 库的元信息（名称 + 描述），包含空库。
	ListSkillLibraries(ctx context.Context) ([]*skills.LibraryInfo, error)

	// CreateSkillLibrary 在 skills root 下创建一个新的 Skill 库目录并写入 library.json。
	CreateSkillLibrary(ctx context.Context, name string, description string) error

	// UpdateSkillLibrary 更新一个已存在的 Skill 库（可改名文件夹 + 更新描述）。
	UpdateSkillLibrary(ctx context.Context, oldName string, newName string, description string) error

	// DeleteSkillLibrary 删除一个已存在的 Skill 库（连同其下所有 skills）。
	DeleteSkillLibrary(ctx context.Context, name string) error
}
