package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"aiproxy/internal/models"
)

// runAliases 模型别名管理命令分发。
func runAliases(dbPath string, jsonOut bool, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(os.Stdout, `模型别名管理:
  aliases list                  列出全部模型别名
  aliases create                新增模型别名
  aliases update                修改模型别名
  aliases delete --id <ID>      删除模型别名
  aliases enable --id <ID>      启用模型别名
  aliases disable --id <ID>     停用模型别名

常用参数:
  --id <ID>         别名 ID
  --name <名称>     别名，如 "all"（全局唯一）
  --targets <配置>  目标模型 JSON 数组，如 [{"model":"gpt-4o","weight":2},{"model":"claude-3-5-sonnet"}]
                    （或使用 --model 多次指定简化形式）
  --model <模型>    目标模型名（可多次指定，权重均等；与 --targets 二选一）
  --enabled <bool>  是否启用（默认 true）
`)
		return nil
	}

	sub := args[0]
	subArgs := args[1:]
	switch sub {
	case "list":
		return aliasList(dbPath, jsonOut, subArgs)
	case "create":
		return aliasCreate(dbPath, jsonOut, subArgs)
	case "update":
		return aliasUpdate(dbPath, jsonOut, subArgs)
	case "delete":
		return aliasDelete(dbPath, jsonOut, subArgs)
	case "enable":
		return aliasSetEnabled(dbPath, jsonOut, true, subArgs)
	case "disable":
		return aliasSetEnabled(dbPath, jsonOut, false, subArgs)
	default:
		return fmt.Errorf("未知别名命令: %s（可用: list/create/update/delete/enable/disable）", sub)
	}
}

// registerAliasTargetFlags 注册目标模型 flags（--targets / --model）到 fs，不解析。
func registerAliasTargetFlags(fs *flag.FlagSet, targetsJSON *string, modelsList *multiFlag) {
	fs.StringVar(targetsJSON, "targets", "", "目标模型 JSON 数组")
	fs.Var(modelsList, "model", "目标模型名（可多次指定）")
}

