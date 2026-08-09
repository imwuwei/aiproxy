package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"aiproxy/internal/models"
)

// runChannels 渠道管理命令分发。
func runChannels(dbPath string, jsonOut bool, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(os.Stdout, `渠道管理:
  channels list                  列出全部渠道
  channels create                新增渠道
  channels update                修改渠道
  channels delete --id <ID>      删除渠道
  channels enable --id <ID>      启用渠道（自动触发模型同步）
  channels disable --id <ID>     停用渠道
  channels test --id <ID>        测试渠道连通性（返回模型数）
  channels sync --id <ID>        同步单个渠道模型

常用参数:
  --id <ID>          渠道 ID
  --name <名称>      渠道名称
  --type <类型>      openai-compatible | anthropic | gemini | custom
  --base-url <URL>   上游 Base URL
  --api-key <KEY>    上游 API Key（可多次指定）
  --api-keys <K1,K2> 多个 API Key，逗号分隔
  --priority <N>     优先级（越小越优先）
`)
		return nil
	}

	sub := args[0]
	subArgs := args[1:]
	switch sub {
	case "list":
		return channelList(dbPath, jsonOut, subArgs)
	case "create":
		return channelCreate(dbPath, jsonOut, subArgs)
	case "update":
		return channelUpdate(dbPath, jsonOut, subArgs)
	case "delete":
		return channelDelete(dbPath, jsonOut, subArgs)
	case "enable":
		return channelSetEnabled(dbPath, jsonOut, true, subArgs)
	case "disable":
		return channelSetEnabled(dbPath, jsonOut, false, subArgs)
	case "test":
		return channelTest(dbPath, jsonOut, subArgs)
	case "sync":
		return channelSync(dbPath, jsonOut, subArgs)
	default:
		return fmt.Errorf("未知渠道命令: %s（可用: list/create/update/delete/enable/disable/test/sync）", sub)
	}
}

// channelFromFlags 从 flags 解析渠道字段。
func channelFromFlags(fs *flag.FlagSet, args []string) (*models.Channel, error) {
	name := fs.String("name", "", "渠道名称")
	chType := fs.String("type", "", "渠道类型")
	baseURL := fs.String("base-url", "", "上游 Base URL")
	var apiKeys multiFlag
	fs.Var(&apiKeys, "api-key", "上游 API Key（可多次指定）")
	apiKeysCSV := fs.String("api-keys", "", "多个 API Key，逗号分隔")
	priority := fs.Int("priority", 0, "优先级（越小越优先）")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	keys := append([]string{}, apiKeys...)
	if *apiKeysCSV != "" {
		for _, k := range strings.Split(*apiKeysCSV, ",") {
			k = strings.TrimSpace(k)
			if k != "" {
				keys = append(keys, k)
			}
		}
	}

	ch := &models.Channel{
		Name:     *name,
		Type:     models.ChannelType(*chType),
		BaseURL:  *baseURL,
		APIKeys:  keys,
		Priority: *priority,
	}
	return ch, nil
}

// flagProvided 判断 args 中是否显式指定了某 flag（支持 --flag 与 --flag=value 两种形式）。
func flagProvided(args []string, name string) bool {
	for _, a := range args {
		if a == "--"+name || strings.HasPrefix(a, "--"+name+"=") {
			return true
		}
	}
	return false
}

// channelList 列出渠道。
func channelList(dbPath string, jsonOut bool, args []string) error {
	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	channels, err := comps.ChannelStore.List()
	if err != nil {
		return err
	}

	if jsonOut {
		return printJSON(channels)
	}

	t := newTablePrinter(os.Stdout)
	t.addRow("ID", "名称", "类型", "优先级", "状态", "启用", "模型数", "最近错误")
	for _, ch := range channels {
		t.addRow(
			strconv.FormatInt(ch.ID, 10),
			ch.Name,
			channelTypeName(ch.Type),
			strconv.Itoa(ch.Priority),
			string(ch.Status),
			boolYesNo(ch.Enabled),
			strconv.Itoa(ch.ModelCount),
			truncateStr(ch.LastError, 40),
		)
	}
	t.print()
	return nil
}

