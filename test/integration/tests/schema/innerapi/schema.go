// Copyright(c) 2026 The Rainway AI Gateway (壬远AI网关) Authors.
//
//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS,
//WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//See the License for the specific language governing permissions and
//limitations under the License.

// Package innerapi 定义 InnerAPI v1 各接口返回值的 schema。
// 由于 InnerAPI 大量字段为动态 key 的 map，本包优先校验顶层固定字段的类型，
// 对动态 map 仅校验其为 object。
package innerapi

import "github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"

// ---------- 公共/复用 schema ----------

// InnerAPIBaseSchema 所有 InnerAPI 导出配置的顶层基础字段（Version 为 string）
var InnerAPIBaseSchema = &testutil.ObjectSchema{
	Required: []string{"Version"},
	Fields: map[string]testutil.FieldSpec{
		"Version": {Type: testutil.TypeString},
	},
}

// ClusterConfSchema /configs/tls_conf/server_data_conf 中 ClusterConf 的 schema
var ClusterConfSchema = &testutil.ObjectSchema{
	Required: []string{"Version", "Config"},
	Fields: map[string]testutil.FieldSpec{
		"Version": {Type: testutil.TypeString},
		"Config":  {Type: testutil.TypeObject},
	},
}

// ModelPriceSchema AIConf.ModelTable.Models 中单个模型价格的 schema
var ModelPriceSchema = &testutil.ObjectSchema{
	Required: []string{"Provider", "Model", "BaseModel", "Mode", "Prices"},
	Optional: []string{"Capabilities", "SupportedParameters", "Limits", "TierPrices", "Metadata"},
	Fields: map[string]testutil.FieldSpec{
		"Provider":            {Type: testutil.TypeString},
		"Model":               {Type: testutil.TypeString},
		"BaseModel":           {Type: testutil.TypeString},
		"Mode":                {Type: testutil.TypeString},
		"Capabilities":        {Type: testutil.TypeArray, Item: &testutil.FieldSpec{Type: testutil.TypeString}},
		"SupportedParameters": {Type: testutil.TypeArray, Item: &testutil.FieldSpec{Type: testutil.TypeString}},
		"Limits":              {Type: testutil.TypeObject},
		"Prices":              {Type: testutil.TypeObject},
		"TierPrices":          {Type: testutil.TypeObject},
		"Metadata":            {Type: testutil.TypeObject},
	},
}

// ModelTableSchema AIConf.ModelTable 的 schema
var ModelTableSchema = &testutil.ObjectSchema{
	Required: []string{"Currency", "Models"},
	Optional: []string{"TimeZone", "Tiers"},
	Fields: map[string]testutil.FieldSpec{
		"Currency": {Type: testutil.TypeString},
		"TimeZone": {Type: testutil.TypeString},
		"Tiers":    {Type: testutil.TypeArray},
		"Models":   {Type: testutil.TypeArray, Elem: ModelPriceSchema},
	},
}

// AIKeyPolicySchema AIConf.KeyPolicy 的 schema
var AIKeyPolicySchema = &testutil.ObjectSchema{
	Required: []string{
		"Strategy", "MaxRetries", "RetryBackoffInitial", "RetryBackoffMax",
		"SessionAffinity", "SessionAffinityTTL", "SessionAffinityRedisPrefix", "SessionAffinityPenaltyEnable",
	},
	Fields: map[string]testutil.FieldSpec{
		"Strategy":                     {Type: testutil.TypeString},
		"MaxRetries":                   {Type: testutil.TypeInt},
		"RetryBackoffInitial":          {Type: testutil.TypeInt},
		"RetryBackoffMax":              {Type: testutil.TypeInt},
		"SessionAffinity":              {Type: testutil.TypeBool},
		"SessionAffinityTTL":           {Type: testutil.TypeInt},
		"SessionAffinityRedisPrefix":   {Type: testutil.TypeString},
		"SessionAffinityPenaltyEnable": {Type: testutil.TypeBool},
	},
}

