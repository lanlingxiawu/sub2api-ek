package service

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// groupMappingRegexCache 缓存分组模型映射中已编译的正则表达式（自动添加全串锚定）。
// key: 去掉 "~" 前缀后的原始正则字符串，value: 编译后的 *regexp.Regexp。
var groupMappingRegexCache sync.Map

// getGroupMappingRegex 返回对应 rawPattern 的编译正则，自动添加 ^(?:...)$ 全串锚定。
// 编译结果在进程生命周期内缓存，避免热路径重复 Compile。
func getGroupMappingRegex(rawPattern string) (*regexp.Regexp, error) {
	anchored := `^(?:` + rawPattern + `)$`
	if v, ok := groupMappingRegexCache.Load(anchored); ok {
		return v.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(anchored)
	if err != nil {
		return nil, err
	}
	actual, _ := groupMappingRegexCache.LoadOrStore(anchored, re)
	return actual.(*regexp.Regexp), nil
}

func NormalizeGroupModelMapping(mapping map[string]string) (map[string]string, error) {
	if mapping == nil {
		return nil, nil
	}
	normalized := make(map[string]string, len(mapping))
	for source, target := range mapping {
		source = strings.TrimSpace(source)
		target = strings.TrimSpace(target)
		if source == "" {
			return nil, fmt.Errorf("model_mapping source cannot be empty")
		}
		if target == "" {
			return nil, fmt.Errorf("model_mapping target for %q cannot be empty", source)
		}
		if strings.Contains(target, "*") {
			return nil, fmt.Errorf("model_mapping target for %q cannot contain wildcard", source)
		}
		if strings.HasPrefix(source, "~") {
			rawPattern := strings.TrimSpace(strings.TrimPrefix(source, "~"))
			if rawPattern == "" {
				return nil, fmt.Errorf("model_mapping regex source cannot be empty")
			}
			if _, err := getGroupMappingRegex(rawPattern); err != nil {
				return nil, fmt.Errorf("invalid model_mapping regex %q: %w", source, err)
			}
			source = "~" + rawPattern
		} else if count := strings.Count(source, "*"); count > 0 {
			if count > 1 || !strings.HasSuffix(source, "*") {
				return nil, fmt.Errorf("model_mapping wildcard source %q must use a single trailing *", source)
			}
		}
		if _, exists := normalized[source]; exists {
			return nil, fmt.Errorf("duplicate model_mapping source %q", source)
		}
		normalized[source] = target
	}
	return normalized, nil
}

type OpenAIMessagesDispatchModelConfig = domain.OpenAIMessagesDispatchModelConfig
type GroupModelsListConfig = domain.GroupModelsListConfig

type Group struct {
	ID             int64
	Name           string
	Description    string
	Platform       string
	RateMultiplier float64
	IsExclusive    bool
	Status         string
	Hydrated       bool // indicates the group was loaded from a trusted repository source

	SubscriptionType    string
	DailyLimitUSD       *float64
	WeeklyLimitUSD      *float64
	MonthlyLimitUSD     *float64
	DefaultValidityDays int

	// 图片生成计费配置（antigravity 和 gemini 平台使用）
	AllowImageGeneration bool
	ImageRateIndependent bool
	ImageRateMultiplier  float64
	ImagePrice1K         *float64
	ImagePrice2K         *float64
	ImagePrice4K         *float64

	// Claude Code 客户端限制
	ClaudeCodeOnly  bool
	FallbackGroupID *int64
	// 无效请求兜底分组（仅 anthropic 平台使用）
	FallbackGroupIDOnInvalidRequest *int64

	// 模型路由配置
	// key: 模型匹配模式（支持 * 通配符，如 "claude-opus-*"）
	// value: 优先账号 ID 列表
	ModelRouting        map[string][]int64
	ModelRoutingEnabled bool

	// MCP XML 协议注入开关（仅 antigravity 平台使用）
	MCPXMLInject bool

	// 支持的模型系列（仅 antigravity 平台使用）
	// 可选值: claude, gemini_text, gemini_image
	SupportedModelScopes []string

	// 分组排序
	SortOrder int

	// OpenAI Messages 调度配置（仅 openai 平台使用）
	AllowMessagesDispatch       bool
	RequireOAuthOnly            bool // 仅允许非 apikey 类型账号关联（OpenAI/Antigravity/Anthropic/Gemini）
	RequirePrivacySet           bool // 调度时仅允许 privacy 已成功设置的账号（OpenAI/Antigravity/Anthropic/Gemini）
	DefaultMappedModel          string
	MessagesDispatchModelConfig OpenAIMessagesDispatchModelConfig
	ModelsListConfig            GroupModelsListConfig

	// RPMLimit 分组级每分钟请求数上限（0 = 不限制）。
	// 一旦设置即接管该分组用户的限流（覆盖用户级 rpm_limit），可被 user-group rpm_override 进一步覆盖。
	RPMLimit int

	// xiugai 修改自动映射功能
	// 分组级模型映射（分组维度，优先于渠道/账号级映射）。
	// key: 匹配模式，value: 目标模型名。
	// 匹配规则（优先级由高到低）：
	//   1. 精确匹配
	//   2. 正则匹配：pattern 以 "~" 开头，去掉前缀后作为 Go regexp 编译匹配
	//   3. 通配符匹配：pattern 以 "*" 结尾，匹配对应前缀
	ModelMapping map[string]string
	// xiugai end

	CreatedAt time.Time
	UpdatedAt time.Time

	AccountGroups           []AccountGroup
	AccountCount            int64
	ActiveAccountCount      int64
	RateLimitedAccountCount int64
}

func (g *Group) IsActive() bool {
	return g.Status == StatusActive
}

func (g *Group) IsSubscriptionType() bool {
	return g.SubscriptionType == SubscriptionTypeSubscription
}

func (g *Group) HasDailyLimit() bool {
	return g.DailyLimitUSD != nil && *g.DailyLimitUSD > 0
}

func (g *Group) HasWeeklyLimit() bool {
	return g.WeeklyLimitUSD != nil && *g.WeeklyLimitUSD > 0
}

func (g *Group) HasMonthlyLimit() bool {
	return g.MonthlyLimitUSD != nil && *g.MonthlyLimitUSD > 0
}

// GetImagePrice 根据 image_size 返回对应的图片生成价格
// 如果分组未配置价格，返回 nil（调用方应使用默认值）
func (g *Group) GetImagePrice(imageSize string) *float64 {
	switch imageSize {
	case "1K":
		return g.ImagePrice1K
	case "2K":
		return g.ImagePrice2K
	case "4K":
		return g.ImagePrice4K
	default:
		// 未知尺寸默认按 2K 计费
		return g.ImagePrice2K
	}
}

// IsGroupContextValid reports whether a group from context has the fields required for routing decisions.
func IsGroupContextValid(group *Group) bool {
	if group == nil {
		return false
	}
	if group.ID <= 0 {
		return false
	}
	if !group.Hydrated {
		return false
	}
	if group.Platform == "" || group.Status == "" {
		return false
	}
	return true
}

// GetRoutingAccountIDs 根据请求模型获取路由账号 ID 列表
// 返回匹配的优先账号 ID 列表，如果没有匹配规则则返回 nil
func (g *Group) GetRoutingAccountIDs(requestedModel string) []int64 {
	if !g.ModelRoutingEnabled || len(g.ModelRouting) == 0 || requestedModel == "" {
		return nil
	}

	// 1. 精确匹配优先
	if accountIDs, ok := g.ModelRouting[requestedModel]; ok && len(accountIDs) > 0 {
		return accountIDs
	}

	// 2. 通配符匹配（前缀匹配）
	for pattern, accountIDs := range g.ModelRouting {
		if matchModelPattern(pattern, requestedModel) && len(accountIDs) > 0 {
			return accountIDs
		}
	}

	return nil
}

// matchModelPattern 检查模型是否匹配模式
// 支持 * 通配符，如 "claude-opus-*" 匹配 "claude-opus-4-20250514"
func matchModelPattern(pattern, model string) bool {
	if pattern == model {
		return true
	}

	// 处理 * 通配符（仅支持末尾通配符）
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(model, prefix)
	}

	return false
}

