// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

import (
	"context"
	"fmt"
	"time"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/plugins"
)

func registerCloudTools(reg *plugins.HostToolRegistry, cl *chain.CloudChain) {
	reg.Register(plugins.HostTool{
		Name: "cloud_list", Category: "cloud", Permission: "cloud:read",
		Description: "List configured cloud drives (115/夸克/123/Alist).",
		InputSchema: noArgsSchema(),
	}, func(ctx context.Context, _ map[string]any) (any, error) {
		return cl.List()
	})
	cloudDriveProps := map[string]any{
		"name": strProp("Display name"), "type": strProp("115 | quark | 123"),
		"token": strProp("Provider cookie or access token; write-only"), "root_path": strProp("Accessible root path"),
		"is_default": boolProp("Use as the default cloud drive"),
	}
	reg.Register(plugins.HostTool{
		Name: "cloud_create", Category: "cloud", Permission: "cloud:write",
		Description: "Create a cloud-drive configuration. Credentials are encrypted by the host.",
		InputSchema: objSchema(cloudDriveProps, []string{"name", "type", "token"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		var in oper.CloudDriveInput
		if err := decodeToolInput(a, &in); err != nil {
			return nil, mcpErr(err)
		}
		return cl.Create(in)
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_update", Category: "cloud", Permission: "cloud:write",
		Description: "Update a cloud-drive configuration; omit token to keep the existing credential.",
		InputSchema: objSchema(map[string]any{"id": intProp("Cloud drive id"), "name": cloudDriveProps["name"], "type": cloudDriveProps["type"], "token": cloudDriveProps["token"], "root_path": cloudDriveProps["root_path"], "is_default": cloudDriveProps["is_default"]}, []string{"id", "name", "type"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		var in oper.CloudDriveInput
		if err := decodeToolInput(a, &in); err != nil {
			return nil, mcpErr(err)
		}
		return cl.Update(uint(argInt(a, "id")), in)
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_delete", Category: "cloud", Permission: "cloud:write", Destructive: true,
		Description: "Delete a cloud-drive configuration.",
		InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := cl.Delete(uint(argInt(a, "id")))
		return okCount("deleted", 1, mcpErr(err))
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_test", Category: "cloud", Permission: "cloud:read",
		Description: "Test connectivity of a configured cloud drive.",
		InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		message, err := cl.Test(ctx, uint(argInt(a, "id")))
		if err != nil {
			return nil, mcpErr(err)
		}
		return map[string]any{"ok": true, "message": message}, nil
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_browse", Category: "cloud", Permission: "cloud:read",
		Description: "Browse a cloud drive directory.",
		InputSchema: objSchema(map[string]any{"drive_id": intProp(""), "path": strProp("")}, []string{"drive_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return cl.Browse(ctx, uint(argInt(a, "drive_id")), argString(a, "path", "/"))
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_quota", Category: "cloud", Permission: "cloud:read",
		Description: "Query storage quota of a cloud drive.",
		InputSchema: objSchema(map[string]any{"drive_id": intProp("")}, []string{"drive_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return cl.Quota(ctx, uint(argInt(a, "drive_id")))
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_rules", Category: "cloud", Permission: "cloud:read",
		Description: "List STRM generation rules.",
		InputSchema: noArgsSchema(),
	}, func(ctx context.Context, _ map[string]any) (any, error) {
		return cl.ListRules()
	})
	ruleProps := map[string]any{
		"name": strProp(""), "drive_id": intProp(""), "cloud_path": strProp(""), "output_dir": strProp(""),
		"mount_root": strProp(""), "template": strProp(""), "recursive": boolProp(""), "keep_ext": boolProp(""), "enabled": boolProp(""),
	}
	reg.Register(plugins.HostTool{
		Name: "cloud_rule_create", Category: "cloud", Permission: "cloud:write",
		Description: "Create a STRM generation rule.",
		InputSchema: objSchema(ruleProps, []string{"name", "drive_id", "cloud_path", "output_dir"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		var in oper.StrmRuleInput
		if err := decodeToolInput(a, &in); err != nil {
			return nil, mcpErr(err)
		}
		return cl.CreateRule(in)
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_rule_update", Category: "cloud", Permission: "cloud:write",
		Description: "Update a STRM generation rule. Omitted fields keep their current values.",
		InputSchema: objSchema(map[string]any{"id": intProp(""), "name": ruleProps["name"], "drive_id": ruleProps["drive_id"], "cloud_path": ruleProps["cloud_path"], "output_dir": ruleProps["output_dir"], "mount_root": ruleProps["mount_root"], "template": ruleProps["template"], "recursive": ruleProps["recursive"], "keep_ext": ruleProps["keep_ext"], "enabled": ruleProps["enabled"]}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		var in oper.StrmRuleInput
		if err := decodeToolInput(a, &in); err != nil {
			return nil, mcpErr(err)
		}
		return cl.UpdateRule(uint(argInt(a, "id")), in)
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_rule_delete", Category: "cloud", Permission: "cloud:write", Destructive: true,
		Description: "Delete a STRM generation rule.",
		InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := cl.DeleteRule(uint(argInt(a, "id")))
		return okCount("deleted", 1, mcpErr(err))
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_records", Category: "cloud", Permission: "cloud:read",
		Description: "List generated STRM records, optionally filtered by cloud drive.",
		InputSchema: objSchema(map[string]any{"drive_id": intProp("0 lists all drives")}, nil),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return cl.ListRecords(uint(argInt(a, "drive_id")))
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_record_delete", Category: "cloud", Permission: "cloud:write", Destructive: true,
		Description: "Delete a generated STRM record.",
		InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := cl.DeleteRecord(uint(argInt(a, "id")))
		return okCount("deleted", 1, mcpErr(err))
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_generate_strm", Category: "cloud", Permission: "cloud:write",
		Description: "Generate .strm files for a STRM rule.",
		InputSchema: objSchema(map[string]any{"rule_id": intProp("")}, []string{"rule_id"}),
		Timeout:     60 * time.Second,
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return cl.GenerateStrm(ctx, uint(argInt(a, "rule_id")))
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_generate_all", Category: "cloud", Permission: "cloud:write",
		Description: "Generate .strm files for all enabled rules; returns the cross-rule summary (rules/generated/failed).",
		InputSchema: noArgsSchema(),
	}, func(ctx context.Context, _ map[string]any) (any, error) {
		// 原先 go + {ok:true}：所有规则都失败（凭据过期、目录不存在）也叫成功，插件据此认为 .strm 已落地。
		// 同步跑完并回汇总；超时由 host 的工具超时兜底，比假成功可信。
		return cl.GenerateAll(ctx), nil
	})
}

// ===== 刷流 =====

func registerBrushFlowTools(reg *plugins.HostToolRegistry, bf *chain.BrushFlowChain) {
	reg.Register(plugins.HostTool{
		Name: "brushflow_tasks_list", Category: "brushflow", Permission: "brushflow:read",
		Description: "List brush-flow tasks.",
		InputSchema: noArgsSchema(),
	}, func(ctx context.Context, _ map[string]any) (any, error) {
		return bf.ListTasks()
	})
	brushProps := map[string]any{
		"name": strProp(""), "site_id": intProp(""), "downloader_id": intProp(""), "enabled": boolProp(""),
		"keyword": strProp(""), "category": strProp(""), "min_size_mb": intProp(""), "max_size_mb": intProp(""),
		"min_seeders": intProp(""), "max_seeders": intProp(""), "free_only": boolProp(""), "paused": boolProp(""),
		"filter_groups": strProp(""), "delete_ratio": numberProp(""), "delete_seed_hours": numberProp(""),
		"delete_upload_mb": intProp(""), "delete_files": boolProp(""), "max_seeds": intProp(""),
		"save_path": strProp(""), "category_dl": strProp(""),
	}
	reg.Register(plugins.HostTool{
		Name: "brushflow_task_create", Category: "brushflow", Permission: "brushflow:write",
		Description: "Create a brush-flow task.",
		InputSchema: objSchema(brushProps, []string{"name", "site_id", "downloader_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		var task models.BrushFlowTask
		if err := decodeToolInput(a, &task); err != nil {
			return nil, mcpErr(err)
		}
		if task.Name == "" || task.SiteID == 0 || task.DownloaderID == 0 {
			return nil, fmt.Errorf("缺少必填参数：name、site_id、downloader_id 都不能为空")
		}
		if task.MaxSeeds <= 0 {
			task.MaxSeeds = 10
		}
		if err := bf.CreateTask(&task); err != nil {
			return nil, mcpErr(err)
		}
		return task, nil
	})
	reg.Register(plugins.HostTool{
		Name: "brushflow_task_update", Category: "brushflow", Permission: "brushflow:write",
		Description: "Update a brush-flow task. Omitted fields keep their current values.",
		InputSchema: objSchema(map[string]any{"id": intProp(""), "name": brushProps["name"], "site_id": brushProps["site_id"], "downloader_id": brushProps["downloader_id"], "enabled": brushProps["enabled"], "keyword": brushProps["keyword"], "category": brushProps["category"], "min_size_mb": brushProps["min_size_mb"], "max_size_mb": brushProps["max_size_mb"], "min_seeders": brushProps["min_seeders"], "max_seeders": brushProps["max_seeders"], "free_only": brushProps["free_only"], "paused": brushProps["paused"], "filter_groups": brushProps["filter_groups"], "delete_ratio": brushProps["delete_ratio"], "delete_seed_hours": brushProps["delete_seed_hours"], "delete_upload_mb": brushProps["delete_upload_mb"], "delete_files": brushProps["delete_files"], "max_seeds": brushProps["max_seeds"], "save_path": brushProps["save_path"], "category_dl": brushProps["category_dl"]}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		task, err := bf.GetTask(uint(argInt(a, "id")))
		if err != nil {
			return nil, mcpErr(err)
		}
		id, totalAdded, totalDeleted, totalUpload := task.ID, task.TotalAdded, task.TotalDeleted, task.TotalUpload
		createdAt, lastSelectAt, lastSweepAt, lastError := task.CreatedAt, task.LastSelectAt, task.LastSweepAt, task.LastError
		if err := decodeToolInput(a, task); err != nil {
			return nil, mcpErr(err)
		}
		task.ID, task.TotalAdded, task.TotalDeleted, task.TotalUpload = id, totalAdded, totalDeleted, totalUpload
		task.CreatedAt, task.LastSelectAt, task.LastSweepAt, task.LastError = createdAt, lastSelectAt, lastSweepAt, lastError
		if err := bf.UpdateTask(task); err != nil {
			return nil, mcpErr(err)
		}
		return task, nil
	})
	reg.Register(plugins.HostTool{
		Name: "brushflow_task_delete", Category: "brushflow", Permission: "brushflow:write", Destructive: true,
		Description: "Delete a brush-flow task.",
		InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := bf.DeleteTask(uint(argInt(a, "id")))
		return okCount("deleted", 1, mcpErr(err))
	})
	reg.Register(plugins.HostTool{
		Name: "brushflow_task_get", Category: "brushflow", Permission: "brushflow:read",
		Description: "Get a brush-flow task by id.",
		InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return bf.GetTask(uint(argInt(a, "id")))
	})
	reg.Register(plugins.HostTool{
		Name: "brushflow_seeds", Category: "brushflow", Permission: "brushflow:read",
		Description: "List seed history of a brush-flow task.",
		InputSchema: objSchema(map[string]any{"task_id": intProp("")}, []string{"task_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return bf.ListSeeds(uint(argInt(a, "task_id")))
	})
	reg.Register(plugins.HostTool{
		Name: "brushflow_run_select", Category: "brushflow", Permission: "brushflow:write",
		Description: "Manually trigger seed selection of a brush-flow task. Fails if the task is disabled (a disabled task does nothing).",
		InputSchema: objSchema(map[string]any{"task_id": intProp("")}, []string{"task_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		// 与 agent 面共用：未启用要失败，跑完要把任务诊断带回，
		// 否则 okCount(...,1,nil) 把空跑说成「已选种」。
		return brushflowRunOutcome(ctx, bf, uint(argInt(a, "task_id")), "select", "selected", bf.RunSelect)
	})
	reg.Register(plugins.HostTool{
		Name: "brushflow_run_sweep", Category: "brushflow", Permission: "brushflow:write",
		Description: "Manually trigger sweep (delete) of a brush-flow task. Fails if the task is disabled (a disabled task deletes nothing).",
		InputSchema: objSchema(map[string]any{"task_id": intProp("")}, []string{"task_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return brushflowRunOutcome(ctx, bf, uint(argInt(a, "task_id")), "sweep", "swept", bf.RunSweep)
	})
}

// ===== RSS =====