// resolveAliasTargets 由已解析的 flag 值生成 targets JSON 字符串。
func resolveAliasTargets(targetsJSON string, modelsList multiFlag) (string, error) {
	if targetsJSON != "" {
		if _, err := models.ParseAliasTargets(targetsJSON); err != nil {
			return "", fmt.Errorf("--targets 格式错误: %v", err)
		}
		return targetsJSON, nil
	}
	if len(modelsList) == 0 {
		return "", errors.New("缺少目标模型（--targets JSON 或 --model 至少提供一个）")
	}
	// 兼容两种写法：--model gpt-4o --model gpt-4o-mini（多次指定），
	// 以及 --model "gpt-4o,gpt-4o-mini"（逗号分隔，自动拆分）。
	targets := make([]models.ModelAliasTarget, 0, len(modelsList))
	for _, m := range modelsList {
		for _, part := range strings.Split(m, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			targets = append(targets, models.ModelAliasTarget{Model: part, Weight: 1})
		}
	}
	if len(targets) == 0 {
		return "", errors.New("目标模型名不能为空")
	}
	b, err := json.Marshal(targets)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// aliasList 列出所有模型别名。
func aliasList(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("list", "aliases list")
	if err := fs.Parse(args); err != nil {
		return err
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	aliases, err := comps.AliasStore.List()
	if err != nil {
		return err
	}

	if jsonOut {
		return printJSON(aliases)
	}

	t := newTablePrinter(os.Stdout)
	t.addRow("ID", "别名", "目标模型", "启用")
	for _, a := range aliases {
		list, err := a.ParseTargets()
		if err != nil {
			t.addRow(strconv.FormatInt(a.ID, 10), a.Name, a.Targets, boolYesNo(a.Enabled))
			continue
		}
		t.addRow(strconv.FormatInt(a.ID, 10), a.Name, list.Render(), boolYesNo(a.Enabled))
	}
	t.print()
	return nil
}

// aliasCreate 新增模型别名。
func aliasCreate(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("create", "aliases create --name <别名> (--targets <JSON> | --model <模型>...) [--enabled <bool>]")
	name := fs.String("name", "", "别名名称")
	enabled := fs.Bool("enabled", true, "是否启用（默认 true）")
	var (
		targetsJSON string
		modelsList  multiFlag
	)
	registerAliasTargetFlags(fs, &targetsJSON, &modelsList)
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*name) == "" {
		return errors.New("缺少参数 --name（别名名称）")
	}
	targets, err := resolveAliasTargets(targetsJSON, modelsList)
	if err != nil {
		return err
	}
	targetList, err := models.ParseAliasTargets(targets)
	if err != nil {
		return err
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	a := models.NewModelAlias(strings.TrimSpace(*name), targetList, *enabled)
	id, err := comps.AliasStore.Create(a)
	if err != nil {
		return err
	}

	if jsonOut {
		return printJSON(map[string]any{
			"id":      id,
			"name":    a.Name,
			"targets": a.Targets,
			"enabled": a.Enabled,
		})
	}
	fmt.Printf("已创建模型别名: %s（ID: %d）\n", a.Name, id)
	return nil
}

// aliasUpdate 修改模型别名。
func aliasUpdate(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("update", "aliases update --id <ID> [--name <别名>] [--targets <JSON> | --model <模型>...] [--enabled true|false|keep]")
	id := fs.Int64("id", 0, "别名 ID")
	name := fs.String("name", "", "别名名称")
	enabled := fs.String("enabled", "keep", "启用状态: true/false/keep（keep 表示不修改）")
	var (
		targetsJSON string
		modelsList  multiFlag
	)
	registerAliasTargetFlags(fs, &targetsJSON, &modelsList)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id <= 0 {
		return errors.New("缺少参数 --id（别名 ID，正整数）")
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	old, err := comps.AliasStore.Get(*id)
	if err != nil {
		return err
	}

	newA := *old
	if flagProvided(args, "name") {
		if strings.TrimSpace(*name) == "" {
			return errors.New("别名名称不能为空")
		}
		newA.Name = strings.TrimSpace(*name)
	}
	if flagProvided(args, "targets") || flagProvided(args, "model") {
		targets, err := resolveAliasTargets(targetsJSON, modelsList)
		if err != nil {
			return err
		}
		newA.Targets = targets
	}

	switch *enabled {
	case "true":
		newA.Enabled = true
	case "false":
		newA.Enabled = false
	case "keep":
	default:
		return fmt.Errorf("参数 --enabled 取值无效: %s（可选 true/false/keep）", *enabled)
	}

	if err := comps.AliasStore.Update(&newA); err != nil {
		return err
	}

	if jsonOut {
		return printJSON(map[string]any{"id": newA.ID, "updated": true})
	}
	fmt.Printf("已更新模型别名: %s（ID: %d）\n", newA.Name, newA.ID)
	return nil
}

// aliasDelete 删除模型别名。
func aliasDelete(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("delete", "aliases delete --id <ID>")
	id := fs.Int64("id", 0, "别名 ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id <= 0 {
		return errors.New("缺少参数 --id（别名 ID，正整数）")
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	a, err := comps.AliasStore.Get(*id)
	if err != nil {
		return err
	}
	if err := comps.AliasStore.Delete(*id); err != nil {
		return err
	}

	if jsonOut {
		return printJSON(map[string]any{"id": *id, "deleted": true})
	}
	fmt.Printf("已删除模型别名: %s（ID: %d）\n", a.Name, *id)
	return nil
}

// aliasSetEnabled 启用/停用模型别名。
func aliasSetEnabled(dbPath string, jsonOut bool, enabled bool, args []string) error {
	fs := newFlagSet("set-enabled", "aliases enable|disable --id <ID>")
	id := fs.Int64("id", 0, "别名 ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id <= 0 {
		return errors.New("缺少参数 --id（别名 ID，正整数）")
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	a, err := comps.AliasStore.Get(*id)
	if err != nil {
		return err
	}
	if err := comps.AliasStore.SetEnabled(*id, enabled); err != nil {
		return err
	}

	verb := "已启用"
	if !enabled {
		verb = "已停用"
	}
	if jsonOut {
		return printJSON(map[string]any{"id": *id, "enabled": enabled})
	}
	fmt.Printf("%s模型别名: %s（ID: %d）\n", verb, a.Name, *id)
	return nil
}