// AIConfSchema ClusterConf.Config.<cluster>.AIConf 的 schema
var AIConfSchema = &testutil.ObjectSchema{
	Required: []string{"KeyPolicy"},
	Optional: []string{"Keys", "ModelMappings", "ModelTable", "MatchPrefix", "StripPrefix", "ModelProtocols", "NormalizeUpstreamError", "PriceMissAction"},
	Fields: map[string]testutil.FieldSpec{
		"Keys":           {Type: testutil.TypeArray},
		"KeyPolicy":      {Type: testutil.TypeObject, Nested: AIKeyPolicySchema},
		"ModelMappings":  {Type: testutil.TypeObject},
		"ModelTable":     {Type: testutil.TypeObject, Nested: ModelTableSchema},
		"MatchPrefix":    {Type: testutil.TypeString},
		"StripPrefix":    {Type: testutil.TypeBool},
		"ModelProtocols": {Type: testutil.TypeArray, Item: &testutil.FieldSpec{Type: testutil.TypeString}},
		"NormalizeUpstreamError": {Type: testutil.TypeObject},
		"PriceMissAction":        {Type: testutil.TypeString},
	},
}

// ServerDataConfSchema /configs/tls_conf/server_data_conf 返回 schema
var ServerDataConfSchema = &testutil.ObjectSchema{
	Required: []string{"Version", "HostTable", "RouteTable", "ClusterConf"},
	Fields: map[string]testutil.FieldSpec{
		"Version":     {Type: testutil.TypeString},
		"HostTable":   {Type: testutil.TypeObject},
		"RouteTable":  {Type: testutil.TypeObject},
		"ClusterConf": {Type: testutil.TypeObject, Nested: ClusterConfSchema},
	},
}

// GSLBSchema /configs/gslb_data/gslb 返回 schema
var GSLBSchema = &testutil.ObjectSchema{
	Required: []string{"Version", "Ts", "Hostname", "Clusters"},
	Fields: map[string]testutil.FieldSpec{
		"Version":  {Type: testutil.TypeString},
		"Ts":       {Type: testutil.TypeString},
		"Hostname": {Type: testutil.TypeString},
		"Clusters": {Type: testutil.TypeObject},
	},
}

// ClusterTableSchema /configs/gslb_data/cluster_table 返回 schema
var ClusterTableSchema = &testutil.ObjectSchema{
	Required: []string{"Version", "Config"},
	Fields: map[string]testutil.FieldSpec{
		"Version": {Type: testutil.TypeString},
		"Config":  {Type: testutil.TypeObject},
	},
}

// ServerCertConfSchema /configs/protocol/server_cert_conf 返回 schema
var ServerCertConfSchema = &testutil.ObjectSchema{
	Required: []string{"Version", "Config"},
	Fields: map[string]testutil.FieldSpec{
		"Version": {Type: testutil.TypeString},
		"Config":  {Type: testutil.TypeObject},
	},
}

// ModAPIKeySchema /configs/mod-api-key 返回 schema
var ModAPIKeySchema = &testutil.ObjectSchema{
	Required: []string{"version", "config", "QuotaPlans", "tokens"},
	Fields: map[string]testutil.FieldSpec{
		"version":    {Type: testutil.TypeString},
		"config":     {Type: testutil.TypeObject},
		"QuotaPlans": {Type: testutil.TypeObject},
		"tokens":     {Type: testutil.TypeObject},
	},
}

// ModBodyProcessSchema /configs/mod-body-process 返回 schema
var ModBodyProcessSchema = &testutil.ObjectSchema{
	Required: []string{"Version", "Config"},
	Fields: map[string]testutil.FieldSpec{
		"Version": {Type: testutil.TypeString},
		"Config":  {Type: testutil.TypeObject},
	},
}

// RateLimitPolicySchema /configs/rate-limit-policy 返回 schema
var RateLimitPolicySchema = &testutil.ObjectSchema{
	Required: []string{"Config", "RateLimitPolicies", "ApikeyRateLimitPolicyBindings", "Version"},
	Fields: map[string]testutil.FieldSpec{
		"Config":                        {Type: testutil.TypeObject},
		"RateLimitPolicies":             {Type: testutil.TypeObject},
		"ApikeyRateLimitPolicyBindings": {Type: testutil.TypeObject},
		"Version":                       {Type: testutil.TypeString},
	},
}

