package v1

import (
	"fmt"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

// 挂载点白名单：必须以 / 开头，禁止相对路径/特殊字符（防注入）。
var mountpointRe = regexp.MustCompile(`^/[a-zA-Z0-9_\-/.]*$`)
var blockDeviceRe = regexp.MustCompile(`^/dev/(sd[a-z]+|nvme\d+n\d+|vd[a-z]+|hd[a-z]+|mmcblk\d+)(p\d+)?$`)

// shellSafeRe restricts to characters safe for embedding in single-quoted shell strings:
// rejects single quotes, backticks, $, semicolons, pipes, and command substitution.
var shellSafeRe = regexp.MustCompile(`^[a-zA-Z0-9_\-/=.:+]+$`)

// validateDevice 校验块设备路径合法性（防注入 /dev/../../etc/passwd）。
func validateDevice(dev string) bool {
	return blockDeviceRe.MatchString(dev)
}

func validateMountpoint(mp string) bool {
	if mp == "" || mp == "/" {
		return false
	}
	return mountpointRe.MatchString(mp) && !strings.Contains(mp, "..")
}

// Partition 对未分区磁盘创建分区 + 格式化 + 挂载（对应 1Panel"分区并挂载"）。
// 高危：会清除目标磁盘所有数据。必须 ConfirmRisk。
func (h *DiskHandler) Partition(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in diskPartitionRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if !in.ConfirmRisk {
		errcode.Respond(c, errcode.RiskConfirmationRequired)
		return
	}
	// 只允许整盘设备（无分区后缀），且校验 fstype。
	if !blockDeviceRe.MatchString(in.Device) || strings.Contains(in.Device, "p") {
		errcode.Respond(c, errcode.InvalidRequest)
		return
	}
	if in.FSType != "ext4" && in.FSType != "xfs" {
		errcode.Respond(c, errcode.InvalidRequest)
		return
	}
	if !validateMountpoint(in.Mountpoint) {
		errcode.Respond(c, errcode.DiskInvalidMountpoint)
		return
	}

	// 安全：再次确认设备不是系统盘（检查是否有分区挂着 /）。
	// 用 parted/wipe + fdisk 创建 GPT 分区表 + 单分区，再 mkfs，最后 mount。
	var steps []string
	steps = append(steps,
		fmt.Sprintf(`if lsblk -no TYPE %s | grep -q part; then echo "DEVICE_HAS_PARTITIONS"; exit 1; fi`, in.Device),
		fmt.Sprintf(`wipefs -a %s 2>/dev/null || true`, in.Device),
		fmt.Sprintf(`parted -s %s mklabel gpt mkpart primary 0%% 100%%`, in.Device),
		`sleep 1`, // 等 udev 刷新
	)
	// 分区名：sdb -> sdb1, nvme0n1 -> nvme0n1p1
	partName := in.Device + "1"
	if strings.Contains(in.Device, "nvme") || strings.Contains(in.Device, "mmcblk") {
		partName = in.Device + "p1"
	}
	mkfs := "mkfs.ext4 -F"
	if in.FSType == "xfs" {
		mkfs = "mkfs.xfs -f"
	}
	steps = append(steps,
		fmt.Sprintf(`%s %s`, mkfs, partName),
		fmt.Sprintf(`mkdir -p %s`, in.Mountpoint),
		fmt.Sprintf(`mount %s %s`, partName, in.Mountpoint),
	)
	script := strings.Join(steps, " && ")
	result, err := h.runner.RunCommandNow(c.Request.Context(), id, script, 60)
	if err != nil {
		respondOpsTargetError(c, err)
		return
	}
	if !result.Success {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": result.Stderr, "stdout": result.Stdout})
		return
	}

	// 自动挂载：写 /etc/fstab（用 UUID 更稳健）。
	if in.AutoMount {
		fstabLine := h.buildFstabLine(c, id, partName, in.Mountpoint, in.FSType, in.NoFail)
		if fstabLine != "" {
			c.JSON(http.StatusOK, gin.H{"ok": true, "fstab": fstabLine, "partition": partName})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "partition": partName})
}

// buildFstabLine 在远程主机查 UUID 并写入 /etc/fstab，返回 fstab 行（失败返回空）。
func (h *DiskHandler) buildFstabLine(c *gin.Context, id uint, partName, mountpoint, fstype string, nofail bool) string {
	uuidCmd := fmt.Sprintf(`blkid -s UUID -o value %s`, partName)
	result, err := h.runner.RunCommandNow(c.Request.Context(), id, uuidCmd, 10)
	if err != nil || !result.Success || strings.TrimSpace(result.Stdout) == "" {
		return ""
	}
	uuid := strings.TrimSpace(result.Stdout)
	// Security: reject any value containing characters unsafe for shell/fstab embedding.
	if !shellSafeRe.MatchString(uuid) || !shellSafeRe.MatchString(fstype) {
		return ""
	}
	opts := "defaults"
	if nofail {
		opts = "defaults,nofail"
	}
	line := fmt.Sprintf("UUID=%s %s %s %s 0 2", uuid, mountpoint, fstype, opts)
	// Use printf %s\n with proper quoting to avoid echo/shell injection.
	// sed uses # as delimiter; mountpoint is validated to not contain #.
	dedup := fmt.Sprintf(`sed -i '\#%s#d' /etc/fstab && printf '%%s\n' '%s' >> /etc/fstab`, mountpoint, line)
	_, _ = h.runner.RunCommandNow(c.Request.Context(), id, dedup, 10)
	return line
}

// Mount 挂载已有文件系统的分区（不格式化）。
func (h *DiskHandler) Mount(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in diskMountRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if !validateDevice(in.Device) {
		errcode.Respond(c, errcode.InvalidRequest)
		return
	}
	if !validateMountpoint(in.Mountpoint) {
		errcode.Respond(c, errcode.DiskInvalidMountpoint)
		return
	}
	// 先检测分区是否已有文件系统；没有则拒绝（应走 Partition）。
	checkCmd := fmt.Sprintf(`blkid -s TYPE -o value %s`, in.Device)
	chk, _ := h.runner.RunCommandNow(c.Request.Context(), id, checkCmd, 10)
	fstype := strings.TrimSpace(chk.Stdout)
	if fstype == "" {
		errcode.Respond(c, errcode.ResourceStateInvalid)
		return
	}
	script := fmt.Sprintf(`mkdir -p %s && mount %s %s`, in.Mountpoint, in.Device, in.Mountpoint)
	result, err := h.runner.RunCommandNow(c.Request.Context(), id, script, 30)
	if err != nil {
		respondOpsTargetError(c, err)
		return
	}
	if !result.Success {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": result.Stderr, "stdout": result.Stdout})
		return
	}
	if in.AutoMount {
		line := h.buildFstabLine(c, id, in.Device, in.Mountpoint, fstype, in.NoFail)
		c.JSON(http.StatusOK, gin.H{"ok": true, "fstab": line})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Umount 卸载挂载点，可选同时从 fstab 移除。
func (h *DiskHandler) Umount(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in diskUmountRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if !validateMountpoint(in.Mountpoint) {
		errcode.Respond(c, errcode.DiskInvalidMountpoint)
		return
	}
	script := fmt.Sprintf(`umount %s`, in.Mountpoint)
	result, err := h.runner.RunCommandNow(c.Request.Context(), id, script, 30)
	if err != nil {
		respondOpsTargetError(c, err)
		return
	}
	if !result.Success {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": result.Stderr, "stdout": result.Stdout})
		return
	}
	if in.RemoveFstab {
		rmCmd := fmt.Sprintf(`sed -i '\#%s#d' /etc/fstab`, in.Mountpoint)
		_, _ = h.runner.RunCommandNow(c.Request.Context(), id, rmCmd, 10)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
