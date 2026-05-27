//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// --- ResolveGroupMappedModel 测试 ---

func TestResolveGroupMappedModel_ExactMatch(t *testing.T) {
	g := &Group{ModelMapping: map[string]string{"gpt-4": "gpt-4o"}}
	model, ok := g.ResolveGroupMappedModel("gpt-4")
	require.True(t, ok)
	require.Equal(t, "gpt-4o", model)
}

func TestResolveGroupMappedModel_NoMatch(t *testing.T) {
	g := &Group{ModelMapping: map[string]string{"gpt-4": "gpt-4o"}}
	model, ok := g.ResolveGroupMappedModel("gpt-5")
	require.False(t, ok)
	require.Equal(t, "gpt-5", model)
}

func TestResolveGroupMappedModel_WildcardMatch(t *testing.T) {
	g := &Group{ModelMapping: map[string]string{"gpt-4*": "gpt-4o"}}
	model, ok := g.ResolveGroupMappedModel("gpt-4-turbo")
	require.True(t, ok)
	require.Equal(t, "gpt-4o", model)
}

// Bug2: 正则全串锚定——"~4" 不应命中 "gpt-4o"，只应命中 "4"
func TestResolveGroupMappedModel_Bug2_RegexFullMatch(t *testing.T) {
	g := &Group{ModelMapping: map[string]string{"~4": "target"}}

	// 字符串 "4" 完整匹配正则 "4"
	model, ok := g.ResolveGroupMappedModel("4")
	require.True(t, ok)
	require.Equal(t, "target", model)

	// "gpt-4o" 不应被子串匹配命中
	_, ok = g.ResolveGroupMappedModel("gpt-4o")
	require.False(t, ok, "正则 '~4' 不应子串匹配 'gpt-4o'")

	// "gpt-4" 也不应被命中
	_, ok = g.ResolveGroupMappedModel("gpt-4")
	require.False(t, ok, "正则 '~4' 不应子串匹配 'gpt-4'")
}

// Bug2: 用户自行写全串锚定的正则仍然正确工作
func TestResolveGroupMappedModel_Bug2_RegexFullPatternWorks(t *testing.T) {
	g := &Group{ModelMapping: map[string]string{"~^gpt-4.*$": "gpt-4o"}}
	model, ok := g.ResolveGroupMappedModel("gpt-4-turbo")
	require.True(t, ok)
	require.Equal(t, "gpt-4o", model)

	_, ok = g.ResolveGroupMappedModel("openai-gpt-4")
	require.False(t, ok, "^gpt-4.*$ 不应匹配 'openai-gpt-4'")
}

// Bug3: 多次调用不因重复编译 panic，且结果一致（缓存正确）
func TestResolveGroupMappedModel_Bug3_RegexCachedConsistency(t *testing.T) {
	g := &Group{ModelMapping: map[string]string{"~^claude-.*": "claude-3-5-sonnet-20241022"}}
	for range 10 {
		model, ok := g.ResolveGroupMappedModel("claude-opus-4")
		require.True(t, ok)
		require.Equal(t, "claude-3-5-sonnet-20241022", model)
	}
}

// Bug4: 空目标不命中，不产生 "model is required" 的误导性错误
func TestResolveGroupMappedModel_Bug4_EmptyTargetSkipped(t *testing.T) {
	g := &Group{ModelMapping: map[string]string{
		"gpt-4":   "",       // 空目标：不应命中
		"gpt-4*":  "",       // 空通配符目标：不应命中
		"~^gpt-5": "",       // 空正则目标：不应命中
		"gpt-3":   "gpt-3o", // 正常规则作为对照
	}}

	// 空目标的精确匹配不应命中
	_, ok := g.ResolveGroupMappedModel("gpt-4")
	require.False(t, ok, "空目标的精确规则不应命中")

	// 空目标的通配符不应命中
	_, ok = g.ResolveGroupMappedModel("gpt-4-turbo")
	require.False(t, ok, "空目标的通配符规则不应命中")

	// 空目标的正则不应命中
	_, ok = g.ResolveGroupMappedModel("gpt-5")
	require.False(t, ok, "空目标的正则规则不应命中")

	// 正常规则正常命中
	model, ok := g.ResolveGroupMappedModel("gpt-3")
	require.True(t, ok)
	require.Equal(t, "gpt-3o", model)
}