// AIRouteSchema /configs/ai-route 返回 schema
var AIRouteSchema = &testutil.ObjectSchema{
	Required: []string{"Version", "RouteRules", "ApikeyRouteTableBindings"},
	Fields: map[string]testutil.FieldSpec{
		"Version":                  {Type: testutil.TypeString},
		"RouteRules":               {Type: testutil.TypeObject},
		"ApikeyRouteTableBindings": {Type: testutil.TypeObject},
	},
}

// EppDataConfigBodySchema /configs/epp_data/config 返回 Config 段的 schema。
// epp_config / assignment 均为 cluster 名动态 key 的 map（见 epp-data.md §3），
// 按本包惯例仅校验为 object；条目级结构由测试中的定向断言覆盖。
var EppDataConfigBodySchema = &testutil.ObjectSchema{
	Required: []string{"epp_config", "assignment"},
	Fields: map[string]testutil.FieldSpec{
		"epp_config": {Type: testutil.TypeObject},
		"assignment": {Type: testutil.TypeObject},
	},
}

// EppDataSchema /configs/epp_data/config 返回 schema（epp-data.md §3.1）
var EppDataSchema = &testutil.ObjectSchema{
	Required: []string{"Version", "Config"},
	Fields: map[string]testutil.FieldSpec{
		"Version": {Type: testutil.TypeString},
		"Config":  {Type: testutil.TypeObject, Nested: EppDataConfigBodySchema},
	},
}

// AICacheRuleExportItemSchema /configs/ai-cache-rule 导出单条规则 schema。
// Required 恰为合同 6 个导出 tag（大写驼峰，与 Open API 词汇不同；二期新增
// enableSemanticCache）；二期/预留字段 cacheKeyFrom/cacheValueFrom/cacheStreamValueFrom/
// cacheToolCallsFrom/responseTemplate/streamResponseTemplate 不得出现（ai-cache-rule.md §3.2）。
var AICacheRuleExportItemSchema = &testutil.ObjectSchema{
	Required: []string{
		"cond", "cacheKeyStrategy", "cacheTTL", "maxBodyBytes", "maxValueBytes",
		"enableSemanticCache",
	},
	Fields: map[string]testutil.FieldSpec{
		"cond":                {Type: testutil.TypeString},
		"cacheKeyStrategy":    {Type: testutil.TypeString, Enum: []interface{}{"lastQuestion", "allQuestions", "disabled"}},
		"cacheTTL":            {Type: testutil.TypeInt},
		"maxBodyBytes":        {Type: testutil.TypeInt},
		"maxValueBytes":       {Type: testutil.TypeInt},
		"enableSemanticCache": {Type: testutil.TypeBool},
	},
}

// AICacheSemanticExportSchema /configs/ai-cache-rule 顶层 Semantic 块 schema。
// camelCase tag（topK/threshold/thresholdRelation）与 BFE SemanticConfFile 逐字段冻结；
// 恒导出（空设置表时以默认值 1/0.15/lt 填充）。
var AICacheSemanticExportSchema = &testutil.ObjectSchema{
	Required: []string{"topK", "threshold", "thresholdRelation"},
	Fields: map[string]testutil.FieldSpec{
		"topK":              {Type: testutil.TypeInt},
		"threshold":         {Type: testutil.TypeNumber},
		"thresholdRelation": {Type: testutil.TypeString, Enum: []interface{}{"lt", "lte", "gt", "gte"}},
	},
}

// AICacheRuleExportBodySchema /configs/ai-cache-rule 返回 Config 段 schema。
// product 键取自 AIRouteInnerProductName（测试环境为 AI_product）；空集合导出 [] 且键 present。
var AICacheRuleExportBodySchema = &testutil.ObjectSchema{
	Required: []string{"AI_product"},
	Fields: map[string]testutil.FieldSpec{
		"AI_product": {Type: testutil.TypeArray, Elem: AICacheRuleExportItemSchema},
	},
}

// AICacheRuleExportSchema /configs/ai-cache-rule 返回 schema。
// Semantic 块二期新增且恒导出（与 Version/Config 并列顶层键）。
var AICacheRuleExportSchema = &testutil.ObjectSchema{
	Required: []string{"Version", "Semantic", "Config"},
	Fields: map[string]testutil.FieldSpec{
		"Version":  {Type: testutil.TypeString},
		"Semantic": {Type: testutil.TypeObject, Nested: AICacheSemanticExportSchema},
		"Config":   {Type: testutil.TypeObject, Nested: AICacheRuleExportBodySchema},
	},
}

