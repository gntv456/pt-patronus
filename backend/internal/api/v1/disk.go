package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/monitor"
)

// DiskHandler 通过 SSH 远程通道在被控机上执行磁盘管理操作。
// 与 Docker 远程模式一致：不直接碰宿主块设备，而是经 RunCommandNow
// 在目标主机执行 lsblk/fdisk/mkfs/mount，复用已有 SSH 凭据与权限链。
type DiskHandler struct {
	runner DockerCommandRunner // 复用同一接口（RunCommandNow）
}

func NewDiskHandler(svc DockerCommandRunner) *DiskHandler {
	return &DiskHandler{runner: svc}
}

// List 返回目标主机的磁盘列表（lsblk 解析，按物理盘归类分区）。
func (h *DiskHandler) List(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	// lsblk --json 输出树状结构；ROTA=1 机械 / 0 固态，含型号/序列号/文件系统/挂载点。
	cmd := `lsblk -b -o NAME,PATH,TYPE,SIZE,MODEL,VENDOR,SERIAL,ROTA,RO,FSTYPE,PARTLABEL,MOUNTPOINT --json`
	result, err := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 15)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if !result.Success {
		c.JSON(http.StatusBadGateway, gin.H{"error": result.Stderr, "stdout": result.Stdout})
		return
	}

	var tree diskTreeResp
	if err := json.Unmarshal([]byte(result.Stdout), &tree); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to parse lsblk output: " + err.Error()})
		return
	}

	resp := diskListResp{Disks: []diskEntry{}}
	for _, dev := range tree.BlockDevices {
		// 只取顶层磁盘（type=disk/rom），分区作为子节点归类。
		if dev.Type != "disk" && dev.Type != "rom" {
			continue
		}
		entry := diskEntry{
			Name:       dev.Path,
			Type:       dev.Type,
			Size:       dev.Size,
			SizeHuman:  fmtBytes(int64(dev.Size)),
			Model:      dev.Model,
			Vendor:     dev.Vendor,
			Serial:     dev.Serial,
			Rota:       dev.Rota,
			Category:   "normal",
			Partitions: []diskPartition{},
		}
		isSystem := false
		hasPartition := false
		for _, child := range dev.Children {
			if child.Type == "part" || child.Type == "lvm" {
				hasPartition = true
				entry.Partitions = append(entry.Partitions, diskPartition{
					Name:       child.Path,
					Size:       child.Size,
					SizeHuman:  fmtBytes(int64(child.Size)),
					FSType:     child.FSType,
					Mountpoint: strings.TrimSpace(child.Mountpoint),
					PartLabel:  child.PartLabel,
				})
				// 系统盘判定：任一分区挂在 / /boot /boot/efi /efi
				mp := strings.TrimSpace(child.Mountpoint)
				if mp == "/" || mp == "/boot" || mp == "/boot/efi" || mp == "/efi" {
					isSystem = true
				}
			}
		}
		entry.IsSystem = isSystem
		switch {
		case isSystem:
			entry.Category = "system"
		case !hasPartition && dev.FSType == "":
			entry.Category = "unpartitioned"
		default:
			entry.Category = "normal"
		}
		resp.Disks = append(resp.Disks, entry)
	}
	c.JSON(http.StatusOK, resp)
}

// _ 让 monitor 包在 import 未直接使用类型时仍保留（RunCommandNow 返回 monitor.CommandResult）。
var _ = monitor.CommandResult{}
var _ = fmt.Sprintf
