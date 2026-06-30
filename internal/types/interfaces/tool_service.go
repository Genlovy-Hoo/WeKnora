package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// ToolLibraryRepository defines data access for tool libraries (groups).
type ToolLibraryRepository interface {
	Create(ctx context.Context, lib *types.ToolLibrary) error
	GetByID(ctx context.Context, tenantID uint64, id string) (*types.ToolLibrary, error)
	List(ctx context.Context, tenantID uint64) ([]*types.ToolLibrary, error)
	Update(ctx context.Context, lib *types.ToolLibrary) error
	Delete(ctx context.Context, tenantID uint64, id string) error
	// EnsureBuiltin makes sure a builtin library row exists for the tenant and
	// returns its id (creates one lazily if missing).
	EnsureBuiltin(ctx context.Context, tenantID uint64) (string, error)
}

// CustomToolRepository defines data access for user-registered HTTP tools.
type CustomToolRepository interface {
	Create(ctx context.Context, tool *types.CustomTool) error
	GetByID(ctx context.Context, tenantID uint64, id string) (*types.CustomTool, error)
	GetByName(ctx context.Context, tenantID uint64, name string) (*types.CustomTool, error)
	ListByLibrary(ctx context.Context, tenantID uint64, libraryID string) ([]*types.CustomTool, error)
	ListEnabled(ctx context.Context, tenantID uint64) ([]*types.CustomTool, error)
	ListByNames(ctx context.Context, tenantID uint64, names []string) ([]*types.CustomTool, error)
	Update(ctx context.Context, tool *types.CustomTool) error
	Delete(ctx context.Context, tenantID uint64, id string) error
}

// ToolLibraryWithTools is the aggregated shape returned by ListAllTools:
// a library plus the tools it contains (builtin tools are injected by the
// service layer for the builtin library).
type ToolLibraryWithTools struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	IsBuiltin   bool               `json:"is_builtin"`
	SortOrder   int                `json:"sort_order"`
	Tools       []ToolDefinition   `json:"tools"`
}

// ToolDefinition is the unified tool metadata used by the agent editor UI
// and the allowed_tools selection. Builtin and custom tools share this shape.
type ToolDefinition struct {
	Name        string             `json:"name"`
	DisplayName string             `json:"display_name"`
	Description string             `json:"description"`
	Group       string             `json:"group"`         // base/rag/wiki_read/wiki_edit/wiki_issue/data/http
	Parameters  types.JSONSchema   `json:"parameters,omitempty"`
	Danger      bool               `json:"danger"`        // require_approval
}

// ToolService defines the business logic for tool libraries and custom tools.
type ToolService interface {
	// Library CRUD
	CreateToolLibrary(ctx context.Context, tenantID uint64, name, description string) (*types.ToolLibrary, error)
	ListToolLibraries(ctx context.Context, tenantID uint64) ([]*types.ToolLibrary, error)
	UpdateToolLibrary(ctx context.Context, tenantID uint64, id, name, description string) error
	DeleteToolLibrary(ctx context.Context, tenantID uint64, id string) error

	// Custom tool CRUD
	CreateCustomTool(ctx context.Context, tenantID uint64, tool *types.CustomTool) (*types.CustomTool, error)
	GetCustomTool(ctx context.Context, tenantID uint64, id string) (*types.CustomTool, error)
	ListCustomToolsByLibrary(ctx context.Context, tenantID uint64, libraryID string) ([]*types.CustomTool, error)
	UpdateCustomTool(ctx context.Context, tenantID uint64, id string, tool *types.CustomTool) error
	DeleteCustomTool(ctx context.Context, tenantID uint64, id string) error

	// Test invokes a custom tool with sample args and returns the HTTP result.
	TestCustomTool(ctx context.Context, tenantID uint64, id string, args types.JSONSchema) (*types.ToolResult, error)

	// ListAllTools aggregates builtin tools (from definitions) and custom tools
	// (from DB) into library-grouped metadata for the agent editor.
	ListAllTools(ctx context.Context, tenantID uint64) ([]*ToolLibraryWithTools, error)

	// ListEnabledCustomTools returns all enabled custom tools for a tenant
	// (used by the agent runtime to register HTTPTool instances).
	ListEnabledCustomTools(ctx context.Context, tenantID uint64) ([]*types.CustomTool, error)

	// ListCustomToolsByNames returns enabled custom tools matching the given
	// names (used by registerCustomTools to resolve allowed_tools entries that
	// the builtin switch did not handle).
	ListCustomToolsByNames(ctx context.Context, tenantID uint64, names []string) ([]*types.CustomTool, error)
}