// AIContextRuleExportItemSchema /configs/ai-context-rule 导出单条规则 schema。
// Required 恰为合同 4 个导出 tag（camelCase，与 Open API 词汇不同）；
// mode 必填四枚举（BFE 对缺失/非法 mode 整文件拒载，控制面 422 前置拦截）；
// 二期字段 override 不得出现（ai-context-rule.md §3.3 合同锁定）。
var AIContextRuleExportItemSchema = &testutil.ObjectSchema{
	Required: []string{"cond", "mode", "maxContextTokens", "reserveTokens"},
	Fields: map[string]testutil.FieldSpec{
		"cond":             {Type: testutil.TypeString},
		"mode":             {Type: testutil.TypeString, Enum: []interface{}{"off", "conservative", "balanced", "aggressive"}},
		"maxContextTokens": {Type: testutil.TypeInt},
		"reserveTokens":    {Type: testutil.TypeInt},
	},
}

// AIContextRewriteExportSchema /configs/ai-context-rule Defaults 块 rewrite 子对象 schema。
// camelCase tag（strength/protectedSurvivalRate）与 BFE RewriteConfFile 逐字段冻结。
var AIContextRewriteExportSchema = &testutil.ObjectSchema{
	Required: []string{"strength", "protectedSurvivalRate"},
	Fields: map[string]testutil.FieldSpec{
		"strength":              {Type: testutil.TypeString, Enum: []interface{}{"lite", "full"}},
		"protectedSurvivalRate": {Type: testutil.TypeNumber},
	},
}

// AIContextDefaultsExportSchema /configs/ai-context-rule 顶层 Defaults 块 schema。
// camelCase tag（7 顶层键）与 BFE DefaultsConfFile 逐字段冻结；恒导出（空设置表时以
// 默认值 0.7/2/2000/trim-all-but-last/4/1200/lite/0.95 填充）；rewrite 为嵌套对象。
var AIContextDefaultsExportSchema = &testutil.ObjectSchema{
	Required: []string{
		"triggerRatio", "keepLatestImages", "toolResultMaxChars",
		"thinkingPolicy", "charsPerToken", "imageTokenEstimate", "rewrite",
	},
	Fields: map[string]testutil.FieldSpec{
		"triggerRatio":       {Type: testutil.TypeNumber},
		"keepLatestImages":   {Type: testutil.TypeInt},
		"toolResultMaxChars": {Type: testutil.TypeInt},
		"thinkingPolicy":     {Type: testutil.TypeString, Enum: []interface{}{"trim-all-but-last", "keep"}},
		"charsPerToken":      {Type: testutil.TypeInt},
		"imageTokenEstimate": {Type: testutil.TypeInt},
		"rewrite":            {Type: testutil.TypeObject, Nested: AIContextRewriteExportSchema},
	},
}

// AIContextRuleExportBodySchema /configs/ai-context-rule 返回 Config 段 schema。
// product 键取自 AIRouteInnerProductName（测试环境为 AI_product）；空集合导出 [] 且键 present。
var AIContextRuleExportBodySchema = &testutil.ObjectSchema{
	Required: []string{"AI_product"},
	Fields: map[string]testutil.FieldSpec{
		"AI_product": {Type: testutil.TypeArray, Elem: AIContextRuleExportItemSchema},
	},
}

// AIContextRuleExportSchema /configs/ai-context-rule 返回 schema。
// Defaults 块恒导出且进入 MD5 签名（设置变更驱动版本流），与 Version/Config 并列顶层键。
var AIContextRuleExportSchema = &testutil.ObjectSchema{
	Required: []string{"Version", "Defaults", "Config"},
	Fields: map[string]testutil.FieldSpec{
		"Version":  {Type: testutil.TypeString},
		"Defaults": {Type: testutil.TypeObject, Nested: AIContextDefaultsExportSchema},
		"Config":   {Type: testutil.TypeObject, Nested: AIContextRuleExportBodySchema},
	},
}

