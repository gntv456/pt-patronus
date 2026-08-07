package v1

import (
	"strconv"
	"strings"
)

// lsblkBool 兼容不同版本 lsblk 的布尔字段输出：
// 旧版 util-linux 输出字符串 "0"/"1"，新版可能输出 true/false。
// 统一归一化为 "0"/"1" 字符串，供下游判定使用。
type lsblkBool string

// UnmarshalJSON 同时接受 bool、数值和字符串，统一转成 "0"/"1"。
func (b *lsblkBool) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if len(s) == 0 || s == "null" {
		*b = ""
		return nil
	}
	// 布尔字面量 true/false（新版 lsblk 以 JSON bool 输出）
	if s == "true" {
		*b = "1"
		return nil
	}
	if s == "false" {
		*b = "0"
		return nil
	}
	// 去掉可能的引号（旧版 lsblk 以字符串 "0"/"1" 输出）
	if unquoted, err := strconv.Unquote(s); err == nil {
		s = unquoted
	}
	switch strings.ToLower(s) {
	case "1", "true", "yes", "on":
		*b = "1"
	case "0", "false", "no", "off", "":
		*b = "0"
	default:
		// 未知值原样保留，避免静默吞错
		*b = lsblkBool(s)
	}
	return nil
}

// diskBlockDevice 对应 lsblk 输出中的一个块设备节点（磁盘或分区）。
// 与 1Panel 磁盘管理对齐：系统盘/未分区/普通盘三类，含分区、文件系统、挂载点等。
type diskBlockDevice struct {
	Name       string            `json:"name"`                  // 内核设备名 sda / sda1 / nvme0n1
	Path       string            `json:"path"`                  // /dev/sda
	Type       string            `json:"type"`                  // disk | part | rom | lvm
	Size       uint64            `json:"size"`                  // 字节
	SizeHuman  string            `json:"size_human"`            // "1.0T" 可读
	Model      string            `json:"model,omitempty"`       // 磁盘型号
	Vendor     string            `json:"vendor,omitempty"`      // 厂商
	Serial     string            `json:"serial,omitempty"`      // 序列号
	Rota       lsblkBool         `json:"rota,omitempty"`        // 1=机械 0=固态/NVMe
	ReadOnly   lsblkBool         `json:"ro,omitempty"`          // 只读标志
	FSType     string            `json:"fstype,omitempty"`      // ext4/xfs/ntfs/...
	PartLabel  string            `json:"partlabel,omitempty"`   // 分区标签
	Mountpoint string            `json:"mountpoint,omitempty"`  // 挂载点（已挂载）
	Children   []diskBlockDevice `json:"children,omitempty"`    // 分区树
}

// diskTreeResp 是磁盘列表的顶层响应。
type diskTreeResp struct {
	BlockDevices []diskBlockDevice `json:"blockdevices"`
}

// diskListResp 是前端消费的扁平化磁盘视图（按磁盘归类分区）。
type diskListResp struct {
	Disks []diskEntry `json:"disks"`
}

// diskEntry 是单个物理磁盘及其分区的聚合视图，带分类标签。
type diskEntry struct {
	Name      string         `json:"name"`     // /dev/sda
	Type      string         `json:"type"`     // disk | rom
	Size      uint64         `json:"size"`
	SizeHuman string         `json:"size_human"`
	Model     string         `json:"model,omitempty"`
	Vendor    string         `json:"vendor,omitempty"`
	Serial    string         `json:"serial,omitempty"`
	Rota      string         `json:"rota,omitempty"`   // 1=HDD 0=SSD
	Category  string         `json:"category"`         // system | unpartitioned | normal
	// 系统盘判定依据：磁盘上有分区挂载在 / 或 /boot 或 /boot/efi
	IsSystem bool `json:"is_system"`
	// 整盘聚合用量（仅统计有 df 数据的分区）
	UsedBytes    uint64  `json:"used_bytes,omitempty"`
	UsedHuman    string  `json:"used_human,omitempty"`
	TotalBytes   uint64  `json:"total_bytes,omitempty"`
	TotalHuman   string  `json:"total_human,omitempty"`
	UsagePercent float64 `json:"usage_percent,omitempty"`
	// 分区列表（普通盘/系统盘有分区，未分区盘为空）
	Partitions []diskPartition `json:"partitions,omitempty"`
}

// diskPartition 是磁盘下的单个分区视图。
type diskPartition struct {
	Name       string `json:"name"`                  // /dev/sda1
	Size       uint64 `json:"size"`
	SizeHuman  string `json:"size_human"`
	FSType     string `json:"fstype,omitempty"`      // ext4/xfs/...
	Mountpoint string `json:"mountpoint,omitempty"`  // 已挂载点；空=未挂载
	PartLabel    string  `json:"partlabel,omitempty"`
	UsedBytes    uint64  `json:"used_bytes,omitempty"`
	UsedHuman    string  `json:"used_human,omitempty"`
	UsagePercent float64 `json:"usage_percent,omitempty"`
}

// diskPartitionRequest 分区并格式化请求体（对应 1Panel"分区并挂载"）。
type diskPartitionRequest struct {
	Device      string `json:"device" binding:"required"`      // /dev/sdb（整盘）
	FSType      string `json:"fstype" binding:"required"`       // ext4 | xfs
	Mountpoint  string `json:"mountpoint" binding:"required"`   // /mnt/data
	AutoMount   bool   `json:"auto_mount"`                      // 写入 /etc/fstab
	NoFail      bool   `json:"nofail"`                          // fstab 加 nofail 选项
	ConfirmRisk bool   `json:"confirm_risk"`                    // 必须显式确认
}

// diskMountRequest 挂载已有文件系统分区（不格式化）。
type diskMountRequest struct {
	Device      string `json:"device" binding:"required"`  // /dev/sda2
	Mountpoint  string `json:"mountpoint" binding:"required"`
	AutoMount   bool   `json:"auto_mount"`
	NoFail      bool   `json:"nofail"`
}

// diskUmountRequest 卸载请求。
type diskUmountRequest struct {
	Mountpoint string `json:"mountpoint" binding:"required"`
	RemoveFstab bool  `json:"remove_fstab"` // 同时从 fstab 移除
}