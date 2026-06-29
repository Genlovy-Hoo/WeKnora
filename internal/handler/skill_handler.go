package handler

import (
	"net/http"
	"os"
	"strings"

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

// SkillLibraryResponse represents a skill library (top-level subdir under skills root) in the UI
type SkillLibraryResponse struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Skills      []SkillInfoResponse `json:"skills"`
}

// ListSkills godoc
// @Summary      获取Skills列表（按库分组）
// @Description  获取所有 Agent Skills 元数据，按所属 Skill 库(library)分组返回
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "按库分组的 Skills 列表"
// @Failure      500  {object}  errors.AppError         "服务器错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills [get]
func (h *SkillHandler) ListSkills(c *gin.Context) {
	ctx := c.Request.Context()

	// 库列表（含空库与描述）作为展示来源，保证新建的空库也能显示
	libs, err := h.skillService.ListSkillLibraries(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to list skill libraries: " + err.Error()))
		return
	}

	// 各 skill 按 Library 归类
	skillsByLibrary := make(map[string][]SkillInfoResponse)
	if skillsMetadata, err := h.skillService.ListPreloadedSkills(ctx); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		// skill 列举失败不阻断库列表展示，仅记录
	} else {
		for _, meta := range skillsMetadata {
			skillsByLibrary[meta.Library] = append(skillsByLibrary[meta.Library], SkillInfoResponse{
				Name:        meta.Name,
				Description: meta.Description,
			})
		}
	}

	libraries := make([]SkillLibraryResponse, 0, len(libs))
	for _, lib := range libs {
		skills := skillsByLibrary[lib.Name]
		if skills == nil {
			// 空库：确保序列化为 [] 而非 null，避免前端 library.skills.length 崩溃
			skills = []SkillInfoResponse{}
		}
		libraries = append(libraries, SkillLibraryResponse{
			Name:        lib.Name,
			Description: lib.Description,
			Skills:      skills,
		})
	}

	// skills_available: true only when sandbox is enabled (docker or local), so frontend can hide/disable Skills UI
	sandboxMode := os.Getenv("WEKNORA_SANDBOX_MODE")
	skillsAvailable := sandboxMode != "" && sandboxMode != "disabled"

	logger.Infof(ctx, "skills_available: %v, sandboxMode: %s, libraries: %d", skillsAvailable, sandboxMode, len(libraries))

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"data":             libraries,
		"skills_available": skillsAvailable,
	})
}

// createLibraryRequest 创建 Skill 库的请求体
type createLibraryRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CreateLibrary godoc
// @Summary      创建 Skill 库
// @Description  在 skills root 下新建一个 Skill 库目录并写入 library.json
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        body  body  createLibraryRequest  true  "库名与描述"
// @Success      200   {object}  map[string]interface{}  "创建成功"
// @Failure      400   {object}  errors.AppError         "参数错误"
// @Failure      409   {object}  errors.AppError         "库已存在"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills/libraries [post]
func (h *SkillHandler) CreateLibrary(c *gin.Context) {
	ctx := c.Request.Context()

	var req createLibraryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}

	if err := h.skillService.CreateSkillLibrary(ctx, req.Name, req.Description); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		// 区分「已存在」与一般参数/服务器错误
		if strings.Contains(err.Error(), "already exists") {
			c.Error(errors.NewBadRequestError(err.Error()))
			return
		}
		c.Error(errors.NewInternalServerError("Failed to create skill library: " + err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "skill library created",
	})
}

// updateLibraryRequest 更新 Skill 库的请求体（可改名 + 改描述）
type updateLibraryRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// UpdateLibrary godoc
// @Summary      修改 Skill 库
// @Description  修改 skills root 下某个 Skill 库的名称（改名文件夹）与描述（library.json）
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        name  path  string                  true  "当前库名"
// @Param        body  body  updateLibraryRequest    true  "新库名与描述"
// @Success      200   {object}  map[string]interface{}  "修改成功"
// @Failure      400   {object}  errors.AppError         "参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills/libraries/{name} [put]
func (h *SkillHandler) UpdateLibrary(c *gin.Context) {
	ctx := c.Request.Context()
	oldName := c.Param("name")
	if oldName == "" {
		c.Error(errors.NewBadRequestError("library name is required"))
		return
	}

	var req updateLibraryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}

	if err := h.skillService.UpdateSkillLibrary(ctx, oldName, req.Name, req.Description); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "skill library updated",
	})
}

// DeleteLibrary godoc
// @Summary      删除 Skill 库
// @Description  删除 skills root 下某个 Skill 库目录（连同其下所有 skills）
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        name  path  string  true  "库名"
// @Success      200   {object}  map[string]interface{}  "删除成功"
// @Failure      400   {object}  errors.AppError         "参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills/libraries/{name} [delete]
func (h *SkillHandler) DeleteLibrary(c *gin.Context) {
	ctx := c.Request.Context()
	name := c.Param("name")
	if name == "" {
		c.Error(errors.NewBadRequestError("library name is required"))
		return
	}

	if err := h.skillService.DeleteSkillLibrary(ctx, name); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "skill library deleted",
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
// @Param        name     path   string  true  "Skill 名称"
// @Param        library  query  string  true  "Skill 库名（用于跨库重名时按库精确解析）"
// @Success      200  {object}  map[string]interface{}  "Skill 详情"
// @Failure      400  {object}  errors.AppError         "参数错误"
// @Failure      404  {object}  errors.AppError         "Skill 不存在"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills/{name} [get]
func (h *SkillHandler) GetSkill(c *gin.Context) {
	ctx := c.Request.Context()
	name := c.Param("name")
	library := c.Query("library")
	if name == "" {
		c.Error(errors.NewBadRequestError("skill name is required"))
		return
	}
	if library == "" {
		c.Error(errors.NewBadRequestError("library is required"))
		return
	}

	skill, err := h.skillService.GetSkillByName(ctx, library, name)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewNotFoundError("Skill not found: " + err.Error()))
		return
	}

	files, err := h.skillService.ListSkillFiles(ctx, library, name)
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
// @Param        name     path   string  true  "Skill 名称"
// @Param        path     query  string  true  "文件相对路径，如 scripts/analyze.py"
// @Param        library  query  string  true  "Skill 库名（用于跨库重名时按库精确解析）"
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
	library := c.Query("library")
	if name == "" || relPath == "" {
		c.Error(errors.NewBadRequestError("skill name and file path are required"))
		return
	}
	if library == "" {
		c.Error(errors.NewBadRequestError("library is required"))
		return
	}

	file, err := h.skillService.GetSkillFile(ctx, library, name, relPath)
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