// Bug4: 空目标精确匹配不阻断后续通配符匹配
func TestResolveGroupMappedModel_Bug4_EmptyExactFallsThrough(t *testing.T) {
	g := &Group{ModelMapping: map[string]string{
		"gpt-4":  "",       // 空精确匹配，应跳过
		"gpt-4*": "gpt-4o", // 通配符应继续生效
	}}
	model, ok := g.ResolveGroupMappedModel("gpt-4")
	require.True(t, ok, "空精确规则跳过后应命中通配符规则")
	require.Equal(t, "gpt-4o", model)
}

func TestNormalizeGroupModelMapping_RejectsInvalidRegex(t *testing.T) {
	_, err := NormalizeGroupModelMapping(map[string]string{
		"~[": "target",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid model_mapping regex")
}

func TestNormalizeGroupModelMapping_RejectsEmptyAndWildcardTarget(t *testing.T) {
	_, err := NormalizeGroupModelMapping(map[string]string{
		"gpt-5": "",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot be empty")

	_, err = NormalizeGroupModelMapping(map[string]string{
		"gpt-*": "target-*",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot contain wildcard")
}

// TestGroup_GetImagePrice_1K 测试 1K 尺寸返回正确价格
func TestGroup_GetImagePrice_1K(t *testing.T) {
	price := 0.10
	group := &Group{
		ImagePrice1K: &price,
	}

	result := group.GetImagePrice("1K")
	require.NotNil(t, result)
	require.InDelta(t, 0.10, *result, 0.0001)
}

// TestGroup_GetImagePrice_2K 测试 2K 尺寸返回正确价格
func TestGroup_GetImagePrice_2K(t *testing.T) {
	price := 0.15
	group := &Group{
		ImagePrice2K: &price,
	}

	result := group.GetImagePrice("2K")
	require.NotNil(t, result)
	require.InDelta(t, 0.15, *result, 0.0001)
}

// TestGroup_GetImagePrice_4K 测试 4K 尺寸返回正确价格
func TestGroup_GetImagePrice_4K(t *testing.T) {
	price := 0.30
	group := &Group{
		ImagePrice4K: &price,
	}

	result := group.GetImagePrice("4K")
	require.NotNil(t, result)
	require.InDelta(t, 0.30, *result, 0.0001)
}

// TestGroup_GetImagePrice_UnknownSize 测试未知尺寸回退 2K
func TestGroup_GetImagePrice_UnknownSize(t *testing.T) {
	price2K := 0.15
	group := &Group{
		ImagePrice2K: &price2K,
	}

	// 未知尺寸 "3K" 应该回退到 2K
	result := group.GetImagePrice("3K")
	require.NotNil(t, result)
	require.InDelta(t, 0.15, *result, 0.0001)

	// 空字符串也回退到 2K
	result = group.GetImagePrice("")
	require.NotNil(t, result)
	require.InDelta(t, 0.15, *result, 0.0001)
}

// TestGroup_GetImagePrice_NilValues 测试未配置时返回 nil
func TestGroup_GetImagePrice_NilValues(t *testing.T) {
	group := &Group{
		// 所有 ImagePrice 字段都是 nil
	}

	require.Nil(t, group.GetImagePrice("1K"))
	require.Nil(t, group.GetImagePrice("2K"))
	require.Nil(t, group.GetImagePrice("4K"))
	require.Nil(t, group.GetImagePrice("unknown"))
}

// TestGroup_GetImagePrice_PartialConfig 测试部分配置
func TestGroup_GetImagePrice_PartialConfig(t *testing.T) {
	price1K := 0.10
	group := &Group{
		ImagePrice1K: &price1K,
		// ImagePrice2K 和 ImagePrice4K 未配置
	}

	result := group.GetImagePrice("1K")
	require.NotNil(t, result)
	require.InDelta(t, 0.10, *result, 0.0001)

	// 2K 和 4K 返回 nil
	require.Nil(t, group.GetImagePrice("2K"))
	require.Nil(t, group.GetImagePrice("4K"))
}