// xiugai 修改自动映射功能
// ResolveGroupMappedModel 查找分组级别的模型映射。
// matched=true 表示命中规则（即使映射结果与原模型名相同）。
// 匹配优先级：精确 > 正则（~ 前缀）> 通配符（* 后缀），同级按 pattern 长度降序取最长匹配。
// 目标值为空字符串的规则视为未配置，不会命中（避免产生误导性的 "model is required" 错误）。
// 正则匹配自动添加全串锚定（^(?:...)$），防止子串误匹配，并在进程级缓存编译结果。
func (g *Group) ResolveGroupMappedModel(requestedModel string) (mappedModel string, matched bool) {
	if g == nil || len(g.ModelMapping) == 0 || requestedModel == "" {
		return requestedModel, false
	}
	// 1. 精确匹配（Bug4: 跳过空目标）
	if target, ok := g.ModelMapping[requestedModel]; ok && target != "" {
		return target, true
	}
	// 2. 正则匹配（pattern 以 "~" 开头）
	type candidate struct {
		pattern string
		target  string
	}
	var regexCandidates, wildcardCandidates []candidate
	for pattern, target := range g.ModelMapping {
		if target == "" {
			continue // Bug4: 跳过空目标，不加入候选
		}
		if strings.HasPrefix(pattern, "~") {
			regexCandidates = append(regexCandidates, candidate{pattern, target})
		} else if strings.HasSuffix(pattern, "*") {
			wildcardCandidates = append(wildcardCandidates, candidate{pattern, target})
		}
	}
	// 正则：按 pattern 长度降序，取第一个匹配
	sort.Slice(regexCandidates, func(i, j int) bool {
		return len(regexCandidates[i].pattern) > len(regexCandidates[j].pattern)
	})
	for _, c := range regexCandidates {
		// Bug2+3: 使用进程级缓存的全串锚定正则（^(?:...)$），避免子串误匹配和重复编译
		re, err := getGroupMappingRegex(c.pattern[1:])
		if err != nil {
			continue
		}
		if re.MatchString(requestedModel) {
			return c.target, true
		}
	}
	// 3. 通配符：按 pattern 长度降序（最长前缀优先）
	sort.Slice(wildcardCandidates, func(i, j int) bool {
		return len(wildcardCandidates[i].pattern) > len(wildcardCandidates[j].pattern)
	})
	for _, c := range wildcardCandidates {
		prefix := c.pattern[:len(c.pattern)-1]
		if strings.HasPrefix(requestedModel, prefix) {
			return c.target, true
		}
	}
	return requestedModel, false
}

// xiugai end
