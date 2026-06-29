package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/agent/skills"
)

// SkillService defines the interface for skill business logic
type SkillService interface {
	// ListPreloadedSkills returns metadata for all preloaded skills
	ListPreloadedSkills(ctx context.Context) ([]*skills.SkillMetadata, error)

	// GetSkillByName retrieves a skill by its name
	GetSkillByName(ctx context.Context, name string) (*skills.Skill, error)

	// ListSkillFiles returns the relative paths of all files in a skill directory
	ListSkillFiles(ctx context.Context, name string) ([]string, error)

	// GetSkillFile returns a single file's content from a skill directory by relative path
	GetSkillFile(ctx context.Context, name string, relPath string) (*skills.SkillFile, error)
}