// channelCreate 新增渠道。
func channelCreate(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("create", "channels create --name <名称> --type <类型> --base-url <URL> --api-key <KEY> [--priority <N>]")
	ch, err := channelFromFlags(fs, args)
	if err != nil {
		return err
	}

	if ok, msg := validateChannel(ch); !ok {
		return errors.New(msg)
	}
	ch.Enabled = true
	ch.Status = models.ChannelStatusOffline

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	id, err := comps.ChannelStore.Create(ch)
	if err != nil {
		return err
	}

	// 创建后自动触发一次模型同步（与 GUI 行为一致）
	_ = comps.ModelSync.SyncChannel(context.Background(), ch)

	if jsonOut {
		return printJSON(map[string]any{
			"id":      id,
			"name":    ch.Name,
			"type":    ch.Type,
			"enabled": ch.Enabled,
		})
	}
	fmt.Printf("已创建渠道: %s（ID: %d）\n", ch.Name, id)
	return nil
}

// channelUpdate 修改渠道。
func channelUpdate(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("update", "channels update --id <ID> [--name <名称>] [--type <类型>] [--base-url <URL>] [--api-key <KEY>] [--priority <N>] [--enabled true|false|keep]")
	id := fs.Int64("id", 0, "渠道 ID")
	enabled := fs.String("enabled", "keep", "启用状态: true/false/keep（keep 表示不修改）")
	ch, err := channelFromFlags(fs, args)
	if err != nil {
		return err
	}
	if *id <= 0 {
		return errors.New("缺少参数 --id（渠道 ID，正整数）")
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	old, err := comps.ChannelStore.Get(*id)
	if err != nil {
		return err
	}

	// 只覆盖显式指定的字段；未传的保留原值
	keysSet := flagProvided(args, "api-key") || flagProvided(args, "api-keys")
	prioritySet := flagProvided(args, "priority")

	newCh := *old
	if flagProvided(args, "name") {
		if strings.TrimSpace(ch.Name) == "" {
			return errors.New("渠道名称不能为空")
		}
		newCh.Name = ch.Name
	}
	if flagProvided(args, "type") {
		if ok, msg := validateType(ch.Type); !ok {
			return errors.New(msg)
		}
		newCh.Type = ch.Type
	}
	if flagProvided(args, "base-url") {
		if strings.TrimSpace(ch.BaseURL) == "" {
			return errors.New("Base URL 不能为空")
		}
		newCh.BaseURL = ch.BaseURL
	}
	if keysSet {
		if len(ch.APIKeys) == 0 {
			return errors.New("API Keys 不能为空（--api-key/--api-keys 至少提供一个）")
		}
		newCh.APIKeys = ch.APIKeys
	}
	if prioritySet {
		newCh.Priority = ch.Priority
	}

	switch *enabled {
	case "true":
		newCh.Enabled = true
	case "false":
		newCh.Enabled = false
	case "keep":
	default:
		return fmt.Errorf("参数 --enabled 取值无效: %s（可选 true/false/keep）", *enabled)
	}

	// 名称非空校验（keep 时保证不产生空名称）
	if strings.TrimSpace(newCh.Name) == "" {
		return errors.New("渠道名称不能为空")
	}

	if err := comps.ChannelStore.Update(&newCh); err != nil {
		return err
	}

	// 最终为启用 → 自动触发模型同步
	if newCh.Enabled {
		_ = comps.ModelSync.SyncChannel(context.Background(), &newCh)
	}

	if jsonOut {
		return printJSON(map[string]any{"id": newCh.ID, "updated": true})
	}
	fmt.Printf("已更新渠道: %s（ID: %d）\n", newCh.Name, newCh.ID)
	return nil
}

// channelDelete 删除渠道。
func channelDelete(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("delete", "channels delete --id <ID>")
	id := fs.Int64("id", 0, "渠道 ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id <= 0 {
		return errors.New("缺少参数 --id（渠道 ID，正整数）")
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	ch, err := comps.ChannelStore.Get(*id)
	if err != nil {
		return err
	}
	if err := comps.ChannelStore.Delete(*id); err != nil {
		return err
	}

	if jsonOut {
		return printJSON(map[string]any{"id": *id, "deleted": true})
	}
	fmt.Printf("已删除渠道: %s（ID: %d）\n", ch.Name, *id)
	return nil
}

// channelSetEnabled 启用/停用渠道。
func channelSetEnabled(dbPath string, jsonOut bool, enabled bool, args []string) error {
	fs := newFlagSet("set-enabled", "channels enable|disable --id <ID>")
	id := fs.Int64("id", 0, "渠道 ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id <= 0 {
		return errors.New("缺少参数 --id（渠道 ID，正整数）")
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	ch, err := comps.ChannelStore.Get(*id)
	if err != nil {
		return err
	}
	if err := comps.ChannelStore.SetEnabled(*id, enabled); err != nil {
		return err
	}
	// 启用后自动触发模型同步（与 GUI 行为一致）
	if enabled {
		ch.Enabled = true
		_ = comps.ModelSync.SyncChannel(context.Background(), ch)
	}

	verb := "已启用"
	if !enabled {
		verb = "已停用"
	}
	if jsonOut {
		return printJSON(map[string]any{"id": *id, "enabled": enabled})
	}
	fmt.Printf("%s渠道: %s（ID: %d）\n", verb, ch.Name, *id)
	return nil
}

// channelTest 测试渠道连通性。
func channelTest(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("test", "channels test --id <ID>")
	id := fs.Int64("id", 0, "渠道 ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id <= 0 {
		return errors.New("缺少参数 --id（渠道 ID，正整数）")
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	ch, err := comps.ChannelStore.Get(*id)
	if err != nil {
		return err
	}
	n, err := comps.ModelSync.TestChannel(ch)
	if err != nil {
		if jsonOut {
			printJSONError(err)
		}
		return fmt.Errorf("渠道 %s 测试失败: %v", ch.Name, err)
	}

	if jsonOut {
		return printJSON(map[string]any{"id": *id, "name": ch.Name, "models": n, "ok": true})
	}
	fmt.Printf("渠道可用: %s（ID: %d），共 %d 个模型\n", ch.Name, *id, n)
	return nil
}

// channelSync 同步单个渠道模型。
func channelSync(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("sync", "channels sync --id <ID>")
	id := fs.Int64("id", 0, "渠道 ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id <= 0 {
		return errors.New("缺少参数 --id（渠道 ID，正整数）")
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	ch, err := comps.ChannelStore.Get(*id)
	if err != nil {
		return err
	}
	if err := comps.ModelSync.SyncChannel(context.Background(), ch); err != nil {
		if jsonOut {
			printJSONError(err)
		}
		return fmt.Errorf("渠道 %s 同步失败: %v", ch.Name, err)
	}
	modelsList, err := comps.ModelStore.ListByChannel(*id)
	if err != nil {
		return err
	}

	if jsonOut {
		return printJSON(map[string]any{"id": *id, "name": ch.Name, "models": modelsList})
	}
	fmt.Printf("渠道 %s（ID: %d）同步完成，共 %d 个模型\n", ch.Name, *id, len(modelsList))
	return nil
}

// validateChannel 校验新增渠道字段。
func validateChannel(ch *models.Channel) (bool, string) {
	if strings.TrimSpace(ch.Name) == "" {
		return false, "渠道名称不能为空"
	}
	if ok, msg := validateType(ch.Type); !ok {
		return false, msg
	}
	if strings.TrimSpace(ch.BaseURL) == "" {
		return false, "Base URL 不能为空"
	}
	if len(ch.APIKeys) == 0 {
		return false, "至少提供一个 API Key（--api-key 或 --api-keys）"
	}
	return true, ""
}

// validateType 校验渠道类型。
func validateType(t models.ChannelType) (bool, string) {
	switch t {
	case models.ChannelTypeOpenAICompatible, models.ChannelTypeAnthropic, models.ChannelTypeGemini, models.ChannelTypeCustom:
		return true, ""
	default:
		return false, fmt.Sprintf("不支持的渠道类型: %s（可选 openai-compatible/anthropic/gemini/custom）", t)
	}
}

// channelTypeName 渠道类型中文名。
func channelTypeName(t models.ChannelType) string {
	switch t {
	case models.ChannelTypeOpenAICompatible:
		return "OpenAI 兼容"
	case models.ChannelTypeAnthropic:
		return "Anthropic"
	case models.ChannelTypeGemini:
		return "Gemini"
	case models.ChannelTypeCustom:
		return "自定义"
	default:
		return string(t)
	}
}

// boolYesNo 布尔值中文显示。
func boolYesNo(b bool) string {
	if b {
		return "是"
	}
	return "否"
}

// truncateStr 超长字符串截断（按 rune，避免中文乱码）。
func truncateStr(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}

// multiFlag 支持重复指定的字符串 flag（如 --api-key）。
type multiFlag []string

func (m *multiFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