// TrafficMirrorRuleExportItemSchema /configs/traffic-mirror-rule 导出单条规则 schema。
// Required 恰为合同 7 个导出 tag（大写驼峰，与 Open API 词汇不同）；规则全字段恒输出
// （含空值零值 {} / [] / ""，不用 omitempty）——removeHeaders 两层默认语义
// （缺省填默认黑名单 / 显式 [] 不剔除）由取值断言覆盖（traffic-mirror-rule.md §3.2）。
var TrafficMirrorRuleExportItemSchema = &testutil.ObjectSchema{
	Required: []string{
		"cond", "mirrorCluster", "percentage", "removeHeaders", "setHeaders", "bodyRewrites", "pathRewrite",
	},
	Fields: map[string]testutil.FieldSpec{
		"cond":          {Type: testutil.TypeString},
		"mirrorCluster": {Type: testutil.TypeString},
		"percentage":    {Type: testutil.TypeInt},
		"removeHeaders": {Type: testutil.TypeArray, Item: &testutil.FieldSpec{Type: testutil.TypeString}},
		"setHeaders":    {Type: testutil.TypeObject},
		"bodyRewrites": {Type: testutil.TypeArray, Elem: &testutil.ObjectSchema{
			Required: []string{"path", "value"},
			Fields: map[string]testutil.FieldSpec{
				"path":  {Type: testutil.TypeString},
				"value": {Type: testutil.TypeString},
			},
		}},
		"pathRewrite": {Type: testutil.TypeString},
	},
}

// TrafficMirrorRuleExportBodySchema /configs/traffic-mirror-rule 返回 Config 段 schema。
// product 键取自 AIRouteInnerProductName（测试环境为 AI_product）；空集合导出 [] 且键 present。
var TrafficMirrorRuleExportBodySchema = &testutil.ObjectSchema{
	Required: []string{"AI_product"},
	Fields: map[string]testutil.FieldSpec{
		"AI_product": {Type: testutil.TypeArray, Elem: TrafficMirrorRuleExportItemSchema},
	},
}

// TrafficMirrorRuleExportSchema /configs/traffic-mirror-rule 返回 schema
var TrafficMirrorRuleExportSchema = &testutil.ObjectSchema{
	Required: []string{"Version", "Config"},
	Fields: map[string]testutil.FieldSpec{
		"Version": {Type: testutil.TypeString},
		"Config":  {Type: testutil.TypeObject, Nested: TrafficMirrorRuleExportBodySchema},
	},
}

// IntentConfigQuestionExportSchema /configs/mod-ai-intent 导出问题元素 schema。
// Required 为全部问题共有的 3 个 PascalCase tag；Criteria/Levels 按 type 互斥出现、
// MinConfidence 为逐问题可选覆盖（BFE 文件合同，intent_questions.data.md）；
// 与 Open API 小写词汇不同，转换发生在导出 Generator。
var IntentConfigQuestionExportSchema = &testutil.ObjectSchema{
	Required: []string{"Name", "Type", "Instructions"},
	Optional: []string{"Criteria", "Levels", "MinConfidence"},
	Fields: map[string]testutil.FieldSpec{
		"Name":         {Type: testutil.TypeString},
		"Type":         {Type: testutil.TypeString, Enum: []interface{}{"choice", "score"}},
		"Instructions": {Type: testutil.TypeString},
		"Criteria":     {Type: testutil.TypeObject},
		"Levels": {Type: testutil.TypeArray, Elem: &testutil.ObjectSchema{
			Required: []string{"Name", "Description"},
			Fields: map[string]testutil.FieldSpec{
				"Name":        {Type: testutil.TypeString},
				"Description": {Type: testutil.TypeString},
			},
		}},
		"MinConfidence": {Type: testutil.TypeNumber},
	},
}

// IntentConfigExportSchema /configs/mod-ai-intent 返回 schema（mod-ai-intent.md §3）。
// Data 即 intent_questions.data 文件内容原样：Version 内嵌文件（时间戳格式
// yyyyMMddHHmmss），无 Config 包装层（ai-route 形态，非 ai-cache 两段结构）。
var IntentConfigExportSchema = &testutil.ObjectSchema{
	Required: []string{"Version", "MinConfidence", "Questions"},
	Fields: map[string]testutil.FieldSpec{
		"Version":       {Type: testutil.TypeString},
		"MinConfidence": {Type: testutil.TypeNumber},
		"Questions":     {Type: testutil.TypeArray, Elem: IntentConfigQuestionExportSchema},
	},
}
