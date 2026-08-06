package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// --- 高层 API ---

// LocalList 列出本机所有容器（含已停止）。
func (c *DockerLocalClient) LocalList(ctx context.Context) (dockerListResp, error) {
	resp := dockerListResp{
		Containers: []dockerListContainer{},
		Images:     []dockerImage{},
		Volumes:    []dockerVolume{},
		Networks:   []dockerNetwork{},
	}
	// 容器
	var apiContainers []struct {
		ID     string   `json:"Id"`
		Names  []string `json:"Names"`
		Image  string   `json:"Image"`
		State  string   `json:"State"`
		Status string   `json:"Status"`
		Ports  []struct {
			IP          string `json:"IP"`
			PrivatePort int    `json:"PrivatePort"`
			PublicPort  int    `json:"PublicPort"`
			Type        string `json:"Type"`
		} `json:"Ports"`
	}
	if _, err := c.get(ctx, "/containers/json?all=true", &apiContainers); err != nil {
		return resp, err
	}
	for _, ct := range apiContainers {
		names := ""
		if len(ct.Names) > 0 {
			names = ct.Names[0]
		}
		resp.Containers = append(resp.Containers, dockerListContainer{
			ID:     ct.ID[:min(12, len(ct.ID))],
			Names:  names,
			Image:  ct.Image,
			Status: ct.Status,
			State:  ct.State,
		})
	}
	// 镜像
	var apiImages []struct {
		RepoTags []string `json:"RepoTags"`
		ID       string   `json:"Id"`
		Size     int64    `json:"Size"`
		Created  int64    `json:"Created"`
	}
	if _, err := c.get(ctx, "/images/json", &apiImages); err == nil {
		for _, img := range apiImages {
			repo, tag := "none", "none"
			if len(img.RepoTags) > 0 {
				parts := strings.SplitN(img.RepoTags[0], ":", 2)
				repo = parts[0]
				if len(parts) > 1 {
					tag = parts[1]
				}
			}
			resp.Images = append(resp.Images, dockerImage{
				Repository: repo,
				Tag:        tag,
				ID:         img.ID,
				Size:       fmtBytes(img.Size),
				CreatedAt:  time.Unix(img.Created, 0).Format("2006-01-02"),
			})
		}
	}
	// 卷
	var apiVolumes struct {
		Volumes []struct {
			Name   string `json:"Name"`
			Driver string `json:"Driver"`
		} `json:"Volumes"`
	}
	if _, err := c.get(ctx, "/volumes", &apiVolumes); err == nil {
		for _, v := range apiVolumes.Volumes {
			resp.Volumes = append(resp.Volumes, dockerVolume{Name: v.Name, Driver: v.Driver})
		}
	}
	// 网络
	var apiNetworks []struct {
		Name   string `json:"Name"`
		ID     string `json:"Id"`
		Driver string `json:"Driver"`
		Scope  string `json:"Scope"`
	}
	if _, err := c.get(ctx, "/networks", &apiNetworks); err == nil {
		for _, n := range apiNetworks {
			resp.Networks = append(resp.Networks, dockerNetwork{
				Name: n.Name, ID: n.ID[:min(12, len(n.ID))], Driver: n.Driver, Scope: n.Scope,
			})
		}
	}
	return resp, nil
}

// LocalStats 获取所有容器的实时资源占用。
func (c *DockerLocalClient) LocalStats(ctx context.Context) ([]dockerStatsRow, error) {
	// /stats?stream=false 一次返回所有容器统计
	// Docker API v1.24+ 的 /stats 需要 per-container。这里先 list 再逐个取。
	var containers []struct {
		ID    string `json:"Id"`
		Names []string `json:"Names"`
	}
	if _, err := c.get(ctx, "/containers/json", &containers); err != nil {
		return nil, err
	}
	var out []dockerStatsRow
	for _, ct := range containers {
		name := ""
		if len(ct.Names) > 0 {
			name = strings.TrimPrefix(ct.Names[0], "/")
		}
		var stats struct {
			Name       string `json:"name"`
			CPUStats   struct {
				CPUUsage struct {
					TotalUsage float64 `json:"total_usage"`
				} `json:"cpu_usage"`
				SystemCPUUsage float64 `json:"system_cpu_usage"`
				OnlineCPUs     int     `json:"online_cpus"`
			} `json:"cpu_stats"`
			PreCPUStats struct {
				CPUUsage struct {
					TotalUsage float64 `json:"total_usage"`
				} `json:"cpu_usage"`
				SystemCPUUsage float64 `json:"system_cpu_usage"`
			} `json:"precpu_stats"`
			MemoryStats struct {
				Usage float64 `json:"usage"`
				Limit float64 `json:"limit"`
			} `json:"memory_stats"`
		}
		if _, err := c.get(ctx, "/containers/"+ct.ID+"/stats?stream=false", &stats); err != nil {
			continue
		}
		cpuPerc := 0.0
		cpuDelta := stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage
		sysDelta := stats.CPUStats.SystemCPUUsage - stats.PreCPUStats.SystemCPUUsage
		if sysDelta > 0 && cpuDelta > 0 {
			cpuPerc = (cpuDelta / sysDelta) * float64(stats.CPUStats.OnlineCPUs) * 100
		}
		memPerc := 0.0
		if stats.MemoryStats.Limit > 0 {
			memPerc = stats.MemoryStats.Usage / stats.MemoryStats.Limit * 100
		}
		out = append(out, dockerStatsRow{
			Name:     name,
			CPUPerc:  fmt.Sprintf("%.2f%%", cpuPerc),
			MemUsage: fmt.Sprintf("%s / %s", fmtBytes(int64(stats.MemoryStats.Usage)), fmtBytes(int64(stats.MemoryStats.Limit))),
			MemPerc:  fmt.Sprintf("%.2f%%", memPerc),
		})
	}
	return out, nil
}

