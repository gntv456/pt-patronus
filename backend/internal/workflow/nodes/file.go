package nodes

import (
	"context"
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

// 本文件新增「文件」分类节点：列文件、删除文件、新建目录。
// 封装 FileManagerChain（本机/WebDAV/Alist/SMB/Agent 统一文件管理）。storage_id 为文件存储后端 ID
// （设置→文件管理），用 number 控件避免新增前端选择器类型；path 支持 {{var.x}} 模板。

// listFilesNode 列出目录条目，写入文本载荷。
type listFilesNode struct {
	fileCh *chain.FileManagerChain
}

func NewListFilesNode(fileCh *chain.FileManagerChain) *listFilesNode {
	return &listFilesNode{fileCh: fileCh}
}

func (n *listFilesNode) Type() string        { return "list_files" }
func (n *listFilesNode) Name() string        { return "列文件" }
func (n *listFilesNode) Category() string    { return "文件" }
func (n *listFilesNode) Description() string { return "列出文件存储后端的目录条目（写入文本载荷）" }
func (n *listFilesNode) Icon() string        { return "files" }
func (n *listFilesNode) DefaultParams() map[string]any {
	return map[string]any{"storage_id": float64(0), "path": "/"}
}
func (n *listFilesNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "storage_id", Label: "存储后端 ID", Type: "number", Required: true, Help: "设置→文件管理 中的后端 ID"},
		{Name: "path", Label: "路径", Type: "text", Default: "/", Help: "支持 {{var.x}}"},
	}
}

func (n *listFilesNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	id := uint(workflow.ParamInt(params, "storage_id"))
	if id == 0 {
		return fmt.Errorf("未选择存储后端")
	}
	path := strings.TrimSpace(render(workflow.ParamString(params, "path"), ecx))
	if path == "" {
		path = "/"
	}
	entries, err := n.fileCh.List(ctx, id, path)
	if err != nil {
		return fmt.Errorf("列文件失败: %w", err)
	}
	writeJSON(ecx, entries)
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("列出 %s：%d 项", path, len(entries)))
	return nil
}

// deleteFileNode 删除文件/目录。
type deleteFileNode struct {
	fileCh *chain.FileManagerChain
}

func NewDeleteFileNode(fileCh *chain.FileManagerChain) *deleteFileNode {
	return &deleteFileNode{fileCh: fileCh}
}

func (n *deleteFileNode) Type() string        { return "delete_file" }
func (n *deleteFileNode) Name() string        { return "删除文件" }
func (n *deleteFileNode) Category() string    { return "文件" }
func (n *deleteFileNode) Description() string { return "删除文件存储后端上的文件或目录" }
func (n *deleteFileNode) Icon() string        { return "delete" }
func (n *deleteFileNode) DefaultParams() map[string]any {
	return map[string]any{"storage_id": float64(0), "path": ""}
}
func (n *deleteFileNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "storage_id", Label: "存储后端 ID", Type: "number", Required: true},
		{Name: "path", Label: "路径", Type: "text", Required: true, Help: "支持 {{var.x}}"},
	}
}

func (n *deleteFileNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	id := uint(workflow.ParamInt(params, "storage_id"))
	if id == 0 {
		return fmt.Errorf("未选择存储后端")
	}
	path := strings.TrimSpace(render(workflow.ParamString(params, "path"), ecx))
	if path == "" {
		return fmt.Errorf("路径为空")
	}
	if err := n.fileCh.Delete(ctx, id, path); err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}
	ecx.Messages = append(ecx.Messages, "已删除："+path)
	return nil
}

// mkdirNode 新建目录。
type mkdirNode struct {
	fileCh *chain.FileManagerChain
}

func NewMkdirNode(fileCh *chain.FileManagerChain) *mkdirNode {
	return &mkdirNode{fileCh: fileCh}
}

func (n *mkdirNode) Type() string        { return "mkdir" }
func (n *mkdirNode) Name() string        { return "新建目录" }
func (n *mkdirNode) Category() string    { return "文件" }
func (n *mkdirNode) Description() string { return "在文件存储后端上新建目录" }
func (n *mkdirNode) Icon() string        { return "folder" }
func (n *mkdirNode) DefaultParams() map[string]any {
	return map[string]any{"storage_id": float64(0), "path": ""}
}
func (n *mkdirNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "storage_id", Label: "存储后端 ID", Type: "number", Required: true},
		{Name: "path", Label: "目录路径", Type: "text", Required: true, Help: "支持 {{var.x}}"},
	}
}

func (n *mkdirNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	id := uint(workflow.ParamInt(params, "storage_id"))
	if id == 0 {
		return fmt.Errorf("未选择存储后端")
	}
	path := strings.TrimSpace(render(workflow.ParamString(params, "path"), ecx))
	if path == "" {
		return fmt.Errorf("路径为空")
	}
	if err := n.fileCh.Mkdir(ctx, id, path); err != nil {
		return fmt.Errorf("新建目录失败: %w", err)
	}
	ecx.Messages = append(ecx.Messages, "已新建目录："+path)
	return nil
}
