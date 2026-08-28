// Package qol 提供查询混淆 (Query Obfuscation Layer) 原语。
//
// 实现语义诱饵生成与 Fisher-Yates 随机置乱注入，
// 用于防止外部搜索引擎/大模型通过语义侧信道探测真实查询意图。
package qol

import (
	"math/rand/v2"
)

// ──────────────────────────────────────────────
// 医疗领域诱饵词库（精简版，生产环境应扩展至 500+ 词条）
// ──────────────────────────────────────────────

var medicalDecoys = []string{
	"普通感冒", "季节性过敏", "轻度头痛", "偶尔失眠", "轻微咳嗽",
	"消化不良", "肌肉酸痛", "皮肤干燥", "眼睛疲劳", "牙齿敏感",
	"关节僵硬", "咽喉不适", "食欲减退", "轻度焦虑", "运动损伤",
	"日晒过敏", "蚊虫叮咬", "口腔溃疡", "轻微湿疹", "血压波动",
}

// generalDecoys 通用领域诱饵词库
var generalDecoys = []string{
	"天气预报查询", "附近餐厅推荐", "新闻头条", "股票行情", "体育赛事",
	"旅游攻略", "电影评分", "音乐排行", "图书推荐", "菜谱搜索",
	"健身计划", "学习资源", "编程语言", "历史事件", "科学知识",
}

// ──────────────────────────────────────────────
// 公开 API
// ──────────────────────────────────────────────

// GenerateMedicalDecoy 随机生成一条医疗领域诱饵查询。
func GenerateMedicalDecoy() string {
	return medicalDecoys[rand.IntN(len(medicalDecoys))]
}

// GenerateGeneralDecoy 随机生成一条通用领域诱饵查询。
func GenerateGeneralDecoy() string {
	return generalDecoys[rand.IntN(len(generalDecoys))]
}

// InjectDecoys 将真实查询与 n 条诱饵混合后随机置乱返回。
// 返回切片长度为 n+1，其中一条为真实查询，其余为诱饵。
// 调用方需自行记录真实查询的索引以识别有效响应。
func InjectDecoys(realQuery string, numDecoys int, domain string) ([]string, int) {
	queries := make([]string, 0, numDecoys+1)
	queries = append(queries, realQuery)

	for i := 0; i < numDecoys; i++ {
		var decoy string
		if domain == "medical" {
			decoy = GenerateMedicalDecoy()
		} else {
			decoy = GenerateGeneralDecoy()
		}
		queries = append(queries, decoy)
	}

	// Fisher-Yates 随机置乱
	realIdx := fisherYatesShuffle(queries)
	return queries, realIdx
}

// fisherYatesShuffle 对切片执行 Fisher-Yates 随机置乱，
// 返回原第一个元素（真实查询）的最终索引。
func fisherYatesShuffle(items []string) int {
	realIdx := 0
	n := len(items)
	for i := n - 1; i > 0; i-- {
		j := rand.IntN(i + 1)
		items[i], items[j] = items[j], items[i]
		if realIdx == i {
			realIdx = j
		} else if realIdx == j {
			realIdx = i
		}
	}
	return realIdx
}

// GenerateDecoySet 生成一组纯诱饵查询（不含真实查询）。
func GenerateDecoySet(count int, domain string) []string {
	decoys := make([]string, count)
	for i := 0; i < count; i++ {
		if domain == "medical" {
			decoys[i] = GenerateMedicalDecoy()
		} else {
			decoys[i] = GenerateGeneralDecoy()
		}
	}
	return decoys
}
