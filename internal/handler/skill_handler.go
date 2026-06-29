package handler

import (
	"net/http"
	"os"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// SkillHandler handles skill-related HTTP requests
type SkillHandler struct {
	skillService interfaces.SkillService
}

// NewSkillHandler creates a new skill handler
func NewSkillHandler(skillService interfaces.SkillService) *SkillHandler {
	return &SkillHandler{
		skillService: skillService,
	}
}

// SkillInfoResponse represents the skill info returned to frontend
type SkillInfoResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ListSkills godoc
// @Summary      获取预装Skills列表
// @Description  获取所有预装的Agent Skills元数据
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "Skills列表"
// @Failure      500  {object}  errors.AppError         "服务器错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills [get]
func (h *SkillHandler) ListSkills(c *gin.Context) {
	ctx := c.Request.Context()

	skillsMetadata, err := h.skillService.ListPreloadedSkills(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to list skills: " + err.Error()))
		return
	}

	// Convert to response format
	var response []SkillInfoResponse
	for _, meta := range skillsMetadata {
		response = append(response, SkillInfoResponse{
			Name:        meta.Name,
			Description: meta.Description,
		})
	}

	// skills_available: true only when sandbox is enabled (docker or local), so frontend can hide/disable Skills UI
	sandboxMode := os.Getenv("WEKNORA_SANDBOX_MODE")
	skillsAvailable := sandboxMode != "" && sandboxMode != "disabled"

	logger.Infof(ctx, "skills_available: %v, sandboxMode: %s", skillsAvailable, sandboxMode)

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"data":             response,
		"skills_available": skillsAvailable,
	})
}

// SkillDetailResponse represents a single skill's metadata and instructions
type SkillDetailResponse struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Instructions string   `json:"instructions"`
	Files        []string `json:"files"`
}

// GetSkill godoc
// @Summary      获取单个 Skill 详情
// @Description  按 name 返回 Skill 元数据与 SKILL.md 正文（instructions）
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        name  path   string  true  "Skill 名称"
// @Success      200  {object}  map[string]interface{}  "Skill 详情"
// @Failure      400  {object}  errors.AppError         "参数错误"
// @Failure      404  {object}  errors.AppError         "Skill 不存在"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills/{name} [get]
func (h *SkillHandler) GetSkill(c *gin.Context) {
	ctx := c.Request.Context()
	name := c.Param("name")
	if name == "" {
		c.Error(errors.NewBadRequestError("skill name is required"))
		return
	}

	skill, err := h.skillService.GetSkillByName(ctx, name)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewNotFoundError("Skill not found: " + err.Error()))
		return
	}

	files, err := h.skillService.ListSkillFiles(ctx, name)
	if err != nil {
		// 文件树列举失败不应阻断详情查看，记录后以空列表返回
		logger.ErrorWithFields(ctx, err, nil)
		files = nil
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": SkillDetailResponse{
			Name:         skill.Name,
			Description:  skill.Description,
			Instructions: skill.Instructions,
			Files:        files,
		},
	})
}

// SkillFileResponse represents a single file within a skill directory
type SkillFileResponse struct {
	Name     string `json:"name"`     // 相对路径，如 "scripts/analyze.py"
	Content  string `json:"content"` // 文件文本内容
	IsScript bool   `json:"is_script"`
}

// GetSkillFile godoc
// @Summary      获取 Skill 内单个文件内容
// @Description  按 skill name 与相对路径 path 返回该文件内容
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        name  path   string  true  "Skill 名称"
// @Param        path  query  string  true  "文件相对路径，如 scripts/analyze.py"
// @Success      200  {object}  map[string]interface{}  "文件内容"
// @Failure      400  {object}  errors.AppError         "参数错误"
// @Failure      404  {object}  errors.AppError         "文件不存在"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills/{name}/file [get]
func (h *SkillHandler) GetSkillFile(c *gin.Context) {
	ctx := c.Request.Context()
	name := c.Param("name")
	relPath := c.Query("path")
	if name == "" || relPath == "" {
		c.Error(errors.NewBadRequestError("skill name and file path are required"))
		return
	}

	file, err := h.skillService.GetSkillFile(ctx, name, relPath)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewNotFoundError("Skill file not found: " + err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": SkillFileResponse{
			Name:     file.Name,
			Content:  file.Content,
			IsScript: file.IsScript,
		},
	})
}
