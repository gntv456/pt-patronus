package v1

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
	Rota       string            `json:"rota,omitempty"`        // 1=机械 0=固态/NVMe
	ReadOnly   string            `json:"ro,omitempty"`          // 只读标志
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
	PartLabel  string `json:"partlabel,omitempty"`
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