// LocalInspect 查看容器详情。
func (c *DockerLocalClient) LocalInspect(ctx context.Context, name string) (dockerInspectContainer, error) {
	var ct dockerInspectContainer
	_, err := c.get(ctx, "/containers/"+name+"/json", &ct)
	return ct, err
}

// LocalLogs 获取容器日志。
func (c *DockerLocalClient) LocalLogs(ctx context.Context, name string, tail int) (string, error) {
	resp, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/containers/%s/logs?stdout=true&stderr=true&tail=%d", name, tail), nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	// Docker 日志流有 8 字节头部（stream type + 长度），需要剥离
	return stripDockerLogHeader(raw), nil
}

// LocalContainerAction 执行容器生命周期操作。
func (c *DockerLocalClient) LocalContainerAction(ctx context.Context, name, action string) error {
	return c.post(ctx, "/containers/"+name+"/"+action, nil)
}

// LocalRemove 删除容器。
func (c *DockerLocalClient) LocalRemove(ctx context.Context, name string, force bool) error {
	path := "/containers/" + name
	if force {
		path += "?force=true"
	}
	resp, err := c.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("remove %s: %s %s", name, resp.Status, string(raw))
	}
	return nil
}

// LocalRemoveImage 删除镜像。
func (c *DockerLocalClient) LocalRemoveImage(ctx context.Context, ref string) error {
	resp, err := c.do(ctx, http.MethodDelete, "/images/"+ref, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("rmi %s: %s %s", ref, resp.Status, string(raw))
	}
	return nil
}


// LocalPull 拉取镜像（流式返回进度，Docker API 的 /images/create 返回 JSON 流）。
func (c *DockerLocalClient) LocalPull(ctx context.Context, ref string) (string, error) {
	// /images/create?fromImage=<ref> 返回逐行 JSON：{"status":"...","id":"..."}
	resp, err := c.do(ctx, http.MethodPost, "/images/create?fromImage="+urlEncode(ref)+"&tag=latest", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("pull %s: %s %s", ref, resp.Status, string(raw))
	}
	return parsePullProgress(raw), nil
}

// postRaw 执行 POST 并返回原始 body（用于 prune 等需解析特殊响应的场景）。
func (c *DockerLocalClient) postRaw(ctx context.Context, path string, body any) ([]byte, error) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = strings.NewReader(string(b))
	}
	resp, err := c.do(ctx, http.MethodPost, path, r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return raw, fmt.Errorf("docker API %s: %s", path, resp.Status)
	}
	return raw, nil
}

// LocalPruneResult 带 JSON 解析的清理结果。
func (c *DockerLocalClient) LocalPruneResult(ctx context.Context, typ string) (map[string]any, error) {
	path := ""
	switch typ {
	case "container":
		path = "/containers/prune"
	case "image":
		path = "/images/prune"
	case "volume":
		path = "/volumes/prune"
	case "network":
		path = "/networks/prune"
	default:
		return nil, fmt.Errorf("unknown prune type: %s", typ)
	}
	raw, err := c.postRaw(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return map[string]any{"raw": string(raw)}, nil
	}
	return result, nil
}

// LocalVolumeCreate 创建卷。
func (c *DockerLocalClient) LocalVolumeCreate(ctx context.Context, name, driver string) error {
	body := map[string]any{"Name": name}
	if driver != "" {
		body["Driver"] = driver
	}
	return c.post(ctx, "/volumes/create", body)
}

// LocalVolumeRemove 删除卷。
func (c *DockerLocalClient) LocalVolumeRemove(ctx context.Context, name string, force bool) error {
	path := "/volumes/" + name
	resp, err := c.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("remove volume %s: %s %s", name, resp.Status, string(raw))
	}
	return nil
}

// LocalNetworkCreate 创建网络。
func (c *DockerLocalClient) LocalNetworkCreate(ctx context.Context, name, driver string) error {
	body := map[string]any{"Name": name}
	if driver != "" {
		body["Driver"] = driver
	}
	return c.post(ctx, "/networks/create", body)
}

// LocalNetworkRemove 删除网络。
func (c *DockerLocalClient) LocalNetworkRemove(ctx context.Context, name string) error {
	resp, err := c.do(ctx, http.MethodDelete, "/networks/"+name, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("remove network %s: %s %s", name, resp.Status, string(raw))
	}
	return nil
}

// parsePullProgress 从 Docker /images/create 的 JSON 流提取人类可读进度。
func parsePullProgress(raw []byte) string {
	var lines []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry struct {
			Status   string `json:"status"`
			ID       string `json:"id"`
			Error    string `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Error != "" {
			lines = append(lines, "ERROR: "+entry.Error)
			continue
		}
		if entry.ID != "" {
			lines = append(lines, entry.ID+": "+entry.Status)
		} else if entry.Status != "" {
			lines = append(lines, entry.Status)
		}
	}
	return strings.Join(lines, "\n")
}

// urlEncode 对镜像名做 URL 编码（处理仓库地址中的特殊字符）。
func urlEncode(s string) string {
	return strings.NewReplacer(" ", "%20", "/", "%2F").Replace(s)
}
